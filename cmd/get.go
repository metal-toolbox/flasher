package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/metal-toolbox/flasher/internal/app"
	"github.com/metal-toolbox/flasher/internal/inventory"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/spf13/cobra"
)

var cmdGet = &cobra.Command{
	Use:   "get",
	Short: "get resources [device|task]",
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
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
	Short: "Get firmware install task attributes for a device",
	Run: func(cmd *cobra.Command, args []string) {
		getTask(cmd.Context())
	},
}

func getTask(ctx context.Context) {
	flasher, err := app.New(ctx, model.AppKindClient, model.InventorySourceServerservice, cfgFile, logLevel)
	if err != nil {
		log.Fatal(err)
	}

	inv, err := inventory.NewServerserviceInventory(ctx, flasher.Config, flasher.Logger)
	if err != nil {
		flasher.Logger.Fatal(err)
	}

	attrs, err := inv.FlasherAttributes(ctx, getTaskFlagSet.deviceID)
	if err != nil {
		if errors.Is(err, inventory.ErrNoAttributes) {
			flasher.Logger.Info(err.Error() + ": " + getTaskFlagSet.deviceID)
			return
		}

		flasher.Logger.Fatal(err)
	}

	b, err := json.MarshalIndent(attrs, "  ", "   ")
	if err != nil {
		flasher.Logger.Fatal(err)
	}

	fmt.Println(string(b))
}

var cmdGetDevice = &cobra.Command{
	Use:   "device",
	Short: "Get firmware install attributes for a device",
	Run: func(cmd *cobra.Command, args []string) {
		getDevice(cmd.Context())
	},
}

func getDevice(ctx context.Context) {
	flasher, err := app.New(ctx, model.AppKindClient, model.InventorySourceServerservice, cfgFile, logLevel)
	if err != nil {
		log.Fatal(err)
	}

	inv, err := inventory.NewServerserviceInventory(ctx, flasher.Config, flasher.Logger)
	if err != nil {
		flasher.Logger.Fatal(err)
	}

	device, err := inv.DeviceByID(ctx, getTaskFlagSet.deviceID)
	if err != nil {
		flasher.Logger.Fatal(err)
	}

	if device.Device.Vendor != "" && device.Device.Model != "" {
		var firmware []model.Firmware

		firmware, err = inv.FirmwareByDeviceVendorModel(ctx, device.Device.Vendor, device.Device.Model)
		if err != nil {
			flasher.Logger.WithField("err", err.Error()).Error("Error in firmware set lookup for device")
		}

		device.Firmware = firmware
	} else {
		flasher.Logger.Error("device vendor/model attributes not available, unable to determine applicable firmware")
	}

	b, err := json.MarshalIndent(device, "", "  ")
	if err != nil {
		flasher.Logger.Fatal(err)
	}

	fmt.Println(string(b))
}

func init() {
	rootCmd.AddCommand(cmdGet)

	cmdGetTask.PersistentFlags().StringVar(&getTaskFlagSet.deviceID, "device-id", "", "inventory device identifier")

	if err := cmdGetTask.MarkPersistentFlagRequired("device-id"); err != nil {
		log.Fatal(err)
	}

	cmdGetDevice.PersistentFlags().StringVar(&getTaskFlagSet.deviceID, "device-id", "", "inventory device identifier")

	if err := cmdGetDevice.MarkPersistentFlagRequired("device-id"); err != nil {
		log.Fatal(err)
	}

	cmdGet.AddCommand(cmdGetTask)
	cmdGet.AddCommand(cmdGetDevice)
}
