package cmd

import "github.com/spf13/cobra"

type deviceFlags struct {
	inventorySource string
}

var (
	deviceFlagSet = &workerFlags{}
)

var cmdDevice = &cobra.Command{
	Use:   "device",
	Short: "Query a device or flag it for firmware install",
	Run: func(cmd *cobra.Command, args []string) {
		runWorker(cmd.Context())
	},
}
