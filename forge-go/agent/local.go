package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/rustic-ai/forge/forge-go/api"
	"github.com/rustic-ai/forge/forge-go/control"
	"github.com/rustic-ai/forge/forge-go/embed"
	"github.com/rustic-ai/forge/forge-go/filesystem"
	"github.com/rustic-ai/forge/forge-go/guild"
	"github.com/rustic-ai/forge/forge-go/guild/store"
	"github.com/rustic-ai/forge/forge-go/messaging"
	"github.com/rustic-ai/forge/forge-go/protocol"
	"github.com/rustic-ai/forge/forge-go/registry"
	"github.com/rustic-ai/forge/forge-go/secrets"
)

// StartLocal bootstraps the central agent coordinator using local settings, starts Miniredis (if not provided),
// connects the control loops, and writes the Spawn payload directly onto the queue mimicking distributed logic.
func StartLocal(ctx context.Context, cfg *Config) (*Agent, error) {
	guildSpec, _, err := guild.ParseFile(cfg.SpecFile)
	if err != nil {
		return nil, fmt.Errorf("failed to parse guild spec: %w", err)
	}

	builtSpec, err := guild.GuildBuilderFromSpec(guildSpec).BuildSpec()
	if err != nil {
		return nil, fmt.Errorf("failed to build guild spec: %w", err)
	}

	builtSpec.Properties["execution_engine"] = "rustic_ai.forge.execution_engine.ForgeExecutionEngine"
	builtSpec.Properties["messaging"] = map[string]interface{}{
		"backend_module": "rustic_ai.redis.messaging.backend",
		"backend_class":  "RedisMessagingBackend",
	}

	dbStore, err := store.NewGormStore(store.DriverSQLite, cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize sqlite store: %w", err)
	}

	gm := store.FromGuildSpec(builtSpec, "local")
	if err := dbStore.CreateGuild(gm); err != nil {
		return nil, fmt.Errorf("failed to persist guild specification to database: %w", err)
	}

	redisAddr := cfg.RedisAddr
	if redisAddr == "" {
		log.Println("No redis address provided. Booting Embedded Miniredis...")
		er, err := embed.StartEmbeddedRedis()
		if err != nil {
			return nil, fmt.Errorf("failed to start miniredis: %w", err)
		}
		redisAddr = er.Addr()
		os.Setenv("FORGE_CLIENT_TYPE", "RedisMessagingBackend")
		os.Setenv("FORGE_CLIENT_PROPERTIES_JSON", fmt.Sprintf(`{"redis_client": {"host": "%s", "port": %s, "db": 0}}`, er.Host(), er.Port()))
	}

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("local redis connection failed: %w", err)
	}

	managerAPIBaseURL := strings.TrimSpace(os.Getenv("FORGE_MANAGER_API_BASE_URL"))
	if managerAPIBaseURL != "" && !managerAPIHealthy(ctx, managerAPIBaseURL) {
		log.Printf("Configured FORGE_MANAGER_API_BASE_URL is unreachable; starting local manager API at a dynamic loopback port: %s", managerAPIBaseURL)
		managerAPIBaseURL = ""
	}
	if managerAPIBaseURL == "" {
		managerListen := strings.TrimSpace(os.Getenv("FORGE_MANAGER_LOCAL_LISTEN"))
		if managerListen == "" {
			discovered, err := discoverLoopbackListenAddress()
			if err != nil {
				return nil, fmt.Errorf("failed to allocate local manager listen address: %w", err)
			}
			managerListen = discovered
		}
		managerAPIBaseURL = "http://" + managerListen

		fsBasePath := filepath.Join(filepath.Dir(cfg.DBPath), "workspaces")
		if err := os.MkdirAll(fsBasePath, 0o755); err != nil {
			return nil, fmt.Errorf("failed to create local workspace base path: %w", err)
		}
		resolver := filesystem.NewFileSystemResolver(fsBasePath)
		fileStore := filesystem.NewLocalFileStore(resolver)
		msgClient := messaging.NewClient(rdb)

		managerServer := api.NewServer(dbStore, rdb, msgClient, fileStore, managerListen)
		go func() {
			if err := managerServer.Start(ctx); err != nil && ctx.Err() == nil {
				log.Printf("local manager API exited with error: %v", err)
			}
		}()

		deadline := time.Now().Add(5 * time.Second)
		for {
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("local manager API did not become ready at %s", managerAPIBaseURL)
			}
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, managerAPIBaseURL+"/healthz", nil)
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					break
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

	reg, err := registry.Load(cfg.RegistryPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load agent registry: %w", err)
	}

	// Apply env-driven filesystem and network injection for Docker/Bwrap modes.
	// FORGE_INJECT_FS: comma-separated "path:mode" pairs (e.g. "/data:rw,/config:ro")
	// FORGE_INJECT_NET: comma-separated hosts (e.g. "host" or "api.example.com:443")
	if injectFS := os.Getenv("FORGE_INJECT_FS"); injectFS != "" {
		for _, entry := range strings.Split(injectFS, ",") {
			parts := strings.SplitN(strings.TrimSpace(entry), ":", 2)
			mode := "rw"
			if len(parts) == 2 {
				mode = parts[1]
			}
			for _, a := range builtSpec.Agents {
				_ = reg.InjectFilesystem(a.ClassName, registry.FilesystemPermission{Path: parts[0], Mode: mode})
			}
			// Also inject for the manager agent
			_ = reg.InjectFilesystem(guild.GuildManagerClassName, registry.FilesystemPermission{Path: parts[0], Mode: mode})
		}
	}
	if injectNet := os.Getenv("FORGE_INJECT_NET"); injectNet != "" {
		nets := strings.Split(injectNet, ",")
		for i := range nets {
			nets[i] = strings.TrimSpace(nets[i])
		}
		for _, a := range builtSpec.Agents {
			_ = reg.InjectNetwork(a.ClassName, nets)
		}
		_ = reg.InjectNetwork(guild.GuildManagerClassName, nets)
	}

	for _, a := range builtSpec.Agents {
		if _, err := reg.Lookup(a.ClassName); err != nil {
			return nil, fmt.Errorf("agent %s requests unregistered class %s: %w", a.ID, a.ClassName, err)
		}
	}
	if _, err := reg.Lookup(guild.GuildManagerClassName); err != nil {
		return nil, fmt.Errorf("system missing required orchestrator class %s: %w", guild.GuildManagerClassName, err)
	}

	supervisorFactory := buildOrgSupervisorFactory(rdb, cfg.DefaultSupervisor)
	sec := secrets.NewChainSecretProvider(secrets.NewEnvSecretProvider())
	cq := control.NewControlQueueHandlerWithFactory(rdb, reg, sec, supervisorFactory, dbStore)

	coordinator := NewAgent(cfg, rdb, nil, reg, sec, cq)
	if err := coordinator.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start control queue handler: %w", err)
	}

	time.Sleep(50 * time.Millisecond)

	managerID := builtSpec.ID + "#manager_agent"
	managerSpec := protocol.AgentSpec{
		ID:          managerID,
		Name:        builtSpec.ID + " Manager",
		Description: "System Agent handling guild lifecycle via ForgeExecutionEngine",
		ClassName:   guild.GuildManagerClassName,
		Properties: map[string]interface{}{
			"guild_spec":           builtSpec,
			"manager_api_base_url": managerAPIBaseURL,
			"organization_id":      "local",
			"manager_api_token":    strings.TrimSpace(os.Getenv("FORGE_MANAGER_API_TOKEN")),
		},
		AdditionalTopics: []string{
			"system_topic",
			"heartbeat_topic",
			"guild_status_topic",
		},
		ListenToDefaultTopic: boolPtr(false),
	}

	req := &protocol.SpawnRequest{
		RequestID: uuid.New().String(),
		GuildID:   builtSpec.ID,
		AgentSpec: managerSpec,
	}

	wrapper := map[string]interface{}{
		"command": "spawn",
		"payload": req,
	}

	wb, err := json.Marshal(wrapper)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal manager spawn request: %w", err)
	}

	log.Printf("Queueing SpawnRequest for orchestrator agent: %s (%s)", managerSpec.Name, managerSpec.ClassName)
	if err := rdb.LPush(ctx, control.ControlQueueRequestKey, wb).Err(); err != nil {
		return nil, fmt.Errorf("failed to submit SpawnRequest for orchestrator: %w", err)
	}

	return coordinator, nil
}

func boolPtr(b bool) *bool { return &b }

func discoverLoopbackListenAddress() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		return "", err
	}
	return addr, nil
}

func managerAPIHealthy(ctx context.Context, baseURL string) bool {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/healthz", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
