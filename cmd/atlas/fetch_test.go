package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

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
			tw.Write([]byte(body))
		}
	}
	tw.Close()
	zw.Close()
	return buf.Bytes()
}

// TestUnpack: the top directory is stripped; links, "..", absolute paths and oversized files never land; nothing is
// written outside the checkout.
func TestUnpack(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "repo")
	b := tarball(t,
		entry{name: "MohammadBnei-x-abc/", typ: tar.TypeDir},
		entry{name: "MohammadBnei-x-abc/docs/adr/0001.md", body: "# ADR-0001: one", typ: tar.TypeReg},
		entry{name: "MohammadBnei-x-abc/../../escape.txt", body: "no", typ: tar.TypeReg},
		entry{name: "MohammadBnei-x-abc//etc/abs.txt", body: "abs", typ: tar.TypeReg},
		entry{name: "MohammadBnei-x-abc/link", link: "/etc/passwd", typ: tar.TypeSymlink},
		entry{name: "MohammadBnei-x-abc/hard", link: "MohammadBnei-x-abc/docs/adr/0001.md", typ: tar.TypeLink},
		entry{name: "MohammadBnei-x-abc/big.bin", size: maxFileBytes + 1, typ: tar.TypeReg},
	)
	if err := unpack(bytes.NewReader(b), dir); err != nil {
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
	os.MkdirAll(dir2, 0o755)
	os.Symlink(base, filepath.Join(dir2, "out"))
	if err := unpack(bytes.NewReader(tarball(t, entry{name: "top/out/pwned.txt", body: "x", typ: tar.TypeReg})), dir2); err == nil {
		t.Error("a write through a symlink succeeded")
	}
	if _, err := os.Stat(filepath.Join(base, "pwned.txt")); err == nil {
		t.Error("escaped through the symlink")
	}
}

// fakeGitHub answers the three calls the poll makes: a branch head (with an ETag), a compare and a tarball.
type fakeGitHub struct {
	*httptest.Server
	mu       sync.Mutex
	head     map[string]string // repo@branch -> sha
	status   string            // compare answer
	tarballs int
	auth     []string
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	f := &fakeGitHub{head: map[string]string{}, status: "ahead"}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.auth = append(f.auth, r.Header.Get("Authorization"))
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/repos/MohammadBnei/"), "/")
		switch {
		case len(parts) == 3 && parts[1] == "commits":
			sha, ok := f.head[parts[0]+"@"+parts[2]]
			if !ok {
				http.NotFound(w, r)
				return
			}
			if r.Header.Get("If-None-Match") == `"`+sha+`"` {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("ETag", `"`+sha+`"`)
			w.Write([]byte(sha))
		case len(parts) == 3 && parts[1] == "compare":
			if f.status == "error" {
				http.Error(w, "gone", http.StatusUnprocessableEntity)
				return
			}
			w.Write([]byte(`{"status":"` + f.status + `"}`))
		case len(parts) == 3 && parts[1] == "tarball":
			f.tarballs++
			w.Write(tarball(t, entry{name: "top/README.md", body: parts[2], typ: tar.TypeReg}))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

// TestGitHubHeads: a tarball is fetched only when the head moves; a head behind the one held is ignored; a repo the
// token cannot read is an error; the token rides every API request.
func TestGitHubHeads(t *testing.T) {
	f := newFakeGitHub(t)
	sha1, sha2, sha0 := strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("c", 40)
	f.head["infra-bootstrap@main"] = sha1
	g, err := newGitHub(f.URL, "tok", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r, err := g.checkout(ctx, "infra-bootstrap", "main")
	if err != nil || r.SHA != sha1 || r.Branch != "main" {
		t.Fatalf("first: %+v %v", r, err)
	}
	if b, _ := os.ReadFile(filepath.Join(r.Dir, "README.md")); string(b) != sha1 {
		t.Errorf("tree: %q", b)
	}
	if r2, _ := g.checkout(ctx, "infra-bootstrap", "main"); r2 != r || f.tarballs != 1 {
		t.Errorf("unchanged head refetched: %d tarballs", f.tarballs)
	}
	f.head["infra-bootstrap@main"] = sha2
	r3, err := g.checkout(ctx, "infra-bootstrap", "main")
	if err != nil || r3.SHA != sha2 || f.tarballs != 2 {
		t.Fatalf("moved: %+v %v %d", r3, err, f.tarballs)
	}
	if _, err := os.Stat(r.Dir); err != nil {
		t.Error("the replaced tree went at once: the reader may still be serving it this round")
	}
	f.head["infra-bootstrap@main"], f.status = sha0, "behind"
	if r4, _ := g.checkout(ctx, "infra-bootstrap", "main"); r4.SHA != sha2 || f.tarballs != 2 {
		t.Errorf("a head behind was taken: %+v", r4)
	}
	if r5, _ := g.checkout(ctx, "infra-bootstrap", "main"); r5.SHA != sha0 || f.tarballs != 3 {
		t.Errorf("the same head behind, a second round running: a real reset, taken: %+v %d", r5, f.tarballs)
	}
	if _, err := os.Stat(r.Dir); err == nil {
		t.Error("the tree replaced two rounds ago was kept")
	}
	if _, err := os.Stat(r3.Dir); err != nil {
		t.Error("the tree replaced this round went at once")
	}
	// a branch retargeted onto the commit already held: no download, and the held tree stays on disk (it was once
	// deleted as "the old tree", being the same path)
	f.head["infra-bootstrap@dev"] = sha0
	r6, err := g.checkout(ctx, "infra-bootstrap", "dev")
	if _, serr := os.Stat(filepath.Join(r6.Dir, "README.md")); err != nil || r6.Branch != "dev" || f.tarballs != 3 || serr != nil {
		t.Errorf("retargeted: %+v %v %d %v", r6, err, f.tarballs, serr)
	}
	f.status = "error"
	f.head["infra-bootstrap@dev"] = sha1
	if r7, _ := g.checkout(ctx, "infra-bootstrap", "dev"); r7.SHA != sha1 {
		t.Errorf("an unreadable compare pinned the old head: %+v", r7)
	}
	if _, err := g.checkout(ctx, "agent-fleet", "main"); err != errNoAccess {
		t.Errorf("no access: %v", err)
	}
	for _, a := range f.auth {
		if a != "Bearer tok" {
			t.Errorf("request without the token: %q", a)
		}
	}
}

// fakeHeads serves internal/extract's fixture repos at a sha the test sets.
type fakeHeads map[string]string // repo -> sha

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
