package command

import (
	"fmt"
	"runtime"

	"github.com/rustic-ai/forge/forge-go/version"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of Forge",
	Long:  `All software has versions. This is Forge's`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Forge Version: %s\n", version.Version)
		fmt.Printf("Git Commit: %s\n", version.GitCommit)
		fmt.Printf("Build Date: %s\n", version.BuildDate)
		fmt.Printf("Go Version: %s\n", runtime.Version())
		fmt.Printf("OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	},
}
