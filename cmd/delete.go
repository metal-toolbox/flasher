package cmd

import (
	"context"
	"log"

	"github.com/metal-toolbox/flasher/internal/app"
	"github.com/metal-toolbox/flasher/internal/inventory"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/spf13/cobra"
)

var cmdDelete = &cobra.Command{
	Use:   "delete",
	Short: "delete resources [task]",
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

// command get task
type deleteTaskFlags struct {
	deviceID string
}

var (
	deleteTaskFlagSet = &deleteTaskFlags{}
)

var cmdDeleteTask = &cobra.Command{
	Use:   "task",
	Short: "Delete the firmware install task attributes on a device",
	Run: func(cmd *cobra.Command, args []string) {
		deleteTask(cmd.Context())
	},
}

func deleteTask(ctx context.Context) {
	flasher, err := app.New(ctx, model.AppKindClient, model.InventorySourceServerservice, cfgFile, logLevel)
	if err != nil {
		log.Fatal(err)
	}

	inv, err := inventory.NewServerserviceInventory(ctx, flasher.Config, flasher.Logger)
	if err != nil {
		flasher.Logger.Fatal(err)
	}

	err = inv.DeleteFlasherAttributes(ctx, deleteTaskFlagSet.deviceID)
	if err != nil {
		flasher.Logger.Fatal(err)
	}

	flasher.Logger.Info("flasher attribute removed from device.")
}

func init() {
	rootCmd.AddCommand(cmdDelete)

	cmdDeleteTask.PersistentFlags().StringVar(&deleteTaskFlagSet.deviceID, "device-id", "", "inventory device identifier")

	if err := cmdDeleteTask.MarkPersistentFlagRequired("device-id"); err != nil {
		log.Fatal(err)
	}

	cmdDelete.AddCommand(cmdDeleteTask)
}
