package inventory

import (
	"context"

	"github.com/metal-toolbox/flasher/internal/model"
)

type Inventory interface {
	// ListDevicesForFwInstall returns a list of devices eligible for firmware installation.
	ListDevicesForFwInstall(ctx context.Context, limit int) ([]model.Device, error)

	// AcquireDevice looks up a device by its identifier and flags or locks it for an update.
	//
	// - The implementation is to check if the device is a eligible based its status or other non-firmware inventory attributes.
	// - The locking mechnism is left to the implementation.
	AquireDevice(ctx context.Context, id string) (model.Device, error)

	// ReleaseDevice looks up a device by its identifier and releases any locks held on the device.
	// The lock release mechnism is left to the implementation.
	ReleaseDevice(ctx context.Context, id string) error

	// SetDeviceFwInstallTaskAttributes - sets the firmware install attributes to the given value on a device.
	SetDeviceFwInstallTaskAttributes(ctx context.Context, taskID, status, info, workerID string) error

	// DeviceFwInstallTaskAttributes - gets the firmware install attributes to the given value for a device.
	DeviceFwInstallTaskAttributes(ctx context.Context, deviceID string) (model.TaskParameters, error)

	// FirmwareByDeviceVendorModel returns the firmware for the device vendor, model.
	FirmwareByDeviceVendorModel(ctx context.Context, deviceVendor, deviceModel string) ([]model.Firmware, error)
}
