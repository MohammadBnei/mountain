package contract

import (
	"fmt"
	"strconv"
)

// The live frame atlas sends on /ws (plan/10 "Read side"): in the hello as "live", and as {t:"live"} whenever it
// changes. ukubi-game's town reads it as atlas's page does, so its shape lives here. Absent means unknown throughout:
// a source that never answered is not a zero.

// Live is the live frame.
type Live struct {
	T        string             `json:"t"`                  // "live"
	At       int64              `json:"at"`                 // unix ms of the last good Prometheus answer (0: none yet)
	AlertsAt int64              `json:"alertsAt,omitempty"` // unix ms of the last good Alertmanager answer (0: none yet, so Alerts is no answer)
	Apps     map[string]AppLive `json:"apps"`
	Ready    map[string]bool    `json:"ready"`  // vm node id -> its k8s node is Ready
	Probes   map[string]bool    `json:"probes"` // node id -> its blackbox probe succeeds
	Land     Land               `json:"land"`
	Alerts   []AlertLive        `json:"alerts"`
	Stale    map[string]int64   `json:"stale"` // source -> failing since (unix ms); its last answer is kept
	// Health is each measured node's state, by Classify: atlas's page and the game's town draw the same one.
	Health map[string]Health `json:"health"`
	// signals: absent = unknown
	ProbeDay map[string]float64 `json:"probeDay,omitempty"` // probed node id -> share of its probes that succeeded, last 24 h
	Pods     map[string]int     `json:"pods,omitempty"`     // vm node id -> pods on its k8s node
	Addons   map[string]float64 `json:"addons,omitempty"`   // addon node id -> its least ready workload's share of ready pods
	Day      *Day               `json:"day,omitempty"`      // the town's last 24 h, when both its signals are known
}

// Day is the town's last 24 h (rolling): stability's calm day (plan/13 §3, owner 2026-10-26: calm days for the town,
// availability per app).
type Day struct {
	Critical []string `json:"critical"` // critical alerts that fired (Prometheus-evaluated: Grafana's own rules don't reach here)
	Degraded []string `json:"degraded"` // apps that were Degraded at any time
	Calm     bool     `json:"calm"`     // neither
}

type AppLive struct {
	Health string  `json:"health,omitempty"`
	Sync   string  `json:"sync,omitempty"`
	RPS    float64 `json:"rps"`
	// signals, summed over the app's Traefik services (latency: the slowest service's p95); nil = unknown
	Err    *float64 `json:"err,omitempty"`    // 5xx per second
	Lat    *float64 `json:"lat,omitempty"`    // p95 response time, seconds
	ReqDay *float64 `json:"reqDay,omitempty"` // requests in the last 24 h
	Avail  *float64 `json:"avail,omitempty"`  // share of those that were not 5xx (unknown with no requests)
}

// Use is what a namespace (or the cluster's capacity) holds: CPU cores, bytes of RAM, bytes of disk.
type Use struct {
	CPU  float64 `json:"cpu"`
	Mem  float64 `json:"mem"`
	Disk float64 `json:"disk"`
}

type Land struct {
	Capacity Use                 `json:"capacity"` // allocatable CPU and RAM, provisioned volume capacity
	Realms   map[string]Use      `json:"realms"`   // realm (or "unattributed") -> share of Capacity, 0..1 per field
	NS       map[string]Use      `json:"ns"`       // namespace -> use, absolute
	NSRealm  map[string]string   `json:"nsRealm"`  // namespace -> the realm its pods count toward
	Nodes    map[string]NodeLand `json:"nodes"`    // k8s node (vm id) -> what runs on it: a city's worked tiles
}

// NodeLand is one k8s node's allocatable CPU and RAM and what each namespace uses of it (disk is not per node:
// volumes are replicated).
type NodeLand struct {
	Capacity Use            `json:"capacity"`
	NS       map[string]Use `json:"ns"`
}

type AlertLive struct {
	FP       string            `json:"fp"`
	Name     string            `json:"name"`
	Severity string            `json:"severity"`
	StartsAt int64             `json:"startsAt"`
	Summary  string            `json:"summary,omitempty"`
	Node     string            `json:"node,omitempty"`  // the node its labels name
	Realm    string            `json:"realm,omitempty"` // that node's realm, or its namespace's
	Labels   map[string]string `json:"labels"`
}

// Event is one change worth flying to: {t:"ev"}.
type Event struct {
	T    string `json:"t"` // "ev"
	ID   int64  `json:"id"`
	TS   int64  `json:"ts"`
	Kind string `json:"kind"` // alert.fire | alert.resolve | health | ready | probe | graph
	Node string `json:"node,omitempty"`
	Text string `json:"text"`
}

// A node's state. Unknown is never drawn as down (00 R7.1: unknown ≠ down).
const (
	Down     = "down"
	Degraded = "degraded"
	OK       = "ok"
	Unknown  = "unknown"
)

// Health is a node's state and why, in words.
type Health struct {
	State string `json:"state"`
	Why   string `json:"why"`
}

// Topology is what Classify needs of the graph: each node's kind, and the runs-on edges (vm|ct -> host, db -> vm).
type Topology struct {
	Kind   map[string]string
	RunsOn map[string][]string // from -> to
}

// AddonHold is how long an add-on stays below all ready before it reads degraded (ms): a DaemonSet rollout takes
// its pods down one node at a time, and is not damage.
const AddonHold = 2 * 60 * 1000

// Classify is each measured node's health from a frame (plan/13 §3 Building health; atlas's page, R8):
//   - app: ArgoCD's health (Degraded degraded, Missing down, Progressing and Suspended ok, Unknown or absent unknown);
//   - a probed node: down when its probe fails; a k8s node (vm): down when not Ready;
//   - host: degraded when some of the k8s nodes on it are not Ready (its own probe, when it has one, comes first);
//   - db: by Majority over the VMs it runs on (each by its probe, else its Ready);
//   - add-on: degraded once below all ready for AddonHold (lowSince: since when, unix ms), unknown when its
//     workloads aren't answered (kube-vip, a static pod, never is).
//
// Only measurable kinds and measured nodes get an entry; anything else is unknown: "nothing live measures it". With
// no Prometheus answer yet (At 0) every entry is unknown.
func Classify(t Topology, f *Live, lowSince map[string]int64, now int64) map[string]Health {
	out := map[string]Health{}
	if f == nil {
		return out
	}
	under := map[string][]string{} // host -> what runs on it
	for from, tos := range t.RunsOn {
		for _, to := range tos {
			under[to] = append(under[to], from)
		}
	}
	node := func(id string) (Health, bool) {
		if up, ok := f.Probes[id]; ok {
			if up {
				return Health{OK, "probe answers"}, true
			}
			return Health{Down, "probe not answering"}, true
		}
		if up, ok := f.Ready[id]; ok {
			if up {
				return Health{OK, "node Ready"}, true
			}
			return Health{Down, "node not Ready"}, true
		}
		return Health{}, false
	}
	for id, kind := range t.Kind {
		if f.At == 0 {
			if measurable(kind) {
				out[id] = Health{Unknown, "no live data yet"}
			}
			continue
		}
		if h, ok := node(id); ok && kind != "app" {
			out[id] = h
			continue
		}
		switch kind {
		case "app":
			a, ok := f.Apps[id]
			if !ok || a.Health == "" {
				out[id] = Health{Unknown, "not in ArgoCD's answer"}
				continue
			}
			why := a.Health
			if a.Sync != "" {
				why += ", " + a.Sync
			}
			st := map[string]string{"Degraded": Degraded, "Missing": Down, "Unknown": Unknown, "Progressing": OK, "Suspended": OK, "Healthy": OK}[a.Health]
			if st == "" {
				st = Unknown
			}
			out[id] = Health{st, why}
		case "host":
			var vms, down int
			for _, vm := range under[id] {
				if up, ok := f.Ready[vm]; ok {
					vms++
					if !up {
						down++
					}
				}
			}
			switch {
			case vms == 0:
				out[id] = Health{Unknown, "no k8s node on it"}
			case down > 0:
				out[id] = Health{Degraded, fmt.Sprintf("%d of %d k8s nodes not Ready", down, vms)}
			default:
				out[id] = Health{OK, fmt.Sprintf("%d k8s nodes Ready", vms)}
			}
		case "db":
			var st []string
			for _, vm := range t.RunsOn[id] {
				h, ok := node(vm)
				if !ok {
					h.State = Unknown
				}
				st = append(st, h.State)
			}
			if len(st) == 0 {
				out[id] = Health{Unknown, "no member it runs on"}
				continue
			}
			up, down := count(st, OK), count(st, Down)
			out[id] = Health{Majority(st), fmt.Sprintf("%d of %d members answer, %d don't", up, len(st), down)}
		case "addon":
			share, ok := f.Addons[id]
			pct := strconv.Itoa(int(share*100)) + "% ready"
			switch since, low := lowSince[id]; {
			case !ok:
				out[id] = Health{Unknown, "its workloads aren't answered"}
			case share >= 1:
				out[id] = Health{OK, "all ready"}
			case low && now-since >= AddonHold:
				out[id] = Health{Degraded, pct + ", for over 2 minutes"}
			default:
				out[id] = Health{OK, pct + ", rolling"}
			}
		case "vm", "ct":
			out[id] = Health{Unknown, "no probe and no k8s node answer for it"}
		}
	}
	return out
}

func measurable(kind string) bool {
	switch kind {
	case "app", "vm", "ct", "host", "db", "addon":
		return true
	}
	return false
}

func count(st []string, s string) int {
	n := 0
	for _, x := range st {
		if x == s {
			n++
		}
	}
	return n
}

// Majority is a quorum's state from its members' (plan/13 S2b2: Postgres is down when a majority of the VMs it runs
// on are, as etcd loses its quorum and Patroni demotes the primary): down when more than half are down, ok when more
// than half are ok, otherwise unknown (1 down, 1 up and 1 unknown is neither). No members: unknown.
func Majority(states []string) string {
	switch n := len(states); {
	case n == 0:
		return Unknown
	case 2*count(states, Down) > n:
		return Down
	case 2*count(states, OK) > n:
		return OK
	}
	return Unknown
}
