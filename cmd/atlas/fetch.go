package main

// GitHub heads for the projector (plan/10 "How atlas hears about changes": truth is a poll of each tracked head).
// Each round asks GitHub for a branch's head sha with a conditional request (a 304 does not count against the rate
// limit) and downloads the tree as a tarball only when the sha moved. The tree is unpacked into the cache directory
// (an emptyDir with a sizeLimit: the clones are a cache, never state) and replaces the previous one.
//
// A move is accepted only forward: a head that is behind the one atlas holds (a stale read) is ignored, and a
// diverged one (a force-push) is accepted and logged. Credentials: MOUNTAIN_GITHUB_TOKEN, a read-only token, until
// A5's ukubi-atlas App replaces it; the token is sent to the API host only (Go drops it on the redirect to
// codeload, whose URL carries its own short-lived token).
//
// The unpacking trusts nothing in the archive: regular files and directories only (no links, devices or FIFOs), paths
// relative and inside the checkout (os.Root), at most maxTreeBytes in total and maxFileBytes per file (larger files are
// left out: no extractor reads them). Every tree on disk (held and retired) shares cacheBudget: a tree that would go
// over it is refused, and the repo is read stale (or skipped) instead of the pod being evicted.

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/MohammadBnei/mountain/contract"
	"github.com/MohammadBnei/mountain/internal/extract"
)

const (
	ghOwner      = contract.GitHubOwner
	maxTreeBytes = 256 << 20 // one tree's cap
	cacheBudget  = 4 << 30   // every tree on disk, held and retired, under the 6 Gi emptyDir (helm/values.yaml)
	maxFileBytes = 8 << 20
	maxEntries   = 200_000
)

var errNoAccess = errors.New("not found (no access)")

type ghHeads struct {
	url, token, dir string
	hc              *http.Client
	etag            map[string]string       // request -> ETag
	body            map[string][]byte       // request -> last 200 body (a 304 replays it)
	have            map[string]extract.Repo // repo -> the checkout on disk
	behind          map[string]string       // repo -> a head seen behind the held one, once
	retired         map[string]string       // repo -> the tree replaced last round, removed when the next one is (the reader may still be serving it)
	size            map[string]int64        // tree directory -> bytes unpacked into it (held and retired trees)
	seq             int                     // tree directories are never reused: <repo>-<sha>-<seq>
	pauseUntil      time.Time
	met             *Metrics // nil = not recorded
}

func newGitHub(apiURL, token, dir string, met *Metrics) (*ghHeads, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	old, _ := os.ReadDir(dir) // a previous run's trees: the cache starts empty, so nothing on disk is trusted stale
	for _, e := range old {
		_ = os.RemoveAll(filepath.Join(dir, e.Name()))
	}
	return &ghHeads{url: strings.TrimRight(apiURL, "/"), token: token, dir: dir, hc: &http.Client{Timeout: 5 * time.Minute},
		etag: map[string]string{}, body: map[string][]byte{}, have: map[string]extract.Repo{}, behind: map[string]string{}, retired: map[string]string{}, size: map[string]int64{}, met: met}, nil
}

func (g *ghHeads) checkout(ctx context.Context, repo, branch string) (extract.Repo, error) {
	b, err := g.get(ctx, "/repos/"+ghOwner+"/"+url.PathEscape(repo)+contract.GitHubCommits+url.PathEscape(branch), contract.GitHubHeadAccept, true)
	if err != nil {
		return extract.Repo{}, err
	}
	sha := strings.TrimSpace(string(b))
	if len(sha) != 40 || strings.Trim(sha, "0123456789abcdef") != "" {
		return extract.Repo{}, fmt.Errorf("head of %s: not a sha", branch)
	}
	cur, ok := g.have[repo]
	if ok && cur.SHA == sha { // the same tree (a branch retargeted onto the same commit only changes the name)
		cur.Branch = branch
		g.have[repo] = cur
		delete(g.behind, repo)
		return cur, nil
	}
	if ok && cur.Branch == branch {
		// A head behind the one held is a stale read (a lagging replica) the first time; seen again on the next
		// round, it is a branch really reset to an older commit, and taken. A compare that fails says nothing:
		// the head GitHub named is the truth, so it is taken.
		switch st, err := g.compare(ctx, repo, cur.SHA, sha); {
		case err != nil:
			slog.Warn(fmt.Sprintf("projector: %s@%s: compare %s...%s: %v: taking the head", repo, branch, cur.SHA[:12], sha[:12], err))
		case st == "behind" && g.behind[repo] != sha:
			slog.Info(fmt.Sprintf("projector: %s@%s: head %s is behind %s: held until the next round says so again", repo, branch, sha[:12], cur.SHA[:12]))
			g.behind[repo] = sha
			return cur, nil
		case st == "behind":
			slog.Info(fmt.Sprintf("projector: %s@%s: reset back to %s from %s: taken", repo, branch, sha[:12], cur.SHA[:12]))
		case st == "diverged":
			slog.Info(fmt.Sprintf("projector: %s@%s: %s does not descend from %s (a force-push): taken", repo, branch, sha[:12], cur.SHA[:12]))
		}
	}
	delete(g.behind, repo)
	dir, err := g.tree(ctx, repo, sha)
	if err != nil {
		return cur, err // cur (when held) is still a whole tree: round reads it, stale, rather than lose the repo
	}
	if ok { // the replaced tree goes one round later: the reader serves the last round's trees until this round ends
		if old := g.retired[repo]; old != "" {
			_ = os.RemoveAll(old)
			delete(g.size, old)
		}
		g.retired[repo] = cur.Dir
	}
	r := extract.Repo{Dir: dir, SHA: sha, Branch: branch}
	g.have[repo] = r
	return r, nil
}

// compare is GitHub's status of head relative to base: ahead, behind, identical or diverged.
func (g *ghHeads) compare(ctx context.Context, repo, base, head string) (string, error) {
	b, err := g.get(ctx, "/repos/"+ghOwner+"/"+url.PathEscape(repo)+contract.GitHubCompare+base+"..."+head+"?per_page=1", contract.GitHubJSONAccept, false)
	if err != nil {
		return "", err
	}
	var c struct{ Status string }
	if err := json.Unmarshal(b, &c); err != nil {
		return "", err
	}
	return c.Status, nil
}

// tree downloads repo at sha and unpacks it into a fresh directory under the cache, within what cacheBudget leaves.
func (g *ghHeads) tree(ctx context.Context, repo, sha string) (string, error) {
	var used int64
	for _, n := range g.size {
		used += n
	}
	limit := min(maxTreeBytes, cacheBudget-used)
	if limit <= 0 {
		return "", fmt.Errorf("tarball %s: cache budget spent (%d of %d bytes)", sha[:12], used, int64(cacheBudget))
	}
	req, err := g.request(ctx, "/repos/"+ghOwner+"/"+url.PathEscape(repo)+contract.GitHubTarball+sha, contract.GitHubJSONAccept)
	if err != nil {
		return "", err
	}
	resp, err := g.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	g.limits(resp.Header)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tarball %s: status %d", sha[:12], resp.StatusCode)
	}
	g.seq++
	dir := filepath.Join(g.dir, fmt.Sprintf("%s-%s-%d", repo, sha[:12], g.seq)) // never the held tree's path
	_ = os.RemoveAll(dir)
	n, err := unpack(resp.Body, dir, limit)
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", fmt.Errorf("tarball %s: %w", sha[:12], err)
	}
	g.size[dir] = n
	return dir, nil
}

// unpack writes a GitHub tarball (one top directory, "<owner>-<repo>-<sha>/") into dir, stripping that directory,
// and answers the bytes written; more than limit bytes of files is an error.
func unpack(r io.Reader, dir string, limit int64) (int64, error) {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return 0, err
	}
	defer func() { _ = root.Close() }()
	tr := tar.NewReader(zr)
	var total int64
	for n := 0; ; n++ {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return total, nil
		}
		if err != nil {
			return total, err
		}
		if n >= maxEntries {
			return total, errors.New("too many entries")
		}
		_, rel, _ := strings.Cut(h.Name, "/") // strip the top directory
		rel = path.Clean(rel)
		if rel == "." || rel == "" || path.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, "../") {
			continue
		}
		name := filepath.FromSlash(rel)
		switch h.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(name, 0o755); err != nil {
				return total, err
			}
		case tar.TypeReg:
			if h.Size > maxFileBytes {
				continue // no extractor reads a file this large
			}
			if total += h.Size; total > limit {
				return total, fmt.Errorf("tree over %d bytes", limit)
			}
			if d := filepath.Dir(name); d != "." {
				if err := root.MkdirAll(d, 0o755); err != nil {
					return total, err
				}
			}
			f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
			if err != nil {
				return total, err
			}
			_, err = io.Copy(f, io.LimitReader(tr, h.Size))
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				return total, err
			}
		}
		// anything else (symlinks, hard links, devices, the pax global header) is left out
	}
}

func (g *ghHeads) request(ctx context.Context, p, accept string) (*http.Request, error) {
	if time.Now().Before(g.pauseUntil) {
		return nil, fmt.Errorf("rate limited until %s", g.pauseUntil.UTC().Format(time.TimeOnly))
	}
	req, err := http.NewRequestWithContext(ctx, "GET", g.url+p, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Accept", accept)
	return req, nil
}

// get fetches p; conditional=true sends the last ETag, and a 304 returns the cached body.
func (g *ghHeads) get(ctx context.Context, p, accept string, conditional bool) ([]byte, error) {
	req, err := g.request(ctx, p, accept)
	if err != nil {
		return nil, err
	}
	if e := g.etag[p]; conditional && e != "" {
		req.Header.Set("If-None-Match", e)
	}
	resp, err := g.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	g.limits(resp.Header)
	switch resp.StatusCode {
	case http.StatusNotModified:
		g.met.SourceOK("github", float64(time.Now().Unix()))
		return g.body[p], nil
	case http.StatusOK:
		b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if err != nil {
			return nil, err
		}
		if conditional {
			g.etag[p], g.body[p] = resp.Header.Get("ETag"), b
		}
		g.met.SourceOK("github", float64(time.Now().Unix()))
		return b, nil
	case http.StatusNotFound:
		return nil, errNoAccess
	case http.StatusForbidden, http.StatusTooManyRequests:
		ra, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
		g.pauseUntil = time.Now().Add(max(time.Duration(ra)*time.Second, time.Minute))
	}
	return nil, fmt.Errorf("%s: status %d", p, resp.StatusCode)
}

// limits reads the token's expiry (mountain_github_token_days_left) and pauses before the rate limit runs out.
func (g *ghHeads) limits(h http.Header) {
	if exp := h.Get("github-authentication-token-expiration"); exp != "" {
		for _, layout := range []string{"2006-01-02 15:04:05 MST", "2006-01-02 15:04:05 -0700"} {
			if t, err := time.Parse(layout, exp); err == nil {
				g.met.TokenDays(float64(time.Until(t).Round(time.Hour).Hours()) / 24)
				break
			}
		}
	}
	left, err1 := strconv.Atoi(h.Get("X-RateLimit-Remaining"))
	reset, err2 := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64)
	if err1 == nil && err2 == nil && left < 100 {
		g.pauseUntil = time.Unix(reset, 0)
	}
}
