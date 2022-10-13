package cmd

import (
	"context"
	"log"

	"github.com/davecgh/go-spew/spew"
	"github.com/metal-toolbox/flasher/internal/app"
	"github.com/metal-toolbox/flasher/internal/inventory"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/spf13/cobra"
)

var cmdGet = &cobra.Command{
	Use:   "get",
	Short: "get resources [inventory|firmware|task]",
	Run: func(cmd *cobra.Command, args []string) {
	},
}

// command get task
type getTaskFlags struct {
	deviceID string
}

var (
	getTaskFlagSet = &getTaskFlags{}
)

var cmdGetTask = &cobra.Command{
	Use:   "task",
	Short: "Get firmware install task attributes on a device",
	Run: func(cmd *cobra.Command, args []string) {
		getTask(cmd.Context())
	},
}

func getTask(ctx context.Context) {
	flasher, err := app.New(ctx, model.AppKindClient, workerFlagSet.inventorySource, cfgFile, logLevel)
	if err != nil {
		log.Fatal(err)
	}

	inv, err := inventory.NewServerserviceInventory(flasher.Config)
	if err != nil {
		flasher.Logger.Fatal(err)
	}

	attrs, err := inv.FwInstallAttributes(ctx, installFirmwareFlagSet.deviceID)
	if err != nil {
		flasher.Logger.Fatal(err)
	}

	spew.Dump(attrs)

}

func init() {
	rootCmd.AddCommand(cmdGet)

	cmdGetTask.PersistentFlags().StringVar(&getTaskFlagSet.deviceID, "device-id", "", "inventory device identifier")

	if err := cmdGetTask.MarkPersistentFlagRequired("device-id"); err != nil {
		log.Fatal(err)
	}

	cmdGet.AddCommand(cmdGetTask)
}
