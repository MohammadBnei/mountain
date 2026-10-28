package main

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MohammadBnei/mountain/internal/extract"
	"github.com/MohammadBnei/mountain/internal/graph"
	"github.com/MohammadBnei/mountain/pkg/errs"
)

// keyUses checks src (a file of this package) against the registry, syntactically (each form below; a type-checked
// guard like the game's TestKeys comes with its codecheck's extraction to mountain/pkg):
//   - every key a constructor of mountain/pkg/errs is called with is one of keys, named directly;
//   - a constructor is only ever called, never taken as a value;
//   - outside errkeys.go, errs.Key and errs.Error are never named (no conversion, literal, new, alias or field type),
//     no .Key field is written, and no name of the registry is declared again (a shadowing const or variable);
//   - atlas has no sources, so From and Down are not used.
//
// It returns the problems and how many constructor calls it checked.
func keyUses(name string, src []byte, keys map[string]bool) (problems []string, checked int) {
	f, err := parser.ParseFile(token.NewFileSet(), name, src, parser.SkipObjectResolution)
	if err != nil {
		return []string{err.Error()}, 0
	}
	pkg := ""
	for _, im := range f.Imports {
		if im.Path.Value == `"github.com/MohammadBnei/mountain/pkg/errs"` {
			pkg = "errs"
			if im.Name != nil {
				pkg = im.Name.Name
			}
		}
	}
	if pkg == "." {
		return []string{"a dot import of pkg/errs"}, 0
	}
	registry := filepath.Base(name) == "errkeys.go"
	sel := func(e ast.Expr) string {
		if s, ok := e.(*ast.SelectorExpr); ok {
			if id, ok := s.X.(*ast.Ident); ok && pkg != "" && id.Name == pkg {
				return s.Sel.Name
			}
		}
		return ""
	}
	ctor := map[string]bool{"Invalid": true, "Precondition": true, "Missing": true, "NotHere": true, "Conflict": true}
	called := map[ast.Expr]bool{}
	declared := func(id *ast.Ident) {
		if !registry && keys[id.Name] {
			problems = append(problems, id.Name+": a registry name declared again")
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			if fn := sel(x.Fun); ctor[fn] {
				called[x.Fun] = true
				checked++
				if id, ok := x.Args[0].(*ast.Ident); !ok || !keys[id.Name] {
					problems = append(problems, fn+": a key that is not one of errkeys.go's constants")
				}
			}
		case *ast.ValueSpec:
			for _, id := range x.Names {
				declared(id)
			}
		case *ast.AssignStmt:
			for _, l := range x.Lhs {
				if id, ok := l.(*ast.Ident); ok && x.Tok == token.DEFINE {
					declared(id)
				}
				if s, ok := l.(*ast.SelectorExpr); ok && s.Sel.Name == "Key" && !registry {
					problems = append(problems, "a write to .Key")
				}
			}
		case *ast.Field:
			for _, id := range x.Names {
				declared(id)
			}
		case *ast.TypeSpec:
			declared(x.Name)
		case *ast.FuncDecl:
			declared(x.Name)
		}
		return true
	})
	ast.Inspect(f, func(n ast.Node) bool {
		if s, ok := n.(*ast.SelectorExpr); ok {
			switch fn := sel(s); {
			case ctor[fn] && !called[s]:
				problems = append(problems, fn+": a constructor taken as a value")
			case fn == "From" || fn == "Down":
				problems = append(problems, fn+": atlas has no sources")
			case (fn == "Key" || fn == "Error") && !registry:
				problems = append(problems, "errs."+fn+" named outside errkeys.go")
			}
		}
		return true
	})
	return problems, checked
}

// registryKeys is errkeys.go's constants.
func registryKeys(t *testing.T) map[string]bool {
	f, err := parser.ParseFile(token.NewFileSet(), "errkeys.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]bool{}
	for _, d := range f.Decls {
		if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.CONST {
			for _, s := range g.Specs {
				for _, n := range s.(*ast.ValueSpec).Names {
					keys[n.Name] = true
				}
			}
		}
	}
	return keys
}

// TestErrKeys: every key atlas answers with is one of errkeys.go's constants.
func TestErrKeys(t *testing.T) {
	keys := registryKeys(t)
	files, _ := filepath.Glob("*.go")
	checked := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		p, n := keyUses(f, src, keys)
		checked += n
		for _, e := range p {
			t.Errorf("%s: %s", f, e)
		}
	}
	if len(keys) < 9 || checked < 9 {
		t.Errorf("%d keys, %d calls checked: the check no longer sees them", len(keys), checked) // the positive control
	}
}

// TestErrKeysForms: each form the check refuses, and the ones it lets through.
func TestErrKeysForms(t *testing.T) {
	keys := map[string]bool{"keyA": true}
	for src, want := range map[string]int{
		`func() { f := errs.Invalid; f(keyA, "x") }`:                    1,
		`[]*errs.Error{{Key: "lit"}}`:                                   1,
		`func() { e := errs.Invalid(keyA, "x"); e.Key = "lit"; _ = e }`: 1,
		`new(errs.Error)`: 1,
		`func() { const keyA errs.Key = "lit"; _ = errs.Missing(keyA, "x") }`: 2,
		`errs.Invalid(keyA, "x")`:                     0,
		`errs.Invalid(keyB, "x")`:                     1,
		`errs.Missing("lit", "x")`:                    1,
		`errs.Precondition(errs.Key("k"), "x")`:       2,
		`errs.Down("sim", nil)`:                       1,
		`errs.From("sim", errs.Unknown, "", "", nil)`: 1,
		`&errs.Error{Code: errs.Internal}`:            1,
	} {
		p, _ := keyUses("x.go", []byte("package main\nimport \"github.com/MohammadBnei/mountain/pkg/errs\"\nvar _ = "+src+"\n"), keys)
		if len(p) != want {
			t.Errorf("%s: %v, want %d problems", src, p, want)
		}
	}
	if p, _ := keyUses("x.go", []byte("package main\nimport e \"github.com/MohammadBnei/mountain/pkg/errs\"\nvar _ = e.Missing(\"lit\", \"x\")\n"), keys); len(p) != 1 {
		t.Errorf("an aliased import: %v", p)
	}
	if p, _ := keyUses("x.go", []byte("package main\nimport . \"github.com/MohammadBnei/mountain/pkg/errs\"\nvar _ = Missing(\"lit\", \"x\")\n"), keys); len(p) != 1 {
		t.Errorf("a dot import: %v", p)
	}
}

// TestErrBodies: each route error answers the model's body, with its key and code, and today's words.
func TestErrBodies(t *testing.T) {
	ctx := context.Background()
	a := testApp(t)
	a.gate.devOpen = true
	base := t.TempDir()
	dir := filepath.Join(base, "infra-bootstrap")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "bin.dat"), []byte("x\x00y"), 0o644)
	os.WriteFile(filepath.Join(dir, "big.md"), []byte(strings.Repeat("a", maxReadBytes+1)), 0o644)
	os.WriteFile(filepath.Join(dir, "ok.md"), []byte(strings.Repeat("a", maxReadBytes)), 0o644)
	a.Files.update(extract.Repos{"infra-bootstrap": {Dir: dir, SHA: "s1"}})
	check := func(target string, status int, code errs.Code, key errs.Key, msg string) {
		t.Helper()
		r := a.serveReq("GET", target, nil)
		var b errs.Body
		h := r.Header
		if h.Get("Cache-Control") != "private, no-store" || h.Get("Vary") != "Cookie" || h.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: headers %v", target, h)
		}
		if r.StatusCode != status || h.Get("Content-Type") != "application/json" || json.NewDecoder(r.Body).Decode(&b) != nil ||
			b != (errs.Body{Code: code, Key: string(key), Message: msg}) {
			t.Errorf("%s: %d %q %+v", target, r.StatusCode, r.Header.Get("Content-Type"), b)
		}
	}
	check("/api/graph", 404, errs.NotFound, keyGraphNone, "atlas: no projection yet")
	check("/api/graph?seq=x", 400, errs.InvalidArgument, keyGraphSeq, "seq is a positive integer")
	check("/api/read?repo=&path=x", 400, errs.InvalidArgument, keyReadPath, "repo and a clean repo-relative path are required")
	check("/api/read?repo=agent-fleet&path=a", 404, errs.NotFound, keyReadRepo, "atlas does not hold that repo")
	check("/api/read?repo=infra-bootstrap&path=bin.dat", 400, errs.FailedPrecondition, keyReadNotText, "not a text file")
	check("/api/read?repo=infra-bootstrap&path=big.md", 400, errs.FailedPrecondition, keyReadTooLarge, "larger than the reader reads")
	check("/api/read?repo=infra-bootstrap&path=nope.md", 404, errs.NotFound, keyReadFile, "no such file at the sha atlas holds")
	if r := a.serveReq("GET", "/api/read?repo=infra-bootstrap&path=ok.md", nil); r.StatusCode != http.StatusOK {
		t.Errorf("a file of exactly maxReadBytes: %d", r.StatusCode) // the positive control for too-large
	}
	if _, _, err := a.Graph.put(ctx, atlasResult([]graph.Node{n("app:a", "1", "s1")}, nil), extract.Repos{"infra-bootstrap": {SHA: "s1"}}, nil); err != nil {
		t.Fatal(err)
	}
	check("/api/graph?seq=99", 404, errs.NotFound, keyGraphNoHistory, "atlas: no history without a database")
}

// TestDBErrBodies: Postgres's answers for an older projection: absent, and the database failing (internal, today's
// words, the cause not sent: pgx's text stays in atlas's own log line).
func TestDBErrBodies(t *testing.T) {
	ctx := context.Background()
	a := testApp(t)
	a.gate.devOpen = true
	a.Graph = openGraphs(ctx, testDSN(t), nil)
	if _, _, err := a.Graph.put(ctx, atlasResult([]graph.Node{n("app:a", "1", "s1")}, nil), extract.Repos{"infra-bootstrap": {SHA: "s1"}}, nil); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		code errs.Code
		key  errs.Key
		msg  string
	}{{errs.NotFound, keyGraphSeqMissing, "atlas: no projection yet"}, {errs.Internal, errs.KeyInternal, "atlas: read failed"}} {
		if c.code == errs.Internal {
			a.Graph.db().Close()
		}
		r := a.serveReq("GET", "/api/graph?seq=99999", nil)
		var b errs.Body
		if r.StatusCode != c.code.Status() || json.NewDecoder(r.Body).Decode(&b) != nil || b != (errs.Body{Code: c.code, Key: string(c.key), Message: c.msg}) {
			t.Errorf("%s: %d %+v", c.key, r.StatusCode, b)
		}
	}
}
