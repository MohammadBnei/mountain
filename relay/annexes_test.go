package main

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/MohammadBnei/mountain/relay/town"
)

func annexFrames(frames []map[string]any) (out []string) {
	for _, f := range frames {
		if f["t"] == "world" && f["kind"] == "annex" {
			s := fmt.Sprint(f["app"], "@", f["host"])
			if f["gone"] == true {
				s += " gone"
			}
			out = append(out, s)
		}
	}
	return
}

func tents(a *App) (out []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, x := range a.State.Annexes {
		out = append(out, x.key())
	}
	slices.Sort(out)
	return
}

// annexRig: one poll = place() then annexes(), with the same answer for both (no Job pods in these tests).
type annexRig struct {
	t     *testing.T
	a     *App
	c     *Client
	pl    *placer
	o     *Out
	now   time.Time
	ready map[string]bool
}

func newAnnexRig(t *testing.T) *annexRig {
	a, c := testApp(t)
	return &annexRig{t: t, a: a, c: c, pl: newPlacer(), o: &Out{app: a, name: "prom"}, now: time.Unix(1_800_000_000, 0), ready: allReady}
}

func (r *annexRig) poll(pods []sample, dt time.Duration) []string {
	r.now = r.now.Add(dt)
	r.pl.place(r.a, r.o, pods, nil, r.ready, r.now)
	r.pl.annexes(r.a, r.o, pods, true, r.ready, r.now)
	frames := drain(r.c)
	if len(frames) > 0 {
		validate(r.t, [][]map[string]any{frames})
	}
	return annexFrames(frames)
}

// wird on h1 (two pods) and h2 (one): its building on h1, a tent on h2.
var wirdSpread = []sample{pod("wird", "wird-a", "k8s-cp-01"), pod("wird", "wird-b", "k8s-worker-01"), pod("wird", "wird-c", "k8s-cp-02")}

// TestAnnexHold: a tent appears and leaves only after annexHold in every poll, a flap starts over, and a steady town
// sends no annex frame at all.
func TestAnnexHold(t *testing.T) {
	r := newAnnexRig(t)
	if got := r.poll(wirdSpread, 0); len(got) != 0 {
		t.Fatalf("a tent at the first sight: %v", got)
	}
	if got := r.poll(wirdSpread, annexHold-time.Second); len(got) != 0 {
		t.Fatalf("a tent before annexHold: %v", got)
	}
	if got := r.poll(wirdSpread, time.Second); fmt.Sprint(got) != "[wird@h2]" || fmt.Sprint(tents(r.a)) != "[wird@h2]" {
		t.Fatalf("after annexHold: %v, state %v", got, tents(r.a))
	}
	for range 10 { // steady state: zero frames
		if got := r.poll(wirdSpread, 15*time.Second); len(got) != 0 {
			t.Fatalf("a steady town sent %v", got)
		}
	}
	h1only := wirdSpread[:2]
	r.poll(h1only, 0)
	r.poll(h1only, annexHold/2)
	r.poll(wirdSpread, time.Second) // back: the departure clock is gone
	r.poll(h1only, time.Second)
	if got := r.poll(h1only, annexHold-2*time.Second); len(got) != 0 {
		t.Fatalf("a flap must restart the clock: %v", got)
	}
	if got := r.poll(h1only, 2*time.Second); fmt.Sprint(got) != "[wird@h2 gone]" || len(tents(r.a)) != 0 {
		t.Fatalf("departure: %v, state %v", got, tents(r.a))
	}
}

// TestAnnexBlind: an empty or failed answer changes nothing and restarts every clock; an app missing from the answer
// keeps its tents; a NotReady host keeps its tents.
func TestAnnexBlind(t *testing.T) {
	r := newAnnexRig(t)
	r.poll(wirdSpread, 0)
	r.poll(wirdSpread, annexHold)
	r.now = r.now.Add(annexHold)
	r.pl.annexes(r.a, r.o, nil, false, r.ready, r.now) // failed
	r.pl.annexes(r.a, r.o, nil, true, r.ready, r.now)  // empty
	if got := annexFrames(drain(r.c)); len(got) != 0 || len(tents(r.a)) != 1 {
		t.Fatalf("blind changed the tents: %v %v", got, tents(r.a))
	}
	other := []sample{pod("traefik", "traefik-1", "k8s-cp-01")} // wird absent from the answer
	for range 3 {
		if got := r.poll(other, annexHold); len(got) != 0 {
			t.Fatalf("an absent app lost its tent: %v", got)
		}
	}
	r.ready = map[string]bool{"h1": true, "h2": false, "h3": true}
	for range 3 {
		if got := r.poll(wirdSpread[:2], annexHold); len(got) != 0 {
			t.Fatalf("a NotReady host lost its tent: %v", got)
		}
	}
	r.ready = allReady
	r.poll(wirdSpread[:2], 0)
	if got := r.poll(wirdSpread[:2], annexHold); fmt.Sprint(got) != "[wird@h2 gone]" {
		t.Fatalf("Ready again, the hold starts then: %v", got)
	}
	r.poll(wirdSpread, 0)
	r.ready = map[string]bool{"h1": true, "h2": false, "h3": true}
	if got := r.poll(wirdSpread, annexHold); len(got) != 0 {
		t.Fatalf("no new tent on a NotReady host: %v", got)
	}
}

// TestAnnexMove: no tent goes up on the host a move is pending to; when the building moves there, its own tent
// there goes first (before the plot frame), and the old host gets its tent after the hold.
func TestAnnexMove(t *testing.T) {
	r := newAnnexRig(t)
	r.poll(wirdSpread, 0)
	r.poll(wirdSpread, annexHold)
	if fmt.Sprint(tents(r.a)) != "[wird@h2]" {
		t.Fatal("setup")
	}
	// h2 takes the majority: a move is pending to h2 (pl.cand), whose tent stays put meanwhile
	toH2 := []sample{pod("wird", "wird-a", "k8s-cp-01"), pod("wird", "wird-c", "k8s-cp-02"), pod("wird", "wird-d", "k8s-worker-02")}
	r.poll(toH2, 0)
	r.now = r.now.Add(moveAfter)
	r.pl.place(r.a, r.o, toH2, nil, r.ready, r.now)
	frames := drain(r.c)
	var order []string
	for _, f := range frames {
		order = append(order, fmt.Sprintf("%v %v", f["kind"], f["gone"] == true))
	}
	if fmt.Sprint(order) != "[annex true plot false]" {
		t.Fatalf("the move: %v", order)
	}
	if p, _ := plotOf(r.a, "wird"); p.Host != "h2" || len(tents(r.a)) != 0 {
		t.Fatalf("after the move: %+v %v", p, tents(r.a))
	}
	r.pl.annexes(r.a, r.o, toH2, true, r.ready, r.now)
	r.poll(toH2, 0)
	if got := r.poll(toH2, annexHold); fmt.Sprint(got) != "[wird@h1]" {
		t.Fatalf("the old host's tent: %v", got)
	}

	// no flash: a host a move is pending to never gets a tent first
	r2 := newAnnexRig(t)
	one := []sample{pod("wird", "wird-a", "k8s-cp-01")}
	r2.poll(one, 0)
	tie := append(slices.Clone(one), pod("wird", "wird-b", "k8s-cp-03")) // a tie keeps h1: h3 is wanted
	surge := append(slices.Clone(tie), pod("wird", "wird-c", "k8s-cp-03"))
	r2.poll(surge, 0) // h3 leads: cand = h3
	for range 3 {
		if got := r2.poll(surge, annexHold/2); len(got) != 0 && !strings.Contains(fmt.Sprint(got), "h1") {
			t.Fatalf("a tent flashed on the pending move target: %v", got)
		}
	}
}

// TestAnnexRuins: ruins take every tent at once, a ruined app is never wanted (no flash), machines never get tents,
// and private workloads or unmapped nodes never make one.
func TestAnnexRuins(t *testing.T) {
	r := newAnnexRig(t)
	pods := append(slices.Clone(wirdSpread), pod("wird", "wird-x", "k8s-cp-03"),
		pod("wird", "wird-adminweb-1", "k8s-cp-03"),     // private workload in a public namespace
		pod("monitoring", "prom-0", "k8s-worker-9.lan"), // unmapped node
		pod("garage", "garage-0", "k8s-cp-02"))          // granary is a machine: no pods count, no tent
	r.poll(pods, 0)
	if got := r.poll(pods, annexHold); fmt.Sprint(got) != "[wird@h2 wird@h3]" {
		t.Fatalf("tents: %v", got)
	}
	r.pl.ruin(r.a, r.o, "wird", true, r.now)
	if got := annexFrames(drain(r.c)); fmt.Sprint(got) != "[wird@h2 gone wird@h3 gone]" {
		t.Fatalf("ruins: %v", got)
	}
	for range 4 {
		if got := r.poll(pods, annexHold); len(got) != 0 {
			t.Fatalf("a ruined app flashed a tent: %v", got)
		}
	}
	r.pl.ruin(r.a, r.o, "wird", false, r.now)
	r.poll(pods, 0)
	if got := r.poll(pods, annexHold); fmt.Sprint(got) != "[wird@h2 wird@h3]" {
		t.Fatalf("rebuilt: %v", got)
	}
}

// TestAnnexQuery: the tents' own pods query is optional and leaves out Job pods.
func TestAnnexQuery(t *testing.T) {
	q := promQ["annexpods"]
	if !optionalQ["annexpods"] || !strings.HasSuffix(q, `unless on (namespace, pod) kube_pod_owner{owner_kind="Job"}`) || !strings.HasPrefix(q, promQ["pods"]) {
		t.Fatalf("annexpods: %s", q)
	}
}

// TestAnnexEvict: a plot takes a tent's hex before a plant's, the tent's gone frame comes first, and a plant may not
// go on a tent; a full island counts tents.
func TestAnnexEvict(t *testing.T) {
	r := newAnnexRig(t)
	// h3 full of other buildings but one hex, where wird's tent stands
	var free []town.Hex
	for _, x := range town.Hexes("h3") {
		if town.Tile("h3", x) == town.Grass {
			free = append(free, x)
		}
	}
	r.a.mu.Lock()
	for i, x := range town.Hexes("h3") { // every plantable hex (grass and meadow) but the tent's
		if town.Plantable("h3", x) && x != free[0] {
			r.a.State.Plots = append(r.a.State.Plots, Plot{App: fmt.Sprint("filler-", i), Host: "h3", Q: x.Q, R: x.R, Role: "home"})
		}
	}
	r.a.State.Plots = append(r.a.State.Plots, Plot{App: "wird", Host: "h1", Q: 1, R: 0, Role: "inn"})
	r.a.State.Annexes = []Annex{{App: "wird", Host: "h3", Q: free[0].Q, R: free[0].R}}
	r.a.mu.Unlock()
	p := &Place{Pose: map[string]any{"h": 3.0, "q": float64(free[0].Q), "r": float64(free[0].R)}, Plots: r.a.State.Plots, Annex: r.a.State.Annexes}
	if _, _, why := validatePlant(p); why != "plot" {
		t.Fatalf("a plant on a tent: %q", why)
	}
	if !islandFull("h3", r.a.State.Plots, r.a.State.Annexes, nil) {
		t.Fatal("islandFull must count tents")
	}
	r.pl.place(r.a, r.o, []sample{pod("dream-analyst", "dream-1", "k8s-cp-03")}, nil, r.ready, r.now)
	frames := drain(r.c)
	var order []string
	for _, f := range frames {
		order = append(order, fmt.Sprintf("%v %v", f["kind"], f["gone"] == true))
	}
	if fmt.Sprint(order) != "[annex true plot false]" || !r.pl.evicted["wird@h3"] {
		t.Fatalf("eviction: %v %v", order, r.pl.evicted)
	}
}

// TestDBAnnexRestore: a restart keeps the tents (no early publish, no flicker) and prunes only what geometry forbids:
// a machine, an unknown app or host, a non-grass hex, a hex a plot, a plant or an earlier tent holds.
func TestDBAnnexRestore(t *testing.T) {
	dsn := testDSN(t)
	a := storedApp(t, dsn)
	pl, o, now := newPlacer(), &Out{app: a, name: "prom"}, time.Now()
	pl.place(a, o, wirdSpread, nil, allReady, now)
	wird, _ := plotOf(a, "wird")
	var grass []town.Hex
	for _, x := range town.Hexes("h2") {
		if town.Tile("h2", x) == town.Grass {
			grass = append(grass, x)
		}
	}
	a.mu.Lock()
	a.State.Annexes = []Annex{
		{App: "wird", Host: "h2", Q: grass[0].Q, R: grass[0].R},    // kept
		{App: "wird", Host: "h2", Q: grass[1].Q, R: grass[1].R},    // a second wird@h2: dropped
		{App: "granary", Host: "h2", Q: grass[2].Q, R: grass[2].R}, // a machine
		{App: "nope", Host: "h2", Q: grass[3].Q, R: grass[3].R},    // not configured
		{App: "traefik-x", Host: "h4", Q: 0, R: 1},                 // no such host
		{App: "gate", Host: "h2", Q: 0, R: 0},                      // the square
		{App: "depot", Host: wird.Host, Q: wird.Q, R: wird.R},      // on a plot
		{App: "vault", Host: "h2", Q: grass[0].Q, R: grass[0].R},   // on an earlier tent
	}
	a.mu.Unlock()
	a.snapshot(now)
	a.store.Close(5 * time.Second)
	b := storedApp(t, dsn)
	if got := fmt.Sprint(tents(b)); got != "[wird@h2]" {
		t.Fatalf("restored tents: %v", got)
	}
}
