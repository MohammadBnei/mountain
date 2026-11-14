package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/MohammadBnei/mountain/realms"
)

// The door's first slice (design-2026-11/door-hybrid-F.md §6): `atlas resolve` and `atlas state`, CLI only. Its
// recipes live here in cmd/atlas, never in contract (the game's sim mirrors contract.Queries).

// door is what both commands read: the alias table over realms.yaml and the source registry.
type door struct {
	al  *realms.Aliases
	reg *realms.Registry
}

func loadDoor() (*door, error) {
	rl, err := realms.Load()
	if err != nil {
		return nil, err
	}
	reg, err := realms.LoadSources()
	if err != nil {
		return nil, err
	}
	al, err := realms.LoadAliases(rl, reg)
	if err != nil {
		return nil, err
	}
	return &door{al: al, reg: reg}, nil
}

// resolveMatch is one row of a resolve answer: the alias that hit, and where the thing runs if known.
type resolveMatch struct {
	realms.Match
	RunsOn string `json:"runs_on,omitempty"`
}

// resolveAnswer is `atlas resolve`'s output (§3's GET /api/resolve, without the route).
type resolveAnswer struct {
	Name       string                         `json:"name"`
	Table      map[string]int                 `json:"table"` // aliases.yaml's digest
	Matches    []resolveMatch                 `json:"matches"`
	IDs        []string                       `json:"ids"`
	Ambiguous  bool                           `json:"ambiguous"`  // more than one id: never pick one
	Historical bool                           `json:"historical"` // every match is a name the thing no longer has
	Selectors  map[string]map[string][]string `json:"selectors,omitempty"`
}

// resolve answers a name. Selectors (each source's names for the id, by field) come only with one id.
func (d *door) resolve(name string) resolveAnswer {
	a := resolveAnswer{Name: name, Table: map[string]int{"digest": d.al.Digest}, Matches: []resolveMatch{}, IDs: []string{}}
	a.Historical = true
	for _, m := range d.al.Resolve(name) {
		a.Matches = append(a.Matches, resolveMatch{Match: m, RunsOn: d.al.RunsOn(m.ID)})
		if len(a.IDs) == 0 || a.IDs[len(a.IDs)-1] != m.ID { // Resolve sorts by id
			a.IDs = append(a.IDs, m.ID)
		}
		a.Historical = a.Historical && m.Status == realms.Historical
	}
	a.Historical = a.Historical && len(a.Matches) > 0
	a.Ambiguous = len(a.IDs) > 1
	if len(a.IDs) == 1 {
		a.Selectors = map[string]map[string][]string{}
		for _, al := range d.al.Things[a.IDs[0]].Aliases {
			if al.Status == realms.Historical {
				continue
			}
			if a.Selectors[al.Source] == nil {
				a.Selectors[al.Source] = map[string][]string{}
			}
			if a.Selectors[al.Source][al.Field] == nil {
				a.Selectors[al.Source][al.Field] = d.al.Names(a.IDs[0], al.Source, al.Field)
			}
		}
	}
	return a
}

// doorCmd runs `atlas resolve` or `atlas state` with args, the answer as JSON on stdout. The exit code is 0; 1 for a
// name that is nothing (or an answer that could not be written); 2 for bad flags or an ambiguous name (state); 3 for
// a state that is partial with no fact at all (every source asked failed: the answer is all unknowns).
func doorCmd(cmd string, args []string, stdout, stderr io.Writer) int {
	d, err := loadDoor()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if cmd == "resolve" {
		if len(args) != 1 {
			_, _ = fmt.Fprintln(stderr, "resolve: one name is required (atlas resolve <name>)")
			return 2
		}
		r := d.resolve(args[0])
		if err := enc.Encode(r); err != nil {
			_, _ = fmt.Fprintln(stderr, "resolve:", err)
			return 1
		}
		if len(r.IDs) == 0 {
			return 1
		}
		return 0
	}
	o, name, err := parseDoor(cmd, args, stderr)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	ans, err := d.state(context.Background(), o, &http.Client{}, name, time.Now)
	switch {
	case errors.Is(err, errAmbiguous):
		_ = enc.Encode(d.resolve(name))
		_, _ = fmt.Fprintf(stderr, "state: %q is ambiguous: ask by one of the ids above\n", name)
		return 2
	case err != nil:
		_ = enc.Encode(d.resolve(name))
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if err := enc.Encode(ans); err != nil {
		_, _ = fmt.Fprintln(stderr, "state:", err)
		return 1
	}
	if ans.Partial && len(ans.Facts) == 0 {
		_, _ = fmt.Fprintf(stderr, "state: %q: no source answered (partial, no facts)\n", name)
		return 3
	}
	return 0
}
