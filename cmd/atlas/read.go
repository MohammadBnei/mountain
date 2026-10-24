package main

// The reader (plan/10 "The ladder", L4): GET /api/read?repo=<name>&path=<repo-relative> answers one file of a repo
// at the sha atlas last read it, as JSON the page builds with createElement and textContent. Markdown is parsed by
// goldmark (raw HTML is never passed on: an HTML block or inline is dropped) into blocks and inlines; any other text
// file comes back as lines. Every string passes a redaction pass first: the clones hold full files, including the
// deliberate ArgoCD webhook literal (infra-bootstrap argocd-application.yaml) and SCRAM verifiers.
//
// Links: http(s) URLs stay external; a relative link, or a github.com link into a repo atlas reads, becomes an
// in-atlas link {doc: "<repo>/<path>"} (cleaned, never above the repo root); anything else (mailto:, javascript:, a
// bare #anchor) is dropped to its text. Files are read through os.Root on the checkout, regular files only, at most
// maxReadBytes, valid UTF-8 without NUL bytes (415 otherwise).

import (
	"bytes"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"

	"github.com/MohammadBnei/mountain/internal/extract"
)

const maxReadBytes = 1 << 20

// Doc is GET /api/read's answer.
type Doc struct {
	Repo     string   `json:"repo"`
	Path     string   `json:"path"`
	SHA      string   `json:"sha"`
	Kind     string   `json:"kind"` // md | text
	Lang     string   `json:"lang,omitempty"`
	Blocks   []Block  `json:"blocks,omitempty"`
	Lines    []string `json:"lines,omitempty"`
	Redacted int      `json:"redacted"` // strings the redaction pass replaced
}

// Block is a markdown block: h (level, in), p (in), list (ordered, items), code (lang, text), quote (blocks), hr,
// table (rows of cells of inlines; the first row is the head).
type Block struct {
	T       string       `json:"t"`
	Level   int          `json:"level,omitempty"`
	Ordered bool         `json:"ordered,omitempty"`
	Lang    string       `json:"lang,omitempty"`
	Text    string       `json:"text,omitempty"`
	In      []Inline     `json:"in,omitempty"`
	Items   [][]Block    `json:"items,omitempty"`
	Blocks  []Block      `json:"blocks,omitempty"`
	Rows    [][][]Inline `json:"rows,omitempty"`
}

// Inline is text, strong, em, del, code, br, or link (href: external; doc: "<repo>/<path>" inside atlas).
type Inline struct {
	T    string   `json:"t"`
	Text string   `json:"text,omitempty"`
	Href string   `json:"href,omitempty"`
	Doc  string   `json:"doc,omitempty"`
	In   []Inline `json:"in,omitempty"`
}

// ---- redaction

var redactions = []*regexp.Regexp{
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?(-----END [A-Z ]*PRIVATE KEY-----|$)`),
	regexp.MustCompile(`\b(ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9]{30,}\b`),
	regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{40,}\b`),
	regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}\b`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`), // a JWT
	regexp.MustCompile(`SCRAM-SHA-256\$[^\s'"]+`),
	regexp.MustCompile(`\$2[aby]\$\d\d\$[./A-Za-z0-9]{53}`), // bcrypt
	// key: value / key=value where the key names a secret and the value is a literal (not a reference or a blank)
	regexp.MustCompile(`(?i)((?:password|passwd|secret|token|api[_-]?key|private[_-]?key|client[_-]?secret)["']?\s*[:=]\s*["']?)([^\s"'{}$<>][^\s"'<>]{7,})`),
}

// highEntropy matches a long run of key-like characters; it is redacted only when it mixes classes enough to be one.
var highEntropy = regexp.MustCompile(`[A-Za-z0-9+/_=-]{32,}`)

func redact(s string, n *int) string {
	for i, re := range redactions {
		s = re.ReplaceAllStringFunc(s, func(m string) string {
			*n++
			if i == len(redactions)-1 { // keep the key, hide the value
				sub := re.FindStringSubmatch(m)
				return sub[1] + "[redacted]"
			}
			return "[redacted]"
		})
	}
	return highEntropy.ReplaceAllStringFunc(s, func(m string) string {
		if !secretish(m) {
			return m
		}
		*n++
		return "[redacted]"
	})
}

// secretish: upper, lower and digits all present, and no long run of one class (a path, a sha or a word list is not
// a key). A 40-hex sha has no upper case, so commit shas stay readable.
func secretish(s string) bool {
	var up, lo, di int
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			up++
		case r >= 'a' && r <= 'z':
			lo++
		case r >= '0' && r <= '9':
			di++
		}
	}
	return up >= 4 && lo >= 4 && di >= 3 && !strings.Contains(s, "--") && !strings.Contains(s, "__")
}

// ---- the files

// repoSet is the checkouts the last round read (the reader's files), swapped whole by the projector.
type repoSet struct {
	mu    sync.Mutex
	repos extract.Repos
}

func (r *repoSet) set(repos extract.Repos) {
	r.mu.Lock()
	r.repos = repos
	r.mu.Unlock()
}

func (r *repoSet) get(name string) (extract.Repo, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	x, ok := r.repos[name]
	return x, ok
}

var (
	errNotText = errors.New("not a text file")
	errTooBig  = errors.New("larger than the reader reads")
)

// readFile reads repo-relative p from a checkout through os.Root: regular files only, ≤ maxReadBytes, text only.
func readFile(dir, p string) ([]byte, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	fi, err := root.Lstat(filepath.FromSlash(p))
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, os.ErrNotExist
	}
	if fi.Size() > maxReadBytes {
		return nil, errTooBig
	}
	b, err := root.ReadFile(filepath.FromSlash(p))
	if err != nil {
		return nil, err
	}
	if bytes.IndexByte(b, 0) >= 0 || !utf8.Valid(b) {
		return nil, errNotText
	}
	return b, nil
}

var langs = map[string]string{".go": "go", ".yaml": "yaml", ".yml": "yaml", ".js": "js", ".mjs": "js", ".ts": "ts", ".tsx": "ts",
	".json": "json", ".sh": "sh", ".bash": "sh", ".sql": "sql", ".tf": "hcl", ".hcl": "hcl", ".py": "py", ".toml": "toml",
	".proto": "proto", ".css": "css", ".html": "html", ".j2": "jinja", ".ini": "ini", ".cfg": "ini"}

func langOf(p string) string {
	if base := path.Base(p); base == "Dockerfile" || strings.HasPrefix(base, "Dockerfile.") {
		return "docker"
	}
	return langs[strings.ToLower(path.Ext(p))]
}

// render turns a file into a Doc. repos is the set of repos a link may point into.
func render(repo, p, sha string, b []byte, inAtlas func(string) bool) Doc {
	d := Doc{Repo: repo, Path: p, SHA: sha}
	ext := strings.ToLower(path.Ext(p))
	if ext != ".md" && ext != ".markdown" {
		d.Kind, d.Lang = "text", langOf(p)
		red := redact(string(b), &d.Redacted)
		d.Lines = strings.Split(strings.TrimRight(red, "\n"), "\n")
		return d
	}
	d.Kind = "md"
	md := goldmark.New(goldmark.WithExtensions(extension.Table, extension.Strikethrough))
	doc := md.Parser().Parse(text.NewReader(b))
	r := &mdWalker{src: b, repo: repo, dir: path.Dir(p), inAtlas: inAtlas, n: &d.Redacted}
	d.Blocks = r.blocks(doc)
	return d
}

type mdWalker struct {
	src     []byte
	repo    string
	dir     string
	inAtlas func(string) bool
	n       *int
}

func (w *mdWalker) blocks(parent ast.Node) []Block {
	out := []Block{}
	for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
		switch x := c.(type) {
		case *ast.Heading:
			out = append(out, Block{T: "h", Level: x.Level, In: w.inlines(x)})
		case *ast.Paragraph, *ast.TextBlock:
			if in := w.inlines(c); len(in) > 0 {
				out = append(out, Block{T: "p", In: in})
			}
		case *ast.List:
			l := Block{T: "list", Ordered: x.IsOrdered()}
			for it := x.FirstChild(); it != nil; it = it.NextSibling() {
				l.Items = append(l.Items, w.blocks(it))
			}
			out = append(out, l)
		case *ast.FencedCodeBlock:
			out = append(out, Block{T: "code", Lang: string(x.Language(w.src)), Text: w.redact(w.lines(x))})
		case *ast.CodeBlock:
			out = append(out, Block{T: "code", Text: w.redact(w.lines(x))})
		case *ast.Blockquote:
			out = append(out, Block{T: "quote", Blocks: w.blocks(x)})
		case *ast.ThematicBreak:
			out = append(out, Block{T: "hr"})
		case *east.Table:
			t := Block{T: "table"}
			for row := x.FirstChild(); row != nil; row = row.NextSibling() {
				var cells [][]Inline
				for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
					cells = append(cells, w.inlines(cell))
				}
				t.Rows = append(t.Rows, cells)
			}
			out = append(out, t)
		case *ast.HTMLBlock:
			// raw HTML is never passed on (plan/10: the page has no innerHTML, and atlas does not build it either)
		default:
			out = append(out, w.blocks(c)...)
		}
	}
	return out
}

func (w *mdWalker) lines(n ast.Node) string {
	var b strings.Builder
	for i := 0; i < n.Lines().Len(); i++ {
		seg := n.Lines().At(i)
		b.Write(seg.Value(w.src))
	}
	return strings.TrimRight(b.String(), "\n")
}

func (w *mdWalker) redact(s string) string { return redact(s, w.n) }

func (w *mdWalker) inlines(parent ast.Node) []Inline {
	out := []Inline{}
	add := func(in Inline) {
		if in.T == "text" && len(out) > 0 && out[len(out)-1].T == "text" { // merge runs: fewer nodes, and a secret split across segments is redacted whole
			out[len(out)-1].Text += in.Text
			return
		}
		out = append(out, in)
	}
	for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
		switch x := c.(type) {
		case *ast.Text:
			add(Inline{T: "text", Text: string(x.Value(w.src))})
			if x.HardLineBreak() {
				add(Inline{T: "br"})
			} else if x.SoftLineBreak() {
				add(Inline{T: "text", Text: " "})
			}
		case *ast.String:
			add(Inline{T: "text", Text: string(x.Value)})
		case *ast.CodeSpan:
			var b strings.Builder
			for t := x.FirstChild(); t != nil; t = t.NextSibling() {
				if s, ok := t.(*ast.Text); ok {
					b.Write(s.Value(w.src))
				}
			}
			add(Inline{T: "code", Text: b.String()})
		case *ast.Emphasis:
			t := "em"
			if x.Level >= 2 {
				t = "strong"
			}
			add(Inline{T: t, In: w.inlines(x)})
		case *east.Strikethrough:
			add(Inline{T: "del", In: w.inlines(x)})
		case *ast.Link:
			add(w.link(string(x.Destination), w.inlines(x)))
		case *ast.AutoLink:
			u := string(x.URL(w.src))
			add(w.link(u, []Inline{{T: "text", Text: u}}))
		case *ast.Image:
			add(Inline{T: "text", Text: "[image: " + plain(w.inlines(x)) + "]"}) // no image is fetched from a repo
		case *ast.RawHTML:
			// dropped, as HTML blocks are
		default:
			for _, in := range w.inlines(c) {
				add(in)
			}
		}
	}
	for i := range out { // redaction last, on merged runs
		out[i] = redactInline(out[i], w.n)
	}
	return out
}

func redactInline(in Inline, n *int) Inline {
	in.Text = redact(in.Text, n)
	for i := range in.In {
		in.In[i] = redactInline(in.In[i], n)
	}
	return in
}

func plain(in []Inline) string {
	var b strings.Builder
	for _, x := range in {
		b.WriteString(x.Text)
		b.WriteString(plain(x.In))
	}
	return b.String()
}

var ghBlob = regexp.MustCompile(`^https://github\.com/MohammadBnei/([A-Za-z0-9._-]+)/(?:blob|tree)/[^/]+/(.+)$`)

// link resolves a destination: external http(s) kept, in-atlas docs resolved, anything else reduced to its text.
func (w *mdWalker) link(dest string, in []Inline) Inline {
	u, err := url.Parse(dest)
	switch {
	case err != nil:
	case u.Scheme == "http" || u.Scheme == "https":
		if m := ghBlob.FindStringSubmatch(dest); m != nil && w.inAtlas(m[1]) {
			if p := cleanRel(strings.SplitN(m[2], "#", 2)[0]); p != "" {
				return Inline{T: "link", Doc: m[1] + "/" + p, In: in}
			}
		}
		return Inline{T: "link", Href: u.String(), In: in}
	case u.Scheme == "" && u.Host == "" && u.Path != "":
		target := u.Path
		if !strings.HasPrefix(target, "/") {
			target = path.Join(w.dir, target)
		}
		if p := cleanRel(target); p != "" {
			return Inline{T: "link", Doc: w.repo + "/" + p, In: in}
		}
	}
	return Inline{T: "em", In: in} // a link atlas will not follow keeps its words
}

// cleanRel is p as a clean repo-relative path, or "" when it would leave the repo.
func cleanRel(p string) string {
	c := path.Clean("/" + p)
	if c == "/" || strings.Contains(p, "\x00") {
		return ""
	}
	if pre := path.Clean(p); strings.HasPrefix(pre, "../") || pre == ".." {
		return ""
	}
	return strings.TrimPrefix(c, "/")
}

// serveRead is GET /api/read (admin-only by the route table).
func serveRead(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Cache-Control", "private, no-store")
	h.Set("Vary", "Cookie")
	h.Set("X-Content-Type-Options", "nosniff")
	q := r.URL.Query()
	repo, p := q.Get("repo"), cleanRel(q.Get("path"))
	if app == nil || repo == "" || p == "" || p != q.Get("path") { // only a path already clean and relative
		http.Error(w, "repo and a clean repo-relative path are required", http.StatusBadRequest)
		return
	}
	rp, ok := app.Files.get(repo)
	if !ok {
		http.Error(w, "atlas does not hold that repo", http.StatusNotFound)
		return
	}
	b, err := readFile(rp.Dir, p)
	switch {
	case errors.Is(err, errNotText):
		http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
		return
	case errors.Is(err, errTooBig):
		http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
		return
	case err != nil:
		http.Error(w, "no such file at the sha atlas holds", http.StatusNotFound)
		return
	}
	d := render(repo, p, rp.SHA, b, func(name string) bool { _, ok := app.Files.get(name); return ok })
	h.Set("Content-Type", "application/json")
	w.Write(mustJSON(d))
}
