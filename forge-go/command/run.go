package command

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/rustic-ai/forge/forge-go/agent"
)

var (
	redisAddr         string
	registryPath      string
	dbPath            string
	defaultSupervisor string
)

func init() {
	runCmd.Flags().StringVarP(&redisAddr, "redis", "r", "", "Redis connection address (e.g., localhost:6379). If omitted, an embedded miniredis instance is started automatically.")
	runCmd.Flags().StringVar(&registryPath, "registry", "", "Path to the forge-agent-registry.yaml file")
	runCmd.Flags().StringVar(&dbPath, "db-path", "", "Path to the local SQLite database file to use")
	runCmd.Flags().StringVar(&defaultSupervisor, "default-supervisor", "", "Force a specific supervisor (process, docker, bwrap) for all agents")
	RootCmd.AddCommand(runCmd)
}

var runCmd = &cobra.Command{
	Use:   "run <spec-file>",
	Short: "Run a guild locally",
	Long:  `Runs a guild locally from a spec file. By default, this provisions an embedded miniredis pub/sub bus dynamically, but you can target an external broker via --redis.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		specFile := args[0]

		fmt.Printf("Starting Forge Guild from: %s\n", specFile)

		flags := map[string]string{
			"redis":              redisAddr,
			"registry":           registryPath,
			"db-path":            dbPath,
			"default-supervisor": defaultSupervisor,
		}

		cfg, err := agent.LoadConfig(flags, args)
		if err != nil {
			return err
		}

		ctx, cancel := context.WithCancel(context.Background())

		ag, err := agent.StartLocal(ctx, cfg)
		if err != nil {
			cancel()
			return err
		}

		fmt.Println("\nAll agents queued via Control Queue. Guild is active. Press Ctrl+C to terminate.")

		agent.WaitForShutdown(cancel, ag)

		return nil
	},
}
