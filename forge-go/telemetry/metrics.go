package telemetry

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	metricapi "go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

type metricState struct {
	provider      metricapi.MeterProvider
	handler       http.Handler
	apiInflight   metricapi.Int64Gauge
	apiRequests   metricapi.Int64Counter
	apiDuration   metricapi.Float64Histogram
	queueDepth    metricapi.Float64Gauge
	queuePublish  metricapi.Int64Counter
	queueConsume  metricapi.Int64Counter
	queueErrors   metricapi.Int64Counter
	messageLead   metricapi.Float64Histogram
	nodesGauge    metricapi.Float64Gauge
	heartbeatHist metricapi.Float64Histogram
	agentsRunning metricapi.Float64Gauge
	placeDuration metricapi.Float64Histogram
	placeErrors   metricapi.Int64Counter
	agentEvict    metricapi.Int64Counter
	availableSlot metricapi.Float64Gauge
	nodeCPU       metricapi.Float64Gauge
	nodeRAM       metricapi.Float64Gauge
	nodeDisk      metricapi.Float64Gauge
	agentCPU      metricapi.Float64Gauge
	agentMemory   metricapi.Float64Gauge
	bootDuration  metricapi.Float64Histogram
	depPullErrors metricapi.Int64Counter
	agentExit     metricapi.Int64Counter

	mu       sync.Mutex
	inflight map[string]int64
}

var (
	stateMu        sync.RWMutex
	activeState    *metricState
	bootstrapState *metricState
)

func init() {
	bootstrapState = mustNewMetricState()
	otel.SetMeterProvider(bootstrapState.provider)
	installMetricState(bootstrapState)
}

func PrometheusHandler() http.Handler {
	return currentMetricState().handler
}

func RecordAPIRequest(method, path, statusCode string, duration time.Duration) {
	state := currentMetricState()
	attrs := metricapi.WithAttributes(
		attribute.String("method", method),
		attribute.String("path", path),
		attribute.String("status_code", statusCode),
	)
	state.apiRequests.Add(context.Background(), 1, attrs)
	state.apiDuration.Record(context.Background(), duration.Seconds(),
		metricapi.WithAttributes(
			attribute.String("method", method),
			attribute.String("path", path),
		),
	)
}

func AddAPIInflight(method, path string, delta int64) {
	state := currentMetricState()
	key := method + "\x00" + path

	state.mu.Lock()
	current := state.inflight[key] + delta
	if current < 0 {
		current = 0
	}
	state.inflight[key] = current
	state.mu.Unlock()

	state.apiInflight.Record(context.Background(), current,
		metricapi.WithAttributes(
			attribute.String("method", method),
			attribute.String("path", path),
		),
	)
}

func SetQueueDepth(queueName string, depth float64) {
	currentMetricState().queueDepth.Record(context.Background(), depth,
		metricapi.WithAttributes(attribute.String("queue_name", queueName)),
	)
}

func AddQueuePublish(topic, messageType string) {
	currentMetricState().queuePublish.Add(context.Background(), 1,
		metricapi.WithAttributes(
			attribute.String("topic", topic),
			attribute.String("type", messageType),
		),
	)
}

func AddQueueConsume(topic, messageType string) {
	currentMetricState().queueConsume.Add(context.Background(), 1,
		metricapi.WithAttributes(
			attribute.String("topic", topic),
			attribute.String("type", messageType),
		),
	)
}

func AddQueueProcessingError(topic, messageType, errorType string) {
	currentMetricState().queueErrors.Add(context.Background(), 1,
		metricapi.WithAttributes(
			attribute.String("topic", topic),
			attribute.String("type", messageType),
			attribute.String("error_type", errorType),
		),
	)
}

func ObserveMessageLeadTime(topic string, duration time.Duration) {
	currentMetricState().messageLead.Record(context.Background(), duration.Seconds(),
		metricapi.WithAttributes(attribute.String("topic", topic)),
	)
}

func SetNodesRegistered(total float64) {
	currentMetricState().nodesGauge.Record(context.Background(), total)
}

func ObserveNodeHeartbeatLatency(nodeID string, latency time.Duration) {
	currentMetricState().heartbeatHist.Record(context.Background(), latency.Seconds(),
		metricapi.WithAttributes(attribute.String("node_id", nodeID)),
	)
}

func SetAgentsRunning(guildID string, total float64) {
	currentMetricState().agentsRunning.Record(context.Background(), total,
		metricapi.WithAttributes(attribute.String("guild_id", guildID)),
	)
}

func ObserveSchedulerPlacementDuration(duration time.Duration) {
	currentMetricState().placeDuration.Record(context.Background(), duration.Seconds())
}

func AddSchedulerPlacementError() {
	currentMetricState().placeErrors.Add(context.Background(), 1)
}

func AddAgentEviction() {
	currentMetricState().agentEvict.Add(context.Background(), 1)
}

func SetAvailableAgentSlots(total float64) {
	currentMetricState().availableSlot.Record(context.Background(), total)
}

func SetNodeCPUUtilization(nodeID string, cpuPercent float64) {
	currentMetricState().nodeCPU.Record(context.Background(), cpuPercent,
		metricapi.WithAttributes(attribute.String("node_id", nodeID)),
	)
}

func SetNodeRAMBytes(nodeID string, used float64) {
	currentMetricState().nodeRAM.Record(context.Background(), used,
		metricapi.WithAttributes(attribute.String("node_id", nodeID)),
	)
}

func SetNodeDiskFreeBytes(nodeID string, free float64) {
	currentMetricState().nodeDisk.Record(context.Background(), free,
		metricapi.WithAttributes(attribute.String("node_id", nodeID)),
	)
}

func SetAgentCPUCores(guildID, agentID, nodeID string, cpu float64) {
	currentMetricState().agentCPU.Record(context.Background(), cpu,
		metricapi.WithAttributes(
			attribute.String("guild_id", guildID),
			attribute.String("agent_id", agentID),
			attribute.String("node_id", nodeID),
		),
	)
}

func SetAgentMemoryBytes(guildID, agentID, nodeID string, bytes float64) {
	currentMetricState().agentMemory.Record(context.Background(), bytes,
		metricapi.WithAttributes(
			attribute.String("guild_id", guildID),
			attribute.String("agent_id", agentID),
			attribute.String("node_id", nodeID),
		),
	)
}

func ObserveSupervisorBootDuration(nodeID, supervisorType string, duration time.Duration) {
	currentMetricState().bootDuration.Record(context.Background(), duration.Seconds(),
		metricapi.WithAttributes(
			attribute.String("node_id", nodeID),
			attribute.String("supervisor_type", supervisorType),
		),
	)
}

func AddSupervisorDependencyPullError(nodeID string) {
	currentMetricState().depPullErrors.Add(context.Background(), 1,
		metricapi.WithAttributes(attribute.String("node_id", nodeID)),
	)
}

func AddAgentExitCode(guildID, agentID, nodeID, code string) {
	currentMetricState().agentExit.Add(context.Background(), 1,
		metricapi.WithAttributes(
			attribute.String("guild_id", guildID),
			attribute.String("agent_id", agentID),
			attribute.String("node_id", nodeID),
			attribute.String("code", code),
		),
	)
}

func newMeterProvider(extraReaders ...sdkmetric.Reader) (*sdkmetric.MeterProvider, http.Handler, error) {
	return newMeterProviderWithOptions(nil, extraReaders...)
}

func newMeterProviderWithOptions(extraOptions []sdkmetric.Option, extraReaders ...sdkmetric.Reader) (*sdkmetric.MeterProvider, http.Handler, error) {
	registry := prometheus.NewRegistry()
	promReader, err := otelprom.New(
		otelprom.WithRegisterer(registry),
		otelprom.WithoutTargetInfo(),
		otelprom.WithoutScopeInfo(),
	)
	if err != nil {
		return nil, nil, err
	}

	opts := []sdkmetric.Option{
		sdkmetric.WithReader(promReader),
		sdkmetric.WithView(metricViews()...),
	}
	for _, reader := range extraReaders {
		opts = append(opts, sdkmetric.WithReader(reader))
	}
	opts = append(opts, extraOptions...)

	provider := sdkmetric.NewMeterProvider(opts...)
	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	return provider, handler, nil
}

func installMeterProvider(provider *sdkmetric.MeterProvider, handler http.Handler) error {
	state, err := newMetricState(provider, handler)
	if err != nil {
		return err
	}
	otel.SetMeterProvider(provider)
	installMetricState(state)
	return nil
}

func resetMetricProvider() {
	if bootstrapState == nil {
		return
	}
	otel.SetMeterProvider(bootstrapState.provider)
	installMetricState(bootstrapState)
}

func metricViews() []sdkmetric.View {
	return []sdkmetric.View{
		histogramView("forge_api_request_duration_seconds", prometheus.DefBuckets),
		histogramView("forge_message_lead_time_seconds", prometheus.DefBuckets),
		histogramView("forge_node_heartbeat_latency_seconds", prometheus.DefBuckets),
		histogramView("forge_scheduler_placement_duration_seconds", []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1}),
		histogramView("forge_supervisor_boot_duration_seconds", []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60}),
	}
}

func histogramView(name string, boundaries []float64) sdkmetric.View {
	return sdkmetric.NewView(
		sdkmetric.Instrument{Name: name},
		sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{Boundaries: boundaries}},
	)
}

func mustNewMetricState() *metricState {
	provider, handler, err := newMeterProvider()
	if err != nil {
		noop := metricnoop.NewMeterProvider()
		state, stateErr := newMetricState(noop, promhttp.Handler())
		if stateErr == nil {
			return state
		}
		panic(err)
	}

	state, err := newMetricState(provider, handler)
	if err != nil {
		panic(err)
	}
	return state
}

func installMetricState(state *metricState) {
	stateMu.Lock()
	activeState = state
	stateMu.Unlock()
}

func currentMetricState() *metricState {
	stateMu.RLock()
	state := activeState
	stateMu.RUnlock()
	if state != nil {
		return state
	}
	return bootstrapState
}

func newMetricState(provider metricapi.MeterProvider, handler http.Handler) (*metricState, error) {
	meter := provider.Meter("github.com/rustic-ai/forge/forge-go/telemetry")
	state := &metricState{
		provider: provider,
		handler:  handler,
		inflight: map[string]int64{},
	}

	var err error
	if state.apiInflight, err = meter.Int64Gauge("forge_api_inflight_requests", metricapi.WithDescription("Current number of in-flight HTTP requests")); err != nil {
		return nil, err
	}
	if state.apiRequests, err = meter.Int64Counter("forge_api_requests", metricapi.WithDescription("Total number of HTTP API requests")); err != nil {
		return nil, err
	}
	if state.apiDuration, err = meter.Float64Histogram("forge_api_request_duration_seconds", metricapi.WithDescription("Duration of HTTP API requests")); err != nil {
		return nil, err
	}
	if state.queueDepth, err = meter.Float64Gauge("forge_queue_depth", metricapi.WithDescription("Current depth of Redis control queues")); err != nil {
		return nil, err
	}
	if state.queuePublish, err = meter.Int64Counter("forge_queue_publish", metricapi.WithDescription("Total messages published to Redis")); err != nil {
		return nil, err
	}
	if state.queueConsume, err = meter.Int64Counter("forge_queue_consume", metricapi.WithDescription("Total messages consumed from Redis")); err != nil {
		return nil, err
	}
	if state.queueErrors, err = meter.Int64Counter("forge_queue_processing_errors", metricapi.WithDescription("Total errors during message processing by supervisors")); err != nil {
		return nil, err
	}
	if state.messageLead, err = meter.Float64Histogram("forge_message_lead_time_seconds", metricapi.WithDescription("Time between message publish and processing start")); err != nil {
		return nil, err
	}
	if state.nodesGauge, err = meter.Float64Gauge("forge_nodes_registered_total", metricapi.WithDescription("Number of active worker nodes currently heartbeating")); err != nil {
		return nil, err
	}
	if state.heartbeatHist, err = meter.Float64Histogram("forge_node_heartbeat_latency_seconds", metricapi.WithDescription("Time since the node's last heartbeat")); err != nil {
		return nil, err
	}
	if state.agentsRunning, err = meter.Float64Gauge("forge_agents_running_total", metricapi.WithDescription("Total active agents")); err != nil {
		return nil, err
	}
	if state.placeDuration, err = meter.Float64Histogram("forge_scheduler_placement_duration_seconds", metricapi.WithDescription("Tracks CPU time for the Best Fit placement loop")); err != nil {
		return nil, err
	}
	if state.placeErrors, err = meter.Int64Counter("forge_scheduler_placement_errors", metricapi.WithDescription("Tracks when the Best Fit algorithm fails to find hardware")); err != nil {
		return nil, err
	}
	if state.agentEvict, err = meter.Int64Counter("forge_agent_evictions", metricapi.WithDescription("When a node dies, how many agents had to be rescheduled or evicted")); err != nil {
		return nil, err
	}
	if state.availableSlot, err = meter.Float64Gauge("forge_available_agent_slots", metricapi.WithDescription("Sum of max_agents across healthy nodes minus current running agents")); err != nil {
		return nil, err
	}
	if state.nodeCPU, err = meter.Float64Gauge("forge_node_cpu_utilization", metricapi.WithDescription("Host CPU saturation (percent)")); err != nil {
		return nil, err
	}
	if state.nodeRAM, err = meter.Float64Gauge("forge_node_ram_bytes", metricapi.WithDescription("Host RAM used bytes")); err != nil {
		return nil, err
	}
	if state.nodeDisk, err = meter.Float64Gauge("forge_node_disk_free_bytes", metricapi.WithDescription("Empty disk space available on worker host")); err != nil {
		return nil, err
	}
	if state.agentCPU, err = meter.Float64Gauge("forge_agent_cpu_cores", metricapi.WithDescription("Per-process agent CPU utilization")); err != nil {
		return nil, err
	}
	if state.agentMemory, err = meter.Float64Gauge("forge_agent_memory_bytes", metricapi.WithDescription("Per-process agent memory utilization")); err != nil {
		return nil, err
	}
	if state.bootDuration, err = meter.Float64Histogram("forge_supervisor_boot_duration_seconds", metricapi.WithDescription("Time taken to resolve dependencies and start Python")); err != nil {
		return nil, err
	}
	if state.depPullErrors, err = meter.Int64Counter("forge_supervisor_dependency_pull_errors", metricapi.WithDescription("Network failures reaching PyPI via uv")); err != nil {
		return nil, err
	}
	if state.agentExit, err = meter.Int64Counter("forge_agent_exit_codes", metricapi.WithDescription("Count of agent process stop or crash events by exit code")); err != nil {
		return nil, err
	}

	if state.handler == nil {
		state.handler = promhttp.Handler()
	}
	return state, nil
}
