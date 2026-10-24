package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MohammadBnei/mountain/internal/extract"
)

const sampleMD = "# ADR-0099: a title\n\n**Status:** Proposed. See [the other one](0098-other.md), [up](../../README.md), " +
	"[away](../../../../etc/passwd), [ext](https://example.com/x), [gh](https://github.com/MohammadBnei/agent-fleet/blob/main/docs/adr/0001.md#x), " +
	"[js](javascript:alert(1)) and <b>raw</b>.\n\n<script>alert(1)</script>\n\n- one `code`\n- two ~~old~~\n\n" +
	"```go\ntoken := \"ghp_abcdefghijklmnopqrstuvwxyz0123456789\"\n```\n\n| a | b |\n|---|---|\n| 1 | password: hunter2hunter2 |\n\n" +
	"sha 0123456789abcdef0123456789abcdef01234567 stays; a key Ab3dEf7hIj1lMn5pQr9tUv2xYz4BcD8fGh0 goes.\n"

func walkInlines(in []Inline, f func(Inline)) {
	for _, x := range in {
		f(x)
		walkInlines(x.In, f)
	}
}

func walkBlocks(bs []Block, f func(Block)) {
	for _, b := range bs {
		f(b)
		walkBlocks(b.Blocks, f)
		for _, it := range b.Items {
			walkBlocks(it, f)
		}
	}
}

// TestRender: markdown becomes blocks and inlines; raw HTML is dropped; links resolve inside atlas or stay external,
// and nothing else is a link; secrets are redacted in text, code and tables; a sha is not a secret.
func TestRender(t *testing.T) {
	d := render("infra-bootstrap", "docs/adr/0099-x.md", "abc", []byte(sampleMD), func(r string) bool { return r == "agent-fleet" || r == "infra-bootstrap" })
	all := string(mustJSON(d))
	for _, bad := range []string{"<script", "<b>", "javascript", "ghp_abc", "hunter2", "Ab3dEf7hIj1lMn5p", "etc/passwd"} {
		if strings.Contains(all, bad) {
			t.Errorf("%q in the rendered doc: %s", bad, all)
		}
	}
	if !strings.Contains(all, "0123456789abcdef0123456789abcdef01234567") || d.Redacted < 3 {
		t.Errorf("the sha was redacted, or too few redactions (%d)", d.Redacted)
	}
	links := map[string]string{}
	var kinds []string
	walkBlocks(d.Blocks, func(b Block) {
		kinds = append(kinds, b.T)
		walk := func(in []Inline) {
			walkInlines(in, func(x Inline) {
				if x.T == "link" {
					links[plain(x.In)] = x.Href + x.Doc
				}
			})
		}
		walk(b.In)
		for _, row := range b.Rows {
			for _, c := range row {
				walk(c)
			}
		}
	})
	want := map[string]string{"the other one": "infra-bootstrap/docs/adr/0098-other.md", "up": "infra-bootstrap/README.md",
		"ext": "https://example.com/x", "gh": "agent-fleet/docs/adr/0001.md"}
	for k, v := range want {
		if links[k] != v {
			t.Errorf("link %q -> %q, want %q", k, links[k], v)
		}
	}
	if _, ok := links["away"]; ok {
		t.Error("a link above the repo root resolved")
	}
	if _, ok := links["js"]; ok {
		t.Error("a javascript: link survived")
	}
	if got := strings.Join(kinds, ","); !strings.Contains(got, "h,p,list") || !strings.Contains(got, "code") || !strings.Contains(got, "table") {
		t.Errorf("blocks: %s", got)
	}
	txt := render("infra-bootstrap", "gitops/x.yaml", "abc", []byte("a: 1\nclientSecret: Zx81kqPw0aLmn2vC\n"), nil)
	if txt.Kind != "text" || txt.Lang != "yaml" || len(txt.Lines) != 2 || strings.Contains(txt.Lines[1], "Zx81") || !strings.HasPrefix(txt.Lines[1], "clientSecret: ") {
		t.Errorf("text: %+v", txt)
	}
}

// TestRead: the endpoint reads a file of a held checkout at its sha; it refuses paths that leave the checkout,
// symlinks, binaries and repos atlas does not hold.
func TestRead(t *testing.T) {
	a, h := testApp(t)
	devOpen = true
	t.Cleanup(func() { devOpen = false })
	mux = h
	base := t.TempDir()
	dir := filepath.Join(base, "infra-bootstrap")
	os.MkdirAll(filepath.Join(dir, "docs"), 0o755)
	os.WriteFile(filepath.Join(dir, "docs", "a.md"), []byte("# A\n\nhello"), 0o644)
	os.WriteFile(filepath.Join(dir, "bin.dat"), []byte("x\x00y"), 0o644)
	os.WriteFile(filepath.Join(base, "outside.md"), []byte("secret"), 0o644)
	os.Symlink(filepath.Join(base, "outside.md"), filepath.Join(dir, "link.md"))
	a.Files.set(extract.Repos{"infra-bootstrap": {Dir: dir, SHA: "s1"}})
	r := serveReq("GET", "/api/read?repo=infra-bootstrap&path=docs/a.md", nil)
	var d Doc
	if r.StatusCode != http.StatusOK || json.NewDecoder(r.Body).Decode(&d) != nil || d.SHA != "s1" || d.Kind != "md" || len(d.Blocks) != 2 || r.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("read: %d %+v", r.StatusCode, d)
	}
	for q, want := range map[string]int{
		"repo=infra-bootstrap&path=../outside.md":   http.StatusBadRequest,
		"repo=infra-bootstrap&path=docs/../../x":    http.StatusBadRequest,
		"repo=infra-bootstrap&path=/etc/passwd":     http.StatusBadRequest,
		"repo=infra-bootstrap&path=link.md":         http.StatusNotFound,
		"repo=infra-bootstrap&path=bin.dat":         http.StatusUnsupportedMediaType,
		"repo=infra-bootstrap&path=docs/missing.md": http.StatusNotFound,
		"repo=agent-fleet&path=README.md":           http.StatusNotFound,
		"repo=&path=x":                              http.StatusBadRequest,
	} {
		if r := serveReq("GET", "/api/read?"+q, nil); r.StatusCode != want {
			t.Errorf("%s: %d, want %d", q, r.StatusCode, want)
		}
	}
}
