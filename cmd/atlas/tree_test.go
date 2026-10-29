package main

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/MohammadBnei/mountain/contract"
	"github.com/MohammadBnei/mountain/internal/extract"
	"github.com/MohammadBnei/ukubi-go/errs"
)

// writeTree writes files (path -> body) under dir.
func writeTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for p, body := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// getTree asks GET /api/tree?<q> and decodes a 200's answer.
func getTree(t *testing.T, a *tapp, q string) contract.Tree {
	t.Helper()
	r := a.serveReq("GET", "/api/tree?"+q, nil)
	var tr contract.Tree
	if r.StatusCode != http.StatusOK || json.NewDecoder(r.Body).Decode(&tr) != nil {
		t.Fatalf("tree %s: %d", q, r.StatusCode)
	}
	return tr
}

// TestTree: GET /api/tree lists every regular file of a held repo with its size, at the sha atlas holds, sorted, minus
// the directories the extractors skip (.git, node_modules, vendor and testdata anywhere, realms.yaml's skip rows and the
// vendored roots) and links; it stops at 20 000 files and says so; it is cached by repo and sha; it answers the
// reader's error keys; and it is a platform admin's only.
func TestTree(t *testing.T) {
	a := testApp(t)
	a.gate.devOpen = true
	base := t.TempDir()
	dir := filepath.Join(base, "infra-bootstrap")
	writeTree(t, dir, map[string]string{
		"README.md":                        "hello",
		"gitops/apps/registry.yaml":        "apps: []\n",
		"docs/adr/0001.md":                 "# ADR-0001",
		".git/HEAD":                        "ref: refs/heads/main\n",
		"web/node_modules/x/index.js":      "x",
		"vendor/y/y.go":                    "y",
		"cmd/z/testdata/z.json":            "{}",
		"kubespray/roles/a.yml":            "a",  // realms.yaml skip: infra-bootstrap
		"pigsty/conf/b.yml":                "b",  // realms.yaml skip: infra-bootstrap
		"k8s-cluster/c.yaml":               "c",  // a vendored root
		"agent-fleet/d.go":                 "d",  // a submodule, read as its own repo
		"gitops/node_modules_not/keep.txt": "ok", // only the exact names are skipped
	})
	_ = os.WriteFile(filepath.Join(base, "outside.txt"), []byte("secret"), 0o644)
	_ = os.Symlink(filepath.Join(base, "outside.txt"), filepath.Join(dir, "link.txt"))
	a.Files.update(extract.Repos{"infra-bootstrap": {Dir: dir, SHA: "s1", Branch: "main"}})

	tr := getTree(t, a, "repo=infra-bootstrap")
	want := []contract.TreeFile{
		{Path: "README.md", Size: 5}, {Path: "docs/adr/0001.md", Size: 10}, {Path: "gitops/apps/registry.yaml", Size: 9},
		{Path: "gitops/node_modules_not/keep.txt", Size: 2},
	}
	if tr.Repo != "infra-bootstrap" || tr.SHA != "s1" || tr.Truncated || !slices.Equal(tr.Files, want) {
		t.Errorf("tree: %+v\nwant files %+v", tr, want)
	}

	// cached by repo@sha: a file appearing under the same sha is not seen; a new sha is a new tree
	writeTree(t, dir, map[string]string{"new.txt": "n"})
	if tr := getTree(t, a, "repo=infra-bootstrap"); len(tr.Files) != len(want) {
		t.Errorf("same sha, %d files: not cached", len(tr.Files))
	}
	a.Files.update(extract.Repos{"infra-bootstrap": {Dir: dir, SHA: "s2", Branch: "main"}})
	if tr := getTree(t, a, "repo=infra-bootstrap"); tr.SHA != "s2" || len(tr.Files) != len(want)+1 {
		t.Errorf("new sha: %s, %d files", tr.SHA, len(tr.Files))
	}

	// truncation: 20 000 files, then truncated
	big := filepath.Join(base, "big")
	_ = os.MkdirAll(big, 0o755)
	for i := range 20_001 {
		if err := os.WriteFile(filepath.Join(big, fmt.Sprintf("f%05d", i)), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a.Files.update(extract.Repos{"agent-fleet": {Dir: big, SHA: "b1"}})
	if tr := getTree(t, a, "repo=agent-fleet"); len(tr.Files) != 20_000 || !tr.Truncated {
		t.Errorf("big: %d files, truncated %v", len(tr.Files), tr.Truncated)
	}

	// the reader's error keys
	for q, want := range map[string]struct {
		code errs.Code
		key  errs.Key
	}{
		"":          {errs.InvalidArgument, keyReadPath},
		"repo=":     {errs.InvalidArgument, keyReadPath},
		"repo=nope": {errs.NotFound, keyReadRepo},
	} {
		r := a.serveReq("GET", "/api/tree?"+q, nil)
		var b errs.Body
		if r.StatusCode != want.code.Status() || json.NewDecoder(r.Body).Decode(&b) != nil || b.Code != want.code || b.Key != string(want.key) {
			t.Errorf("tree?%s: %d %+v, want %s %s", q, r.StatusCode, b, want.code, want.key)
		}
	}

	// admin-only
	a.gate.devOpen = false
	if r := a.serveReq("GET", "/api/tree?repo=infra-bootstrap", nil); r.StatusCode != http.StatusForbidden {
		t.Errorf("anonymous: %d, want 403", r.StatusCode)
	}
}

// TestTreeRefusesMachine: the machine principal's token is refused on /api/tree (ADR-0068: its routes are the
// town's); an admin's cookie reads it.
func TestTreeRefusesMachine(t *testing.T) {
	a, p := withAuth(t)
	admin := adminCookie(t, a, p)
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	a.gate.machine = withMachine(t, certPEM(t, key, &key.PublicKey))
	tok := "Bearer " + mint(t, key, `{"alg":"RS256"}`, good(10*time.Minute))
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"README.md": "x"})
	a.Files.update(extract.Repos{"infra-bootstrap": {Dir: dir, SHA: "s1"}})

	if r := a.serveReq("GET", "/api/tree?repo=infra-bootstrap", nil, "Authorization", tok); r.StatusCode != http.StatusUnauthorized ||
		!strings.HasPrefix(r.Header.Get("WWW-Authenticate"), "Bearer") {
		t.Errorf("machine: %d, want 401", r.StatusCode)
	}
	if r := a.serveReq("GET", "/api/graph", nil, "Authorization", tok); r.StatusCode == http.StatusUnauthorized || r.StatusCode == http.StatusForbidden {
		t.Errorf("machine on its own route: %d (the token is good)", r.StatusCode)
	}
	if r := a.serveReq("GET", "/api/tree?repo=infra-bootstrap", []*http.Cookie{admin}); r.StatusCode != http.StatusOK {
		t.Errorf("admin: %d, want 200", r.StatusCode)
	}
}
