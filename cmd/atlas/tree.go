package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/MohammadBnei/mountain/contract"
	"github.com/MohammadBnei/mountain/internal/extract"
	"github.com/MohammadBnei/ukubi-go/errs"
)

const maxTreeFiles = 20_000

var errTreeFull = errors.New("tree full")

// serveTree is GET /api/tree?repo= (admin-only by the route table): every regular file of the tree atlas holds for
// repo, with its size, minus the directories the extractors skip (extract.SkipDir), at most maxTreeFiles (then
// truncated). The answer is cached by repo@sha in its own small cache (a tree at a sha never changes, and one answer
// can be ~1.5 MB, so it would push rendered documents out of theirs).
func (a *App) serveTree(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Cache-Control", "private, no-store")
	h.Set("Vary", "Cookie")
	h.Set("X-Content-Type-Options", "nosniff")
	repo := r.URL.Query().Get("repo")
	if repo == "" {
		errs.Write(w, errs.Invalid(keyReadPath, "repo is required"))
		return
	}
	rp, ok := a.Files.get(repo)
	if !ok {
		errs.Write(w, errs.Missing(keyReadRepo, "atlas does not hold that repo"))
		return
	}
	key := repo + "@" + rp.SHA
	out, ok := a.trees.get(key)
	if !ok {
		select { // the walk shares the renders' two slots: up to maxTreeFiles stats, once per sha (then cached)
		case a.readSlots <- struct{}{}:
		case <-r.Context().Done():
			return
		}
		t, err := treeOf(a, repo, rp)
		<-a.readSlots
		if err != nil {
			slog.ErrorContext(r.Context(), fmt.Sprintf("atlas: tree %s@%s: %v", repo, rp.SHA, err))
			errs.Write(w, internalError("atlas: the tree could not be listed"))
			return
		}
		out = mustJSON(t)
		a.trees.put(key, out)
	}
	h.Set("Content-Type", "application/json")
	_, _ = w.Write(out)
}

func treeOf(a *App, repo string, rp extract.Repo) (contract.Tree, error) {
	t := contract.Tree{Repo: repo, SHA: rp.SHA, Files: []contract.TreeFile{}}
	x := &extract.Ctx{Repos: extract.Repos{repo: rp}}
	err := x.Walk(repo, func(dir string) bool { return extract.SkipDir(a.Realms, repo, dir) }, func(p string, size int64) error {
		if len(t.Files) == maxTreeFiles {
			t.Truncated = true
			return errTreeFull
		}
		t.Files = append(t.Files, contract.TreeFile{Path: p, Size: size})
		return nil
	})
	if errors.Is(err, errTreeFull) {
		err = nil
	}
	return t, err
}
