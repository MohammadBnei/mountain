package main

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"testing"
	"time"

	"github.com/MohammadBnei/mountain/internal/extract"
)

// TestTreeMachine: the machine principal reads a building's rooms, GET /api/tree and GET /api/read (ADR-0068 as
// amended); the page and a path under /api/read/ (the catch-all GET /) still refuse it.
func TestTreeMachine(t *testing.T) {
	a, _ := withAuth(t)
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	a.gate.machine = withMachine(t, certPEM(t, key, &key.PublicKey))
	tok := "Bearer " + mint(t, key, `{"alg":"RS256"}`, good(10*time.Minute))
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"README.md": "# x"})
	a.Files.update(extract.Repos{"infra-bootstrap": {Dir: dir, SHA: "s1"}})
	for _, p := range []string{"/api/tree?repo=infra-bootstrap", "/api/read?repo=infra-bootstrap&path=README.md"} {
		if r := a.serveReq("GET", p, nil, "Authorization", tok); r.StatusCode != http.StatusOK {
			t.Errorf("machine GET %s: %d, want 200", p, r.StatusCode)
		}
	}
	for _, p := range []string{"/", "/api/read/"} {
		if r := a.serveReq("GET", p, nil, "Authorization", tok); r.StatusCode != http.StatusUnauthorized {
			t.Errorf("machine GET %s: %d, want 401", p, r.StatusCode)
		}
	}
}
