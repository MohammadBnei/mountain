package realms

import (
	"crypto/sha256"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/MohammadBnei/mountain/internal/graph"
)

// The identity cascade (conception-2026-11-21 §5, §16 decisions 6 and 10): the System a thing belongs to, deduced from
// the event's own hints against ArgoCD's resource list and the alias table, never from a label. Pure: the same hints
// and the same Systems give the same Tag. No namespace rung: in kube-system it gave plausible, wrong answers.

// Ref is one object of an owner chain as the adapter read it: the thing first, its top owner last, through any kind
// (operator custom resources and Nodes included). Group is its API group, "" for the core group (ownerReferences'
// apiVersion before the slash).
type Ref struct {
	Group      string `json:"group,omitempty"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name"`
	TrackingID string `json:"tracking_id,omitempty"` // argocd.argoproj.io/tracking-id, <app>:<group>/<kind>:<ns>/<name>
	Instance   string `json:"instance,omitempty"`    // app.kubernetes.io/instance: ArgoCD's label tracking, and Helm's release
}

// ManagedField is one metadata.managedFields entry of the top owner, as the apiserver returns it (fieldsV1 dropped).
type ManagedField struct {
	Manager     string `json:"manager"`
	Operation   string `json:"operation"`
	Time        string `json:"time"`
	Subresource string `json:"subresource,omitempty"`
}

// Hints are what a trusted adapter says about the thing an event is about. The caller builds them from that adapter's
// own fields only (cmd/atlas/tags.go hintsOf: the event's source first), never from an event's data whole, so a
// client that can Publish cannot hand the cascade a chain or an app:
//   - owner_chain: the chain, thing first (contract events.yaml's owner_chain[]);
//   - managed_fields: the top owner's managedFields, verbatim, beside the chain (events.yaml's field_manager, which
//     names one manager: the apiserver keeps no creator, so the choice is the resolver's, below);
//   - app: the Application a deploy.* event is about (events.yaml's refs.app; its subject is argocd/<app>);
//   - repo: the forge repo a forge event is about, host/owner/repo (the Forgejo adapter's source, git.bnei.dev/o/r).
//
// Empty hints are Unresolved. ponytail: no adapter-stamped registry snapshot id (§5): atlas holds ArgoCD's list
// itself and the Tag stamps the version it resolved against; read one when an adapter resolves against a list atlas
// does not hold.
type Hints struct {
	OwnerChain    []Ref          `json:"owner_chain,omitempty"`
	ManagedFields []ManagedField `json:"managed_fields,omitempty"`
	App           string         `json:"app,omitempty"`
	Repo          string         `json:"repo,omitempty"`
}

// How a Tag's System was found, strongest first, or why there is none.
const (
	Applied    = "applied"    // ArgoCD's own resource list holds the top owner it tracks, or the event is about an Application or a repo one deploys from: the only rung that may route a wake
	Placement  = "placement"  // a Node owns it (static pods, per-node operator pods): the machine's System
	Manager    = "manager"    // a field manager of its top owner names an applied workload, or is an alias (talos)
	Tracked    = "tracked"    // only a tracking id ArgoCD's list does not back (self-set, or copied): never routes
	Ambiguous  = "ambiguous"  // two Systems match at the first rung that matched: never pick one
	Unresolved = "unresolved" // nothing placed it: drift
)

// Tag is a thing's System, how it was found, and the version of what it was found against (re-tag on a newer one).
// Every rung but Applied is a clue anyone who can create the object can set (an annotation, a --field-manager), so
// only Applied may route a wake. Atlas does not enforce that: it writes every tag with its resolution, and the bridge
// enforces it (a System filter delivers applied tags only by default, and SubscribeResponse carries the tag for a
// consumer that reads the others).
type Tag struct {
	System          string   `json:"system,omitempty"`
	Resolution      string   `json:"resolution"`
	RegistryVersion string   `json:"registry_version"`
	Candidates      []string `json:"candidates,omitempty"` // an ambiguous tag's Systems, sorted
}

// Application is the part of an applications.argoproj.io object the cascade reads (kubectl get applications -o json).
type Application struct {
	Metadata struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
	} `json:"metadata"`
	Spec struct {
		Source  *AppSource  `json:"source"`
		Sources []AppSource `json:"sources"` // multi-source: every common-app-chart app (the chart's repo, then its values')
	} `json:"spec"`
	Status struct {
		Resources []AppResource `json:"resources"`
	} `json:"status"`
}

// AppSource is one of an Application's sources: only its repo is read.
type AppSource struct {
	RepoURL string `json:"repoURL"`
}

// AppResource is one entry of an Application's status.resources.
type AppResource struct {
	Group     string `json:"group"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Status    string `json:"status"` // Synced, OutOfSync, Missing (not in the cluster), or unset
}

// Systems is what the cascade resolves against at one version: ArgoCD's resource list, and the alias table.
type Systems struct {
	Version  string              // a hash of what is indexed below, and the alias table's digest (registry_version)
	apps     map[string]bool     // the Applications' names
	applied  map[string][]string // group/kind/ns/name -> the Systems whose Application lists it in the cluster
	managers map[string][]string // an applied workload's name -> its Systems: a controller's field manager is its name
	repos    map[string][]string // repoKey of a source's repoURL -> the Systems that deploy from it
	al       *Aliases
}

func refKey(group, kind, ns, name string) string { return group + "/" + kind + "/" + ns + "/" + name }

// AppName is an Application's name as the k8s watch adapter sends it (refs.app): its name in argocd's namespace, else
// <namespace>/<name>.
func AppName(ns, name string) string {
	if ns == "" || ns == "argocd" {
		return name
	}
	return ns + "/" + name
}

// repoKey is a repoURL as host/owner/repo, lower case, whatever its form (https, ssh://, scp-like git@host:o/r, a
// port, .git): the Forgejo adapter's source has the same shape (git.bnei.dev/o/r).
func repoKey(u string) string {
	u = strings.TrimSuffix(strings.TrimSuffix(strings.ToLower(u), "/"), ".git")
	if _, rest, ok := strings.Cut(u, "://"); ok {
		u = rest
	} else {
		u = strings.Replace(u, ":", "/", 1) // git@host:o/r
	}
	host, path, _ := strings.Cut(u, "/")
	if _, h, ok := strings.Cut(host, "@"); ok {
		host = h
	}
	host, _, _ = strings.Cut(host, ":")
	return host + "/" + path
}

// NewSystems indexes the Applications. A Missing resource is not in the cluster: whatever holds its name there now,
// ArgoCD did not apply it. Version is a hash of the index and the alias table's digest, so the same Applications
// (statuses aside: a Synced to OutOfSync flip changes no answer) and the same aliases.yaml give the same version,
// and an aliases.yaml change re-tags at a new one.
func NewSystems(apps []Application, al *Aliases) *Systems {
	s := &Systems{apps: map[string]bool{}, applied: map[string][]string{}, managers: map[string][]string{}, repos: map[string][]string{}, al: al}
	for _, a := range apps {
		name := AppName(a.Metadata.Namespace, a.Metadata.Name)
		s.apps[name] = true
		id := graph.ID(graph.KindApp, name)
		srcs := a.Spec.Sources
		if a.Spec.Source != nil {
			srcs = append(srcs, *a.Spec.Source)
		}
		for _, src := range srcs {
			if src.RepoURL != "" {
				k := repoKey(src.RepoURL)
				s.repos[k] = addSorted(s.repos[k], id)
			}
		}
		for _, r := range a.Status.Resources {
			if r.Status == "Missing" {
				continue
			}
			k := refKey(r.Group, r.Kind, r.Namespace, r.Name)
			s.applied[k] = addSorted(s.applied[k], id)
			if r.Kind == "Deployment" || r.Kind == "DaemonSet" || r.Kind == "StatefulSet" {
				s.managers[r.Name] = addSorted(s.managers[r.Name], id)
			}
		}
	}
	h := sha256.New()
	for _, m := range []map[string][]string{s.applied, s.repos, {"": slices.Sorted(maps.Keys(s.apps))}} {
		for _, k := range slices.Sorted(maps.Keys(m)) {
			_, _ = fmt.Fprintf(h, "%q %q\n", k, m[k]) // a hash never fails a write
		}
		h.Write([]byte{0})
	}
	digest := 0
	if al != nil {
		digest = al.Digest
	}
	s.Version = fmt.Sprintf("argo-%x/aliases-%08x", h.Sum(nil)[:6], uint32(digest))
	return s
}

func addSorted(ids []string, id string) []string {
	if i, found := slices.BinarySearch(ids, id); !found {
		ids = slices.Insert(ids, i, id)
	}
	return ids
}

// System runs the cascade on h's top owner: tracking ArgoCD backs, placement, field manager, then a tracking id it
// does not back (a forged or copied one loses to the rungs below it). The first rung with a match answers; two
// Systems there is Ambiguous. The resolver never guesses. An event about an Application, or a repo Applications deploy
// from, is Applied when ArgoCD has it (a repo several deploy from, infra-bootstrap, is Ambiguous: one row each).
// Tracking is the annotation (ArgoCD's default; every object ArgoCD listed live carried it), or else the
// app.kubernetes.io/instance label (label tracking), and the label only at the applied rung: Helm sets it to the
// release name on everything it renders, so unbacked it says nothing (in dump2, 69 objects had only the label, none
// in ArgoCD's list).
func (s *Systems) System(h Hints) Tag {
	t := Tag{Resolution: Unresolved, RegistryVersion: s.Version}
	if len(h.OwnerChain) == 0 {
		switch {
		case h.App != "" && s.apps[h.App]:
			return t.of(Applied, []string{graph.ID(graph.KindApp, h.App)})
		case h.Repo != "":
			return t.of(Applied, s.repos[repoKey(h.Repo)])
		}
		return t
	}
	top := h.OwnerChain[len(h.OwnerChain)-1]
	app, _, tracked := strings.Cut(top.TrackingID, ":")
	tracked = tracked && app != ""
	backer := app
	if !tracked {
		backer = top.Instance
	}
	if lists := s.applied[refKey(top.Group, top.Kind, top.Namespace, top.Name)]; backer != "" && len(lists) > 0 {
		return t.of(Applied, addSorted(slices.Clone(lists), graph.ID(graph.KindApp, backer))) // another app listing it, or two: Ambiguous
	}
	// The core v1 Node only: a custom resource named Node (longhorn.io's, which owns its InstanceManagers) is no machine.
	// A Node's own field managers (kubelet, talos) say who writes the Node, not whose its pods are.
	if top.Group == "" && top.Kind == "Node" {
		return t.of(Placement, s.aliased("node", top.Name))
	}
	var ids []string
	for _, m := range h.ManagedFields {
		if m.Subresource != "" { // a status writer (kube-controller-manager) writes every workload's status
			continue
		}
		for _, id := range append(slices.Clone(s.managers[m.Manager]), s.aliased("field-manager", m.Manager)...) {
			ids = addSorted(ids, id)
		}
	}
	if len(ids) > 0 {
		// Every spec writer, not managedFields[0]: the apiserver keeps no creator and reorders the list as managers
		// write, so [0] would flap. A later writer no workload is named after (kubectl-patch) adds nothing.
		return t.of(Manager, ids)
	}
	if tracked {
		return t.of(Tracked, []string{graph.ID(graph.KindApp, app)})
	}
	return t
}

// aliased are the ids the k8s-api source gives name in field now, sorted: a historical row is a name the thing no
// longer has.
func (s *Systems) aliased(field, name string) []string {
	var ids []string
	for _, m := range s.al.Resolve(field + "=" + name) {
		if m.Source == "k8s-api" && m.Status != Historical {
			ids = addSorted(ids, m.ID)
		}
	}
	return ids
}

// of is t found by rung r among ids: none is Unresolved, two or more Ambiguous.
func (t Tag) of(r string, ids []string) Tag {
	switch len(ids) {
	case 0:
	case 1:
		t.System, t.Resolution = ids[0], r
	default:
		t.Resolution, t.Candidates = Ambiguous, ids
	}
	return t
}
