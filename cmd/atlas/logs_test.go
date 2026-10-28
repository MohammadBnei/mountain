package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/MohammadBnei/mountain/pkg/telemetry"
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

// TestServerLog: net/http's own error lines come through the default logger as ERROR, with their words.
func TestServerLog(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	t.Cleanup(func() { slog.SetDefault(old) })
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	serverLog().Printf("http: Accept error: accept tcp [::]:8080: accept4: too many open files; retrying in 1s")
	var m map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &m); err != nil || m["level"] != "ERROR" || m["msg"] != "http: Accept error: accept tcp [::]:8080: accept4: too many open files; retrying in 1s" {
		t.Errorf("%s: %v", buf.Bytes(), err)
	}
}

// TestServersLog: every http.Server atlas builds sends net/http's lines through serverLog.
func TestServersLog(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	servers := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			if sel, ok := lit.Type.(*ast.SelectorExpr); !ok || sel.Sel.Name != "Server" {
				return true
			}
			if id, ok := lit.Type.(*ast.SelectorExpr).X.(*ast.Ident); !ok || id.Name != "http" {
				return true
			}
			servers++
			for _, e := range lit.Elts {
				if kv, ok := e.(*ast.KeyValueExpr); ok && fmt.Sprint(kv.Key) == "ErrorLog" {
					if call, ok := kv.Value.(*ast.CallExpr); ok && fmt.Sprint(call.Fun) == "serverLog" {
						return true
					}
				}
			}
			t.Errorf("%s: an http.Server without ErrorLog: serverLog()", f)
			return true
		})
	}
	if servers < 2 {
		t.Errorf("%d http.Servers: the check no longer sees them", servers) // the positive control
	}
}

// TestLogWriter: the service logs on stdout; atlas project on stderr, its stdout being its result line.
func TestLogWriter(t *testing.T) {
	if logWriter([]string{"atlas", "project", "--repos", "x"}) != os.Stderr || logWriter([]string{"atlas"}) != os.Stdout ||
		logWriter([]string{"atlas", "--listen", ":8080"}) != os.Stdout {
		t.Error("logWriter")
	}
}

// TestFatal: fatal writes log.Fatal's words as one ERROR line through the default logger, and exits 1. It runs in a
// child process (the test binary again), since it exits.
func TestFatal(t *testing.T) {
	if os.Getenv("ATLAS_TEST_FATAL") == "1" {
		slog.SetDefault(telemetry.NewLogger(os.Stdout))
		fatal(errors.New("listen tcp :8080: bind: address already in use"))
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestFatal$")
	cmd.Env = append(os.Environ(), "ATLAS_TEST_FATAL=1")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 1 {
		t.Fatalf("exit: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &m); err != nil || m["level"] != "ERROR" || m["msg"] != "listen tcp :8080: bind: address already in use" {
		t.Errorf("%s: %v", out.Bytes(), err)
	}
}
