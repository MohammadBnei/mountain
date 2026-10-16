package main

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MohammadBnei/mountain/relay/worldcore"
	"github.com/coder/websocket"
)

// refToy is the seam's reference validator (what every toy does at least): pos → quantised, y recomputed, above the snowline.
var refToy = Toy{Validate: func(p *Place) (map[string]any, float64, string) {
	pos, why := p.Spot("pos")
	if why != "" {
		return nil, 0, why
	}
	return map[string]any{"pos": pos}, 1, ""
}}

// withToys registers refToy under each name no lane has registered yet, for the test's duration, and resets the limits.
func withToys(t *testing.T, a *App, names ...string) {
	app = a
	for _, n := range names {
		if _, ok := toys[n]; !ok {
			toys[n] = refToy
			t.Cleanup(func() { delete(toys, n) })
		}
	}
	visits.Lock()
	visits.at = map[string][]time.Time{}
	visits.Unlock()
}

func visitor(sub, name string) *Client {
	return &Client{Role: roleVisitor, RoleExp: time.Now().Add(time.Hour), Sub: sub, Name: name, send: make(chan []byte, 4)}
}

func ackOf(t *testing.T, b []byte) (id, why string) {
	t.Helper()
	var f struct{ T, ID, Error string }
	if json.Unmarshal(b, &f) != nil || f.T != "mark" {
		t.Fatalf("ack %s", b)
	}
	return f.ID, f.Error
}

func marksOf(frames []map[string]any) (out []map[string]any) {
	for _, f := range frames {
		if f["t"] == "world" && f["kind"] == "mark" {
			out = append(out, f)
		}
	}
	return out
}

// TestHostileMarks plays the hostile fixture's marks (relay-hostile.json; refToy stands in for unlanded toys): every
// hostile one is rejected or neutralised, the honest ones land with y recomputed and only sanitised fields public.
func TestHostileMarks(t *testing.T) {
	a, c := testApp(t)
	withToys(t, a, "cairns", "toss", "footprints", "toy-clouds")
	f, err := loadFixture("../www/dev/fixtures/relay-hostile.json")
	if err != nil {
		t.Fatal(err)
	}
	f.step.Store(1)
	acks := f.playMarks()
	if again := f.playMarks(); again != nil {
		t.Fatalf("a step's marks played twice: %v", again)
	}
	f.step.Store(2)
	acks = append(acks, f.playMarks()...)
	want := map[string]string{"c1": "", "t1": "", "f1": "", "k1": "", "forge": "", "low": "snowline", "far": "reach", "toy": "toy",
		"text": "", "anon": "auth", "view": "auth", "name": "", "spam1": "rate", "spam2": "rate"}
	got := map[string]string{}
	none := 0
	for _, b := range acks {
		if b == "(none)" { // the oversized and the malformed frame: dropped unanswered (on /ws the read limit closes first)
			none++
			continue
		}
		var x struct{ Ref, ID, Error string }
		json.Unmarshal([]byte(b), &x)
		if got[x.Ref] = x.Error; x.Error == "" && x.ID == "" {
			t.Errorf("ack without id or error: %s", b)
		}
	}
	for ref, w := range want {
		if g, ok := got[ref]; !ok || g != w {
			t.Errorf("mark %s: got %q (answered %v), want %q", ref, g, ok, w)
		}
	}
	if none != 2 {
		t.Errorf("%d frames unanswered, want 2 (oversized, malformed)", none)
	}
	frames := drain(c)
	ms := marksOf(frames)
	if len(ms) != 7 || len(a.State.Marks) != 7 {
		t.Fatalf("%d world:mark frames, %d in state; want 7", len(ms), len(a.State.Marks))
	}
	solid := worldcore.Solid(a.World.Base, a.Land)
	for _, m := range ms {
		pos := m["pose"].(map[string]any)["pos"].([]any)
		x, y, z := pos[0].(float64), pos[1].(float64), pos[2].(float64)
		i, j := gridCell(x), gridCell(z)
		if cx, cz := worldcore.CellXZ(i, j); cx != x || cz != z || y != float64(solid[j*worldcore.Grid+i])*worldcore.HeightQ || y <= a.Metrics.Snowline {
			t.Errorf("mark %v: not quantised, y not heightSolid, or under the snowline %v", m, a.Metrics.Snowline)
		}
		if s := string(mustJSON(m)); strings.Contains(s, "script") || strings.Contains(s, "10.233") || strings.Contains(s, "u-") {
			t.Errorf("leak in %s", s)
		}
	}
	bys := map[any]bool{}
	for _, m := range ms {
		bys[m["by"]] = true
	}
	if !bys["ana_k"] || !bys["bo-42"] || !bys["dee"] || bys["admin_root"] || len(bys) != 4 { // + nil: unnamed / denied handles
		t.Errorf("handles %v", bys)
	}
	validate(t, [][]map[string]any{frames})
}

// TestMarkLimits: 1 per minute and 20 per 24 h per account; another account is not limited by it.
func TestMarkLimits(t *testing.T) {
	a, _ := testApp(t)
	withToys(t, a, "test-toy")
	c, pose := visitor("u1", "ana"), map[string]any{"pos": []any{3.0, 0.0, 3.0}}
	t0 := time.Now()
	c.RoleExp = t0.Add(48 * time.Hour)
	for i := range 20 {
		if _, why := ackOf(t, a.place(c, "", "test-toy", pose, false, t0.Add(time.Duration(i)*time.Minute))); why != "" {
			t.Fatalf("mark %d: %s", i, why)
		}
	}
	if _, why := ackOf(t, a.place(c, "", "test-toy", pose, false, t0.Add(19*time.Minute+30*time.Second))); why != "rate" {
		t.Errorf("2 in a minute: %q", why)
	}
	var q struct{ Next, Left *int }
	json.Unmarshal(a.place(c, "", "test-toy", pose, false, t0.Add(19*time.Minute+30*time.Second)), &q)
	if q.Next == nil || q.Left == nil || *q.Left != 0 || *q.Next != int((24*time.Hour-19*time.Minute-30*time.Second)/time.Second) {
		t.Errorf("rate quota next %v left %v", q.Next, q.Left)
	}
	if _, why := ackOf(t, a.place(c, "", "test-toy", pose, false, t0.Add(time.Hour))); why != "rate" {
		t.Errorf("21st in a day: %q", why)
	}
	b := a.place(visitor("u2", ""), "", "test-toy", pose, false, t0.Add(time.Hour))
	if _, why := ackOf(t, b); why != "" {
		t.Errorf("another account: %q", why)
	}
	if json.Unmarshal(b, &q); q.Next == nil || *q.Next != 60 || *q.Left != 19 {
		t.Errorf("accept quota %s", b)
	}
	if _, why := ackOf(t, a.place(c, "", "test-toy", pose, false, t0.Add(24*time.Hour+time.Minute))); why != "" {
		t.Errorf("a day later: %q", why)
	}
	exp := visitor("u3", "")
	exp.RoleExp = t0.Add(-time.Second)
	if _, why := ackOf(t, a.place(exp, "", "test-toy", pose, false, t0)); why != "auth" {
		t.Errorf("lapsed session: %q", why)
	}
}

// TestMarkCap: hello.state.marks keeps the newest marksMax; gone removes.
func TestMarkCap(t *testing.T) {
	s := newState()
	for i := range marksMax + 5 {
		b := mustJSON(map[string]any{"t": "world", "kind": "mark", "id": ULID() + string(rune('a'+i%26)), "mark": "toss", "pose": map[string]any{}, "weight": 1})
		var f frameF
		json.Unmarshal(b, &f)
		s.apply(f, b)
	}
	if len(s.Marks) != marksMax {
		t.Fatalf("%d marks", len(s.Marks))
	}
	last := s.Marks[len(s.Marks)-1].ID
	b := mustJSON(map[string]any{"t": "world", "kind": "mark", "id": last, "mark": "toss", "pose": map[string]any{}, "weight": 0, "gone": true})
	var f frameF
	json.Unmarshal(b, &f)
	s.apply(f, b)
	if len(s.Marks) != marksMax-1 || s.Marks[len(s.Marks)-1].ID == last {
		t.Errorf("gone kept: %d", len(s.Marks))
	}
}

// TestMarkDelete: admin-only mark.delete (08, the W3 verification item carried to W4), also while visitors are off.
func TestMarkDelete(t *testing.T) {
	a, c := testApp(t)
	withToys(t, a, "test-toy")
	id, why := ackOf(t, a.place(visitor("u1", ""), "", "test-toy", map[string]any{"pos": []any{3.0, 0.0, 3.0}}, false, time.Now()))
	if why != "" {
		t.Fatal(why)
	}
	admin := &Client{Role: roleAdmin, RoleExp: time.Now().Add(time.Hour), Sub: "adm", send: make(chan []byte, 4)}
	if _, why := ackOf(t, a.unmark(visitor("u1", ""), "", id, time.Now())); why != "auth" {
		t.Errorf("visitor delete: %q", why)
	}
	if _, why := ackOf(t, a.unmark(admin, "", "nope", time.Now())); why != "unknown" {
		t.Errorf("unknown id: %q", why)
	}
	a.Metrics.Visitors = false
	drain(c)
	clientMsg(admin, mustJSON(map[string]any{"t": "mark.delete", "ref": "d1", "id": id}))
	if got, why := ackOf(t, <-admin.send); got != id || why != "" {
		t.Errorf("admin delete: %s %s", got, why)
	}
	ms := marksOf(drain(c))
	if len(ms) != 1 || ms[0]["gone"] != true || ms[0]["id"] != id || len(a.State.Marks) != 0 || a.MarkSubs[id] != "" {
		t.Errorf("gone frame %v, state %v", ms, a.State.Marks)
	}
}

// TestKillSwitch: VISITORS_ENABLED=false (the mounted file, re-read live) rejects marks and tells clients at once.
func TestKillSwitch(t *testing.T) {
	a, c := testApp(t)
	withToys(t, a, "test-toy")
	dir := t.TempDir()
	old := flagFile
	flagFile = filepath.Join(dir, "flags", "VISITORS_ENABLED")
	t.Cleanup(func() { flagFile = old })
	t.Setenv("VISITORS_ENABLED", "")
	if !visitorsFlag() {
		t.Fatal("no volume, no env: enabled")
	}
	t.Setenv("VISITORS_ENABLED", "false")
	if visitorsFlag() {
		t.Fatal("no volume, env false: disabled")
	}
	os.Mkdir(filepath.Dir(flagFile), 0o755)
	if !visitorsFlag() {
		t.Fatal("the volume without the key (deleted in Infisical): enabled, not the env bound at start")
	}
	os.WriteFile(flagFile, []byte("true\n"), 0o644)
	if !visitorsFlag() {
		t.Fatal("the file wins over the env")
	}
	once := func() { ctx, cancel := context.WithCancel(context.Background()); cancel(); a.watchVisitors(ctx) }
	os.WriteFile(flagFile, []byte("false\n"), 0o644)
	once()
	m := drain(c)
	if len(m) != 1 || m[0]["t"] != "metrics" || m[0]["visitors"] != false {
		t.Fatalf("frames %v", m)
	}
	pose := map[string]any{"pos": []any{3.0, 0.0, 3.0}}
	if _, why := ackOf(t, a.place(visitor("u1", ""), "", "test-toy", pose, false, time.Now())); why != "off" {
		t.Errorf("off: %q", why)
	}
	os.WriteFile(flagFile, []byte("1"), 0o644)
	once()
	if _, why := ackOf(t, a.place(visitor("u1", ""), "", "test-toy", pose, false, time.Now())); why != "" {
		t.Errorf("back on: %q", why)
	}
	validate(t, [][]map[string]any{m})
}

// TestToyGuards: a toy's panic or a pose sanitize refuses (text, NaN) rejects the mark; nothing is broadcast.
func TestToyGuards(t *testing.T) {
	a, c := testApp(t)
	withToys(t, a)
	for name, pose := range map[string]map[string]any{"panic": nil, "text": {"note": "hi"}, "nan": {"y": math.NaN()}, "big": {"n": make([]float64, 17)}} {
		toys["guard-"+name] = Toy{Validate: func(*Place) (map[string]any, float64, string) {
			if pose == nil {
				panic("lane bug")
			}
			return pose, 1, ""
		}}
		t.Cleanup(func() { delete(toys, "guard-"+name) })
		if _, why := ackOf(t, a.place(visitor("u-"+name, ""), "", "guard-"+name, map[string]any{}, false, time.Now())); why != "bad" {
			t.Errorf("%s: %q", name, why)
		}
	}
	toys["guard-reason"] = Toy{Validate: func(*Place) (map[string]any, float64, string) { return nil, 0, "Too Close 3 m" }}
	t.Cleanup(func() { delete(toys, "guard-reason") })
	if _, why := ackOf(t, a.place(visitor("u-r", ""), "", "guard-reason", map[string]any{}, false, time.Now())); why != "bad" {
		t.Errorf("free-text reason: %q", why)
	}
	if ms := marksOf(drain(c)); len(ms) != 0 {
		t.Errorf("broadcast %v", ms)
	}
}

func TestHandle(t *testing.T) {
	for in, want := range map[string]string{"Ana_K": "", "bo": "", "mountain": "", "operator9": "", "killian": "killian", "model_t": "model_t", "a-very-long-handle-of-21": "", "x y z": "", "SysAdmin": "", "<b>hi</b>": "", "dee-42": "dee-42"} {
		if got := handle(in); got != want {
			t.Errorf("handle(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestMarkOverWS: the session carries sub + username to the socket; a visitor's mark is acked and broadcast, an
// anonymous one is refused, and a frame over 4 KB closes the connection.
func TestMarkOverWS(t *testing.T) {
	p := newFakeIdP(t)
	auth = newAuth(p.URL, "mountain", "secret", origin+"/auth/callback", []byte(strings.Repeat("k", 32)), false)
	t.Cleanup(func() { auth = nil })
	vis := session(t, p, "mountain-visitors")
	var me struct{ Role, Handle string }
	json.NewDecoder(serveAuth("GET", "/auth/me", []*http.Cookie{vis}).Body).Decode(&me)
	if me.Role != "visitor" || me.Handle != "ana_k" {
		t.Errorf("/auth/me %+v", me)
	}
	a, _ := testApp(t)
	withToys(t, a, "test-toy")
	srv := httptest.NewServer(a.Hub)
	defer srv.Close()
	conn := func(c *http.Cookie) *websocket.Conn {
		h := http.Header{"Origin": {origin}}
		if c != nil {
			h.Set("Cookie", c.String())
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		t.Cleanup(cancel)
		w, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), &websocket.DialOptions{HTTPHeader: h})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { w.CloseNow() })
		w.SetReadLimit(-1)
		readT(t, w) // hello
		return w
	}
	v, anon := conn(vis), conn(nil)
	put := func(w *websocket.Conn, b []byte) error {
		return w.Write(context.Background(), websocket.MessageText, b)
	}
	put(v, mustJSON(map[string]any{"t": "mark", "ref": "a1", "mark": "test-toy", "pose": map[string]any{"pos": []float64{3, 0, 3}}, "named": true}))
	var mark, ack map[string]any
	for mark == nil || ack == nil {
		switch f := readT(t, v); f["t"] {
		case "world":
			mark = f
		case "mark":
			ack = f
		}
	}
	if ack["ref"] != "a1" || ack["id"] != mark["id"] || mark["by"] != "ana_k" || a.MarkSubs[mark["id"].(string)] != "u1" {
		t.Errorf("ack %v, mark %v", ack, mark)
	}
	put(anon, mustJSON(map[string]any{"t": "mark", "ref": "a2", "mark": "test-toy", "pose": map[string]any{"pos": []float64{3, 0, 3}}}))
	for f := readT(t, anon); f["t"] != "mark"; f = readT(t, anon) {
	}
	put(anon, []byte(`{"t":"mark","pad":"`+strings.Repeat("x", maxFrame)+`"}`))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		if _, _, err := anon.Read(ctx); err != nil {
			if websocket.CloseStatus(err) != websocket.StatusMessageTooBig {
				t.Errorf("oversized frame: %v", err)
			}
			break
		}
	}
}

// TestDBMarks: marks, their accounts and the rate limits survive a restart; the delete reaches the row.
func TestDBMarks(t *testing.T) {
	dsn := testDSN(t)
	a := storedApp(t, dsn)
	withToys(t, a, "test-toy")
	pose := map[string]any{"pos": []any{3.0, 0.0, 3.0}}
	id, why := ackOf(t, a.place(visitor("u1", "ana_k"), "", "test-toy", pose, true, time.Now()))
	id2, _ := ackOf(t, a.place(visitor("u2", ""), "", "test-toy", pose, false, time.Now()))
	if why != "" || id2 == "" {
		t.Fatal(why)
	}
	a.snapshot(time.Now())
	admin := &Client{Role: roleAdmin, RoleExp: time.Now().Add(time.Hour), Sub: "adm"}
	if _, why := ackOf(t, a.unmark(admin, "", id2, time.Now())); why != "" {
		t.Fatal(why)
	}
	a.store.Close(5 * time.Second)

	visits.Lock()
	visits.at = map[string][]time.Time{}
	visits.Unlock()
	b := storedApp(t, dsn)
	app = b
	if len(b.State.Marks) != 1 || b.State.Marks[0].ID != id || b.State.Marks[0].By != "ana_k" || b.MarkSubs[id] != "u1" {
		t.Fatalf("restored marks %+v subs %v", b.State.Marks, b.MarkSubs)
	}
	if _, why := ackOf(t, b.place(visitor("u1", ""), "", "test-toy", pose, false, time.Now())); why != "rate" {
		t.Errorf("rate after restart: %q", why)
	}
	var handle, deleted string
	b.store.pool.QueryRow(context.Background(), "SELECT m.handle, coalesce((SELECT 'yes' FROM marks WHERE id = $2 AND deleted_at IS NOT NULL), 'no') FROM marks m WHERE id = $1", id, id2).Scan(&handle, &deleted)
	if handle != "ana_k" || deleted != "yes" {
		t.Errorf("row handle %q, deleted %q", handle, deleted)
	}
	b.store.Close(5 * time.Second)
}
