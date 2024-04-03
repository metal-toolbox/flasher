package cmd

import (
	"fmt"

	"github.com/metal-toolbox/flasher/internal/version"
	"github.com/spf13/cobra"
)

var cmdVersion = &cobra.Command{
	Use:   "version",
	Short: "Print Flasher version along with dependency information.",
	Run: func(_ *cobra.Command, args []string) {
		fmt.Printf(
			"commit: %s\nbranch: %s\ngit summary: %s\nbuildDate: %s\nversion: %s\nGo version: %s\nbmclib version: %s\nFleetDB API version: %s",
			version.GitCommit, version.GitBranch, version.GitSummary, version.BuildDate, version.AppVersion, version.GoVersion, version.BmclibVersion, version.FleetDBAPIVersion)

	},
}

func init() {
	rootCmd.AddCommand(cmdVersion)
}
