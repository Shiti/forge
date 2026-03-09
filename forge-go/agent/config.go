package agent

import (
	"errors"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	SpecFile          string
	RedisAddr         string
	RegistryPath      string
	DBPath            string
	DefaultSupervisor string
}

type ServerConfig struct {
	DatabaseURL             string
	RedisURL                string
	EmbeddedRedisAddr       string
	ListenAddress           string
	ManagerAPIBaseURL       string
	DataDir                 string
	DependencyConfig        string
	WithClient              bool
	ClientNodeID            string
	ClientMetricsAddr       string
	ClientCPUs              int
	ClientMemory            int
	ClientGPUs              int
	ClientDefaultSupervisor string
	LeaderElectionMode      string   // "redis" or "raft"
	RaftBindAddr            string   // e.g. "127.0.0.1:8500"
	GossipBindAddr          string   // e.g. "127.0.0.1:8400"
	GossipJoinPeers         []string // e.g. ["127.0.0.1:8400"]
}

type ClientConfig struct {
	ServerURL         string
	RedisURL          string
	CPUs              int
	Memory            int
	GPUs              int
	NodeID            string
	MetricsAddr       string
	DefaultSupervisor string // Allows operator to force docker/bwrap etc
}

func LoadConfig(flags map[string]string, args []string) (*Config, error) {
	v := viper.New()
	v.SetEnvPrefix("FORGE")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	v.SetConfigName("forge")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("$HOME/.forge")
	_ = v.ReadInConfig()

	for key, val := range flags {
		v.Set(key, val)
	}

	if len(args) == 0 {
		return nil, errors.New("spec file required")
	}
	specFile := args[0]

	cfg := &Config{
		SpecFile:          specFile,
		RedisAddr:         v.GetString("redis"),
		RegistryPath:      v.GetString("registry"),
		DBPath:            v.GetString("db-path"),
		DefaultSupervisor: v.GetString("default-supervisor"),
	}

	if cfg.DBPath == "" {
		cfg.DBPath = "forge.db"
	}

	return cfg, nil
}
