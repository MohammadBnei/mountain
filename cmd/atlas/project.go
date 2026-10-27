package main

// The projector (plan/10 "Projection"): one goroutine. Each round it learns the heads of the repos it reads
// (GitHub, or local checkouts under --repos), runs the extractors over them, and stores the result when anything
// changed. infra-bootstrap is read first: its registry names the branch every other source is read at (phase 1 of
// the vector), then the others follow (phase 2).
//
// A round that cannot read a repo the last projection read is refused (errLostRepos), never stored as that repo's
// nodes all closing; the alarm is mountain_source_last_success{source="projection"} ageing into MountainSourceStale.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/MohammadBnei/mountain/internal/extract"
	"github.com/MohammadBnei/mountain/realms"
)

// heads is where a round's checkouts come from: GitHub tarballs (fetch.go) or local directories.
type heads interface {
	// checkout returns repo at the head of branch, on disk, or why it can't be read this round.
	checkout(ctx context.Context, repo, branch string) (extract.Repo, error)
}

// errUnchanged: every head is where the last stored round read it, so the extractors are not run again (a projection
// is a pure function of the extractor versions and the vector).
var errUnchanged = errors.New("unchanged")

// roundCache carries what the last round learned: the branches infra-bootstrap's registry names (at its sha) and the
// vector the last stored projection was made from. nil = no cache (atlas project, tests).
type roundCache struct {
	infra  string
	branch map[string]string
	vector string // set by the caller once a round is stored
}

func vectorKey(repos extract.Repos) string {
	var b strings.Builder
	for _, name := range slices.Sorted(maps.Keys(repos)) {
		fmt.Fprintf(&b, "%s=%s@%s;", name, repos[name].Branch, repos[name].SHA)
	}
	return b.String()
}

// round reads every source at its tracked branch and runs the extractors over them; with a cache, it returns
// errUnchanged (and the repos) when no head moved since the last stored round. unread is each source's last read
// failure, kept by the caller across rounds so a failure is logged once per change (nil: a map for this round only).
func round(ctx context.Context, src heads, rl *realms.Realms, c *roundCache, unread map[string]string) (extract.Result, extract.Repos, error) {
	if unread == nil {
		unread = map[string]string{}
	}
	repos := extract.Repos{}
	infra, err := src.checkout(ctx, rl.Sources[0], "main")
	if err != nil && infra.Dir == "" {
		return extract.Result{}, nil, fmt.Errorf("%s: %w", rl.Sources[0], err)
	}
	if err != nil {
		log.Printf("projector: %s: %v: reading the tree held at %s", rl.Sources[0], err, infra.SHA[:min(12, len(infra.SHA))])
	}
	repos[rl.Sources[0]] = infra
	var branch map[string]string
	if c != nil && c.infra == infra.SHA && c.infra != "" {
		branch = c.branch
	} else {
		// phase 1: the branches ArgoCD deploys from, read from infra-bootstrap alone
		first, err := extract.Run(repos, rl, extract.All)
		if err != nil {
			return extract.Result{}, repos, err // the reader still learns infra-bootstrap's new tree (its old one may go next round)
		}
		branch = map[string]string{}
		for _, t := range first.Tracked { // sorted: a repo deployed from two branches is read at the first (vos-monolith: dev)
			if branch[t.Repo] == "" {
				branch[t.Repo] = t.Branch
			}
		}
		if c != nil {
			c.infra, c.branch = infra.SHA, branch
		}
	}
	for _, name := range rl.Sources[1:] {
		b := branch[name]
		if b == "" {
			b = "main"
		}
		r, err := src.checkout(ctx, name, b)
		if err != nil {
			if msg := err.Error(); unread[name] != msg { // once per change, not every round
				log.Printf("projector: %s@%s: %v", name, b, err)
				unread[name] = msg
			}
			if r.Dir == "" {
				continue // left out: the store refuses the round if the last projection read it
			}
		} else {
			delete(unread, name)
		}
		repos[name] = r
	}
	if c != nil && c.vector == vectorKey(repos) {
		return extract.Result{}, repos, errUnchanged
	}
	res, err := extract.Run(repos, rl, extract.All)
	return res, repos, err
}

// project runs rounds until ctx ends.
func (a *App) project(ctx context.Context) {
	var src heads
	switch {
	case a.Opts.Repos != "":
		src = localHeads(a.Opts.Repos)
	case a.Opts.GHToken != "":
		dir := os.Getenv("ATLAS_CACHE")
		if dir == "" {
			dir = "/var/cache/atlas"
		}
		gh, err := newGitHub(a.Opts.GHURL, a.Opts.GHToken, dir)
		if err != nil {
			log.Printf("projector: %v: off", err)
			return
		}
		src = gh
		SourceLastSuccess.Set("github", float64(time.Now().Unix())) // seeded: a token that never works still ages into the alert
	default:
		log.Printf("projector: MOUNTAIN_GITHUB_TOKEN unset and no --repos: serving the stored projection only")
		return
	}
	SourceLastSuccess.Set("projection", float64(time.Now().Unix()))
	cache := &roundCache{}
	t := time.NewTicker(a.Opts.Every)
	defer t.Stop()
	lastErr := ""
	for {
		rc, cancel := context.WithTimeout(ctx, 10*time.Minute)
		err := a.projectOnce(rc, src, cache)
		cancel()
		switch { // logged once per change, not per round
		case err != nil && err.Error() != lastErr:
			log.Printf("projector: %v", err)
			lastErr = err.Error()
		case err == nil && lastErr != "":
			log.Printf("projector: recovered")
			lastErr = ""
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (a *App) projectOnce(ctx context.Context, src heads, c *roundCache) error {
	res, repos, err := round(ctx, src, a.Realms, c, a.unread)
	if repos != nil {
		a.Files.update(repos) // what the reader serves: the trees this round read, at their shas; a repo it did not read keeps the last
	}
	if errors.Is(err, errUnchanged) {
		SourceLastSuccess.Set("projection", float64(time.Now().Unix()))
		return nil
	}
	if err != nil {
		return err
	}
	counted := map[string]bool{} // only the sources realms.yaml still lists count as lost when unread
	for _, s := range a.Realms.Sources {
		counted[s] = true
	}
	seq, changed, err := a.Graph.put(ctx, res, repos, counted)
	if err != nil {
		return err
	}
	c.vector = vectorKey(repos)
	SourceLastSuccess.Set("projection", float64(time.Now().Unix()))
	if changed {
		log.Printf("projector: projection %d: %d repos, %d nodes, %d edges, %d drift", seq, len(repos), len(res.Graph.Nodes), len(res.Graph.Edges), len(res.Graph.Drift))
		a.Hub.Broadcast(mustJSON(map[string]any{"t": "graph", "seq": seq}))
		a.Live.event("graph", "", fmt.Sprintf("projection %d", seq))
	}
	return nil
}

// localHeads reads checkouts named by repo under a directory, at whatever they have checked out (branch drift
// says when that is not the tracked branch).
type localHeads string

func (d localHeads) checkout(_ context.Context, repo, _ string) (extract.Repo, error) {
	all, err := extract.Checkouts(string(d))
	if err != nil {
		return extract.Repo{}, err
	}
	r, ok := all[repo]
	if !ok {
		return extract.Repo{}, errors.New("not checked out")
	}
	return r, nil
}

// projectCmd is `atlas project`: one round over local checkouts, stored.
func projectCmd(args []string) error {
	f := flag.NewFlagSet("project", flag.ContinueOnError)
	dir := f.String("repos", "", "directory holding the repo checkouts (one per GitHub repo name)")
	dsn := f.String("dsn", os.Getenv("DATABASE_URL"), "Postgres URL (default: DATABASE_URL)")
	drop := f.Bool("drop-repos", false, "accept that repos the last projection read are gone (their nodes close)")
	if err := f.Parse(args); err != nil {
		return err
	}
	if *dir == "" || *dsn == "" {
		return errors.New("project: --repos and a DSN (--dsn or DATABASE_URL) are required")
	}
	rl, err := realms.Load()
	if err != nil {
		return err
	}
	ctx := context.Background()
	res, repos, err := round(ctx, localHeads(*dir), rl, nil, nil)
	if err != nil {
		return err
	}
	g := &Graphs{}
	if err := g.attach(ctx, *dsn); err != nil {
		return err
	}
	defer g.close()
	var counted map[string]bool // every repo the last projection read
	if *drop {
		counted = map[string]bool{}
	}
	seq, changed, err := g.put(ctx, res, repos, counted)
	if err != nil {
		return err
	}
	fmt.Printf("atlas: projection %d (changed: %v): %d repos, %d nodes, %d edges, %d drift, skipped %v\n",
		seq, changed, len(repos), len(res.Graph.Nodes), len(res.Graph.Edges), len(res.Graph.Drift), res.Skipped)
	return nil
}
