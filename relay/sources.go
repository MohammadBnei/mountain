package main

// Source registry (frozen W2) and the frame types sources emit.
//
// A source is one file (src_prom.go, src_alerts.go, src_github.go) that registers itself:
//
//	func init() { Register("prom", func(a *App) (Source, error) { return &promSource{url: a.Opts.PromURL}, nil }) }
//
// W1b lane A: src_alerts.go registers "alerts" the same way and polls a.Opts.AMURL (/api/v2/alerts; the fixture
// hook serves it too). Alert.FP = a.FP(fingerprint). It reports firing alerts through Out.Fire / Out.Resolve (hello.state.alerts + ev, atomically),
// liveness through Out.OK, failures through Out.Stale (metrics stale[name]); it clears stale[name] on recovery.
//
// and emits through *Out. Frames must validate against www/dev/schema/*.json.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"slices"
	"strings"
	"time"
)

// Source is one upstream (Prometheus, Alertmanager, GitHub).
type Source interface {
	Name() string
	Run(ctx context.Context, out *Out) error // returns on error; restarted after 5 s
}

var registry = map[string]func(*App) (Source, error){}

// Register adds a source constructor; call it from init().
func Register(name string, ctor func(*App) (Source, error)) { registry[name] = ctor }

// FixtureHook serves a scripted Prometheus/Alertmanager timeline in-process for
// --source=fixture:<file> and returns the URLs the sources should poll (lane A sets it from init()).
var FixtureHook func(ctx context.Context, file string) (promURL, amURL string, err error)

func startSources(ctx context.Context, a *App) error {
	if file, ok := strings.CutPrefix(a.Opts.Source, "fixture:"); ok {
		if FixtureHook == nil {
			log.Printf("sources: no fixture support compiled in, running none")
			return nil
		}
		p, am, err := FixtureHook(ctx, file)
		if err != nil {
			return err
		}
		a.Opts.PromURL, a.Opts.AMURL = p, am
	}
	for name, ctor := range registry {
		s, err := ctor(a)
		if err != nil {
			return err
		}
		out := &Out{app: a, name: name}
		// seeded at start: a source that never succeeds (e.g. --prom-url=http://invalid) still ages into MountainSourceStale
		SourceLastSuccess.Set(name, float64(time.Now().Unix()))
		go func() {
			for ctx.Err() == nil {
				if err := s.Run(ctx, out); err != nil && ctx.Err() == nil {
					log.Printf("source %s: %v", name, err)
				}
				select {
				case <-ctx.Done():
				case <-time.After(5 * time.Second):
				}
			}
		}()
	}
	return nil
}

// Out is a source's only way to the world: events, world deltas, metrics, liveness.
type Out struct {
	app  *App
	name string
}

// Ev sanitises fields (Cfg.clean: the closed per-kind spec, copied into a fresh map), broadcasts
// {t:"ev", id, ts, kind, ...fields} (02 event enum) and returns its id ("" if dropped).
func (o *Out) Ev(kind string, fields map[string]any) string {
	id, b := o.evFrame(kind, fields)
	o.app.Hub.Broadcast(b)
	return id
}

// World sanitises and broadcasts {t:"world", kind, ...payload} (kind = deposit|vein|glacier|ash|site|mark|cloud|rain).
func (o *Out) World(kind string, payload map[string]any) {
	o.app.Hub.Broadcast(o.worldFrame(kind, payload))
}

// frame cleans fields for "<t>:<kind>" and stamps t/kind last (a field can't overwrite them); nil if dropped.
func (o *Out) frame(t, kind string, fields map[string]any) map[string]any {
	f, ok := o.app.Cfg.clean(t+":"+kind, fields)
	if !ok {
		log.Printf("sanitize: dropped %s %s", t, kind)
		return nil
	}
	f["t"], f["kind"] = t, kind
	return f
}

func (o *Out) evFrame(kind string, fields map[string]any) (string, []byte) {
	f := o.frame("ev", kind, fields)
	if f == nil {
		return "", nil
	}
	id := ULID()
	f["id"], f["ts"] = id, time.Now().UnixMilli()
	return id, mustJSON(f)
}

func (o *Out) worldFrame(kind string, payload map[string]any) []byte {
	if f := o.frame("world", kind, payload); f != nil {
		return mustJSON(f)
	}
	return nil
}

// FP is Alert.FP for an Alertmanager fingerprint: the first 16 hex of sha256(salt + fingerprint).
// "" without a salt, which sanitize drops (fail closed: an unsalted fp would link across resets).
func (a *App) FP(amFingerprint string) string {
	if a.Opts.FPSalt == "" {
		return ""
	}
	h := sha256.Sum256([]byte(a.Opts.FPSalt + amFingerprint))
	return hex.EncodeToString(h[:8])
}

// Alert is one firing alert in hello.state.alerts (02 alerts row; persisted through the alert.fire/resolve events and snapshots).
type Alert struct {
	FP       string `json:"fp"`       // salted hash, 16 hex (never the raw fingerprint)
	Sev      string `json:"sev"`      // info | warning | critical
	Layer    string `json:"layer"`    // bedrock | k8s | apps (config.yaml alertLayers)
	StartsAt int64  `json:"startsAt"` // unix ms, Alertmanager startsAt
}

// State is hello.state (www/dev/schema/hello.json). ponytail: alerts only; health, replicas, nodes, quorum and
// builds join it when a model needs them after a reload (they are re-derived within one poll today).
type State struct {
	Alerts []Alert `json:"alerts"`
}

// Fire adds a to hello.state.alerts (replacing the same fp) and broadcasts ev alert.fire{fp, sev, layer, startsAt}.
// startsAt rides the event so a live client and a reloaded one agree (cauldron breakMin), even after a relay restart re-fires.
// Hub lock held across both: a connecting client sees the alert in its hello or as the event, never neither.
// Returns the ev id ("" if sanitize dropped it; state is then unchanged).
func (o *Out) Fire(a Alert) string {
	return o.stateEv("alert.fire", map[string]any{"fp": a.FP, "sev": a.Sev, "layer": a.Layer, "startsAt": int(a.StartsAt)}, func(s *State) { s.set(a) })
}

// Resolve removes a from hello.state.alerts and broadcasts ev alert.resolve{fp, sev, layer, durS}; same lock rule.
func (o *Out) Resolve(a Alert, durS int) string {
	return o.stateEv("alert.resolve", map[string]any{"fp": a.FP, "sev": a.Sev, "layer": a.Layer, "durS": durS}, func(s *State) { s.drop(a.FP) })
}

// set adds a, replacing the same fp; drop removes fp. Shared by Fire/Resolve and the DB restore (store.go).
func (s *State) set(a Alert) {
	s.Alerts = append(slices.DeleteFunc(s.Alerts, func(x Alert) bool { return x.FP == a.FP }), a)
}
func (s *State) drop(fp string) {
	s.Alerts = slices.DeleteFunc(s.Alerts, func(x Alert) bool { return x.FP == fp })
}

func (o *Out) stateEv(kind string, fields map[string]any, mut func(*State)) string {
	id, b := o.evFrame(kind, fields)
	if b == nil {
		return ""
	}
	h := o.app.Hub
	h.mu.Lock() // lock order h.mu -> app.mu, as Hello
	defer h.mu.Unlock()
	o.app.mu.Lock()
	mut(&o.app.State)
	o.app.mu.Unlock()
	h.broadcastLocked(b)
	return id
}

// Stale marks this source stale in the metrics frame (stale[name] = since, unix ms; kept while it stays down)
// and broadcasts it. A source clears it with Metrics(func(m) { delete(m.Stale, name) }) on its next good poll.
func (o *Out) Stale() {
	o.Metrics(func(m *Metrics) {
		if _, ok := m.Stale[o.name]; !ok {
			m.Stale[o.name] = time.Now().UnixMilli()
		}
	})
}

// Metrics mutates the current metrics frame under the app lock, then broadcasts it.
func (o *Out) Metrics(update func(*Metrics)) {
	o.app.mu.Lock()
	update(&o.app.Metrics)
	b := mustJSON(o.app.Metrics)
	o.app.mu.Unlock()
	o.app.Hub.Broadcast(b)
}

// OK records a successful poll (mountain_source_last_success{source}).
func (o *Out) OK() { SourceLastSuccess.Set(o.name, float64(time.Now().Unix())) }

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// Metrics is the metrics frame (02), every 15 s; equals world.metrics on the client.
type Metrics struct {
	T              string                `json:"t"` // "metrics"
	Apps           map[string]AppMetrics `json:"apps"`
	Storage        Storage               `json:"storage"`
	CPU            float64               `json:"cpu"`
	Mem            float64               `json:"mem"`
	Heat           float64               `json:"heat"`
	BuildsInFlight int                   `json:"buildsInFlight"`
	Wind           XZ                    `json:"wind"`
	Snowline       float64               `json:"snowline"` // world units
	Rx             float64               `json:"rx"`
	River          float64               `json:"river"`
	Hosts          map[string]Host       `json:"hosts"`
	Stale          map[string]int64      `json:"stale"` // source -> since (unix ms)
}

// AppMetrics values are bucketed (B suffix) before they leave the relay.
type AppMetrics struct {
	RpsB   int  `json:"rpsB"`
	ErrB   int  `json:"errB"`
	Spike  bool `json:"spike"`
	BytesB int  `json:"bytesB"`
}

// XZ is a horizontal vector in world units.
type XZ struct {
	X float64 `json:"x"`
	Z float64 `json:"z"`
}

type Storage struct {
	Pools  []Pool `json:"pools"`
	Garage any    `json:"garage"` // always null: unmeasured
	Pigsty any    `json:"pigsty"` // always null: unmeasured
}

type Pool struct {
	ID    string `json:"id"`
	App   string `json:"app"`   // allowlisted app or "other"
	Class string `json:"class"` // longhorn | local
	Host  string `json:"host,omitempty"`
	CapB  int    `json:"capB"`
	UsedB *int   `json:"usedB"` // null = unmeasured
}

type Host struct {
	Alloc float64 `json:"alloc"`
	Ready bool    `json:"ready"`
	Load  float64 `json:"load"`
}

func emptyMetrics(snowline float64) Metrics {
	return Metrics{T: "metrics", Apps: map[string]AppMetrics{}, Storage: Storage{Pools: []Pool{}}, Snowline: snowline,
		Hosts: map[string]Host{"h1": {}, "h2": {}, "h3": {}}, Stale: map[string]int64{}}
}

// ULID returns a 26-char Crockford ULID (48-bit ms time + 80 random bits).
func ULID() string {
	const enc = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	var b [16]byte
	ms := uint64(time.Now().UnixMilli())
	for i := 0; i < 6; i++ {
		b[i] = byte(ms >> (40 - 8*i))
	}
	rand.Read(b[6:])
	out := make([]byte, 26)
	// 128 bits -> 26 base32 chars, most significant first (first char holds 3 bits)
	var hi, lo uint64
	for i := 0; i < 8; i++ {
		hi = hi<<8 | uint64(b[i])
		lo = lo<<8 | uint64(b[i+8])
	}
	for i := 25; i >= 0; i-- {
		out[i] = enc[lo&31]
		lo = lo>>5 | hi<<59
		hi >>= 5
	}
	return string(out)
}
