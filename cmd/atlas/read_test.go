package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// TestRedactShapes: the shapes a first redaction pass missed (A3 review): a secret in a link's URL, a key and its
// value in separate nodes or cells, a YAML block scalar, *_PASS keys, URL userinfo, Discord and Anthropic tokens.
func TestRedactShapes(t *testing.T) {
	md := "[hook](https://discord.com/api/webhooks/1234/abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJ) " +
		"[q](https://x.example/cb?token=hunter2hunter2) <https://x.example/?api_key=hunter3hunter3> " +
		"[u](https://admin:hunter4hunter4@x.example/) [fine](https://example.com/docs?page=2)\n\n" +
		"`password`: hunter5hunter5 and **token** = hunter6hunter6, but `token`: is described below.\n\n" +
		"| key | value | note |\n|---|---|---|\n| ARGOCD_ADMIN_PASSWORD | hunter7hunter7 | rotate yearly |\n" +
		"| `GITHUB_TOKEN` | the CI token, from Infisical | |\n| DB_PASS | ${DB_PASS} | a reference stays |\n\n" +
		"```yaml\npassword: |\n  hunter8hunter8\n  hunter9hunter9\nnext: 1\n```\n\n" +
		"DB_PASS: DBUser.Wiki and postgres://dbuser_meta:DBUser.Meta@10.0.0.1:5432/meta and postgres://u:${PW}@h/db; " +
		"MTA1MjM0NTY3ODkwMTIzNDU2Nw.GaBcDe.abcdefghijklmnopqrstuvwxyz0123 sk-ant-api03-abcdefghijklmnopqrstuvwxyz.\n"
	d := render("infra-bootstrap", "docs/x.md", "abc", []byte(md), func(string) bool { return false })
	all := string(mustJSON(d))
	for _, bad := range []string{"abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJ", "hunter", "DBUser.Wiki", "DBUser.Meta", "GaBcDe", "sk-ant-api03"} {
		if strings.Contains(all, bad) {
			t.Errorf("%q in the rendered doc: %s", bad, all)
		}
	}
	for _, keep := range []string{"https://example.com/docs?page=2", "the CI token, from Infisical", "${DB_PASS}", "${PW}", "rotate yearly", "next: 1", "dbuser_meta"} {
		if !strings.Contains(all, keep) {
			t.Errorf("%q redacted or lost: %s", keep, all)
		}
	}
	var hrefs []string
	walkBlocks(d.Blocks, func(b Block) {
		walkInlines(b.In, func(x Inline) {
			if x.Href != "" {
				hrefs = append(hrefs, x.Href)
			}
		})
	})
	if strings.Join(hrefs, " ") != "https://example.com/docs?page=2" {
		t.Errorf("only the clean URL stays a link: %v", hrefs)
	}
}

// TestRenderBounds: deep nesting comes back flat instead of overflowing the stack; escapes and entities read as a
// renderer shows them; a GitHub link with a query or a lower-case owner still resolves; a path the page's router
// would refuse stays text.
func TestRenderBounds(t *testing.T) {
	start := time.Now()
	for _, bad := range []string{strings.Repeat(">", 500000) + " deep", strings.Repeat("*a ", 170000), strings.Repeat("[", 256000)} {
		if d := render("r", "x.md", "s", []byte(bad), nil); d.Kind != "text" {
			t.Errorf("a pathological file was parsed as markdown (%.10q…)", bad)
		}
	}
	if time.Since(start) > 2*time.Second*raceSlowdown {
		t.Errorf("pathological files took %v", time.Since(start))
	}
	nested := strings.Repeat("> ", 32) + "deep\n\n" + strings.Repeat("- ", 30) + "list\n" // within mdSafe, past maxDepth
	if b := mustJSON(render("r", "x.md", "s", []byte(nested), nil)); !strings.Contains(string(b), "deep") || !strings.Contains(string(b), "list") {
		t.Errorf("deep nesting lost its text: %.300s", b)
	}
	d := render("infra-bootstrap", "docs/x.md", "abc", []byte("\\*not em\\* AT&amp;T &#65; "+
		"[g](https://github.com/mohammadbnei/agent-fleet/blob/main/docs/a.md?plain=1#L3) [p](odd(name).md)\n"), func(r string) bool { return r == "agent-fleet" })
	all := string(mustJSON(d))
	if got := plain(d.Blocks[0].In); !strings.HasPrefix(got, "*not em* AT&T A ") {
		t.Errorf("escapes and entities: %q", got)
	}
	if !strings.Contains(all, `"doc":"agent-fleet/docs/a.md"`) || strings.Contains(all, `"doc":"infra-bootstrap/`) {
		t.Errorf("links: %s", all)
	}
}

// TestRedactCountsOnce: a value hidden by its shape is not hidden (and counted) again by its key.
func TestRedactCountsOnce(t *testing.T) {
	n := 0
	if got := redact("token: ghp_abcdefghijklmnopqrstuvwxyz0123456789 and postgres://u:[redacted]@h/db", &n); n != 1 || !strings.HasPrefix(got, "token: [redacted] and") {
		t.Errorf("%d redactions: %q", n, got)
	}
}

// TestRefWithSlash: a GitHub link on a branch named with a slash lands on the file atlas holds; without one, the
// ref is the first segment.
func TestRefWithSlash(t *testing.T) {
	held := map[string]bool{"docs/a.md": true}
	exists := func(repo, p string) bool { return repo == "agent-fleet" && held[p] }
	md := "[x](https://github.com/MohammadBnei/agent-fleet/blob/feat/new-thing/docs/a.md) [y](https://github.com/MohammadBnei/agent-fleet/tree/main/docs/b.md)\n"
	all := string(mustJSON(renderIn("infra-bootstrap", "x.md", "s", []byte(md), func(r string) bool { return r == "agent-fleet" }, exists)))
	if !strings.Contains(all, `"doc":"agent-fleet/docs/a.md"`) || !strings.Contains(all, `"doc":"agent-fleet/docs/b.md"`) {
		t.Errorf("refs: %s", all)
	}
}

// TestRead: the endpoint reads a file of a held checkout at its sha; it refuses paths that leave the checkout,
// symlinks, binaries and repos atlas does not hold.
func TestRead(t *testing.T) {
	a := testApp(t)
	a.gate.devOpen = true
	base := t.TempDir()
	dir := filepath.Join(base, "infra-bootstrap")
	_ = os.MkdirAll(filepath.Join(dir, "docs"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "docs", "a.md"), []byte("# A\n\nhello"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "bin.dat"), []byte("x\x00y"), 0o644)
	_ = os.WriteFile(filepath.Join(base, "outside.md"), []byte("secret"), 0o644)
	_ = os.Symlink(filepath.Join(base, "outside.md"), filepath.Join(dir, "link.md"))
	a.Files.update(extract.Repos{"infra-bootstrap": {Dir: dir, SHA: "s1"}})
	r := a.serveReq("GET", "/api/read?repo=infra-bootstrap&path=docs/a.md", nil)
	var d Doc
	if r.StatusCode != http.StatusOK || json.NewDecoder(r.Body).Decode(&d) != nil || d.SHA != "s1" || d.Kind != "md" || len(d.Blocks) != 2 || r.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("read: %d %+v", r.StatusCode, d)
	}
	for q, want := range map[string]int{
		"repo=infra-bootstrap&path=../outside.md":   http.StatusBadRequest,
		"repo=infra-bootstrap&path=docs/../../x":    http.StatusBadRequest,
		"repo=infra-bootstrap&path=/etc/passwd":     http.StatusBadRequest,
		"repo=infra-bootstrap&path=link.md":         http.StatusNotFound,
		"repo=infra-bootstrap&path=bin.dat":         http.StatusBadRequest, // failed_precondition, read.not_text (was 415)
		"repo=infra-bootstrap&path=docs/missing.md": http.StatusNotFound,
		"repo=agent-fleet&path=README.md":           http.StatusNotFound,
		"repo=&path=x":                              http.StatusBadRequest,
	} {
		if r := a.serveReq("GET", "/api/read?"+q, nil); r.StatusCode != want {
			t.Errorf("%s: %d, want %d", q, r.StatusCode, want)
		}
	}
}
