package control

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/rustic-ai/forge/forge-go/guild/store"
	"github.com/rustic-ai/forge/forge-go/helper/envvars"
	"github.com/rustic-ai/forge/forge-go/protocol"
	"github.com/rustic-ai/forge/forge-go/registry"
	"github.com/rustic-ai/forge/forge-go/secrets"
	"github.com/rustic-ai/forge/forge-go/supervisor"
)

const defaultOrganizationID = "default-org"

type SupervisorFactory func(orgID string) supervisor.AgentSupervisor

// ControlQueueHandler wiring layer connecting the Redis listener to the localized ProcessSupervisor
type ControlQueueHandler struct {
	rdb        *redis.Client
	registry   *registry.Registry
	secrets    secrets.SecretProvider
	sup        supervisor.AgentSupervisor
	supByOrg   map[string]supervisor.AgentSupervisor
	supMu      sync.RWMutex
	supFactory SupervisorFactory
	agentOrg   map[string]string
	agentMu    sync.RWMutex
	store      store.Store
	listener   *ControlQueueListener
	responder  *ControlQueueResponder
}

// NewControlQueueHandler creates a fully integrated control handler
func NewControlQueueHandler(
	rdb *redis.Client,
	reg *registry.Registry,
	sec secrets.SecretProvider,
	sup supervisor.AgentSupervisor,
	db store.Store,
) *ControlQueueHandler {
	return NewControlQueueHandlerWithQueue(rdb, reg, sec, sup, db, ControlQueueRequestKey)
}

func NewControlQueueHandlerWithFactory(
	rdb *redis.Client,
	reg *registry.Registry,
	sec secrets.SecretProvider,
	factory SupervisorFactory,
	db store.Store,
) *ControlQueueHandler {
	return NewControlQueueHandlerWithQueueFactory(rdb, reg, sec, factory, db, ControlQueueRequestKey)
}

// NewControlQueueHandlerWithQueue creates a fully integrated control handler bound to a specific Redis queue.
func NewControlQueueHandlerWithQueue(
	rdb *redis.Client,
	reg *registry.Registry,
	sec secrets.SecretProvider,
	sup supervisor.AgentSupervisor,
	db store.Store,
	queueKey string,
) *ControlQueueHandler {
	return newControlQueueHandler(rdb, reg, sec, sup, nil, db, queueKey)
}

func NewControlQueueHandlerWithQueueFactory(
	rdb *redis.Client,
	reg *registry.Registry,
	sec secrets.SecretProvider,
	factory SupervisorFactory,
	db store.Store,
	queueKey string,
) *ControlQueueHandler {
	return newControlQueueHandler(rdb, reg, sec, nil, factory, db, queueKey)
}

func newControlQueueHandler(
	rdb *redis.Client,
	reg *registry.Registry,
	sec secrets.SecretProvider,
	sup supervisor.AgentSupervisor,
	factory SupervisorFactory,
	db store.Store,
	queueKey string,
) *ControlQueueHandler {
	listener := NewControlQueueListenerWithQueue(rdb, queueKey)
	responder := NewControlQueueResponder(rdb)

	handler := &ControlQueueHandler{
		rdb:        rdb,
		registry:   reg,
		secrets:    sec,
		sup:        sup,
		supFactory: factory,
		supByOrg:   make(map[string]supervisor.AgentSupervisor),
		agentOrg:   make(map[string]string),
		store:      db,
		listener:   listener,
		responder:  responder,
	}

	listener.OnSpawn = handler.handleSpawn
	listener.OnStop = handler.handleStop

	return handler
}

// Start spawns the background Redis BRPOP polling loop
func (h *ControlQueueHandler) Start(ctx context.Context) error {
	go h.listener.Start(ctx)
	return nil
}

// Stop terminates the background listener blocking routine
func (h *ControlQueueHandler) Stop() {
	h.listener.Stop()
	if h.supFactory == nil {
		return
	}

	h.supMu.RLock()
	supervisors := make([]supervisor.AgentSupervisor, 0, len(h.supByOrg))
	for _, sup := range h.supByOrg {
		supervisors = append(supervisors, sup)
	}
	h.supMu.RUnlock()

	for _, sup := range supervisors {
		_ = sup.StopAll(context.Background())
	}
}

// handleSpawn orchestrates booting an agent based on the remote SpawnRequest
func (h *ControlQueueHandler) handleSpawn(ctx context.Context, req *protocol.SpawnRequest) {
	slog.Info("handleSpawn: received spawn request", "agent_id", req.AgentSpec.ID, "class", req.AgentSpec.ClassName, "guild", req.GuildID, "request_id", req.RequestID)

	entry, err := h.registry.Lookup(req.AgentSpec.ClassName)
	if err != nil {
		slog.Error("handleSpawn: registry lookup failed", "class", req.AgentSpec.ClassName, "error", err)
		h.responder.SendError(ctx, req.RequestID, fmt.Sprintf("failed to lookup agent class %s from registry: %v", req.AgentSpec.ClassName, err))
		return
	}
	slog.Info("handleSpawn: registry lookup OK", "agent_id", req.AgentSpec.ID, "entry_id", entry.ID)

	var guildSpec *protocol.GuildSpec
	var guildOrgID string
	if h.store != nil {
		guildModel, err := h.store.GetGuild(req.GuildID)
		if err == nil {
			slog.Info("handleSpawn: guild store lookup OK", "guild", req.GuildID)
			guildOrgID = guildModel.OrganizationID
			guildSpec = store.ToGuildSpec(guildModel)
		} else {
			slog.Warn("handleSpawn: guild store lookup failed, using spawn payload fallback", "guild", req.GuildID, "error", err)
		}
	}
	if guildSpec == nil {
		guildSpec = extractGuildSpec(req.ClientProperties)
	}
	if guildSpec == nil {
		guildSpec = extractGuildSpec(req.AgentSpec.Properties)
	}
	if guildSpec == nil {
		guildSpec = &protocol.GuildSpec{
			ID: req.GuildID,
			Properties: map[string]interface{}{
				"messaging": map[string]interface{}{
					"backend_module": "rustic_ai.redis.messaging.backend",
					"backend_class":  "RedisMessagingBackend",
					"backend_config": map[string]interface{}{},
				},
			},
		}
	}

	if req.MessagingConfig != nil {
		if guildSpec.Properties == nil {
			guildSpec.Properties = make(map[string]interface{})
		}
		guildSpec.Properties["messaging"] = map[string]interface{}{
			"backend_module": req.MessagingConfig.BackendModule,
			"backend_class":  req.MessagingConfig.BackendClass,
			"backend_config": req.MessagingConfig.BackendConfig,
		}
	}

	envVars, err := envvars.BuildAgentEnv(ctx, guildSpec, &req.AgentSpec, entry, h.secrets)
	if err != nil {
		slog.Error("handleSpawn: env var build failed", "agent_id", req.AgentSpec.ID, "error", err)
		h.responder.SendError(ctx, req.RequestID, fmt.Sprintf("failed to build environment variables: %v", err))
		return
	}
	slog.Info("handleSpawn: env vars built OK", "agent_id", req.AgentSpec.ID, "env_count", len(envVars))

	orgID := h.resolveOrganizationForSpawn(req, guildOrgID)
	sup := h.supervisorForOrganization(orgID)
	if sup == nil {
		h.responder.SendError(ctx, req.RequestID, "no supervisor available for organization")
		return
	}

	err = sup.Launch(ctx, req.GuildID, &req.AgentSpec, h.registry, envVars)
	if err != nil {
		slog.Error("handleSpawn: supervisor launch failed", "agent_id", req.AgentSpec.ID, "org", orgID, "error", err)
		h.responder.SendError(ctx, req.RequestID, fmt.Sprintf("failed to launch process via supervisor: %v", err))
		return
	}
	h.recordAgentOrganization(req.GuildID, req.AgentSpec.ID, orgID)
	slog.Info("handleSpawn: supervisor launch OK", "agent_id", req.AgentSpec.ID, "org", orgID)

	msg := &protocol.SpawnResponse{
		RequestID: req.RequestID,
		Success:   true,
		Message:   "agent process spawned successfully",
	}

	if pSup, ok := sup.(*supervisor.ProcessSupervisor); ok {
		if status, _ := pSup.Status(ctx, req.GuildID, req.AgentSpec.ID); status == "running" {
			nodeID, _ := os.Hostname()
			if nodeID == "" {
				nodeID = "localhost"
			}
			msg.NodeID = nodeID

			for retries := 0; retries < 5; retries++ {
				actualPid, err := pSup.GetPID(ctx, req.GuildID, req.AgentSpec.ID)
				if err == nil && actualPid > 0 {
					msg.PID = actualPid
					break
				}
				time.Sleep(100 * time.Millisecond)
			}

			if msg.PID <= 0 {
				h.responder.SendError(ctx, req.RequestID, "timed out waiting to retrieve valid PID for spawned agent")
				return
			}
		}
	}

	_ = h.responder.SendResponse(ctx, req.RequestID, msg)
}

func (h *ControlQueueHandler) handleStop(ctx context.Context, req *protocol.StopRequest) {
	orgID := h.resolveOrganizationForStop(req)
	sup := h.supervisorForOrganization(orgID)
	if sup == nil {
		h.responder.SendError(ctx, req.RequestID, "no supervisor available for organization")
		return
	}

	err := sup.Stop(ctx, req.GuildID, req.AgentID)
	if err != nil {
		h.responder.SendError(ctx, req.RequestID, fmt.Sprintf("failed to stop agent %s: %v", req.AgentID, err))
		return
	}
	h.forgetAgentOrganization(req.GuildID, req.AgentID)

	msg := &protocol.StopResponse{
		RequestID: req.RequestID,
		Success:   true,
		Message:   "agent process stopped gracefully",
	}

	_ = h.responder.SendResponse(ctx, req.RequestID, msg)
}

func normalizeOrganizationID(orgID string) string {
	if strings.TrimSpace(orgID) == "" {
		return defaultOrganizationID
	}
	return orgID
}

func organizationFromValue(v interface{}) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func agentOrgKey(guildID, agentID string) string {
	return guildID + "::" + agentID
}

func (h *ControlQueueHandler) resolveOrganizationForSpawn(req *protocol.SpawnRequest, guildOrgID string) string {
	if orgID := strings.TrimSpace(req.OrganizationID); orgID != "" {
		return orgID
	}
	if orgID := organizationFromValue(req.ClientProperties["organization_id"]); orgID != "" {
		return orgID
	}
	if req.AgentSpec.Properties != nil {
		if orgID := organizationFromValue(req.AgentSpec.Properties["organization_id"]); orgID != "" {
			return orgID
		}
	}
	if orgID := strings.TrimSpace(guildOrgID); orgID != "" {
		return orgID
	}
	return defaultOrganizationID
}

func (h *ControlQueueHandler) resolveOrganizationForStop(req *protocol.StopRequest) string {
	if orgID := strings.TrimSpace(req.OrganizationID); orgID != "" {
		return orgID
	}

	key := agentOrgKey(req.GuildID, req.AgentID)
	h.agentMu.RLock()
	if orgID, ok := h.agentOrg[key]; ok && orgID != "" {
		h.agentMu.RUnlock()
		return orgID
	}
	h.agentMu.RUnlock()

	if h.store != nil {
		if guildModel, err := h.store.GetGuild(req.GuildID); err == nil && strings.TrimSpace(guildModel.OrganizationID) != "" {
			return guildModel.OrganizationID
		}
	}

	return defaultOrganizationID
}

func (h *ControlQueueHandler) supervisorForOrganization(orgID string) supervisor.AgentSupervisor {
	orgID = normalizeOrganizationID(orgID)
	if h.supFactory == nil {
		return h.sup
	}

	h.supMu.RLock()
	sup, ok := h.supByOrg[orgID]
	h.supMu.RUnlock()
	if ok {
		return sup
	}

	created := h.supFactory(orgID)
	if created == nil {
		return nil
	}

	h.supMu.Lock()
	if existing, exists := h.supByOrg[orgID]; exists {
		h.supMu.Unlock()
		return existing
	}
	h.supByOrg[orgID] = created
	h.supMu.Unlock()
	return created
}

func (h *ControlQueueHandler) recordAgentOrganization(guildID, agentID, orgID string) {
	if h.supFactory == nil {
		return
	}
	h.agentMu.Lock()
	h.agentOrg[agentOrgKey(guildID, agentID)] = normalizeOrganizationID(orgID)
	h.agentMu.Unlock()
}

func (h *ControlQueueHandler) forgetAgentOrganization(guildID, agentID string) {
	if h.supFactory == nil {
		return
	}
	h.agentMu.Lock()
	delete(h.agentOrg, agentOrgKey(guildID, agentID))
	h.agentMu.Unlock()
}

// extractGuildSpec attempts to unmarshal a guild spec from a properties map.
func extractGuildSpec(props map[string]interface{}) *protocol.GuildSpec {
	gsRaw, ok := props["guild_spec"]
	if !ok || gsRaw == nil {
		return nil
	}
	gsBytes, err := json.Marshal(gsRaw)
	if err != nil {
		return nil
	}
	var parsed protocol.GuildSpec
	if err := json.Unmarshal(gsBytes, &parsed); err != nil {
		return nil
	}
	return &parsed
}
