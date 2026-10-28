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
// maxReadBytes, valid UTF-8 without NUL bytes (failed_precondition otherwise, keyed read.too_large or read.not_text).

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
	"github.com/yuin/goldmark/util"

	"github.com/MohammadBnei/mountain/internal/extract"
	"github.com/MohammadBnei/mountain/pkg/errs"
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
	regexp.MustCompile(`\$2[aby]\$\d\d\$[./A-Za-z0-9]{53}`),                                 // bcrypt
	regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_-]{20,}`),                                       // an Anthropic key
	regexp.MustCompile(`\b[MNO][A-Za-z0-9_-]{23,25}\.[A-Za-z0-9_-]{6}\.[A-Za-z0-9_-]{27,}`), // a Discord bot token
	regexp.MustCompile(`(?i)discord(?:app)?\.com/api/webhooks/\d+/[A-Za-z0-9_-]+`),          // a Discord webhook
}

// keyValue is key: value / key=value where the key names a secret and the value is a literal (not a reference or a
// blank). The key stays, the value goes. secretKeys alone are the key names, also used for a key and its value in separate
// nodes (a table row, `password`: value) and for YAML block scalars (password: |).
const secretKeys = `(?:password|passwd|pass|pwd|secret|token|api[_-]?key|private[_-]?key|client[_-]?secret|session[_-]?key|access[_-]?key|secret[_-]?key)`

var (
	keyValue   = regexp.MustCompile(`(?i)((?:\b|_)` + secretKeys + `\b["']?\s*[:=]\s*["']?)([^\s"'{}$<>][^\s"'<>]{7,})`)
	valueAfter = regexp.MustCompile(`^(["']?\s*[:=]\s*["']?)([^\s"'{}$<>][^\s"'<>]{7,})`)    // ": value", its key in the node before
	keyBefore  = regexp.MustCompile(`(?i)(?:^|\b|_)` + secretKeys + `["']?\s*$`)             // that node: it ends with a key name
	keyCell    = regexp.MustCompile(`(?i)^\s*[A-Za-z0-9_.-]*(?:\b|_)` + secretKeys + `\s*$`) // a table cell holding a key name only
	blockKey   = regexp.MustCompile(`(?i)^(\s*)[\w.-]*(?:\b|_)` + secretKeys + `["']?\s*:\s*[|>][-+0-9]*\s*$`)
	userinfo   = regexp.MustCompile(`(://[^/\s:@]+:)([^@\s/]+)(@)`) // scheme://user:password@host
	queryKey   = regexp.MustCompile(`(?i)^(?:[A-Za-z0-9_.-]*` + secretKeys + `|sig|key|code)$`)
)

// highEntropy matches a long run of key-like characters; it is redacted only when it mixes classes enough to be one.
// The = is padding only: inside the run it would join a name to its value (PROJ=<uuid>) into one "mixed" token; and
// no /, which would join a URL's path segments into one.
var highEntropy = regexp.MustCompile(`[A-Za-z0-9+_-]{32,}={0,2}`)

func redact(s string, n *int) string {
	s = redactBlockScalars(s, n)
	for _, re := range redactions {
		s = re.ReplaceAllStringFunc(s, func(string) string { *n++; return "[redacted]" })
	}
	s = keyValue.ReplaceAllStringFunc(s, func(m string) string {
		sub := keyValue.FindStringSubmatch(m)
		if !literal(sub[2]) {
			return m
		}
		*n++
		return sub[1] + "[redacted]"
	})
	s = userinfo.ReplaceAllStringFunc(s, func(m string) string {
		sub := userinfo.FindStringSubmatch(m)
		if strings.HasPrefix(sub[2], "$") || strings.HasPrefix(sub[2], "{") || strings.HasPrefix(sub[2], "<") || strings.HasPrefix(sub[2], "[redacted]") { // a reference, not a value; or hidden already
			return m
		}
		*n++
		return sub[1] + "[redacted]" + sub[3]
	})
	return highEntropy.ReplaceAllStringFunc(s, func(m string) string {
		if !secretish(m) {
			return m
		}
		*n++
		return "[redacted]"
	})
}

// literal: a value after a secret's key that reads as one, not as prose or a pointer to one: not a path (/run/…), not
// a label ending in ":" ("token: cloudflare: failed…"), not a lower-case word ("password: infeasible to crack").
func literal(v string) bool {
	if strings.HasPrefix(v, "/") || strings.HasSuffix(v, ":") || strings.HasPrefix(v, "[redacted]") { // [redacted]: hidden already, counted once
		return false
	}
	for _, r := range v {
		if r < 'a' || r > 'z' {
			return true
		}
	}
	return false
}

// redactBlockScalars hides the lines of a YAML block scalar under a secret key (password: |, then the value on the
// lines indented below it).
func redactBlockScalars(s string, n *int) string {
	if !strings.ContainsAny(s, "|>") {
		return s
	}
	lines := strings.Split(s, "\n")
	for i := 0; i < len(lines); i++ {
		m := blockKey.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		hid := false
		for j := i + 1; j < len(lines); j++ {
			l := lines[j]
			if strings.TrimSpace(l) == "" {
				continue
			}
			ind := len(l) - len(strings.TrimLeft(l, " \t"))
			if ind <= len(m[1]) {
				break
			}
			lines[j] = l[:ind] + "[redacted]"
			hid = true
			i = j
		}
		if hid {
			*n++
		}
	}
	return strings.Join(lines, "\n")
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

// repoSet is the checkouts the reader serves: each repo at the tree the latest round read it at. A round overlays
// the repos it read; one it did not read (a failed checkout, a round cut short) keeps its last tree, which the
// GitHub source removes only when it replaces it, one round after the reader moved on.
type repoSet struct {
	mu    sync.Mutex
	repos extract.Repos
}

func (r *repoSet) update(repos extract.Repos) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.repos == nil {
		r.repos = extract.Repos{}
	}
	for name, x := range repos {
		r.repos[name] = x
	}
}

func (r *repoSet) get(name string) (extract.Repo, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	x, ok := r.repos[name]
	return x, ok
}

// exists: repo-relative p is a regular file in the tree atlas holds for repo.
func (r *repoSet) exists(repo, p string) bool {
	x, ok := r.get(repo)
	if !ok {
		return false
	}
	root, err := os.OpenRoot(x.Dir)
	if err != nil {
		return false
	}
	defer func() { _ = root.Close() }()
	fi, err := root.Lstat(filepath.FromSlash(p))
	return err == nil && fi.Mode().IsRegular()
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
	defer func() { _ = root.Close() }()
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

// render turns a file into a Doc. inAtlas says whether atlas holds a repo a link may point into.
func render(repo, p, sha string, b []byte, inAtlas func(string) bool) Doc {
	return renderIn(repo, p, sha, b, inAtlas, nil)
}

// renderIn is render with exists (repo, path → a file atlas holds), which places a GitHub link's ref when the ref has
// a slash in it (blob/feat/x/docs/a.md: which segments are the branch is only known by which path exists).
func renderIn(repo, p, sha string, b []byte, inAtlas func(string) bool, exists func(repo, p string) bool) Doc {
	d := Doc{Repo: repo, Path: p, SHA: sha}
	ext := strings.ToLower(path.Ext(p))
	if ext != ".md" && ext != ".markdown" || !mdSafe(b) {
		d.Kind, d.Lang = "text", langOf(p)
		red := redact(string(b), &d.Redacted)
		d.Lines = strings.Split(strings.TrimRight(red, "\n"), "\n")
		return d
	}
	d.Kind = "md"
	md := goldmark.New(goldmark.WithExtensions(extension.Table, extension.Strikethrough))
	doc := md.Parser().Parse(text.NewReader(b))
	r := &mdWalker{src: b, repo: repo, dir: path.Dir(p), inAtlas: inAtlas, exists: exists, n: &d.Redacted}
	d.Blocks = r.blocks(doc)
	return d
}

// mdSafe: the file is shaped like prose, not like the inputs goldmark parses in quadratic time (a paragraph of 64 KiB
// or more, quotes nested more than 32 deep: 200 KB of ">" or 500 KB of "*a " cost 15 to 18 s of CPU). The largest
// real document, infra-bootstrap's bootstrap-test-notes.md (260 KB), parses in about 0.1 s. Anything else is served
// as text, which is still readable.
func mdSafe(b []byte) bool {
	para := 0
	for _, l := range bytes.Split(b, []byte("\n")) {
		if len(bytes.TrimSpace(l)) == 0 {
			para = 0
			continue
		}
		if para += len(l); para >= 64<<10 {
			return false
		}
		depth := 0
		for _, c := range l {
			if c == '>' {
				depth++
			} else if c != ' ' && c != '\t' {
				break
			}
		}
		if depth > 32 {
			return false
		}
	}
	return true
}

// maxDepth bounds how deep the walker follows nested blocks and inlines (quotes in quotes, lists in lists, emphasis
// in emphasis): deeper content comes back flat, as its plain text. Unbounded, a file of 512 KB of ">" overflows the
// goroutine stack and kills atlas.
const maxDepth = 24

type mdWalker struct {
	src     []byte
	repo    string
	dir     string
	inAtlas func(string) bool
	exists  func(repo, p string) bool // nil: a ref is one segment
	n       *int
	depth   int
}

func (w *mdWalker) blocks(parent ast.Node) []Block {
	out := []Block{}
	if w.depth >= maxDepth {
		if t := strings.TrimSpace(string(parent.Text(w.src))); t != "" { //nolint:staticcheck // Node.Text: no drop-in non-deprecated text of a whole subtree; swapping it risks changing the depth-capped render (behaviour)
			out = append(out, Block{T: "p", In: []Inline{{T: "text", Text: w.redact(t)}}})
		}
		return out
	}
	w.depth++
	defer func() { w.depth-- }()
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
				for i := 0; row != x.FirstChild() && i+1 < len(cells); i++ { // | PASSWORD | value |: the value's cell, when it is one bare token (not in the head row)
					if keyCell.MatchString(plain(cells[i])) && bareValue(plain(cells[i+1])) {
						cells[i+1] = []Inline{{T: "text", Text: "[redacted]"}}
						*w.n++
					}
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

// bareValue is a table cell that looks like a value rather than a description: one token of 8 or more characters,
// a literal (above), not a reference ($VAR, {{ }}, <placeholder>) or a note (*(required)*).
func bareValue(s string) bool {
	s = strings.TrimSpace(s)
	return len(s) >= 8 && !strings.ContainsAny(s, " \t") && !strings.ContainsAny(s[:1], "${<(*[`") && literal(s)
}

func (w *mdWalker) inlines(parent ast.Node) []Inline {
	out := []Inline{}
	if w.depth >= maxDepth {
		if t := string(parent.Text(w.src)); t != "" { //nolint:staticcheck // as in blocks: Node.Text has no drop-in replacement (behaviour)
			out = append(out, Inline{T: "text", Text: w.redact(t)})
		}
		return out
	}
	w.depth++
	defer func() { w.depth-- }()
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
			v := x.Value(w.src)
			if !x.IsRaw() { // what a renderer would show: \* as *, &amp; as &
				v = util.UnescapePunctuations(util.ResolveNumericReferences(util.ResolveEntityNames(v)))
			}
			add(Inline{T: "text", Text: string(v)})
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
		// `password`: value or **token** = value: the key in one node, the value at the start of the next text run
		if i > 0 && out[i].T == "text" && keyBefore.MatchString(plain(out[i-1:i])) {
			if m := valueAfter.FindStringSubmatchIndex(out[i].Text); m != nil && literal(out[i].Text[m[4]:m[5]]) {
				out[i].Text = out[i].Text[:m[3]] + "[redacted]" + out[i].Text[m[5]:]
				*w.n++
			}
		}
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

var (
	ghBlob = regexp.MustCompile(`^(?i:github\.com)/(?i:MohammadBnei)/([A-Za-z0-9._-]+)/(?:blob|tree)/(.+)$`) // <ref>/<path>, the ref maybe with slashes
	// docPath is the character set the page's router takes in an L4 path (router.js PATH): a doc link outside it would
	// open the map instead of the file, so it stays text.
	docPath = regexp.MustCompile(`^[A-Za-z0-9._@+,=~ /-]+$`)
)

// link resolves a destination: external http(s) kept, in-atlas docs resolved, anything else reduced to its text. A
// URL carrying a secret (a webhook, a token in its query, a password in its userinfo) is not a link at all: its text
// is redacted with the rest, and an href is never redacted into something else that still points somewhere.
func (w *mdWalker) link(dest string, in []Inline) Inline {
	u, err := url.Parse(dest)
	switch {
	case err != nil:
	case u.Scheme == "http" || u.Scheme == "https":
		if hrefSecret(u) {
			*w.n++
			return Inline{T: "em", In: in}
		}
		if m := ghBlob.FindStringSubmatch(u.Host + u.Path); m != nil && w.inAtlas(m[1]) {
			if p := w.refPath(m[1], m[2]); p != "" && docPath.MatchString(p) {
				return Inline{T: "link", Doc: m[1] + "/" + p, In: in}
			}
		}
		return Inline{T: "link", Href: u.String(), In: in}
	case u.Scheme == "" && u.Host == "" && u.Path != "":
		target := u.Path
		if !strings.HasPrefix(target, "/") {
			target = path.Join(w.dir, target)
		}
		if p := cleanRel(target); p != "" && docPath.MatchString(p) {
			return Inline{T: "link", Doc: w.repo + "/" + p, In: in}
		}
	}
	return Inline{T: "em", In: in} // a link atlas will not follow keeps its words
}

// refPath splits <ref>/<path> at the first split whose path atlas holds (main/docs/a.md, feat/x/docs/a.md); with no
// such path, or nothing to check against, the ref is the first segment.
func (w *mdWalker) refPath(repo, rest string) string {
	segs := strings.Split(rest, "/")
	if w.exists != nil {
		for k := 1; k < len(segs) && k <= 4; k++ { // a branch name of at most four segments
			if p := cleanRel(strings.Join(segs[k:], "/")); p != "" && w.exists(repo, p) {
				return p
			}
		}
	}
	if len(segs) < 2 {
		return ""
	}
	return cleanRel(strings.Join(segs[1:], "/"))
}

// hrefSecret: the URL holds something redaction would hide (userinfo, a secret-named query key, a known token shape).
func hrefSecret(u *url.URL) bool {
	if _, set := u.User.Password(); set {
		return true
	}
	for k := range u.Query() {
		if queryKey.MatchString(k) {
			return true
		}
	}
	var n int
	redact(u.String(), &n)
	return n > 0
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
func (a *App) serveRead(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Cache-Control", "private, no-store")
	h.Set("Vary", "Cookie")
	h.Set("X-Content-Type-Options", "nosniff")
	q := r.URL.Query()
	repo, p := q.Get("repo"), cleanRel(q.Get("path"))
	if repo == "" || p == "" || p != q.Get("path") { // only a path already clean and relative
		errs.Write(w, errs.Invalid(keyReadPath, "repo and a clean repo-relative path are required"))
		return
	}
	rp, ok := a.Files.get(repo)
	if !ok {
		errs.Write(w, errs.Missing(keyReadRepo, "atlas does not hold that repo"))
		return
	}
	b, err := readFile(rp.Dir, p)
	switch {
	case errors.Is(err, errNotText):
		errs.Write(w, errs.Precondition(keyReadNotText, err.Error()))
		return
	case errors.Is(err, errTooBig):
		errs.Write(w, errs.Precondition(keyReadTooLarge, err.Error()))
		return
	case err != nil:
		errs.Write(w, errs.Missing(keyReadFile, "no such file at the sha atlas holds"))
		return
	}
	key := repo + "@" + rp.SHA + ":" + p
	out, ok := a.reads.get(key)
	if !ok {
		select { // at most two renders at once: a pathological file costs seconds of CPU, once per sha (then cached)
		case a.readSlots <- struct{}{}:
		case <-r.Context().Done():
			return
		}
		d := renderIn(repo, p, rp.SHA, b, func(name string) bool { _, ok := a.Files.get(name); return ok }, a.Files.exists)
		<-a.readSlots
		out = mustJSON(d)
		a.reads.put(key, out)
	}
	h.Set("Content-Type", "application/json")
	_, _ = w.Write(out)
}

// readCache holds rendered documents by repo, sha and path (a file at a sha never changes): the last 64, by insertion.
type readCache struct {
	mu    sync.Mutex
	m     map[string][]byte
	order []string
}

func (c *readCache) get(k string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, ok := c.m[k]
	return b, ok
}

func (c *readCache) put(k string, b []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.m[k]; ok {
		return
	}
	c.m[k] = b
	c.order = append(c.order, k)
	if len(c.order) > 64 {
		delete(c.m, c.order[0])
		c.order = c.order[1:]
	}
}
