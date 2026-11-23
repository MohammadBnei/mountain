package realms

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
)

// app is an Application listing res, each group, kind, namespace, name and ArgoCD status ("" = Synced).
func app(name string, res ...[5]string) Application {
	var a Application
	a.Metadata.Name = name
	for _, r := range res {
		a.Status.Resources = append(a.Status.Resources, AppResource{Group: r[0], Kind: r[1], Namespace: r[2], Name: r[3], Status: r[4]})
	}
	return a
}

// mf is managedFields as manager/operation[/subresource] entries (times as the dump had them; the resolver reads none).
func mf(entries ...string) []ManagedField {
	var out []ManagedField
	for _, e := range entries {
		p := append(strings.SplitN(e, "/", 3), "")
		out = append(out, ManagedField{Manager: p[0], Operation: p[1], Subresource: p[2], Time: "2026-11-20T10:41:48Z"})
	}
	return out
}

func checkTag(t *testing.T, name string, got, want Tag) {
	t.Helper()
	if got.System != want.System || got.Resolution != want.Resolution || got.RegistryVersion != want.RegistryVersion ||
		!slices.Equal(got.Candidates, want.Candidates) {
		t.Errorf("%s: %+v, want %+v", name, got, want)
	}
}

// TestSystem: the cascade over the shapes the live Talos cluster had (spike S1's dump2, 2026-11-21: owners.json,
// workloads.json, apps.json), unless a case says otherwise.
func TestSystem(t *testing.T) {
	al, _, _ := loadAliases(t)
	hq := app("hq", [5]string{"apps", "Deployment", "hq", "hq"}, [5]string{"apps", "Deployment", "hq", "gone", "Missing"})
	hq.Spec.Sources = []AppSource{{RepoURL: "git@github.com:MohammadBnei/infra-bootstrap.git"}, {RepoURL: "https://git.bnei.dev/MohammadBnei/hq.git"}}
	game := app("ukubi-game")
	game.Spec.Sources = []AppSource{{RepoURL: "https://github.com/MohammadBnei/infra-bootstrap"}}
	game.Spec.Source = &AppSource{RepoURL: "ssh://git@git.bnei.dev:2222/MohammadBnei/ukubi-game"} // both set: never live, read alike
	s := NewSystems([]Application{
		hq, game,
		app("longhorn", [5]string{"apps", "DaemonSet", "longhorn-system", "longhorn-manager"}),
		app("platform-provisioner", [5]string{"apps", "Deployment", "agent-fleet", "provisioner"}),
		// a shared resource (ArgoCD's SharedResourceWarning) and two workloads of one name
		app("a", [5]string{"", "ConfigMap", "x", "shared"}, [5]string{"apps", "Deployment", "a", "controller"}),
		app("b", [5]string{"", "ConfigMap", "x", "shared"}, [5]string{"apps", "Deployment", "b", "controller"}),
	}, al)
	rs := Ref{Group: "apps", Kind: "ReplicaSet", Namespace: "hq", Name: "hq-6d9f", TrackingID: "hq:apps/Deployment:hq/hq"}
	for _, c := range []struct {
		name string
		h    Hints
		want Tag
	}{
		{"pod -> rs -> deploy ArgoCD applied", Hints{OwnerChain: []Ref{{Kind: "Pod", Namespace: "hq", Name: "hq-6d9f-x"}, rs,
			{Group: "apps", Kind: "Deployment", Namespace: "hq", Name: "hq", TrackingID: "hq:apps/Deployment:hq/hq"}},
			ManagedFields: mf("argocd-controller/Apply", "kube-controller-manager/Update/status")},
			Tag{System: "app:hq", Resolution: Applied}},
		{"a self-set tracking id ArgoCD never applied", Hints{OwnerChain: []Ref{{Kind: "Pod", Namespace: "hq", Name: "evil", TrackingID: "authentik:/Pod:hq/evil"}},
			ManagedFields: mf("kubectl-run/Update")},
			Tag{System: "app:authentik", Resolution: Tracked}},
		{"a forged tracking id falls through to the manager rung", Hints{OwnerChain: []Ref{{Group: "batch", Kind: "Job", Namespace: "agent-fleet", Name: "w", TrackingID: "authentik:batch/Job:agent-fleet/w"}},
			ManagedFields: mf("provisioner/Update")},
			Tag{System: "app:platform-provisioner", Resolution: Manager}},
		{"a replicaset's copied tracking id, its deployment not in the chain", Hints{OwnerChain: []Ref{{Kind: "Pod", Namespace: "hq", Name: "hq-6d9f-x"}, rs},
			ManagedFields: mf("kube-controller-manager/Update", "kube-controller-manager/Update/status")},
			Tag{System: "app:hq", Resolution: Tracked}},
		{"the listed name, another group", Hints{OwnerChain: []Ref{{Group: "evil.io", Kind: "Deployment", Namespace: "hq", Name: "hq", TrackingID: "hq:apps/Deployment:hq/hq"}}},
			Tag{System: "app:hq", Resolution: Tracked}},
		{"a name ArgoCD lists as Missing, recreated by someone else", Hints{OwnerChain: []Ref{{Group: "apps", Kind: "Deployment", Namespace: "hq", Name: "gone", TrackingID: "hq:apps/Deployment:hq/gone"}}},
			Tag{System: "app:hq", Resolution: Tracked}},
		{"a tracking id one app claims, another app lists", Hints{OwnerChain: []Ref{{Group: "apps", Kind: "Deployment", Namespace: "hq", Name: "hq", TrackingID: "other:apps/Deployment:hq/hq"}}},
			Tag{Resolution: Ambiguous, Candidates: []string{"app:hq", "app:other"}}},
		{"two Applications list one object", Hints{OwnerChain: []Ref{{Kind: "ConfigMap", Namespace: "x", Name: "shared", TrackingID: "a:/ConfigMap:x/shared"}}},
			Tag{Resolution: Ambiguous, Candidates: []string{"app:a", "app:b"}}},
		{"a static pod on a Node the alias table knows", Hints{OwnerChain: []Ref{{Kind: "Pod", Namespace: "kube-system", Name: "kube-apiserver-esprimo-1"}, {Kind: "Node", Name: "esprimo-1"}}},
			Tag{System: "vm:esprimo-1", Resolution: Placement}},
		// owners.json: the InstanceManager's owner is longhorn.io/v1beta2 Node esprimo-2, a namespaced Longhorn CR, not the
		// core Node; the dump has not that CR, so its managedFields here are the longhorn-manager that writes them
		{"longhorn's per-node InstanceManager, owned by longhorn.io's Node CR", Hints{OwnerChain: []Ref{{Group: "longhorn.io", Kind: "InstanceManager", Namespace: "longhorn-system", Name: "instance-manager-0add9d9906a8a4f3ab194dcce64d0ae0"},
			{Group: "longhorn.io", Kind: "Node", Namespace: "longhorn-system", Name: "esprimo-2"}}, ManagedFields: mf("longhorn-manager/Update", "longhorn-manager/Update/status")},
			Tag{System: "app:longhorn", Resolution: Manager}},
		{"a CR named Node, on a name the alias table places", Hints{OwnerChain: []Ref{{Group: "example.io", Kind: "Node", Name: "esprimo-2"}}},
			Tag{Resolution: Unresolved}},
		{"label tracking ArgoCD backs", Hints{OwnerChain: []Ref{{Group: "apps", Kind: "Deployment", Namespace: "hq", Name: "hq", Instance: "hq"}}},
			Tag{System: "app:hq", Resolution: Applied}},
		{"an instance label alone: Helm's release name, not a tracking", Hints{OwnerChain: []Ref{{Kind: "ConfigMap", Namespace: "hq", Name: "values", Instance: "hq"}}},
			Tag{Resolution: Unresolved}},
		{"forge event on a repo one Application deploys from", Hints{Repo: "git.bnei.dev/MohammadBnei/hq"}, Tag{System: "app:hq", Resolution: Applied}},
		{"forge event, an ssh repoURL with a port", Hints{Repo: "git.bnei.dev/mohammadbnei/ukubi-game"}, Tag{System: "app:ukubi-game", Resolution: Applied}},
		{"forge event on a repo no Application deploys from", Hints{Repo: "git.bnei.dev/MohammadBnei/nope"}, Tag{Resolution: Unresolved}},
		{"forge event on the GitHub mirror's host: another repo", Hints{Repo: "github.com/MohammadBnei/hq"}, Tag{Resolution: Unresolved}},
		{"forge event on a repo every app's chart comes from", Hints{Repo: "github.com/MohammadBnei/infra-bootstrap"}, Tag{Resolution: Ambiguous, Candidates: []string{"app:hq", "app:ukubi-game"}}},
		{"cilium, applied by Talos", Hints{OwnerChain: []Ref{{Kind: "Pod", Namespace: "kube-system", Name: "cilium-abcde"}, {Group: "apps", Kind: "DaemonSet", Namespace: "kube-system", Name: "cilium"}},
			ManagedFields: mf("talos/Apply", "kube-controller-manager/Update/status")},
			Tag{System: "platform:talos", Resolution: Manager}},
		{"csi-attacher, created by longhorn-manager", Hints{OwnerChain: []Ref{{Group: "apps", Kind: "Deployment", Namespace: "longhorn-system", Name: "csi-attacher"}},
			ManagedFields: mf("longhorn-manager/Update", "kube-controller-manager/Update/status")},
			Tag{System: "app:longhorn", Resolution: Manager}},
		{"a worker Job, created by the provisioner, patched by hand", Hints{OwnerChain: []Ref{{Group: "batch", Kind: "Job", Namespace: "agent-fleet", Name: "worker-7223c28fd0f64222b85a"}},
			ManagedFields: mf("provisioner/Update", "kubectl-patch/Update", "kube-controller-manager/Update/status")},
			Tag{System: "app:platform-provisioner", Resolution: Manager}},
		{"managedFields[0] is not the creator", Hints{OwnerChain: []Ref{{Group: "batch", Kind: "Job", Namespace: "agent-fleet", Name: "worker-7223c28fd0f64222b85a"}},
			ManagedFields: mf("kube-controller-manager/Update/status", "kubectl-patch/Update", "provisioner/Update")},
			Tag{System: "app:platform-provisioner", Resolution: Manager}},
		{"a manager two applied workloads are named", Hints{OwnerChain: []Ref{{Group: "batch", Kind: "Job", Namespace: "a", Name: "j"}}, ManagedFields: mf("controller/Update")},
			Tag{Resolution: Ambiguous, Candidates: []string{"app:a", "app:b"}}},
		{"no namespace rung: a bare pod in hq's namespace", Hints{OwnerChain: []Ref{{Kind: "Pod", Namespace: "hq", Name: "debug"}}, ManagedFields: mf("kubectl-run/Update")},
			Tag{Resolution: Unresolved}},
		{"deploy.synced about an Application ArgoCD has", Hints{App: "hq"}, Tag{System: "app:hq", Resolution: Applied}},
		{"deploy.health about an Application ArgoCD does not have", Hints{App: "nope"}, Tag{Resolution: Unresolved}},
		{"no hints", Hints{}, Tag{Resolution: Unresolved}},
	} {
		c.want.RegistryVersion = s.Version
		checkTag(t, c.name, s.System(c.h), c.want)
	}
}

// TestSystemDump2: real objects of the live Talos cluster (testdata/dump2-systems.json's note), real Applications.
// Over all 335 of dump2's workloads, walked the same way (outside the repo: the dump is not committed): 272 applied,
// 52 manager, 11 unresolved (the 9 static pods of the three Talos Nodes, which aliases.yaml does not list yet, and
// the 2 InstanceManager pods, whose longhorn.io Node owner the dump does not hold), 0 ambiguous.
func TestSystemDump2(t *testing.T) {
	b, err := os.ReadFile("testdata/dump2-systems.json")
	if err != nil {
		t.Fatal(err)
	}
	var d struct {
		Apps  []Application
		Cases []struct {
			Hints Hints
			Want  Tag
		}
	}
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatal(err)
	}
	al, _, _ := loadAliases(t)
	s := NewSystems(d.Apps, al)
	for _, c := range d.Cases {
		c.Want.RegistryVersion = s.Version
		checkTag(t, c.Hints.OwnerChain[0].Name, s.System(c.Hints), c.Want)
	}
	if len(d.Cases) != 7 {
		t.Errorf("%d cases, want 7", len(d.Cases))
	}
}

// TestSystemsVersion: the version moves with what the cascade reads (a resource, a repo, the alias table), not with a
// status it does not read (Synced to OutOfSync).
func TestSystemsVersion(t *testing.T) {
	al, rl, reg := loadAliases(t)
	al2, err := ParseAliases(append(slices.Clone(aliasesYAML), "\n"...), rl, reg)
	if err != nil {
		t.Fatal(err)
	}
	v := func(al *Aliases, apps ...Application) string { return NewSystems(apps, al).Version }
	base := v(al, app("hq", [5]string{"apps", "Deployment", "hq", "hq"}))
	if got := v(al, app("hq", [5]string{"apps", "Deployment", "hq", "hq", "OutOfSync"})); got != base {
		t.Errorf("a status flip moved the version: %s, was %s", got, base)
	}
	for name, got := range map[string]string{
		"a resource Missing": v(al, app("hq", [5]string{"apps", "Deployment", "hq", "hq", "Missing"})),
		"another resource":   v(al, app("hq", [5]string{"apps", "Deployment", "hq", "hq2"})),
		"another app":        v(al, app("hq2", [5]string{"apps", "Deployment", "hq", "hq"})),
		"aliases.yaml":       v(al2, app("hq", [5]string{"apps", "Deployment", "hq", "hq"})),
	} {
		if got == base {
			t.Errorf("%s: the version stayed %s", name, base)
		}
	}
}

// TestSystemHistorical: a retired name places nothing.
func TestSystemHistorical(t *testing.T) {
	_, rl, reg := loadAliases(t)
	al, err := ParseAliases([]byte(`things:
  platform:old:
    aliases:
      - {source: k8s-api, field: field-manager, name: old-installer, status: historical, key: name}
`), rl, reg)
	if err != nil {
		t.Fatal(err)
	}
	s := NewSystems(nil, al)
	h := Hints{OwnerChain: []Ref{{Kind: "ConfigMap", Namespace: "x", Name: "y"}}, ManagedFields: mf("old-installer/Apply")}
	checkTag(t, "historical field manager", s.System(h), Tag{Resolution: Unresolved, RegistryVersion: s.Version})
}
