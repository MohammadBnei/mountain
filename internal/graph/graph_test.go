package graph

import "testing"

func TestValid(t *testing.T) {
	for id, want := range map[string]bool{
		"app:wird": true, "svc:agent-fleet/core": true, "adr:infra-bootstrap/0064": true, "dns:wird.bnei.dev": true,
		"pg:pg-proxmox-1": true, "endpoint:pg-proxmox/vip": true,
		"App:wird": false, "pg:": false, "endpoints:x": false, "wird": false, "app:": false, "bad:x": false, "app:with space": false,
	} {
		if Valid(id) != want {
			t.Errorf("Valid(%q) = %v", id, !want)
		}
	}
	for k, meaning := range Kinds {
		if meaning == "" || !Valid(k+":x") {
			t.Errorf("kind %q: meaning %q, Valid(%q:x) = %v", k, meaning, k, Valid(k+":x"))
		}
	}
	if ID(KindApp, "Wird") != "app:wird" {
		t.Error("ID does not lower-case")
	}
}

func TestMerge(t *testing.T) {
	var g Graph
	a := Node{ID: "app:a", Kind: KindApp, Source: Source{Repo: "r", Path: "x"}}
	if err := g.Merge(Graph{Nodes: []Node{a, {ID: "app:b", Kind: KindApp}}}); err != nil {
		t.Fatal(err)
	}
	if err := g.Merge(Graph{Nodes: []Node{a}}); err != nil {
		t.Fatalf("the same node twice: %v", err)
	}
	b := a
	b.Source.Path = "y"
	if err := g.Merge(Graph{Nodes: []Node{b}}); err == nil {
		t.Fatal("one node read two ways merged")
	}
	if err := g.Merge(Graph{Nodes: []Node{{ID: "nope"}}}); err == nil {
		t.Fatal("a bad id merged")
	}
	if g.Nodes[0].ID != "app:a" || g.Nodes[1].ID != "app:b" {
		t.Errorf("not sorted: %v", g.Nodes)
	}
	e1 := Edge{From: "app:a", To: "app:b", Kind: EdgeSupersede, Source: Source{Repo: "r", Path: "2.md"}}
	e2 := e1
	e2.Source.Path = "1.md"
	if err := g.Merge(Graph{Edges: []Edge{e1, e2}}); err != nil || len(g.Edges) != 1 || g.Edges[0].Source.Path != "1.md" {
		t.Fatalf("one edge stated by two files: %v %+v", err, g.Edges)
	}
	e3 := e1
	e3.Attrs = map[string]string{"x": "y"}
	if err := g.Merge(Graph{Edges: []Edge{e3}}); err == nil {
		t.Fatal("one edge with two contents merged")
	}
	g.Edges = []Edge{{From: "app:a", To: "app:c", Kind: EdgeGit}}
	if g.Check() == nil {
		t.Error("an edge to a missing node passed Check")
	}
}
