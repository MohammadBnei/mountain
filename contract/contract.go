// Package contract is what atlas reads from the systems it does not own, on the wire: the PromQL it sends, the
// Alertmanager request it makes, the labels it joins on, and the Forgejo routes it polls. atlas uses it as its own
// copy; ukubi-game's sim mirrors these sources and imports it, so a query atlas adds or changes is a query the
// sim's mirror must answer (its coverage test fails otherwise). It also holds the live frame atlas sends and how a
// node's health is read from it (live.go), which the game's town draws. The standard library only: anything may
// depend on it.
package contract

// Queries is every PromQL atlas must have from Prometheus (an instant query, no time= parameter), by name. One failed
// query fails the whole poll. The optional ones are Signals.
var Queries = map[string]string{
	"info":    `argocd_app_info`,
	"ready":   `kube_node_status_condition{condition="Ready",status="true"}`,
	"probes":  `max by (target) (probe_success{target=~"guest-.*|pve-.*|ext-.*"})`,
	"cpu":     `sum by (namespace) (rate(container_cpu_usage_seconds_total{container!="",image!=""}[5m]))`,
	"mem":     `sum by (namespace) (container_memory_working_set_bytes{container!="",image!=""})`,
	"disk":    `sum by (namespace) (kubelet_volume_stats_used_bytes)`,
	"cpucap":  `sum(kube_node_status_allocatable{resource="cpu"})`,
	"memcap":  `sum(kube_node_status_allocatable{resource="memory"})`,
	"diskcap": `sum(kubelet_volume_stats_capacity_bytes)`,
	"req":     `sum by (exported_service) (rate(traefik_service_requests_total[2m]))`,
	// per node: the city's worked tiles (cadvisor's series carry the node; allocatable is per node)
	"cpunode":    `sum by (namespace, node) (rate(container_cpu_usage_seconds_total{container!="",image!=""}[5m]))`,
	"memnode":    `sum by (namespace, node) (container_memory_working_set_bytes{container!="",image!=""})`,
	"cpucapnode": `sum by (node) (kube_node_status_allocatable{resource="cpu"})`,
	"memcapnode": `sum by (node) (kube_node_status_allocatable{resource="memory"})`,
	// each Application's finished syncs since its controller started, by phase (Succeeded, or Failed and Error): a
	// counter per (name, phase), diffed by atlas itself so no sync between two polls is missed. The max across scrape
	// targets, so two targets (or a pod's new name) are one series; the controller restarting resets it to 0
	// (Prometheus restarting does not). Same exporter as info: answers whenever it does, empty when nothing synced
	// yet. A required query: any reader's fake or sim of this contract must answer it.
	"syncs": `max by (name, phase) (argocd_app_sync_total{phase=~"Succeeded|Failed|Error"})`,
}

// Signal is an optional query: what it measures may be missing (a metric not scraped yet, a histogram not configured),
// so a failed or empty answer is *unknown*, never zero, and never fails the poll. A healthy state is not left empty:
// the signals that count trouble fill their quiet case with an explicit zero from a series that exists whenever the measured thing does
// (the request counter for errors, argocd_app_info for degraded apps, Alertmanager's always-firing Watchdog for
// critical alerts: no Watchdog means the rules did not run, so the day is unknown, not calm). Every seconds is how
// often it is asked: the day windows are rolling 24 h ([1d]) and heavy, so they are asked every 5 min. Known
// undercount: increase() misses a series' first count, so a 5xx series born today (Traefik creates it on its first
// error) loses that error. Latency with no traffic is NaN: unknown, which is right.
type Signal struct {
	Query string
	Every int // seconds
}

// Signals is every optional query (plan/13 slice 2), by name. The sim's mirror must answer these too (its coverage
// test), so a change here ships with the sim's mirror of it.
var Signals = map[string]Signal{
	// per Traefik service: 5xx per second (0 when none, from the request counter), and the p95 response time
	"err": {`sum by (exported_service) (rate(traefik_service_requests_total{code=~"5.."}[2m])) or 0 * sum by (exported_service) (rate(traefik_service_requests_total[2m]))`, 15},
	"lat": {`histogram_quantile(0.95, sum by (exported_service, le) (rate(traefik_service_request_duration_seconds_bucket[5m])))`, 15},
	// the last 24 h per Traefik service: requests, and the 5xx among them (an app's availability is 1 - errday/reqday)
	"reqday": {`sum by (exported_service) (increase(traefik_service_requests_total[1d]))`, 300},
	"errday": {`sum by (exported_service) (increase(traefik_service_requests_total{code=~"5.."}[1d])) or 0 * sum by (exported_service) (increase(traefik_service_requests_total[1d]))`, 300},
	// the last 24 h per probe target: the share of probes that succeeded
	"probeday": {`avg by (target) (avg_over_time(probe_success{target=~"guest-.*|pve-.*|ext-.*"}[1d]))`, 300},
	// the town's calm day: each critical alert that fired in the last 24 h (one series per alert name), or a single 0
	// when none did and the Watchdog proves the rules were evaluated
	"critday": {`count by (alertname) (max_over_time(ALERTS{alertstate="firing",severity="critical"}[1d])) or on () (0 * max(max_over_time(ALERTS{alertname="Watchdog"}[1d])))`, 300},
	// the last 24 h per Application: 1 if it was Degraded at any time, else 0
	"degradedday": {`max by (name) (max_over_time(argocd_app_info{health_status="Degraded"}[1d])) or 0 * max by (name) (argocd_app_info)`, 300},
	// the add-ons' workloads (kubespray's, in kube-system and metallb-system): the share of each one's pods that are
	// ready; an add-on's health is its least ready workload. An add-on installed into another namespace (a changed
	// metallb_namespace) is not asked for, so it reads unknown: widen these selectors with it.
	"dsready":     {`min by (namespace, daemonset) (kube_daemonset_status_number_ready{namespace=~"kube-system|metallb-system"} / kube_daemonset_status_desired_number_scheduled{namespace=~"kube-system|metallb-system"})`, 15},
	"deployready": {`min by (namespace, deployment) (kube_deployment_status_replicas_available{namespace=~"kube-system|metallb-system"} / kube_deployment_spec_replicas{namespace=~"kube-system|metallb-system"})`, 15},
	// running pods per k8s node (finished Jobs' pods left out): a house's population
	"pods": {`count by (node) (kube_pod_info * on (namespace, pod) group_left () (kube_pod_status_phase{phase="Running"} == 1))`, 15},
}

// Rule is one instance of an alert rule: the severity it fires at and its for: in seconds (how long its condition
// must hold before it fires; 0 = on the first evaluation). A name may have two (a warning and a critical rung).
type Rule struct {
	Severity string
	For      int
}

// PromQueryPath is the Prometheus HTTP API route the queries go to (as ?query=<url-escaped PromQL>).
const PromQueryPath = "/api/v1/query"

// AlertsPath is the Alertmanager request atlas makes: the active alerts that are neither silenced nor inhibited.
const AlertsPath = "/api/v2/alerts?active=true&silenced=false&inhibited=false"

// AlertsSkipped are alert names atlas never shows (Alertmanager's own heartbeat and inhibitor helper).
var AlertsSkipped = []string{"Watchdog", "InfoInhibitor"}

// The labels (and the one annotation) atlas joins on.
const (
	LabelName            = "name"             // argocd_app_info: the Application, → app:<lower name>
	LabelHealth          = "health_status"    // argocd_app_info
	LabelSync            = "sync_status"      // argocd_app_info
	LabelPhase           = "phase"            // argocd_app_sync_total: Succeeded, Failed, Error (Running, Terminating not asked)
	LabelNode            = "node"             // a k8s node, → vm:<lower node>
	LabelNamespace       = "namespace"        // land, and an alert's app when one app owns the namespace
	LabelTarget          = "target"           // a blackbox probe target (guest-*, pve-*, ext-*), through realms.ProbeNode
	LabelExportedService = "exported_service" // Traefik's <ns>-<svc>-<port>@<provider>, → its app
	LabelDaemonSet       = "daemonset"        // kube-state-metrics: a DaemonSet's name
	LabelDeployment      = "deployment"       // kube-state-metrics: a Deployment's name
	LabelAlertName       = "alertname"
	LabelSeverity        = "severity" // only warning and critical are drawn
	AnnotationSummary    = "summary"
)

// ForgeOwner owns every repo atlas reads, on Forgejo as on GitHub.
const ForgeOwner = "MohammadBnei"

// The Forgejo API routes atlas polls (under --forge-url, /api/v1), each under /repos/<ForgeOwner>/<repo>.
const (
	ForgeBranches = "/branches/" // + <branch> (slashes kept): {"commit": {"id": <the head sha>}, …}; a tag never answers
	ForgeArchive  = "/archive/"  // + <sha>.tar.gz: a gzip tar, a pax global header then one top-level directory, "<repo>/"
)

// GitHubOwner and the GitHub REST routes atlas polled before it moved to Forgejo. Deprecated: atlas no longer uses
// them; they stay while ukubi-game's world.go and mirror/github still do.
const GitHubOwner = ForgeOwner

// The GitHub REST routes, each under /repos/<GitHubOwner>/<repo>, and the Accept header each sends.
const (
	GitHubCommits    = "/commits/" // + <branch>, conditional (If-None-Match): the raw head sha
	GitHubHeadAccept = "application/vnd.github.sha"
	GitHubCompare    = "/compare/" // + <base>...<head>?per_page=1; status: ahead | behind | identical | diverged
	GitHubTarball    = "/tarball/" // + <sha>: a gzip tar with one top-level directory
	GitHubJSONAccept = "application/vnd.github+json"
)
