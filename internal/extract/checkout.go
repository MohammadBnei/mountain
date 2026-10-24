package extract

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var shaRe = regexp.MustCompile(`^[0-9a-f]{40}$`)

// Checkouts is every git checkout directly under dir, by directory name, with the sha its HEAD is at. It reads .git
// itself (no git binary: the relay's image has none).
func Checkouts(dir string) (Repos, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := Repos{}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		root := filepath.Join(dir, e.Name())
		sha, err := Head(root)
		if err != nil {
			continue // not a checkout
		}
		out[e.Name()] = Repo{Dir: root, SHA: sha}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("extract: no git checkouts under %s", dir)
	}
	return out, nil
}

// Head is the sha root's HEAD points at: detached, a loose ref, or a packed one.
func Head(root string) (string, error) {
	gitDir := filepath.Join(root, ".git")
	if fi, err := os.Stat(gitDir); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("extract: %s is not a checkout", root)
	}
	b, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return "", err
	}
	head := strings.TrimSpace(string(b))
	if shaRe.MatchString(head) {
		return head, nil
	}
	ref, ok := strings.CutPrefix(head, "ref: ")
	if !ok || strings.Contains(ref, "..") {
		return "", fmt.Errorf("extract: %s: odd HEAD", root)
	}
	if b, err := os.ReadFile(filepath.Join(gitDir, filepath.FromSlash(ref))); err == nil {
		if s := strings.TrimSpace(string(b)); shaRe.MatchString(s) {
			return s, nil
		}
	}
	packed, err := os.ReadFile(filepath.Join(gitDir, "packed-refs"))
	if err != nil {
		return "", fmt.Errorf("extract: %s: %s not found", root, ref)
	}
	sc := bufio.NewScanner(bytes.NewReader(packed))
	for sc.Scan() {
		if sha, name, ok := strings.Cut(sc.Text(), " "); ok && name == ref && shaRe.MatchString(sha) {
			return sha, nil
		}
	}
	return "", fmt.Errorf("extract: %s: %s not found", root, ref)
}
