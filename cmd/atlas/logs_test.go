package main

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// logUses lists src's uses of the log package other than its Logger type: a line written through it is INFO under
// the JSON logger, whatever it says (plan/15 §7: every site has its level). It also counts the slog calls.
func logUses(name string, src []byte) (uses []string, slogCalls int, err error) {
	f, err := parser.ParseFile(token.NewFileSet(), name, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, 0, err
	}
	logName, slogName := "", ""
	for _, im := range f.Imports {
		p, _ := strconv.Unquote(im.Path.Value)
		n := filepath.Base(p)
		if im.Name != nil {
			n = im.Name.Name
		}
		switch p {
		case "log":
			if n == "." {
				uses = append(uses, "import . log")
			}
			logName = n
		case "log/slog":
			slogName = n
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		s, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := s.X.(*ast.Ident)
		switch {
		case !ok:
		case logName != "" && id.Name == logName && s.Sel.Name != "Logger":
			uses = append(uses, "log."+s.Sel.Name)
		case slogName != "" && id.Name == slogName && strings.ContainsAny(s.Sel.Name[:1], "DIWE") && !strings.HasPrefix(s.Sel.Name, "New"):
			slogCalls++
		}
		return true
	})
	return uses, slogCalls, nil
}

// TestLogLevels: atlas writes nothing through the log package (only its Logger type, for net/http's ErrorLog).
func TestLogLevels(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	calls := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		uses, n, err := logUses(f, src)
		if err != nil {
			t.Fatal(err)
		}
		calls += n
		for _, u := range uses {
			t.Errorf("%s: %s: the log package writes INFO, give the line its level with slog", f, u)
		}
	}
	if calls < 25 {
		t.Errorf("%d slog calls: the check no longer sees them", calls) // the positive control
	}
}

// TestLogUses: each form the check refuses, and what it lets through.
func TestLogUses(t *testing.T) {
	for src, want := range map[string]int{
		`import "log"; var _ = log.Printf`:                              1,
		`import l "log"; func f() { l.Fatal(nil) }`:                     1,
		`import . "log"; var _ = Printf`:                                1,
		`import "log"; var _ *log.Logger`:                               0,
		`import "log/slog"; func f() { slog.Warn("x") }`:                0,
		`import "log"; func f() { log.SetOutput(nil) }`:                 1,
		`type t struct{ Printf func() }; var log t; var _ = log.Printf`: 0,
	} {
		u, _, err := logUses("x.go", []byte("package x\n"+src+"\n"))
		if err != nil {
			t.Fatal(err)
		}
		if len(u) != want {
			t.Errorf("%s: %v, want %d", src, u, want)
		}
	}
}

// TestServerLog: net/http's own error lines come through the default logger as WARN, with their words.
func TestServerLog(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	t.Cleanup(func() { slog.SetDefault(old) })
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	serverLog().Printf("http: TLS handshake error from 1.2.3.4: EOF")
	var m map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &m); err != nil || m["level"] != "WARN" || m["msg"] != "http: TLS handshake error from 1.2.3.4: EOF" {
		t.Errorf("%s: %v", buf.Bytes(), err)
	}
}
