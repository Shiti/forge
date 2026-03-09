package scheduler

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rustic-ai/forge/forge-go/scheduler/leader"
)

type Reconciler struct {
	registry     *NodeRegistry
	placementMap *PlacementMap
	redisClient  *redis.Client
	elector      leader.LeaderElector
}

func NewReconciler(r *NodeRegistry, p *PlacementMap, rdb *redis.Client, el leader.LeaderElector) *Reconciler {
	return &Reconciler{
		registry:     r,
		placementMap: p,
		redisClient:  rdb,
		elector:      el,
	}
}

func (r *Reconciler) Start(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if r.elector != nil && !r.elector.IsLeader() {
				continue
			}
			r.reconcile(ctx)
		}
	}
}

func (r *Reconciler) reconcile(ctx context.Context) {
	r.registry.mu.RLock()
	var deadNodes []string
	now := time.Now()
	for nodeID, state := range r.registry.nodes {
		if now.Sub(state.LastHeartbeat) > 15*time.Second {
			deadNodes = append(deadNodes, nodeID)
		}
	}
	r.registry.mu.RUnlock()

	for _, nodeID := range deadNodes {
		slog.Default().Warn("Detected dead node, reconciling orphaned agents", "node_id", nodeID)

		orphans := r.placementMap.AgentsOnNode(nodeID)
		r.registry.Deregister(nodeID)

		for _, o := range orphans {
			r.placementMap.Remove(o.GuildID, o.AgentID)

			wrapper := map[string]interface{}{
				"command": "spawn",
			}
			var rawPayload interface{}
			if err := json.Unmarshal(o.Payload, &rawPayload); err == nil {
				wrapper["payload"] = rawPayload
				wrappedBytes, _ := json.Marshal(wrapper)

				r.redisClient.LPush(ctx, "forge:control:requests", wrappedBytes)
				slog.Default().Info("Re-enqueued orphaned agent for cluster redistribution", "guild", o.GuildID, "agent", o.AgentID)
			} else {
				slog.Default().Error("Failed to deserialize orphaned payload buffer", "error", err)
			}
		}
	}
}
