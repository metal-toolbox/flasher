package inventory

import (
	"context"

	"github.com/metal-toolbox/flasher/internal/model"
)

type Inventory interface {
	// ListDevicesForFwInstall returns a list of devices eligible for firmware installation.
	ListDevicesForFwInstall(ctx context.Context, limit int) ([]model.Device, error)

	// AcquireDeviceByID looks up a device by its identifier and flags or locks it for an update.
	//
	// - The implementation is to check if the device is a eligible based its status or other non-firmware inventory attributes.
	// - The locking mechnism is left to the implementation.
	AquireDeviceByID(ctx context.Context, ID string) (model.Device, error)

	// SetDeviceFwInstallTaskAttributes - sets the firmware install attributes to the given value on a device.
	SetDeviceFwInstallTaskAttributes(ctx context.Context, taskID string, status string, info string) error

	// DeviceFwInstallTaskAttributes - gets the firmware install attributes to the given value for a device.
	DeviceFwInstallTaskAttributes(ctx context.Context, deviceID string) (error, model.TaskParameters)

	// FirmwareConfiguration returns the firmware install configuration applicable on the device
	FirmwareConfiguration(ctx context.Context, device model.Device) ([]model.Firmware, error)
}
