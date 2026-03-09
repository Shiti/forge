package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

//
// RED API Metrics
//

var APIRequestsTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "forge_api_requests_total",
		Help: "Total number of HTTP API requests",
	},
	[]string{"method", "path", "status_code"},
)

var APIRequestDuration = promauto.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "forge_api_request_duration_seconds",
		Help:    "Duration of HTTP API requests",
		Buckets: prometheus.DefBuckets,
	},
	[]string{"method", "path"},
)

var APIInflightRequests = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "forge_api_inflight_requests",
		Help: "Current number of in-flight HTTP requests",
	},
	[]string{"method", "path"},
)

//
// Message Queue Metrics
//

var QueueDepth = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "forge_queue_depth",
		Help: "Current depth of Redis control queues",
	},
	[]string{"queue_name"},
)

var QueuePublishTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "forge_queue_publish_total",
		Help: "Total messages published to Redis",
	},
	[]string{"topic", "type"},
)

var QueueConsumeTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "forge_queue_consume_total",
		Help: "Total messages consumed from Redis",
	},
	[]string{"topic", "type"},
)

var QueueProcessingErrorsTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "forge_queue_processing_errors_total",
		Help: "Total errors during message processing by supervisors",
	},
	[]string{"topic", "type", "error_type"},
)

var MessageLeadTime = promauto.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "forge_message_lead_time_seconds",
		Help:    "Time between message publish and processing start",
		Buckets: prometheus.DefBuckets,
	},
	[]string{"topic"},
)

//
// Cluster State Metrics
//

var NodesRegistered = promauto.NewGauge(
	prometheus.GaugeOpts{
		Name: "forge_nodes_registered_total",
		Help: "Number of active worker nodes currently heartbeating",
	},
)

var NodeHeartbeatLatency = promauto.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "forge_node_heartbeat_latency_seconds",
		Help:    "Time since the node's last heartbeat",
		Buckets: prometheus.DefBuckets,
	},
	[]string{"node_id"},
)

var AgentsRunning = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "forge_agents_running_total",
		Help: "Total active agents",
	},
	[]string{"guild_id"},
)

var SchedulerPlacementDuration = promauto.NewHistogram(
	prometheus.HistogramOpts{
		Name:    "forge_scheduler_placement_duration_seconds",
		Help:    "Tracks CPU time for the Best Fit placement loop",
		Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1},
	},
)

var SchedulerPlacementErrors = promauto.NewCounter(
	prometheus.CounterOpts{
		Name: "forge_scheduler_placement_errors_total",
		Help: "Tracks when the Best Fit algorithm fails to find hardware",
	},
)

var AgentEvictions = promauto.NewCounter(
	prometheus.CounterOpts{
		Name: "forge_agent_evictions_total",
		Help: "When a node dies, how many agents had to be rescheduled/evicted",
	},
)

var AvailableAgentSlots = promauto.NewGauge(
	prometheus.GaugeOpts{
		Name: "forge_available_agent_slots",
		Help: "Sum of max_agents across healthy nodes minus current running agents",
	},
)

//
// Worker Node & Agent USE Metrics
//

var NodeCPUUtilization = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "forge_node_cpu_utilization",
		Help: "Host CPU saturation (percent)",
	},
	[]string{"node_id"},
)

var NodeRAMBytes = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "forge_node_ram_bytes",
		Help: "Host RAM used bytes",
	},
	[]string{"node_id"},
)

var NodeDiskFreeBytes = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "forge_node_disk_free_bytes",
		Help: "Empty disk space available on worker host",
	},
	[]string{"node_id"},
)

var AgentCPUCores = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "forge_agent_cpu_cores",
		Help: "Per-process agent CPU utilization",
	},
	[]string{"guild_id", "agent_id", "node_id"},
)

var AgentMemoryBytes = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "forge_agent_memory_bytes",
		Help: "Per-process agent memory utilization",
	},
	[]string{"guild_id", "agent_id", "node_id"},
)

var SupervisorBootDuration = promauto.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "forge_supervisor_boot_duration_seconds",
		Help:    "Time taken to resolve dependencies and start Python",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
	},
	[]string{"node_id", "supervisor_type"},
)

var SupervisorDependencyPullErrors = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "forge_supervisor_dependency_pull_errors_total",
		Help: "Network failures reaching PyPI via uv",
	},
	[]string{"node_id"},
)

var AgentExitCodes = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "forge_agent_exit_codes_total",
		Help: "Count of agent process stop/crash events by exit code",
	},
	[]string{"guild_id", "agent_id", "node_id", "code"},
)
