package inventory

import (
	"context"

	"github.com/metal-toolbox/flasher/internal/fixtures"
	"github.com/metal-toolbox/flasher/internal/model"
)

type Mock struct{}

func NewMockInventory() (Inventory, error) {
	return &Mock{}, nil
}

// DeviceByID returns device attributes by its identifier
func (s *Mock) DeviceByID(ctx context.Context, id string) (*InventoryDevice, error) {
	return nil, nil
}

func (s *Mock) DevicesForFwInstall(ctx context.Context, limit int) ([]InventoryDevice, error) {
	devices := []InventoryDevice{
		{Device: fixtures.Devices[fixtures.Device1.String()]},
		{Device: fixtures.Devices[fixtures.Device2.String()]},
	}

	return devices, nil
}

func (s *Mock) AquireDevice(ctx context.Context, deviceID, workerID string) (InventoryDevice, error) {
	// updates the server service attribute
	// - the device should not have any active flasher tasks
	// - the device state should be maintenance

	return InventoryDevice{Device: fixtures.Devices[fixtures.Device1.String()]}, nil
}

func (s *Mock) FirmwareByDeviceVendorModel(ctx context.Context, deviceVendor, deviceModel string) ([]model.Firmware, error) {
	return fixtures.Firmware, nil
}

// FlasherAttributes - gets the firmware install attributes for the device.
func (s *Mock) FlasherAttributes(ctx context.Context, deviceID string) (FwInstallAttributes, error) {
	return FwInstallAttributes{}, nil
}

// SetFlasherAttributes - sets the firmware install attributes to the given values on a device.
func (s *Mock) SetFlasherAttributes(ctx context.Context, deviceID string, attrs *FwInstallAttributes) error {
	return nil
}

// DeleteFlasherAttributes - removes the firmware install attributes from a device.
func (s *Mock) DeleteFlasherAttributes(ctx context.Context, deviceID string) error {
	return nil
}

// ReleaseDevice looks up a device by its identifier and releases any locks held on the device.
func (s *Mock) ReleaseDevice(ctx context.Context, id string) error {
	return nil
}

// FirmwareInstalled returns the component installed firmware versions
func (s *Mock) FirmwareInstalled(ctx context.Context, deviceID string) (model.Components, error) {
	return nil, nil
}
