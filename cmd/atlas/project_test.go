package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestProjectFlags: --dsn defaults to DATABASE_URL, read only when --dsn is not given, and -h never prints its value
// (the URL holds the database password); an explicit empty --dsn is still refused.
func TestProjectFlags(t *testing.T) {
	const secret = "postgres://u:secretpw@h/db"
	env := func(k string) string {
		if k == "DATABASE_URL" {
			return secret
		}
		return ""
	}
	var out bytes.Buffer
	if _, _, _, err := parseProject([]string{"-h"}, env, &out); err == nil || strings.Contains(out.String(), "secretpw") || !strings.Contains(out.String(), "-dsn") {
		t.Errorf("-h: %v\n%s", err, out.String())
	}
	if _, dsn, _, err := parseProject([]string{"--repos", "r"}, env, &out); err != nil || dsn != secret {
		t.Errorf("the env default: %q %v", dsn, err)
	}
	if _, dsn, _, err := parseProject([]string{"--repos", "r", "--dsn", "postgres://other"}, env, &out); err != nil || dsn != "postgres://other" {
		t.Errorf("--dsn wins: %q %v", dsn, err)
	}
	if _, _, _, err := parseProject([]string{"--repos", "r", "--dsn="}, env, &out); err == nil || !strings.Contains(err.Error(), "required") {
		t.Errorf("an explicit empty --dsn: %v", err)
	}
	if _, _, _, err := parseProject([]string{"--repos", "r"}, func(string) string { return "" }, &out); err == nil {
		t.Error("no DSN at all is accepted")
	}
}
