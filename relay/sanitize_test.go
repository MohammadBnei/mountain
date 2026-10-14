package main

import "testing"

// TestSanitizeCorpus: hostile label values (IPs, namespaces, hostnames, versions, non-allowlisted
// apps) never come out of the lookups, and clean() passes only the closed per-kind field set.
func TestSanitizeCorpus(t *testing.T) {
	cfg, err := loadConfig(configYAML)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ ns, workload, want string }{
		{"wird", "wird-7d9f8c-abcde", "wird"},
		{"wird", "wird-adminweb-6c9-qwert", ""}, // ns/deployment key wins: not public
		{"vos", "vos-monolith-1", "vos-monolith"},
		{"vos-dev", "x", "vos-monolith-dev"},
		{"has-journal", "has-journal-0", ""},
		{"kube-system", "coredns-1", ""},
		{"agent-fleet", "agent-fleet-core-0", ""},
		{"10.0.3.17", "", ""},
		{"k8s-worker-01.bnei.lan", "", ""},
		{"wird ", "", ""},
	} {
		if got := cfg.appOfNS(c.ns, c.workload); got != c.want {
			t.Errorf("appOfNS(%q, %q) = %q, want %q", c.ns, c.workload, got, c.want)
		}
	}
	for in, want := range map[string]string{
		"wird": "wird", "vos-monolith": "vos-monolith", "has-journal": "", "hq": "", "agent-fleet-core": "",
		"10.0.3.17": "", "wird-adminweb": "", "v1.2.3": "", "platform-prometheus": "", "WIRD": "",
	} {
		if got := cfg.appOfArgo(in); got != want {
			t.Errorf("appOfArgo(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{
		"wird-wird-865ce724ee1dd18d881f@kubernetescrd":               "wird",
		"wird-wird-adminweb-1a2b@kubernetescrd":                      "",
		"vos-vos-monolith-dd4e33317329b43f479b@kubernetescrd":        "vos-monolith",
		"vos-dev-vos-monolith-dev-1@kubernetescrd":                   "vos-monolith-dev",
		"has-journal-has-journal-7844a73ed8cf7d178ddd@kubernetescrd": "",
		"monitoring-grafana-7dae99210d857670c8cc@kubernetescrd":      "",
		"wird@kubernetescrd":                                         "",
		"10.0.3.17:8080":                                             "",
	} {
		if got := cfg.appOfService(in); got != want {
			t.Errorf("appOfService(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"k8s-cp-01": "h1", "k8s-cp-03": "h3", "k8s-worker-01.bnei.lan": "", "192.168.1.201": "", "h1": "", "": ""} {
		if got := cfg.host(in); got != want {
			t.Errorf("host(%q) = %q, want %q", in, got, want)
		}
	}

	ok := map[string]any{"app": "wird", "ok": true, "namespace": "wird", "pod": "wird-1", "instance": "10.0.0.1:80", "version": "v1.2.3"}
	if f, pass := cfg.clean("ev:argo.sync", ok); !pass || len(f) != 2 || f["app"] != "wird" {
		t.Errorf("clean must keep exactly app+ok: %v %v", f, pass)
	}
	for _, c := range []struct {
		kind string
		in   map[string]any
	}{
		{"ev:argo.sync", map[string]any{"app": "has-journal", "ok": true}},
		{"ev:argo.sync", map[string]any{"app": "10.0.3.17", "ok": true}},
		{"ev:argo.sync", map[string]any{"app": "wird"}},
		{"ev:argo.sync", map[string]any{"app": "wird", "ok": "true"}},
		{"ev:node.ready", map[string]any{"host": "k8s-cp-01", "ready": true}},
		{"ev:quorum", map[string]any{"up": []string{"h1", "pve-bnei"}}},
		{"ev:argo.health", map[string]any{"app": "wird", "status": "Healthy v2.11"}},
		{"ev:replicas", map[string]any{"app": "wird", "bucket": 9}},
		{"ev:pod.churn", map[string]any{"app": "wird", "restarts": -1, "oom": false, "crashloop": false}},
		{"ev:pod.evicted", map[string]any{"app": "wird"}},
		{"world:site", map[string]any{"app": "wird", "x": 1.0, "z": 1.0, "y": 1.0, "tier": 1, "group": "admin"}},
		{"world:deposit", map[string]any{"id": "10.0.3.17", "x0": 1, "z0": 1, "w": 1, "h": 1, "cells": "AAA="}},
	} {
		if f, pass := cfg.clean(c.kind, c.in); pass {
			t.Errorf("clean(%s, %v) passed: %v", c.kind, c.in, f)
		}
	}
}

// TestRotGuard: infra namespaces and Traefik's @internal services are not rot; an unknown router is.
func TestRotGuard(t *testing.T) {
	cfg, _ := loadConfig(configYAML)
	n := func() float64 { Unmapped.mu.Lock(); defer Unmapped.mu.Unlock(); return Unmapped.v["ns"] }
	before := n()
	cfg.appOfNS("kube-system-rotguard", "coredns-1")
	cfg.appOfService("api-rotguard@internal")
	if n() != before {
		t.Fatal("infra namespace or @internal service counted as unmapped")
	}
	cfg.appOfService("newapp-rotguard-web-1a2b@kubernetescrd")
	if n() != before+1 {
		t.Fatal("an unknown Traefik router must count as unmapped")
	}
}
