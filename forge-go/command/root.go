package command

import (
	"github.com/spf13/cobra"
)

var (
	logLevel  string
	logFormat string
)

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "forge",
	Short: "Forge is the Rustic AI agent guild orchestrator runtime",
	Long:  `Forge is a cross-platform Go runtime that handles process spawning, monitoring, and orchestration for Rustic AI agent guilds.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return RootCmd.Execute()
}

func init() {
	RootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	RootCmd.PersistentFlags().StringVar(&logFormat, "log-format", "text", "Log format (text, json)")
}
