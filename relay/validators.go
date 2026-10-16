package main

// Visitor marks (W4): the toy registry (frozen W2 name, 01), the client mark frames, rate limits, the kill switch and
// the toy tick. Orchestrator-owned seam; each toy's rules live in toy_<name>.go (one lane each), registered from init():
//
//	func init() { RegisterToy("cairns", Toy{Validate: validateCairn}) }
//
// Client → relay (06 WS protocol; schema www/dev/schema/mark.json):
//
//	{t:"mark", ref?, mark:<toy>, pose:{…}, named?}   session visitor|admin; named = sign it with the moderated handle
//	{t:"mark.delete", ref?, id}                      admin only (08 moderation; detail mode)
//
// The sender alone hears back {t:"mark", ref?, id} or {t:"mark", ref?, error}; an accepted mark reaches everyone as
// world:mark {id, mark, pose, weight, by?} (by: the handle, only when named and clean), a deleted one as world:mark
// {id, mark, pose:{}, weight:0, gone:true}. Both ride the hub tap like any world frame: hello.state.marks (≤ marksMax,
// the oldest drops out), world_deltas, scrub. The private side (sub, handle) goes to the marks table only (003).
// Errors: off (VISITORS_ENABLED=false) | auth (not a visitor/admin, or the session lapsed) | toy (unknown) | rate
// (1/min, 20 per 24 h per account) | bad (malformed, or the toy's pose failed sanitize) | unknown (delete: no such
// active mark) | a toy's own reason ([a-z-]{1,24}: snowline, reach, slope, …).

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/MohammadBnei/mountain/relay/worldcore"
)

const (
	marksMax    = 2000 // active marks, global cap (06 limits)
	cloudsMax   = 256  // live toy clouds kept in hello.state
	markEvery   = time.Minute
	marksPerDay = 20 // per account, rolling 24 h
	anonHandle  = "a visitor"
)

// Toy is one surface toy's relay side.
type Toy struct {
	// Validate checks a visitor's pose (client input: untrusted) against the world and returns the public pose and
	// weight, or a reject reason ([a-z-]{1,24}). It recomputes y from heightSolid, quantises x/z and keeps the mark
	// above the snowline (03 surface-layer rules); Place.Spot does all three. The pose must hold only finite numbers,
	// bools and []float64 (≤ 16 keys, ≤ 16 numbers each: sanitize fPose), never text. Called with no lock held,
	// serialised with every other placement; a panic rejects the mark ("bad").
	Validate func(p *Place) (pose map[string]any, weight float64, reject string)
	// Tick (optional) runs every toyTick on one goroutine with no lock held: read a.State / a.Metrics / a.Land /
	// a.MarkSubs under a.mu, release it, then emit through o.World (never while holding a.mu: lock order h.mu → a.mu).
	// toy-clouds: drift (pos = f(spawn, ∫wind)), merge and rain → world:cloud / world:rain; footprints: erosion weight.
	Tick func(a *App, o *Out, now time.Time)
}

var toys = map[string]Toy{}

// RegisterToy adds a toy; call it from init(). name is its model folder and world:mark `mark` ([a-z-]{1,24}).
func RegisterToy(name string, t Toy) { toys[name] = t }

// toyTick is the Tick period (fixture mode: the step).
var toyTick = 15 * time.Second

// Place is what Validate sees: the request and a copy of the world taken when it arrived.
type Place struct {
	Pose     map[string]any // the client's pose, decoded JSON (numbers are float64, arrays []any): untrusted
	Sub      string         // the account (OIDC sub): never public; rain needs 3 distinct ones (toy-clouds)
	Handle   string         // its public name: the moderated handle when it opted in (named), else "a visitor"
	Solid    []int16        // heightSolid = h.base + h.land, HEIGHT_Q steps, GRID², z outer
	Snowline float64        // world units (metrics.snowline)
	Wind     XZ
	Marks    []Mark // the active marks, oldest first (a copy)
	Now      time.Time
}

// Mark is a world:mark payload (hello.state.marks).
type Mark struct {
	ID     string         `json:"id"`
	Mark   string         `json:"mark"`
	Pose   map[string]any `json:"pose"`
	Weight float64        `json:"weight"`
	By     string         `json:"by,omitempty"`
}

// Cloud is a world:cloud payload (hello.state.clouds). Emit it as {id: ULID, pos: []float64{x, y, z},
// state: drift|rain|gone, credits: []string of handles}.
type Cloud struct {
	ID      string     `json:"id"`
	Pos     [3]float64 `json:"pos"`
	State   string     `json:"state"`
	Credits []string   `json:"credits"`
}

// Num is the finite number at key k of the client pose.
func (p *Place) Num(k string) (float64, bool) {
	f, ok := p.Pose[k].(float64)
	return f, ok && finite(f)
}

// Vec is the n finite numbers at key k of the client pose.
func (p *Place) Vec(k string, n int) ([]float64, bool) {
	l, ok := p.Pose[k].([]any)
	if !ok || len(l) != n {
		return nil, false
	}
	out := make([]float64, n)
	for i, x := range l {
		if out[i], ok = x.(float64); !ok || !finite(out[i]) {
			return nil, false
		}
	}
	return out, true
}

// Ground quantises (x, z) to its GRID cell (01 Units: cell i at x = −100 + i·CELL, world.cell on the client) and
// returns that point and heightSolid there in world units; ok = false outside the world (or NaN).
func (p *Place) Ground(x, z float64) (qx, y, qz float64, ok bool) {
	if !(x >= -100 && x <= 100 && z >= -100 && z <= 100) {
		return 0, 0, 0, false
	}
	i, j := gridCell(x), gridCell(z)
	qx, qz = worldcore.CellXZ(i, j)
	return qx, float64(p.Solid[j*worldcore.Grid+i]) * worldcore.HeightQ, qz, true
}

// Slope is |∇heightSolid| (rise over run) at the cell of (x, z), central differences.
func (p *Place) Slope(x, z float64) float64 {
	i, j := gridCell(x), gridCell(z)
	h := func(i, j int) float64 {
		i, j = max(0, min(worldcore.Grid-1, i)), max(0, min(worldcore.Grid-1, j))
		return float64(p.Solid[j*worldcore.Grid+i]) * worldcore.HeightQ
	}
	return math.Hypot(h(i+1, j)-h(i-1, j), h(i, j+1)-h(i, j-1)) / (2 * worldcore.Cell)
}

// Spot reads pose[k] = [x, y, z] and returns [qx, y, qz]: x/z quantised to the cell, the client's y discarded and
// recomputed from heightSolid, above the snowline. Rejects: bad (missing, not 3 finite numbers), reach (outside the
// world), snowline (at or below it).
func (p *Place) Spot(k string) ([]float64, string) {
	v, ok := p.Vec(k, 3)
	if !ok {
		return nil, "bad"
	}
	x, y, z, ok := p.Ground(v[0], v[2])
	switch {
	case !ok:
		return nil, "reach"
	case y <= p.Snowline:
		return nil, "snowline"
	}
	return []float64{x, y, z}, ""
}

func gridCell(c float64) int {
	return max(0, min(worldcore.Grid-1, int(math.Round((c+100)/worldcore.Cell))))
}

// ---------- handles (08 moderation) ----------

var handleRE = regexp.MustCompile(`^[a-z0-9_-]{3,20}$`)

// handleDenyExact / handleDeny: whole handles and substrings a handle may not be or contain (impersonation + abuse).
// The SAME list as the authentik `mountain-handle` enrollment policy (infra-bootstrap
// authentik-blueprint-mountain-enrollment.yaml): change both together. ponytail: a short list; extend it on the first report.
var handleDenyExact = []string{"root", "mod", "mods", "staff", "system", "null", "none", "undefined", "anonymous", "mountain",
	"visitor", "everyone", "here", "sex", "kill", "rape"}
var handleDeny = []string{"admin", "authentik", "ukubi", "bnei", "moderat", "official", "support", "operator", "fuck", "shit",
	"cunt", "bitch", "nigg", "fag", "nazi", "hitler", "porn"}

// handle is the public form of an OIDC username: 3–20 [a-z0-9_-] as is (upper case is refused, not folded: "Ana_K"
// is not "ana_k"), not denylisted; "" otherwise. Applied at every use (not only at sign-in), so a denylist change also
// covers live sessions.
func handle(name string) string {
	if !handleRE.MatchString(name) || slices.Contains(handleDenyExact, name) {
		return ""
	}
	for _, d := range handleDeny {
		if strings.Contains(name, d) {
			return ""
		}
	}
	return name
}

// ---------- rate limits (06: 1/min, 20/day per account) ----------

// visits holds each account's accepted marks of the last 24 h (seeded from the marks table at boot, store.go). Its
// lock also serialises placements: check, validate, record. ponytail: one global lock; marks are ≤ 1/min/account.
var visits = struct {
	sync.Mutex
	at map[string][]time.Time
}{at: map[string][]time.Time{}}

func allowVisit(sub string, now time.Time) bool {
	l := slices.DeleteFunc(visits.at[sub], func(t time.Time) bool { return now.Sub(t) >= 24*time.Hour })
	if len(l) == 0 {
		delete(visits.at, sub)
		return true
	}
	visits.at[sub] = l
	return len(l) < marksPerDay && now.Sub(l[len(l)-1]) >= markEvery
}

// quota is sub's mark budget at now (visits locked): whole seconds until the next mark is allowed (0 = now) and the
// marks left in the rolling 24 h. The toolbar's countdown reads it from the ack.
func quota(sub string, now time.Time) (next, left int) {
	l := slices.DeleteFunc(slices.Clone(visits.at[sub]), func(t time.Time) bool { return now.Sub(t) >= 24*time.Hour })
	left = marksPerDay - len(l)
	var wait time.Duration
	if len(l) > 0 {
		wait = markEvery - now.Sub(l[len(l)-1])
	}
	if left <= 0 {
		wait = max(wait, 24*time.Hour-now.Sub(l[0]))
	}
	return int((max(wait, 0) + time.Second - 1) / time.Second), max(left, 0)
}

// ---------- client frames ----------

var refRE, reasonRE = regexp.MustCompile(`^[A-Za-z0-9]{1,16}$`), regexp.MustCompile(`^[a-z-]{1,24}$`)

// markAck is the sender's answer: {t:"mark", ref?, id} or {t:"mark", ref?, error}; an accept or a rate refusal also
// carries the sender's quota {next, left}.
func markAck(ref, id, why string, q ...[2]int) []byte {
	f := map[string]any{"t": "mark"}
	for _, x := range q {
		f["next"], f["left"] = x[0], x[1]
	}
	if refRE.MatchString(ref) {
		f["ref"] = ref
	}
	if why != "" {
		f["error"] = why
	} else {
		f["id"] = id
	}
	return mustJSON(f)
}

// markMsg handles {t:"mark"} and {t:"mark.delete"} (clientMsg, scrub.go) and answers the sender.
func (a *App) markMsg(c *Client, msg []byte) {
	var f struct {
		T, Ref, Mark, ID string
		Pose             map[string]any
		Named            bool
	}
	if json.Unmarshal(msg, &f) != nil {
		send(c, markAck("", "", "bad"))
		return
	}
	if f.T == "mark.delete" {
		send(c, a.unmark(c, f.Ref, f.ID, time.Now()))
		return
	}
	send(c, a.place(c, f.Ref, f.Mark, f.Pose, f.Named, time.Now()))
}

func (c *Client) canMark(now time.Time) bool {
	return (c.Role == roleVisitor || c.Role == roleAdmin) && c.Sub != "" && now.Before(c.RoleExp)
}

// place validates one mark through its toy and, accepted, broadcasts it and stores the private record.
func (a *App) place(c *Client, ref, toy string, pose map[string]any, named bool, now time.Time) (ack []byte) {
	a.mu.Lock()
	on := a.Metrics.Visitors
	a.mu.Unlock()
	t, known := toys[toy]
	switch {
	case !on:
		return markAck(ref, "", "off")
	case !c.canMark(now):
		return markAck(ref, "", "auth")
	case !known || t.Validate == nil:
		return markAck(ref, "", "toy")
	case pose == nil:
		return markAck(ref, "", "bad")
	}
	visits.Lock()
	defer visits.Unlock()
	if !allowVisit(c.Sub, now) {
		n, l := quota(c.Sub, now)
		return markAck(ref, "", "rate", [2]int{n, l})
	}
	h := anonHandle
	if x := handle(c.Name); named && x != "" {
		h = x
	}
	a.mu.Lock()
	p := &Place{Pose: pose, Sub: c.Sub, Handle: h, Solid: worldcore.Solid(a.World.Base, a.Land), Snowline: a.Metrics.Snowline,
		Wind: a.Metrics.Wind, Marks: slices.Clone(a.State.Marks), Now: now}
	a.mu.Unlock()
	out, w, why := runValidate(toy, t, p)
	if why != "" {
		if !reasonRE.MatchString(why) {
			why = "bad"
		}
		return markAck(ref, "", why)
	}
	id := ULID()
	m := map[string]any{"id": id, "mark": toy, "pose": out, "weight": w}
	if h != anonHandle {
		m["by"] = h
	}
	b := (&Out{app: a, name: "marks"}).worldFrame("mark", m)
	if b == nil { // the toy built a pose sanitize refuses (logged there)
		return markAck(ref, "", "bad")
	}
	visits.at[c.Sub] = append(visits.at[c.Sub], now)
	a.mu.Lock()
	if len(a.MarkSubs) > 2*marksMax { // ponytail: marks that fell off the cap leave their sub behind; rebuild now and then
		live := map[string]string{}
		for _, x := range a.State.Marks {
			live[x.ID] = a.MarkSubs[x.ID]
		}
		a.MarkSubs = live
	}
	a.MarkSubs[id] = c.Sub
	a.mu.Unlock()
	a.Hub.Broadcast(b)
	a.store.mark(id, c.Sub, h, toy, now, out, w)
	n, l := quota(c.Sub, now)
	return markAck(ref, id, "", [2]int{n, l})
}

// runValidate runs a toy's Validate; a panic (a lane bug reached by client input) rejects instead of killing the relay.
func runValidate(name string, t Toy, p *Place) (pose map[string]any, w float64, why string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("marks: %s Validate panicked: %v", name, r)
			pose, w, why = nil, 0, "bad"
		}
	}()
	return t.Validate(p)
}

// unmark is the admin mark.delete: the mark leaves hello.state.marks for everyone (world:mark gone) and its row gets
// deleted_at. It works with visitors off (moderation must not need them on).
func (a *App) unmark(c *Client, ref, id string, now time.Time) []byte {
	if c.Role != roleAdmin || !now.Before(c.RoleExp) {
		return markAck(ref, "", "auth")
	}
	a.mu.Lock()
	i := slices.IndexFunc(a.State.Marks, func(m Mark) bool { return m.ID == id })
	var m Mark
	if i >= 0 {
		m = a.State.Marks[i]
		delete(a.MarkSubs, id)
	}
	a.mu.Unlock()
	if i < 0 {
		return markAck(ref, "", "unknown")
	}
	a.Hub.Broadcast((&Out{app: a, name: "marks"}).worldFrame("mark", map[string]any{"id": id, "mark": m.Mark, "pose": map[string]any{}, "weight": 0.0, "gone": true}))
	a.store.unmark(id, now)
	log.Printf("marks: %s (%s) deleted by an admin", id, m.Mark)
	return markAck(ref, id, "")
}

// ---------- kill switch (06: VISITORS_ENABLED, no deploy) ----------

// flagFile is VISITORS_ENABLED as a file: helm mounts that key of the Infisical-synced Secret (values.yaml
// extraVolumes), and the kubelet refreshes a mounted Secret in place, so flipping it in Infisical reaches a running
// relay within a few minutes without a restart. No flags directory (local, fixture) = the env var, read each poll.
var flagFile, flagEvery = "/etc/mountain/flags/VISITORS_ENABLED", 10 * time.Second

// visitorsFlag: "false", "0", "off" or "no" disable; anything else (or nothing) enables.
func visitorsFlag() bool {
	v, err := os.ReadFile(flagFile)
	s := string(v)
	if _, dir := os.Stat(filepath.Dir(flagFile)); err != nil && dir != nil { // no volume (local, fixture): the env var
		s = os.Getenv("VISITORS_ENABLED")
	} // the volume without the file = the key was deleted in Infisical = enabled (not the env bound at pod start)
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "false", "0", "off", "no":
		return false
	}
	return true
}

// watchVisitors polls the flag; a change rides a metrics frame at once (metrics.visitors: clients hide or show the
// surface layer live) and place() rejects marks while it is off.
func (a *App) watchVisitors(ctx context.Context) {
	t := time.NewTicker(flagEvery)
	defer t.Stop()
	for {
		v := visitorsFlag()
		a.mu.Lock()
		same := a.Metrics.Visitors == v
		a.mu.Unlock()
		if !same {
			log.Printf("visitors: VISITORS_ENABLED → %v", v)
			(&Out{app: a, name: "visitors"}).Metrics(func(m *Metrics) { m.Visitors = v })
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// tickToys runs every toy's Tick every toyTick.
func (a *App) tickToys(ctx context.Context) {
	t := time.NewTicker(toyTick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			a.mu.Lock()
			on := a.Metrics.Visitors
			a.mu.Unlock()
			if !on { // the kill switch freezes the visitor layer too: no drift, rain or erosion reaches the wire
				continue
			}
			for name, toy := range toys {
				if toy.Tick != nil {
					tick(name, toy, a, now)
				}
			}
		}
	}
}

func tick(name string, t Toy, a *App, now time.Time) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("marks: %s Tick panicked: %v", name, r)
		}
	}()
	t.Tick(a, &Out{app: a, name: name}, now)
}
