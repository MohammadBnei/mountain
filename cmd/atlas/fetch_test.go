package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MohammadBnei/mountain/internal/extract"
	"github.com/MohammadBnei/mountain/realms"
)

type entry struct {
	name, body, link string
	typ              byte
	size             int64
}

func tarball(t *testing.T, es ...entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for _, e := range es {
		h := &tar.Header{Name: e.name, Typeflag: e.typ, Linkname: e.link, Mode: 0o644, Size: int64(len(e.body))}
		if e.typ == tar.TypeXGlobalHeader { // git archive's, the commit id in it
			h = &tar.Header{Name: e.name, Typeflag: e.typ, PAXRecords: map[string]string{"comment": e.body}}
		}
		if e.size > 0 {
			h.Size = e.size
		}
		if e.typ != tar.TypeReg {
			h.Size = 0
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if e.typ == tar.TypeReg {
			body := e.body
			if e.size > 0 {
				body = strings.Repeat("x", int(e.size))
			}
			_, _ = tw.Write([]byte(body))
		}
	}
	_ = tw.Close()
	_ = zw.Close()
	return buf.Bytes()
}

// TestUnpack: the pax global header is left out and the top directory stripped; links, "..", absolute paths and oversized files never land; nothing is
// written outside the checkout.
func TestUnpack(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "repo")
	b := tarball(t,
		entry{name: "pax_global_header", body: "abc", typ: tar.TypeXGlobalHeader},
		entry{name: "MohammadBnei-x-abc/", typ: tar.TypeDir},
		entry{name: "MohammadBnei-x-abc/docs/adr/0001.md", body: "# ADR-0001: one", typ: tar.TypeReg},
		entry{name: "MohammadBnei-x-abc/../../escape.txt", body: "no", typ: tar.TypeReg},
		entry{name: "MohammadBnei-x-abc//etc/abs.txt", body: "abs", typ: tar.TypeReg},
		entry{name: "MohammadBnei-x-abc/link", link: "/etc/passwd", typ: tar.TypeSymlink},
		entry{name: "MohammadBnei-x-abc/hard", link: "MohammadBnei-x-abc/docs/adr/0001.md", typ: tar.TypeLink},
		entry{name: "MohammadBnei-x-abc/big.bin", size: maxFileBytes + 1, typ: tar.TypeReg},
	)
	if _, err := unpack(bytes.NewReader(b), dir, maxTreeBytes); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "docs/adr/0001.md")); err != nil || string(got) != "# ADR-0001: one" {
		t.Errorf("file: %q %v", got, err)
	}
	for _, p := range []string{filepath.Join(base, "escape.txt"), filepath.Join(dir, "etc/abs.txt"), filepath.Join(dir, "link"), filepath.Join(dir, "hard"), filepath.Join(dir, "big.bin")} {
		if _, err := os.Lstat(p); err == nil {
			t.Errorf("%s was written", p)
		}
	}
	// a link planted inside the tree cannot be followed by a later entry either
	dir2 := filepath.Join(base, "repo2")
	_ = os.MkdirAll(dir2, 0o755)
	_ = os.Symlink(base, filepath.Join(dir2, "out"))
	if _, err := unpack(bytes.NewReader(tarball(t, entry{name: "top/out/pwned.txt", body: "x", typ: tar.TypeReg})), dir2, maxTreeBytes); err == nil {
		t.Error("a write through a symlink succeeded")
	}
	if _, err := os.Stat(filepath.Join(base, "pwned.txt")); err == nil {
		t.Error("escaped through the symlink")
	}
}

// TestUnpackBlocks: a tree is counted as the disk uses it (the emptyDir's sizeLimit counts blocks), each file and
// directory rounded up to blockSize, and the limit holds on that count.
func TestUnpackBlocks(t *testing.T) {
	b := tarball(t,
		entry{name: "top/d/", typ: tar.TypeDir},
		entry{name: "top/d/a", body: "x", typ: tar.TypeReg},
		entry{name: "top/b", size: blockSize + 1, typ: tar.TypeReg},
		entry{name: "top/empty", typ: tar.TypeReg},
	)
	if n, err := unpack(bytes.NewReader(b), filepath.Join(t.TempDir(), "r"), maxTreeBytes); err != nil || n != 4*blockSize {
		t.Errorf("unpacked: %d %v, want %d (d, a, two blocks of b, nothing for empty)", n, err, 4*blockSize)
	}
	if _, err := unpack(bytes.NewReader(b), filepath.Join(t.TempDir(), "r"), 3*blockSize); err == nil {
		t.Error("4 blocks unpacked within a limit of 3")
	}
}

// fakeForge answers the two Forgejo routes the poll makes, under /api/v1 as the real one (API is the base URL to give
// newForge): a branch (its head commit's id; a name that is only a tag is a 404, as on Forgejo) and an archive, laid
// out as Forgejo's (a pax global header, then "<repo>/"). status, when set, answers every request instead (with
// retryAfter as its Retry-After).
type fakeForge struct {
	*httptest.Server
	API        string
	mu         sync.Mutex
	head       map[string]string // repo@branch -> sha
	tag        map[string]string // repo@tag -> sha: a same-named tag must never answer for a branch
	status     int
	retryAfter string
	archives   int
	auth       []string
}

func newFakeForge(t *testing.T) *fakeForge {
	f := &fakeForge{head: map[string]string{}, tag: map[string]string{}}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.auth = append(f.auth, r.Header.Get("Authorization"))
		if f.status != 0 {
			w.Header().Set("Retry-After", f.retryAfter)
			w.WriteHeader(f.status)
			return
		}
		// the escaped path: a branch sent as feat%2Fx would decode to feat/x in r.URL.Path and pass unseen
		rest, ok := strings.CutPrefix(r.URL.EscapedPath(), "/api/v1/repos/MohammadBnei/")
		repo, rest, _ := strings.Cut(rest, "/")
		switch {
		case !ok:
			http.NotFound(w, r)
		case strings.HasPrefix(rest, "branches/"), strings.HasPrefix(rest, "tags/"):
			kind, name, _ := strings.Cut(rest, "/")
			refs := f.head
			if kind == "tags" {
				refs = f.tag
			}
			sha, ok := refs[repo+"@"+name]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(`{"name":"x","commit":{"id":"` + sha + `","message":"m"}}`))
		case strings.HasPrefix(rest, "archive/") && strings.HasSuffix(rest, ".tar.gz"):
			f.archives++
			sha := strings.TrimSuffix(strings.TrimPrefix(rest, "archive/"), ".tar.gz")
			_, _ = w.Write(tarball(t, entry{name: "pax_global_header", body: sha, typ: tar.TypeXGlobalHeader},
				entry{name: repo + "/", typ: tar.TypeDir}, entry{name: repo + "/README.md", body: sha, typ: tar.TypeReg}))
		default:
			http.NotFound(w, r)
		}
	}))
	f.API = f.URL + "/api/v1"
	t.Cleanup(f.Close)
	return f
}

// TestForgeHeads: an archive is fetched only when the head moves, forward or back; a branch with a slash is asked
// for as is; a repo the token cannot read is an error; the token rides every API request.
func TestForgeHeads(t *testing.T) {
	f := newFakeForge(t)
	sha1, sha2, sha0 := strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("c", 40)
	f.head["infra-bootstrap@main"] = sha1
	f.tag["infra-bootstrap@main"] = strings.Repeat("d", 40)
	g, err := newForge(f.API, "tok", t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r, err := g.checkout(ctx, "infra-bootstrap", "main")
	if err != nil || r.SHA != sha1 || r.Branch != "main" { // the tag main answers with dddd
		t.Fatalf("first: %+v %v", r, err)
	}
	if b, _ := os.ReadFile(filepath.Join(r.Dir, "README.md")); string(b) != sha1 {
		t.Errorf("tree: %q", b)
	}
	if r2, _ := g.checkout(ctx, "infra-bootstrap", "main"); r2 != r || f.archives != 1 {
		t.Errorf("unchanged head refetched: %d archives", f.archives)
	}
	f.head["infra-bootstrap@main"] = sha2
	r3, err := g.checkout(ctx, "infra-bootstrap", "main")
	if err != nil || r3.SHA != sha2 || f.archives != 2 {
		t.Fatalf("moved: %+v %v %d", r3, err, f.archives)
	}
	if _, err := os.Stat(r.Dir); err != nil {
		t.Error("the replaced tree went at once: the reader may still be serving it this round")
	}
	g.startRound()
	if _, err := os.Stat(r.Dir); err == nil {
		t.Error("the tree replaced last round was kept into the next")
	}
	f.head["infra-bootstrap@main"] = sha0
	if r4, _ := g.checkout(ctx, "infra-bootstrap", "main"); r4.SHA != sha0 || f.archives != 3 {
		t.Errorf("a branch reset back was not taken: %+v %d", r4, f.archives)
	}
	// a branch retargeted onto the commit already held: no download, and the held tree stays on disk (it was once
	// deleted as "the old tree", being the same path)
	f.head["infra-bootstrap@feat/x"] = sha0
	r6, err := g.checkout(ctx, "infra-bootstrap", "feat/x")
	if _, serr := os.Stat(filepath.Join(r6.Dir, "README.md")); err != nil || r6.Branch != "feat/x" || f.archives != 3 || serr != nil {
		t.Errorf("retargeted: %+v %v %d %v", r6, err, f.archives, serr)
	}
	if _, err := g.checkout(ctx, "agent-fleet", "main"); err != errNoAccess {
		t.Errorf("no access: %v", err)
	}
	for _, a := range f.auth {
		if a != "token tok" {
			t.Errorf("request without the token: %q", a)
		}
	}
}

// TestForgeRefusals: a 429 pauses the poll for its Retry-After (at least a minute) and asks nothing until then; a 401
// or 403 is an error, not errNoAccess, and does not pause.
func TestForgeRefusals(t *testing.T) {
	f := newFakeForge(t)
	f.head["infra-bootstrap@main"] = strings.Repeat("a", 40)
	ctx := context.Background()
	for _, st := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		g, err := newForge(f.API, "tok", t.TempDir(), nil)
		if err != nil {
			t.Fatal(err)
		}
		f.status = st
		if _, err := g.checkout(ctx, "infra-bootstrap", "main"); err == nil || errors.Is(err, errNoAccess) || !g.pauseUntil.IsZero() {
			t.Errorf("%d: %v, paused until %v", st, err, g.pauseUntil)
		}
	}
	g, err := newForge(f.API, "tok", t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for ra, want := range map[string]time.Duration{"": time.Minute, "5": time.Minute, "600": 10 * time.Minute} {
		g.pauseUntil = time.Time{}
		f.status, f.retryAfter = http.StatusTooManyRequests, ra
		start := time.Now()
		if _, err := g.checkout(ctx, "infra-bootstrap", "main"); err == nil {
			t.Fatalf("429 (Retry-After %q): no error", ra)
		}
		if got := g.pauseUntil.Sub(start); got < want || got > want+5*time.Second {
			t.Errorf("429 (Retry-After %q): paused %v, want %v", ra, got, want)
		}
		f.status, f.auth = 0, nil
		if _, err := g.checkout(ctx, "infra-bootstrap", "main"); err == nil || len(f.auth) != 0 {
			t.Errorf("429 (Retry-After %q): asked during the pause: %v, %d requests", ra, err, len(f.auth))
		}
	}
}

// fakeHeads serves internal/extract's fixture repos at a sha the test sets.
type fakeHeads map[string]string // repo -> sha

func (fakeHeads) startRound() {}

func (f fakeHeads) checkout(_ context.Context, repo, branch string) (extract.Repo, error) {
	sha, ok := f[repo]
	if !ok {
		return extract.Repo{}, errNoAccess
	}
	return extract.Repo{Dir: filepath.Join("..", "..", "internal", "extract", "testdata", repo), SHA: sha, Branch: branch}, nil
}

// TestRoundCache: a round whose heads all sit where the last stored round read them runs no extractor; a moved head
// runs them again; an unreadable source is left out (and the store, not the round, decides whether that is a loss).
func TestRoundCache(t *testing.T) {
	rl, err := realms.Load()
	if err != nil {
		t.Fatal(err)
	}
	h := fakeHeads{"infra-bootstrap": "a1", "agent-fleet": "b1"}
	c := &roundCache{}
	ctx := context.Background()
	unread := map[string]string{} // the projector's, kept across its rounds
	res, repos, err := round(ctx, h, rl, c, unread)
	if err != nil || len(res.Graph.Nodes) == 0 || len(repos) != 2 || c.infra != "a1" {
		t.Fatalf("first: %v %d nodes %v", err, len(res.Graph.Nodes), repos)
	}
	c.vector = vectorKey(repos)
	if _, _, err := round(ctx, h, rl, c, unread); err != errUnchanged {
		t.Errorf("same heads: %v", err)
	}
	h["agent-fleet"] = "b2"
	if res, _, err := round(ctx, h, rl, c, unread); err != nil || len(res.Graph.Nodes) == 0 {
		t.Errorf("a moved head: %v", err)
	}
}

// roundCounter is fakeHeads that counts its round starts.
type roundCounter struct {
	fakeHeads
	starts int
}

func (r *roundCounter) startRound() { r.starts++ }

// TestRoundStarts: every round starts the source's round first, unchanged heads included, so the trees the last round
// retired are removed even when nothing moved.
func TestRoundStarts(t *testing.T) {
	rl, err := realms.Load()
	if err != nil {
		t.Fatal(err)
	}
	h := &roundCounter{fakeHeads: fakeHeads{"infra-bootstrap": "a1"}}
	c, unread := &roundCache{}, map[string]string{}
	_, repos, err := round(context.Background(), h, rl, c, unread)
	if err != nil {
		t.Fatal(err)
	}
	c.vector = vectorKey(repos)
	if _, _, err := round(context.Background(), h, rl, c, unread); err != errUnchanged {
		t.Fatalf("same heads: %v", err)
	}
	if h.starts != 2 {
		t.Errorf("%d round starts in 2 rounds", h.starts)
	}
}

// TestProjectNoToken: with no token and no --repos, or a cache directory it cannot create, the projector stops at
// once, but seeds forge, so that deployment ages into MountainSourceStale instead of going quiet.
func TestProjectNoToken(t *testing.T) {
	file := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for name, o := range map[string]Opts{"no token": {}, "no cache": {ForgeToken: "tok", Cache: filepath.Join(file, "cache")}} {
		m, err := newMetrics()
		if err != nil {
			t.Fatal(err)
		}
		(&App{Opts: o, Met: m}).project(context.Background())
		if _, _, b := scrape(t, m); !strings.Contains(b, `mountain_source_last_success{source="forge"}`) {
			t.Errorf("%s: forge not seeded:\n%s", name, b)
		}
		_ = m.Close()
	}
}
