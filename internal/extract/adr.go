package extract

import (
	"bufio"
	"bytes"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/MohammadBnei/mountain/internal/graph"
)

// adrExtractor reads every checked-out repo's docs/adr: one node per ADR (title, status), the links between ADRs
// their headers state, and "governs" edges from the components whose files cite them (plan/10 "Nodes and edges").
var adrExtractor = Extractor{Name: "adr", Version: 1, Run: adrs}

const adrDir = "docs/adr"

var (
	adrFileRe  = regexp.MustCompile(`^docs/adr/(\d{4})-[A-Za-z0-9._-]+\.md$`)
	titleRe    = regexp.MustCompile(`^#\s*(?:ADR[- ]?\d{4}\s*[:—–-]+\s*)?(.+?)\s*$`)
	statusRe   = regexp.MustCompile(`^\s*(?:-\s*)?(?:\*\*Status:?\*\*:?|Status:)\s*(.+)$`)
	relationRe = regexp.MustCompile(`^\s*(?:-\s*)?\*\*(Related|Amends|Supersedes|Superseded by):?\*\*:?\s*(.+)$`)
	linkTextRe = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	indexRowRe = regexp.MustCompile(`^\|\s*\[(?:ADR-)?(\d{4})\]\(`)
)

// textExt are the files a citation is looked for in; anything else (images, lockfiles' cousins) is skipped.
var textExt = map[string]bool{".md": true, ".go": true, ".yaml": true, ".yml": true, ".ts": true, ".tsx": true,
	".js": true, ".mjs": true, ".sql": true, ".tf": true, ".sh": true, ".proto": true, ".toml": true, ".html": true}

// skipDirs are vendored or generated trees: their citations are not the repo's.
var skipDirs = map[string]bool{"node_modules": true, "vendor": true, "dist": true, "build": true, "testdata": true,
	"kubespray": true, "pigsty": true, "gen": true}

const maxCiteFile = 1 << 20

type adrKey struct {
	repo string
	num  string
}

func (k adrKey) id() string { return graph.ID(graph.KindADR, k.repo+"/"+k.num) }

func adrs(x *Ctx) error {
	known := map[adrKey]bool{}
	hasADRs := map[string]bool{}
	type header struct {
		key  adrKey
		src  graph.Source
		rels [][2]string // (relation, text)
	}
	var headers []header
	for _, repo := range sortedKeys(x.Repos) {
		files := x.Glob(repo, adrDir+"/[0-9][0-9][0-9][0-9]-*.md")
		indexed := index(x, repo)
		for _, p := range files {
			m := adrFileRe.FindStringSubmatch(p)
			if m == nil {
				continue
			}
			b, src, err := x.Read(repo, p)
			if err != nil {
				return err
			}
			k := adrKey{repo, m[1]}
			if known[k] {
				x.Drift(graph.Drift{Kind: graph.DriftAmbiguous, Subject: k.id(), Detail: "two files carry this number", Source: src})
				continue
			}
			known[k], hasADRs[repo] = true, true
			title, status, rels := parseHeader(b)
			x.Node(graph.Node{ID: k.id(), Kind: graph.KindADR, Source: src,
				Attrs: map[string]string{"repo": repo, "number": k.num, "title": title, "status": status}})
			headers = append(headers, header{k, src, rels})
			if indexed != nil && !indexed[k.num] {
				x.Drift(graph.Drift{Kind: graph.DriftMirror, Subject: k.id(), Detail: "not in docs/adr/README.md", Source: src})
			}
		}
		for num := range indexed {
			if !known[adrKey{repo, num}] {
				x.Drift(graph.Drift{Kind: graph.DriftUnresolved, Subject: adrKey{repo, num}.id(), Detail: "in docs/adr/README.md, no file", Source: graph.Source{Repo: repo, Path: adrDir + "/README.md", SHA: x.Repos[repo].SHA}})
			}
		}
	}
	cite := citer{x: x, known: known, hasADRs: hasADRs, counts: map[string]map[adrKey]int{}, srcOf: map[string]graph.Source{}, missing: map[string]map[adrKey]int{}, amb: map[string]int{}}
	for _, h := range headers {
		for _, rel := range h.rels {
			kind := map[string]string{"Related": graph.EdgeRelated, "Amends": graph.EdgeAmends, "Supersedes": graph.EdgeSupersede, "Superseded by": graph.EdgeSupersede}[rel[0]]
			for _, t := range cite.line(h.key.repo, h.src.Path, rel[1]) {
				if !known[t] || t == h.key {
					continue
				}
				from, to := h.key.id(), t.id()
				if rel[0] == "Superseded by" {
					from, to = to, from
				}
				x.Edge(graph.Edge{From: from, To: to, Kind: kind, Origin: graph.Declared, Source: h.src})
			}
		}
	}
	return cite.governs()
}

// index is the set of numbers docs/adr/README.md lists, or nil when the repo has no index.
func index(x *Ctx, repo string) map[string]bool {
	b, _, err := x.Read(repo, adrDir+"/README.md")
	if err != nil {
		return nil
	}
	out := map[string]bool{}
	for _, l := range strings.Split(string(b), "\n") {
		if m := indexRowRe.FindStringSubmatch(l); m != nil {
			out[m[1]] = true
		}
	}
	return out
}

// parseHeader reads an ADR's title, its status (the first words of its status line) and its relation lines.
func parseHeader(b []byte) (title, status string, rels [][2]string) {
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for n := 0; sc.Scan() && n < 40; n++ {
		l := sc.Text()
		switch {
		case title == "" && strings.HasPrefix(l, "# "):
			title = titleRe.FindStringSubmatch(l)[1]
		case strings.HasPrefix(l, "## "):
			return
		case status == "" && statusRe.MatchString(l):
			status = shortStatus(statusRe.FindStringSubmatch(l)[1])
		case relationRe.MatchString(l):
			m := relationRe.FindStringSubmatch(l)
			rels = append(rels, [2]string{m[1], m[2]})
		}
	}
	return
}

// shortStatus keeps a status line's first clause: "Proposed. Builds on …" -> "Proposed".
func shortStatus(s string) string {
	s = linkTextRe.ReplaceAllString(s, "$1")
	for _, sep := range []string{". ", ", ", " (", " — ", "; "} {
		if i := strings.Index(s, sep); i > 0 {
			s = s[:i]
		}
	}
	return strings.TrimRight(strings.TrimSpace(s), ".")
}

// citer resolves ADR citations to (repo, number) and counts them per citing component.
type citer struct {
	x       *Ctx
	known   map[adrKey]bool
	hasADRs map[string]bool
	counts  map[string]map[adrKey]int // owner id -> cited ADR -> count
	srcOf   map[string]graph.Source   // owner id -> its repo
	missing map[string]map[adrKey]int // citing repo -> cited ADR that doesn't exist -> count
	amb     map[string]int            // citing repo -> bare citations with no ADR home
}

var (
	laneRe     = regexp.MustCompile(`\blane [A-Z] ADR-\d{4}\b`)
	mdLinkRe   = regexp.MustCompile(`\]\(([^)\s]*?)(\d{4})-[A-Za-z0-9._-]+\.md(?:#[^)\s]*)?\)`)
	prefixedRe = regexp.MustCompile("\\b(infra-bootstrap|agent-fleet|mountain|ukubi-bridge|pi-lab)(?:'s)?[\\s:]+`?(?:ADR[- ]|(?:docs/)?adr/)(\\d{4})\\b")
	repoPathRe = regexp.MustCompile(`\b(infra-bootstrap|agent-fleet|mountain|ukubi-bridge|pi-lab)/docs/adr/(\d{4})\b`)
	docsADRRe  = regexp.MustCompile(`\bdocs/adr/(\d{4})\b`)
	shortRe    = regexp.MustCompile(`\badr/(\d{4})\b`) // agent-fleet's own shorthand (its CLAUDE.md), used across repos
	bareRe     = regexp.MustCompile(`\bADR[- ](\d{4})\b`)
)

// home is the repo a bare citation in repo means: its own ADRs, else realms.yaml's adrHome, else "" (ambiguous).
func (c *citer) home(repo string) string {
	if c.hasADRs[repo] {
		return repo
	}
	return c.x.Realms.ADRHome[repo]
}

// line resolves every citation in text (a line, or a whole file of repo). Each form is matched once: a match is blanked before
// the next, looser form runs.
func (c *citer) line(repo, file, l string) []adrKey {
	var out []adrKey
	blank := func(re *regexp.Regexp, f func(m []string) (adrKey, bool)) {
		l = re.ReplaceAllStringFunc(l, func(s string) string {
			if k, ok := f(re.FindStringSubmatch(s)); ok {
				out = append(out, k)
			}
			return strings.Repeat(" ", len(s))
		})
	}
	blank(laneRe, func([]string) (adrKey, bool) { return adrKey{}, false }) // pi-lab's pre-renumbering ids
	blank(mdLinkRe, func(m []string) (adrKey, bool) {
		target := path.Clean(path.Join(path.Dir(file), m[1]+m[2]+"-x.md"))
		if strings.HasPrefix(target, adrDir+"/") {
			return adrKey{repo, m[2]}, true
		}
		for _, r := range []string{"infra-bootstrap", "agent-fleet", "mountain", "ukubi-bridge", "pi-lab"} {
			if strings.Contains(m[1], r+"/") && strings.Contains(m[1], adrDir+"/") {
				return adrKey{r, m[2]}, true
			}
		}
		return adrKey{}, false
	})
	blank(prefixedRe, func(m []string) (adrKey, bool) { return adrKey{m[1], m[2]}, true })
	blank(repoPathRe, func(m []string) (adrKey, bool) { return adrKey{m[1], m[2]}, true })
	bare := func(m []string) (adrKey, bool) {
		h := c.home(repo)
		if h == "" {
			c.amb[repo]++
			return adrKey{}, false
		}
		return adrKey{h, m[1]}, true
	}
	blank(docsADRRe, bare)
	blank(shortRe, func(m []string) (adrKey, bool) { return adrKey{"agent-fleet", m[1]}, true })
	blank(bareRe, bare)
	return out
}

// governs scans every text file of every repo and emits component -> ADR edges with their citation counts.
func (c *citer) governs() error {
	for _, repo := range sortedKeys(c.x.Repos) {
		subs := submodules(c.x, repo)
		skip := func(dir string) bool { return skipDirs[path.Base(dir)] || subs[dir] }
		err := c.x.Walk(repo, skip, func(p string, size int64) error {
			if !textExt[path.Ext(p)] || size > maxCiteFile || strings.HasPrefix(p, adrDir+"/") {
				return nil // ADR files link through their headers; vendored and binary files are not the repo's words
			}
			b, _, err := c.x.Read(repo, p)
			if err != nil {
				return err
			}
			owner := c.x.Realms.Owner(repo, p)
			if owner == "" {
				owner = graph.ID(graph.KindRepo, repo)
			}
			{ // the whole file at once: a repo prefix often ends the line before the "ADR-NNNN" it names
				for _, k := range c.line(repo, p, string(b)) {
					if !c.known[k] {
						if c.missing[repo] == nil {
							c.missing[repo] = map[adrKey]int{}
						}
						c.missing[repo][k]++
						continue
					}
					if c.counts[owner] == nil {
						c.counts[owner] = map[adrKey]int{}
					}
					c.counts[owner][k]++
				}
			}
			c.srcOf[owner] = graph.Source{Repo: repo, SHA: c.x.Repos[repo].SHA}
			return nil
		})
		if err != nil {
			return err
		}
	}
	for _, owner := range sortedKeys(c.counts) {
		for k, n := range c.counts[owner] {
			c.x.Edge(graph.Edge{From: owner, To: k.id(), Kind: graph.EdgeGoverns, Origin: graph.Declared,
				Attrs: map[string]string{"count": strconv.Itoa(n)}, Source: c.srcOf[owner]})
		}
	}
	for _, repo := range sortedKeys(c.missing) {
		for k, n := range c.missing[repo] {
			if _, checked := c.x.Repos[k.repo]; !checked {
				continue // the cited repo isn't checked out: nothing to say about it
			}
			detail := fmt.Sprintf("cited %d times from %s, no such ADR", n, repo)
			var elsewhere []string
			for _, r := range sortedKeys(c.x.Repos) {
				if c.known[adrKey{r, k.num}] {
					elsewhere = append(elsewhere, r)
				}
			}
			if len(elsewhere) > 0 { // a hint for the person fixing the citation, never a guess atlas acts on
				detail += "; " + k.num + " exists in " + strings.Join(elsewhere, ", ")
			}
			c.x.Drift(graph.Drift{Kind: graph.DriftUnresolved, Subject: k.id(), Detail: detail, Source: graph.Source{Repo: repo, SHA: c.x.Repos[repo].SHA}})
		}
	}
	for _, repo := range sortedKeys(c.amb) {
		c.x.Drift(graph.Drift{Kind: graph.DriftAmbiguous, Subject: graph.ID(graph.KindRepo, repo), Detail: fmt.Sprintf("%d bare ADR citations, and the repo has no ADRs or adrHome", c.amb[repo]), Source: graph.Source{Repo: repo, SHA: c.x.Repos[repo].SHA}})
	}
	return nil
}

var submoduleRe = regexp.MustCompile(`(?m)^\s*path\s*=\s*(\S+)\s*$`)

// submodules are a repo's .gitmodules paths: another repo's files, read (if at all) as that repo.
func submodules(x *Ctx, repo string) map[string]bool {
	out := map[string]bool{}
	b, _, err := x.Read(repo, ".gitmodules")
	if err != nil {
		return out
	}
	for _, m := range submoduleRe.FindAllStringSubmatch(string(b), -1) {
		out[strings.TrimSuffix(m[1], "/")] = true
	}
	return out
}
