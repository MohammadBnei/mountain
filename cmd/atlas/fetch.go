package main

// Forge heads for the projector (plan/10 "How atlas hears about changes": truth is a poll of each tracked head).
// Each round asks Forgejo for a branch's head sha (the branch's own route, so a tag of the same name never answers)
// and downloads the tree as an archive only when the sha moved. The tree is unpacked into the cache directory (an
// emptyDir with a sizeLimit: the clones are a cache, never state) and replaces the previous one. ponytail: a plain GET
// each round, no conditional request: Forgejo's API sends no ETag and has no rate limit by default.
//
// Every move of a head is taken (checkout says why none is held back). Credentials: MOUNTAIN_FORGE_TOKEN, a Forgejo
// token with read:repository; it is sent to the API host only (Go drops it on a redirect to another host, as an
// archive served from object storage would be).
//
// The unpacking trusts nothing in the archive: regular files and directories only (no links, devices or FIFOs), paths
// relative and inside the checkout (os.Root), at most maxTreeBytes in total and maxFileBytes per file (larger files are
// left out: no extractor reads them). Sizes are counted as the disk uses them, in blocks (blockSize), since that is
// what the emptyDir's sizeLimit counts. Every tree on disk shares cacheBudget: the held ones and the ones this round
// replaced (the reader serves the last round's trees until this round ends, so they are removed when the next round
// starts). A tree that would go over it is refused, and the repo is read stale (or skipped) instead of the pod being
// evicted.

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	owner        = contract.ForgeOwner
	maxFileBytes = 8 << 20
	maxEntries   = 200_000
	blockSize    = 4 << 10 // the filesystem block: a file or directory uses its size rounded up to it

	// maxTreeBytes is one tree, on disk: the largest source, vos-monolith, is ~225 MiB tracked (2026-10). Raise it
	// when a source outgrows it: its tree is refused every round and the repo read stale, which the projector's WARN
	// says (the repo and the size reached). cacheBudget bounds the sum.
	maxTreeBytes = 256 << 20
)

// cacheBudget is every tree on disk, held and retired, under the 6 Gi emptyDir (helm/values.yaml): the 2 Gi above it
// is headroom for what the block count misses (filesystem metadata). A var so tests lower it.
var cacheBudget int64 = 4 << 30

var errNoAccess = errors.New("not found (no access)")

type forgeHeads struct {
	url, token, dir string
	hc              *http.Client
	have            map[string]extract.Repo // repo -> the checkout on disk
	retired         []string                // the trees replaced this round, removed when the next round starts (the reader serves them until then)
	size            map[string]int64        // tree directory -> bytes it uses on disk (held and retired trees)
	seq             int                     // tree directories are never reused: <repo>-<sha>-<seq>
	pauseUntil      time.Time
	met             *Metrics // nil = not recorded
}

func newForge(apiURL, token, dir string, met *Metrics) (*forgeHeads, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	old, _ := os.ReadDir(dir) // a previous run's trees: the cache starts empty, so nothing on disk is trusted stale
	for _, e := range old {
		_ = os.RemoveAll(filepath.Join(dir, e.Name()))
	}
	return &forgeHeads{url: strings.TrimRight(apiURL, "/"), token: token, dir: dir, hc: &http.Client{Timeout: 5 * time.Minute},
		have: map[string]extract.Repo{}, size: map[string]int64{}, met: met}, nil
}

// startRound removes the trees the last round replaced: the reader moved off them when that round ended.
func (g *forgeHeads) startRound() {
	for _, d := range g.retired {
		_ = os.RemoveAll(d)
		delete(g.size, d)
	}
	g.retired = nil
}

func (g *forgeHeads) checkout(ctx context.Context, repo, branch string) (extract.Repo, error) {
	b, err := g.get(ctx, "/repos/"+owner+"/"+url.PathEscape(repo)+contract.ForgeBranches+(&url.URL{Path: branch}).EscapedPath())
	if err != nil {
		return extract.Repo{}, err
	}
	var br struct{ Commit struct{ ID string } }
	if err := json.Unmarshal(b, &br); err != nil {
		return extract.Repo{}, fmt.Errorf("head of %s: %w", branch, err)
	}
	sha := br.Commit.ID
	if len(sha) != 40 || strings.Trim(sha, "0123456789abcdef") != "" {
		return extract.Repo{}, fmt.Errorf("head of %s: not a sha", branch)
	}
	cur, ok := g.have[repo]
	if ok && cur.SHA == sha { // the same tree (a branch retargeted onto the same commit only changes the name)
		cur.Branch = branch
		g.have[repo] = cur
		return cur, nil
	}
	// ponytail: every move is taken, forward or back. The GitHub poll held a head behind the one held for a round (a
	// lagging replica's stale read); single-node Forgejo has no replica to lag, so a head behind is a real reset.
	dir, err := g.tree(ctx, repo, sha)
	if err != nil {
		return cur, err // cur (when held) is still a whole tree: round reads it, stale, rather than lose the repo
	}
	if ok { // the replaced tree goes when the next round starts: the reader serves the last round's trees until then
		g.retired = append(g.retired, cur.Dir)
	}
	r := extract.Repo{Dir: dir, SHA: sha, Branch: branch}
	g.have[repo] = r
	return r, nil
}

// tree downloads repo at sha and unpacks it into a fresh directory under the cache, within what cacheBudget leaves.
func (g *forgeHeads) tree(ctx context.Context, repo, sha string) (string, error) {
	var used int64
	for _, n := range g.size {
		used += n
	}
	limit := min(maxTreeBytes, cacheBudget-used)
	if limit <= 0 {
		return "", fmt.Errorf("archive %s: cache budget spent (%d of %d bytes)", sha[:12], used, int64(cacheBudget))
	}
	req, err := g.request(ctx, "/repos/"+owner+"/"+url.PathEscape(repo)+contract.ForgeArchive+sha+".tar.gz")
	if err != nil {
		return "", err
	}
	resp, err := g.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("archive %s: status %d", sha[:12], resp.StatusCode)
	}
	g.seq++
	dir := filepath.Join(g.dir, fmt.Sprintf("%s-%s-%d", repo, sha[:12], g.seq)) // never the held tree's path
	_ = os.RemoveAll(dir)
	n, err := unpack(resp.Body, dir, limit)
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", fmt.Errorf("archive %s: %w", sha[:12], err)
	}
	g.size[dir] = n
	return dir, nil
}

// unpack writes a Forgejo archive (git archive's: a pax global header, then one top directory, "<repo>/") into dir,
// stripping that directory, and answers the bytes it uses on disk, each file and directory rounded up to blockSize;
// more than limit is an error.
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
			if total += blockSize; total > limit {
				return total, overLimit(total, limit)
			}
			if err := root.MkdirAll(name, 0o755); err != nil {
				return total, err
			}
		case tar.TypeReg:
			if h.Size > maxFileBytes {
				continue // no extractor reads a file this large
			}
			if total += (h.Size + blockSize - 1) / blockSize * blockSize; total > limit {
				return total, overLimit(total, limit)
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

// overLimit is a refused tree's error: the round logs it at WARN with the repo, once per change.
func overLimit(total, limit int64) error {
	return fmt.Errorf("tree refused at %d bytes, over its limit of %d (maxTreeBytes, or what cacheBudget leaves)", total, limit)
}

func (g *forgeHeads) request(ctx context.Context, p string) (*http.Request, error) {
	if time.Now().Before(g.pauseUntil) {
		return nil, fmt.Errorf("rate limited until %s", g.pauseUntil.UTC().Format(time.TimeOnly))
	}
	req, err := http.NewRequestWithContext(ctx, "GET", g.url+p, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+g.token)
	return req, nil
}

// get fetches p. A 404 is errNoAccess (Forgejo hides a repo the token cannot read). A 429 (Cloudflare's, when the
// poll goes the public way; Forgejo itself has no rate limit by default) pauses the poll for its Retry-After, at least
// a minute. A 401 (a token Forgejo does not know) or 403 (one without read:repository) is an error like any other: it
// does not pause, and forge's last success ages into MountainSourceStale.
func (g *forgeHeads) get(ctx context.Context, p string) ([]byte, error) {
	req, err := g.request(ctx, p)
	if err != nil {
		return nil, err
	}
	resp, err := g.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
		b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if err != nil {
			return nil, err
		}
		g.met.SourceOK("forge", float64(time.Now().Unix()))
		return b, nil
	case http.StatusNotFound:
		return nil, errNoAccess
	case http.StatusTooManyRequests:
		ra, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
		g.pauseUntil = time.Now().Add(max(time.Duration(ra)*time.Second, time.Minute))
	}
	return nil, fmt.Errorf("%s: status %d", p, resp.StatusCode)
}
