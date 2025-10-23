package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	// Version information - can be set via ldflags during build
	Version   = "1.0.0"
	BuildDate = "unknown"
	GitCommit = "unknown"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "runrun",
	Short: "RunRun - Task Execution Platform",
	Long: `RunRun is a web-based task execution platform that allows you to
schedule, execute, and monitor tasks through a modern web interface.

Features:
  - Web UI for task management
  - Real-time log streaming
  - Task execution history
  - User authentication
  - RESTful API`,
	Version: Version,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	// Global flags can be added here if needed
	rootCmd.SetVersionTemplate(fmt.Sprintf("RunRun version %s (built %s, commit %s)\n", Version, BuildDate, GitCommit))
}
