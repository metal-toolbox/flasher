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

func (s *Mock) ListDevicesForFwInstall(ctx context.Context, limit int) ([]model.Device, error) {
	devices := []model.Device{
		fixtures.Devices[fixtures.Device1.String()],
		fixtures.Devices[fixtures.Device2.String()],
	}

	return devices, nil
}

func (s *Mock) AquireDevice(ctx context.Context, id string) (model.Device, error) {
	// updates the server service attribute
	// - the device should not have any active flasher tasks
	// - the device state should be maintenance

	return fixtures.Devices[fixtures.Device1.String()], nil
}

func (s *Mock) FirmwareByDeviceVendorModel(ctx context.Context, deviceVendor, deviceModel string) ([]model.Firmware, error) {
	return fixtures.Firmware, nil
}

// FwInstallAttributes - gets the firmware install attributes for the device.
func (s *Mock) FwInstallAttributes(ctx context.Context, deviceID string) (InstallAttributes, error) {
	return InstallAttributes{}, nil
}

// SetFwInstallAttributes - sets the firmware install attributes to the given values on a device.
func (s *Mock) SetFwInstallAttributes(ctx context.Context, deviceID string, attrs *InstallAttributes) error {
	return nil
}

// ReleaseDevice looks up a device by its identifier and releases any locks held on the device.
func (s *Mock) ReleaseDevice(ctx context.Context, id string) error {
	return nil
}
