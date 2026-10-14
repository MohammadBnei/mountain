package main

// Public sanitisation (02 "Public allowlist"). Built by allowlist, never by deletion: every public
// value is either looked up in Config (apps, hosts) or checked against a closed per-kind field spec
// and copied into a NEW map. A label value never reaches the wire as-is unless it IS an allowlist key.

import (
	"regexp"
	"strings"
	"sync"
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
// to an allowlisted app; "" = other. Unknown namespaces count as unmapped.
func (c *Config) appOfNS(ns, workload string) string {
	for k, app := range c.Namespaces {
		if dep, ok := strings.CutPrefix(k, ns+"/"); ok && (workload == dep || strings.HasPrefix(workload, dep+"-")) {
			return app
		}
	}
	app, ok := c.Namespaces[ns]
	if !ok {
		unmapped("ns", ns)
	}
	return app
}

// appOfArgo maps an ArgoCD Application name (argocd_app_* have no dest_namespace, 02 gotchas).
func (c *Config) appOfArgo(name string) string {
	if _, ok := c.Apps[name]; ok {
		return name
	}
	if app, ok := c.Namespaces[name]; ok {
		return app
	}
	unmapped("app", name)
	return ""
}

// appOfService maps Traefik exported_service "<ns>-<route>-<hash>@kubernetescrd" (02 gotchas) by
// the longest "<ns>-" or "<ns>-<deployment>-" prefix (namespaces contain dashes, so a split cannot work).
func (c *Config) appOfService(svc string) string {
	s, _, _ := strings.Cut(svc, "@")
	best := ""
	for k := range c.Namespaces {
		if strings.HasPrefix(s, strings.Replace(k, "/", "-", 1)+"-") && len(k) > len(best) {
			best = k
		}
	}
	if best == "" {
		unmapped("ns", svc)
		return ""
	}
	return c.Namespaces[best]
}

// host maps a Kubernetes node (or probe target) to h1..h3; "" = unmapped.
func (c *Config) host(node string) string {
	if h := c.Nodes[node]; hosts[h] {
		return h
	}
	unmapped("node", node)
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
	fGroup         // platform|user
	fULID          //
	fB64           // base64 grid
)

// spec: frame kind -> field -> type. Every field is required; nothing else passes.
var spec = map[string]map[string]int{
	"ev:argo.sync":   {"app": fApp, "ok": fBool},
	"ev:argo.health": {"app": fApp, "status": fHealth},
	"ev:pod.churn":   {"app": fApp, "restarts": fNat, "oom": fBool, "crashloop": fBool},
	"ev:node.ready":  {"host": fHost, "ready": fBool},
	"ev:quorum":      {"up": fHosts},
	"ev:replicas":    {"app": fApp, "bucket": fBucket},
	"ev:app.gone":    {"app": fApp},
	"world:deposit":  {"id": fULID, "x0": fNat, "z0": fNat, "w": fNat, "h": fNat, "cells": fB64},
	"world:site":     {"app": fApp, "x": fNum, "z": fNum, "y": fNum, "tier": fNat, "group": fGroup},
}

var (
	ulidRE   = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{26}$`)
	b64RE    = regexp.MustCompile(`^[A-Za-z0-9+/]*=*$`)
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
		v, ok := in[k]
		if !ok || !c.valid(typ, v) {
			return nil, false
		}
		out[k] = v
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
		_, ok := v.(float64)
		return ok
	case fGroup:
		return v == "platform" || v == "user"
	case fULID:
		s, ok := v.(string)
		return ok && ulidRE.MatchString(s)
	case fB64:
		s, ok := v.(string)
		return ok && b64RE.MatchString(s)
	}
	return false
}
