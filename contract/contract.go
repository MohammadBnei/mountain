// Package contract is what atlas reads from the systems it does not own, on the wire: the PromQL it sends, the
// Alertmanager request it makes, the labels it joins on, and the GitHub routes it polls. atlas uses it as its own
// copy; ukubi-game's sim mirrors these sources and imports it, so a query atlas adds or changes is a query the
// sim's mirror must answer (its coverage test fails otherwise). No imports: anything may depend on it.
package contract

// Queries is every PromQL atlas sends to Prometheus (an instant query, no time= parameter), by name. One failed
// query fails the whole poll.
var Queries = map[string]string{
	"info":    `argocd_app_info`,
	"ready":   `kube_node_status_condition{condition="Ready",status="true"}`,
	"probes":  `max by (target) (probe_success{target=~"guest-.*|pve-.*"})`,
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
	LabelNode            = "node"             // a k8s node, → vm:<lower node>
	LabelNamespace       = "namespace"        // land, and an alert's app when one app owns the namespace
	LabelTarget          = "target"           // a blackbox probe target (guest-*, pve-*), through realms.ProbeNode
	LabelExportedService = "exported_service" // Traefik's <ns>-<svc>-<port>@<provider>, → its app
	LabelAlertName       = "alertname"
	LabelSeverity        = "severity" // only warning and critical are drawn
	AnnotationSummary    = "summary"
)

// GitHubOwner owns every repo atlas reads.
const GitHubOwner = "MohammadBnei"

// The GitHub REST routes atlas polls (each under /repos/<GitHubOwner>/<repo>), and the Accept header each sends.
const (
	GitHubHeadRoute    = "/commits/{branch}" // conditional (If-None-Match): the raw head sha
	GitHubHeadAccept   = "application/vnd.github.sha"
	GitHubCompareRoute = "/compare/{base}...{head}" // ?per_page=1; status: ahead | behind | identical | diverged
	GitHubTarballRoute = "/tarball/{sha}"           // a gzip tar with one top-level directory
	GitHubJSONAccept   = "application/vnd.github+json"
)
