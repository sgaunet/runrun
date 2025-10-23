package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	// showDetailed controls whether to show detailed version information
	showDetailed bool
)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Long: `Display version information for RunRun including build details,
Go version, and platform information.`,
	Run: showVersion,
}

func init() {
	rootCmd.AddCommand(versionCmd)

	versionCmd.Flags().BoolVarP(&showDetailed, "detailed", "d", false, "Show detailed version information")
}

func showVersion(cmd *cobra.Command, args []string) {
	if showDetailed {
		fmt.Printf("RunRun Task Execution Platform\n")
		fmt.Printf("Version:     %s\n", Version)
		fmt.Printf("Build Date:  %s\n", BuildDate)
		fmt.Printf("Git Commit:  %s\n", GitCommit)
		fmt.Printf("Go Version:  %s\n", runtime.Version())
		fmt.Printf("Platform:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
		fmt.Printf("Compiler:    %s\n", runtime.Compiler)
	} else {
		fmt.Printf("runrun version %s\n", Version)
	}
}
