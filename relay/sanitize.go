package main

// Public sanitisation (02 "Public allowlist"). Built by allowlist, never by deletion: every public
// value is either looked up in Config (apps, hosts) or checked against a closed per-kind field spec
// and copied into a NEW map. A label value never reaches the wire as-is unless it IS an allowlist key.

import (
	"math"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/MohammadBnei/mountain/relay/town"
)

var hosts = map[string]bool{"h1": true, "h2": true, "h3": true}

var unmappedSeen sync.Map

// unmapped counts each distinct unmapped value once (mountain_unmapped_total{kind}), not once per poll.
func unmapped(kind, v string) {
	if _, dup := unmappedSeen.LoadOrStore(kind+"\x00"+v, true); !dup {
		Unmapped.Add(kind, 1)
	}
}

// appOfNS maps a namespace (+ workload name, matched as "<ns>/<deployment>" key or pod-name prefix)
// to an allowlisted app; "" = other. Not counted as unmapped: pod/PVC namespaces are mostly infra
// (kube-system, monitoring, ...); the rot guard counts what can be public instead: Traefik routers
// (appOfService) and Argo apps (appOfArgo) outside every namespace config.yaml knows.
func (c *Config) appOfNS(ns, workload string) string {
	for k, app := range c.Namespaces {
		if dep, ok := strings.CutPrefix(k, ns+"/"); ok && (workload == dep || strings.HasPrefix(workload, dep+"-")) {
			return app
		}
	}
	return c.Namespaces[ns]
}

// appOfArgo maps an ArgoCD Application name (argocd_app_sync_total has no dest_namespace, 02 gotchas).
// destNS (argocd_app_info's dest_namespace, "" when unknown) decides rot: an app deploying into a
// namespace config.yaml knows (public or not) is mapped on purpose; only an unknown one counts.
func (c *Config) appOfArgo(name, destNS string) string {
	if _, ok := c.Apps[name]; ok {
		return name
	}
	if app, ok := c.Namespaces[name]; ok {
		return app
	}
	if _, known := c.Namespaces[destNS]; destNS != "" && !known {
		unmapped("app", name)
	}
	return ""
}

// appOfService maps Traefik exported_service "<ns>-<route>-<hash>@kubernetescrd" (02 gotchas) by
// the longest "<ns>-" or "<ns>-<deployment>-" prefix (namespaces contain dashes, so a split cannot work).
func (c *Config) appOfService(svc string) string {
	s, provider, _ := strings.Cut(svc, "@")
	best := ""
	for k := range c.Namespaces {
		if strings.HasPrefix(s, strings.Replace(k, "/", "-", 1)+"-") && len(k) > len(best) {
			best = k
		}
	}
	if best == "" {
		if provider != "internal" { // Traefik's own api/dashboard/ping
			unmapped("ns", svc)
		}
		return ""
	}
	return c.Namespaces[best]
}

// host maps a Kubernetes node (or probe target) to h1..h3; "" = unmapped.
func (c *Config) host(node string) string {
	if h := c.Nodes[node]; hosts[h] {
		return h
	}
	if node != "" { // "": a PVC no pod mounts has no node, nothing to map
		unmapped("node", node)
	}
	return ""
}

// hostQuiet is host for an alert's node label: unmapped values are not counted (most alerts have no node).
func (c *Config) hostQuiet(node string) string {
	if h := c.Nodes[node]; hosts[h] {
		return h
	}
	return ""
}

// Field types of the closed spec.
const (
	fApp    = iota // allowlisted app key
	fHost          // h1..h3
	fHosts         // []string of h1..h3
	fBool          //
	fNat           // int >= 0
	fBucket        // int 0..8
	fHealth        // ArgoCD health enum
	fNum           // float64
	fGroup         // platform|user|service
	fULID          //
	fFP            // 16 hex (salted alert fingerprint hash)
	fSev           // info|warning|critical
	fLayer         // bedrock|k8s|apps
	fPos           // int >= 1
	fDay           // YYYY-MM-DD
	fTitle         // string <= 120 matching a config.yaml titles pattern (anchored)
	fBuild         // queued|running|success|failure
	fToy           // a registered toy name (validators.go RegisterToy)
	fPose          // a toy's sanitised pose: ≤ 16 keys [a-z][A-Za-z0-9]{0,15} -> finite number | bool | ≤ 16 finite numbers (no text)
	fHandle        // a moderated handle (handle(), validators.go) or "a visitor"
	fHexC          // int in [-town.Radius, town.Radius]: a local hex coordinate
	fKind          // a town building kind (town.Kinds)
)

// spec: frame kind -> field -> type. Every field is required, except a "name?" field, which is left out when
// missing or invalid (an ADR title off the allowlist: date only, 02); nothing else passes.
var spec = map[string]map[string]int{
	"ev:argo.sync":     {"app": fApp, "ok": fBool},
	"ev:argo.health":   {"app": fApp, "status": fHealth},
	"ev:pod.churn":     {"app": fApp, "restarts": fNat, "oom": fBool, "crashloop": fBool},
	"ev:node.ready":    {"host": fHost, "ready": fBool},
	"ev:quorum":        {"up": fHosts},
	"ev:replicas":      {"app": fApp, "bucket": fBucket},
	"ev:app.gone":      {"app": fApp},
	"ev:alert.fire":    {"fp": fFP, "sev": fSev, "layer": fLayer, "startsAt": fNat, "host?": fHost, "app?": fApp},
	"ev:alert.resolve": {"fp": fFP, "sev": fSev, "layer": fLayer, "durS": fNat, "host?": fHost, "app?": fApp},
	// the src_github events (W3c)
	"ev:build":         {"app": fApp, "state": fBuild},
	"ev:gitops.commit": {"n": fPos},
	"ev:adr.merge":     {"day": fDay, "title?": fTitle},
	// W4 seam: the visitor layer (validators.go, toy_<name>.go). by = the opt-in handle; gone = removed (cap, admin)
	"world:mark": {"id": fULID, "mark": fToy, "pose": fPose, "weight": fNum, "by?": fHandle, "gone?": fBool},
	// R7: town plots (plots.go). ruinAt = app.gone time (unix ms), gone = the plot is freed
	"world:plot": {"app": fApp, "host": fHost, "q": fHexC, "r": fHexC, "role": fKind, "ruinAt?": fNat, "gone?": fBool},
}

var (
	ulidRE   = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{26}$`)
	fpRE     = regexp.MustCompile(`^[0-9a-f]{16}$`)
	healthOK = map[string]bool{"Healthy": true, "Progressing": true, "Degraded": true, "Suspended": true, "Missing": true, "Unknown": true}
)

// clean returns a fresh map holding exactly the spec fields of kind, or false if any is missing or invalid.
func (c *Config) clean(kind string, in map[string]any) (map[string]any, bool) {
	sp, ok := spec[kind]
	if !ok {
		return nil, false
	}
	out := make(map[string]any, len(sp))
	for k, typ := range sp {
		k, opt := strings.CutSuffix(k, "?")
		v, ok := in[k]
		if ok = ok && c.valid(typ, v); ok {
			out[k] = v
		} else if !opt {
			return nil, false
		}
	}
	return out, true
}

func (c *Config) valid(typ int, v any) bool {
	switch typ {
	case fApp:
		s, ok := v.(string)
		_, in := c.Apps[s]
		return ok && in
	case fHost:
		s, ok := v.(string)
		return ok && hosts[s]
	case fHosts:
		l, ok := v.([]string)
		for _, s := range l {
			ok = ok && hosts[s]
		}
		return ok && len(l) <= 3
	case fBool:
		_, ok := v.(bool)
		return ok
	case fNat:
		n, ok := v.(int)
		return ok && n >= 0
	case fBucket:
		n, ok := v.(int)
		return ok && n >= 0 && n <= 8
	case fHealth:
		s, ok := v.(string)
		return ok && healthOK[s]
	case fNum:
		if _, ok := v.(int); ok { // a whole number from a decoded fixture (natural): still a number
			return true
		}
		f, ok := v.(float64)
		return ok && finite(f)
	case fGroup:
		return v == "platform" || v == "user" || v == "service"
	case fULID:
		s, ok := v.(string)
		return ok && ulidRE.MatchString(s)
	case fFP:
		s, ok := v.(string)
		return ok && fpRE.MatchString(s)
	case fSev:
		return v == "info" || v == "warning" || v == "critical"
	case fLayer:
		return v == "bedrock" || v == "k8s" || v == "apps"
	case fPos:
		n, ok := v.(int)
		return ok && n >= 1
	case fDay:
		s, ok := v.(string)
		return ok && dayRE.MatchString(s)
	case fTitle:
		s, ok := v.(string)
		return ok && len(s) <= 120 && slices.ContainsFunc(c.titles, func(re *regexp.Regexp) bool { return re.MatchString(s) })
	case fBuild:
		return v == "queued" || v == "running" || v == "success" || v == "failure"
	case fToy:
		s, ok := v.(string)
		_, in := toys[s]
		return ok && in
	case fPose:
		m, ok := v.(map[string]any)
		if !ok || len(m) > 16 {
			return false
		}
		for k, x := range m {
			if !poseKeyRE.MatchString(k) || !poseValue(x) {
				return false
			}
		}
		return true
	case fHandle:
		s, ok := v.(string)
		return ok && (s == anonHandle || handle(s) == s)
	case fHexC:
		n, ok := v.(int)
		return ok && n >= -town.Radius && n <= town.Radius
	case fKind:
		s, ok := v.(string)
		return ok && slices.Contains(town.Kinds, s)
	}
	return false
}

var poseKeyRE = regexp.MustCompile(`^[a-z][A-Za-z0-9]{0,15}$`)

func poseValue(x any) bool {
	switch x := x.(type) {
	case bool, int:
		return true
	case float64:
		return finite(x)
	case []float64:
		return len(x) <= 16 && finite(x...)
	}
	return false
}

// finite: json.Marshal fails on NaN and ±Inf (mustJSON would panic), and no pose needs them.
func finite(fs ...float64) bool {
	for _, f := range fs {
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return false
		}
	}
	return true
}
