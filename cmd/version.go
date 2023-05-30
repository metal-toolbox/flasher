package cmd

import (
	"fmt"

	"github.com/metal-toolbox/flasher/internal/version"
	"github.com/spf13/cobra"
)

var cmdVersion = &cobra.Command{
	Use:   "version",
	Short: "Print Flasher version along with dependency information.",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf(
			"commit: %s\nbranch: %s\ngit summary: %s\nbuildDate: %s\nversion: %s\nGo version: %s\nbmclib version: %s\nserverservice version: %s",
			version.GitCommit, version.GitBranch, version.GitSummary, version.BuildDate, version.AppVersion, version.GoVersion, version.BmclibVersion, version.ServerserviceVersion)

	},
}

func init() {
	rootCmd.AddCommand(cmdVersion)
}
