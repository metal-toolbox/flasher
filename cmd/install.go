package cmd

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/metal-toolbox/flasher/internal/app"
	"github.com/metal-toolbox/flasher/internal/inventory"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/spf13/cobra"
)

// install root command
var cmdInstall = &cobra.Command{
	Use:   "install",
	Short: "install [firmware]",
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

// command install firmware
type installFirmwareFlags struct {
	deviceID        string
	inventorySource string
}

var (
	installFirmwareFlagSet = &installFirmwareFlags{}
)

var cmdInstallFirmware = &cobra.Command{
	Use:   "firmware",
	Short: "Install firmware on a device",
	Run: func(cmd *cobra.Command, args []string) {
		runInstall(cmd.Context())
	},
}

func runInstall(ctx context.Context) {
	flasher, err := app.New(ctx, model.AppKindClient, workerRunFlagSet.inventorySource, cfgFile, logLevel)
	if err != nil {
		log.Fatal(err)
	}

	switch {
	case strings.HasSuffix(installFirmwareFlagSet.inventorySource, ".yml"), strings.HasSuffix(workerRunFlagSet.inventorySource, ".yaml"):
		fwInstallYAMLInventory(ctx, flasher)

	case installFirmwareFlagSet.inventorySource == model.InventorySourceServerservice:
		fwInstallServerserviceInventory(ctx, flasher)
	}
}

func fwInstallServerserviceInventory(ctx context.Context, flasher *app.App) {
	inv, err := inventory.NewServerserviceInventory(flasher.Config)
	if err != nil {
		flasher.Logger.Fatal(err)
	}

	attrs := &inventory.InstallAttributes{
		Status:    string(model.StateQueued),
		Requester: os.Getenv("USER"),
	}

	if err := inv.SetFwInstallAttributes(ctx, installFirmwareFlagSet.deviceID, attrs); err != nil {
		flasher.Logger.Fatal(err)
	}

	flasher.Logger.Info("device flagged for firmware install.")
}

func fwInstallYAMLInventory(ctx context.Context, flasher *app.App) {
	//	inv, err = inventory.NewYamlInventory(deviceFlagSet.inventorySource)
	//	if err != nil {
	//		log.Fatal(err)
	//	}
	//
}

// install command flags
func init() {
	rootCmd.AddCommand(cmdInstall)

	cmdInstallFirmware.PersistentFlags().StringVar(&installFirmwareFlagSet.inventorySource, "inventory-source", "", "inventory source to lookup devices for update - 'serverservice' or an inventory file with a .yml/.yaml extenstion")

	if err := cmdInstallFirmware.MarkPersistentFlagRequired("inventory-source"); err != nil {
		log.Fatal(err)
	}

	cmdInstallFirmware.PersistentFlags().StringVar(&installFirmwareFlagSet.deviceID, "device-id", "", "inventory device identifier")

	if err := cmdInstallFirmware.MarkPersistentFlagRequired("device-id"); err != nil {
		log.Fatal(err)
	}

	cmdInstall.AddCommand(cmdInstallFirmware)
}
