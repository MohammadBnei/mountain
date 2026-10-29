package realms

import (
	"slices"
	"strings"
	"testing"
)

// TestLayers: the disc's rings Hub→Rim (plan/17 D1), a layer for every group but the land's, written out (no globs),
// the owner's default table, and a component's own layer over its group's; Parse refuses a group with none, a layer
// out of LayerOrder, and an override out of it.
func TestLayers(t *testing.T) {
	if !slices.Equal(LayerOrder, []string{"core", "workshops", "data", "services", "edge"}) {
		t.Errorf("LayerOrder %v", LayerOrder)
	}
	r, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	keys := 0
	for key, fn := range r.Functions {
		l, ok := r.Layers[key]
		if fn == FnLand {
			if ok {
				t.Errorf("%s is the land, never a building: layer %q", key, l)
			}
			continue
		}
		keys++
		if !slices.Contains(LayerOrder, l) {
			t.Errorf("%s: layer %q, not one of %v", key, l, LayerOrder)
		}
	}
	if keys != 26 || len(r.Layers) != keys {
		t.Errorf("%d non-land functions keys, %d layers keys: each written out", keys, len(r.Layers))
	}
	for key := range r.Layers {
		if _, ok := r.Functions[key]; !ok || strings.Contains(key, "*") {
			t.Errorf("layers key %q is no functions key", key)
		}
	}
	for key, want := range map[string]string{
		"platform/nodes": "core", "platform/network": "core", "platform/metrics": "core", "fleet/control": "core",
		"fleet/cluster-access": "core", "fleet/session-pod": "core", "operations/delivery": "core", "operations/atlas": "core",
		"data/postgres": "data", "data/redis": "data", "data/s3": "data", "data/nfs": "data",
		"platform/storage": "services", "platform/gpu": "services", "operations/build": "services",
		"operations/observability": "services", "operations/events": "services", "guard/authentik": "services",
		"guard/secrets": "services", "fleet/hermes": "services", "apps/sites": "services", "apps/products": "services",
		"apps/ai": "services", "apps/tools": "services",
		"platform/edge": "edge", "guard/mesh": "edge",
	} {
		if got := r.Layers[key]; got != want {
			t.Errorf("layer of %s = %q, want %q", key, got, want)
		}
	}
	for id, c := range r.Components {
		if c.Layer != "" && !slices.Contains(LayerOrder, c.Layer) {
			t.Errorf("%s: layer %q, not one of %v", id, c.Layer, LayerOrder)
		}
	}

	base := "realms: [infra, code]\nfunctions: {infra/g: utility, infra/h: land}\nhosts: {a: h1}\npveNodeName: a\nsources: [infra-bootstrap]\n"
	p, err := Parse([]byte(base + "layers: {infra/g: core}\ncomponents: {app:x: {realm: infra, group: g}, app:y: {realm: infra, group: g, layer: edge}}"))
	if err != nil {
		t.Fatalf("minimal: %v", err)
	}
	if p.Layers["infra/g"] != "core" || p.Components["app:y"].Layer != "edge" || p.Components["app:x"].Layer != "" {
		t.Errorf("minimal: %v %+v", p.Layers, p.Components)
	}
	for name, y := range map[string]string{
		"group no layer": "layers: {}",
		"unknown layer":  "layers: {infra/g: moon}",
		"bad override":   "layers: {infra/g: core}\ncomponents: {app:x: {realm: infra, group: g, layer: moon}}",
	} {
		if _, err := Parse([]byte(base + y)); err == nil {
			t.Errorf("%s: parsed", name)
		}
	}
}
