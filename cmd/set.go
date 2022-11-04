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
var cmdSet = &cobra.Command{
	Use:   "set",
	Short: "set [install-firmware]",
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

// command install firmware
type installFirmwareFlags struct {
	force           bool
	deviceID        string
	inventorySource string
}

var (
	installFirmwareFlagSet = &installFirmwareFlags{}
)

var cmdSetInstallFirmware = &cobra.Command{
	Use:   "install-firmware",
	Short: "Set the install firmware attribute on a device",
	Run: func(cmd *cobra.Command, args []string) {
		runSetInstallFirmware(cmd.Context())
	},
}

func runSetInstallFirmware(ctx context.Context) {
	flasher, err := app.New(ctx, model.AppKindClient, workerRunFlagSet.inventorySource, cfgFile, logLevel)
	if err != nil {
		log.Fatal(err)
	}

	switch {
	case strings.HasSuffix(installFirmwareFlagSet.inventorySource, ".yml"), strings.HasSuffix(workerRunFlagSet.inventorySource, ".yaml"):
		fwInstallYAMLInventory(ctx, flasher)
		return

	case installFirmwareFlagSet.inventorySource == model.InventorySourceServerservice:
		fwInstallServerserviceInventory(ctx, flasher)
		return
	default:
		log.Fatal("expected a cli flag")
	}
}

func fwInstallServerserviceInventory(ctx context.Context, flasher *app.App) {
	inv, err := inventory.NewServerserviceInventory(ctx, flasher.Config, flasher.Logger)
	if err != nil {
		flasher.Logger.Fatal(err)
	}

	attrs := &inventory.FwInstallAttributes{
		TaskParameters: model.TaskParameters{ForceInstall: installFirmwareFlagSet.force},
		Requester:      os.Getenv("USER"),
		Status:         string(model.StateRequested),
	}

	if err := inv.SetFlasherAttributes(ctx, installFirmwareFlagSet.deviceID, attrs); err != nil {
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
	rootCmd.AddCommand(cmdSet)

	cmdSetInstallFirmware.PersistentFlags().StringVar(&installFirmwareFlagSet.inventorySource, "inventory-source", "", "inventory source to lookup devices for update - 'serverservice' or an inventory file with a .yml/.yaml extenstion")

	if err := cmdSetInstallFirmware.MarkPersistentFlagRequired("inventory-source"); err != nil {
		log.Fatal(err)
	}

	cmdSetInstallFirmware.PersistentFlags().StringVar(&installFirmwareFlagSet.deviceID, "device-id", "", "inventory device identifier")

	if err := cmdSetInstallFirmware.MarkPersistentFlagRequired("device-id"); err != nil {
		log.Fatal(err)
	}

	cmdSetInstallFirmware.PersistentFlags().BoolVar(&installFirmwareFlagSet.force, "force", false, "force install (ignores currently installed firmware)")

	cmdSet.AddCommand(cmdSetInstallFirmware)
}
