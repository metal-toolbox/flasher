package inventory

import (
	"context"
	"errors"

	"github.com/metal-toolbox/flasher/internal/model"
)

const (
	InventorySourceYAML = "inventorySourceYAML"
)

var (
	ErrYamlSource = errors.New("error in Yaml inventory")
)

// Yaml type implements the inventory interface
type Yaml struct {
	YamlFile     string
	fwConfigFile string
}

// NewYamlInventory returns a Yaml type that implements the inventory interface.
func NewYamlInventory(YamlFile string) (Inventory, error) {
	return &Yaml{YamlFile: YamlFile}, nil
}

func (c *Yaml) ListDevicesForFwInstall(ctx context.Context, limit int) ([]model.Device, error) {
	return nil, nil
}

// AcquireDevice looks up a device by its identifier and flags or locks it for an update.
//
// - The implementation is to check if the device is a eligible based its status or other non-firmware inventory attributes.
// - The locking mechnism is left to the implementation.
func (c *Yaml) AquireDevice(ctx context.Context, id string) (model.Device, error) {
	return model.Device{}, nil
}

// ReleaseDevice looks up a device by its identifier and releases any locks held on the device.
// The lock release mechnism is left to the implementation.
func (c *Yaml) ReleaseDevice(ctx context.Context, id string) error {
	return nil
}

// SetDeviceFwInstallTaskAttributes - sets the firmware install attributes to the given value on a device.
func (c *Yaml) SetDeviceFwInstallTaskAttributes(ctx context.Context, taskID, status, info, workerID string) error {
	return nil
}

// DeviceFwInstallTaskAttributes - gets the firmware install attributes to the given value for a device.
func (c *Yaml) DeviceFwInstallTaskAttributes(ctx context.Context, deviceID string) (model.TaskParameters, error) {
	return model.TaskParameters{}, nil
}

// FirmwareByDeviceVendorModel returns the firmware for the device vendor, model.
func (c *Yaml) FirmwareByDeviceVendorModel(ctx context.Context, deviceVendor, deviceModel string) ([]model.Firmware, error) {
	return nil, nil
}
