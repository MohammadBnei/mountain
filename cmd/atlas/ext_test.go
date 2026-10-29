package main

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/MohammadBnei/mountain/contract"
	"github.com/MohammadBnei/mountain/realms"
)

// TestExtProbeHealth: a moon's health is its blackbox probe (ext-<name>), through the existing probes join; a moon
// declared but never probed (ext:pi-lab, until it has a target) reads unknown; a target for an undeclared moon is no node.
func TestExtProbeHealth(t *testing.T) {
	rl, err := realms.Parse([]byte("realms: [infra, code]\nfunctions: {infra/g: utility}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\n" +
		"components: {app:x: {realm: infra, group: g}}\n" +
		`needs: {app:x: ["declared:ext:github", "declared:ext:anthropic", "declared:ext:pi-lab"]}`))
	if err != nil {
		t.Fatal(err)
	}
	idx := &GraphIndex{Realm: map[string]string{"app:x": "infra"}, NSApps: map[string][]string{}, Has: map[string]bool{"app:x": true},
		Kind: map[string]string{"app:x": "app", "ext:github": "ext", "ext:anthropic": "ext", "ext:pi-lab": "ext"}, RunsOn: map[string][]string{}}
	for id := range idx.Kind {
		idx.Has[id] = true
	}
	l := newLive(rl, NewHub(false, func() []byte { return nil }, nil), nil)
	l.idx = func() *GraphIndex { return idx }
	srv := fakeProm(t, func() map[string]string {
		return map[string]string{"probes": pvec(`{"target":"ext-github"}`, "1", `{"target":"ext-anthropic"}`, "0", `{"target":"ext-cloudflare"}`, "1")}
	})
	p := &promPoll{l: l, prom: promHTTP{base: srv.URL, hc: http.DefaultClient}, idx: l.idx}
	if err := p.poll(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	s := l.st
	if !s.Probes["ext:github"] || s.Probes["ext:anthropic"] || len(s.Probes) != 2 {
		t.Errorf("probes: %v (ext-cloudflare is not declared here)", s.Probes)
	}
	for id, want := range map[string]string{"ext:github": contract.OK, "ext:anthropic": contract.Down, "ext:pi-lab": contract.Unknown} {
		if h, ok := s.Health[id]; !ok || h.State != want {
			t.Errorf("%s: %+v (set %v), want %s", id, h, ok, want)
		}
	}
}
