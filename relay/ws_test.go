package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	for _, c := range []struct{ peer, xff, cf, want string }{
		{"162.158.1.2:443", "", "203.0.113.9", "203.0.113.9"},            // Cloudflare peer: trust CF-Connecting-IP
		{"198.51.100.7:443", "", "203.0.113.9", "198.51.100.7"},          // foreign peer: ignore the header
		{"10.233.64.12:80", "162.158.1.2", "203.0.113.9", "203.0.113.9"}, // via Traefik, CF hop
		{"10.233.64.12:80", "1.2.3.4, 198.51.100.7", "203.0.113.9", "198.51.100.7"},
		{"10.233.64.12:80", "", "203.0.113.9", "10.233.64.12"},
		{"[2606:4700::1]:443", "", "2001:db8::1", "2001:db8::1"},
		{"162.158.1.2:443", "", "not-an-ip", "162.158.1.2"},
	} {
		r := httptest.NewRequest("GET", "/ws", nil)
		r.RemoteAddr = c.peer
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		r.Header.Set("CF-Connecting-IP", c.cf)
		if got := cfClientIP(r); got != c.want {
			t.Errorf("peer %s xff %q cf %q: got %s, want %s", c.peer, c.xff, c.cf, got, c.want)
		}
	}
}

// TestPublicMux: /api/topology lists allowlisted sites only (schema-valid world:site payloads),
// and a cross-origin /ws upgrade through the real public mux gets 403.
func TestPublicMux(t *testing.T) {
	a, _ := testApp(t)
	app = a
	for name := range a.Cfg.Apps {
		liveApps[name] = true
	}
	liveApps["has-journal"] = true  // live but not allowlisted: no site
	Public.Handle("GET /ws", a.Hub) // serve() registers it at startup
	srv := httptest.NewServer(Public)
	defer srv.Close()
	ws := &httptest.Server{URL: srv.URL + "/ws"} // dial() targets srv.URL

	resp, err := http.Get(srv.URL + "/api/topology")
	if err != nil {
		t.Fatal(err)
	}
	var topo struct {
		Sites []map[string]any
		Hosts []string
	}
	json.NewDecoder(resp.Body).Decode(&topo)
	resp.Body.Close()
	if len(topo.Sites) != len(a.Cfg.Apps) || len(topo.Hosts) != 3 {
		t.Fatalf("topology: %+v", topo)
	}
	frames := []map[string]any{}
	for _, s := range topo.Sites {
		if _, ok := a.Cfg.Apps[s["app"].(string)]; !ok {
			t.Errorf("non-allowlisted site %v", s)
		}
		s["t"], s["kind"] = "world", "site"
		frames = append(frames, s)
	}
	validate(t, [][]map[string]any{frames})

	for _, o := range []string{"https://evil.example", "http://mountain.bnei.dev", ""} {
		_, resp, err := dial(t, ws, o)
		if err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
			t.Errorf("origin %q: want 403, got %v %v", o, resp, err)
		}
	}
	c, _, err := dial(t, ws, "https://mountain.bnei.dev")
	if err != nil {
		t.Fatalf("same-origin upgrade: %v", err)
	}
	c.CloseNow()
}
