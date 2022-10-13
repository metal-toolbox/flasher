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

	// SetFwInstallAttributes - sets the firmware install attributes for a device.
	SetFwInstallAttributes(ctx context.Context, deviceID string, attrs *InstallAttributes) error

	// FwInstallAttributes - gets the firmware install attributes to the given value for a device.
	FwInstallAttributes(ctx context.Context, deviceID string) (InstallAttributes, error)

	// FirmwareByDeviceVendorModel returns the firmware for the device vendor, model.
	FirmwareByDeviceVendorModel(ctx context.Context, deviceVendor, deviceModel string) ([]model.Firmware, error)
}

// InstallAttributes is the server service attribute stored in serverservice
type InstallAttributes struct {
	model.TaskParameters `json:"parameters"`

	FlasherTaskID string `json:"flasher_task_id"`
	Status        string `json:"status"`
	Info          string `json:"info"`
	Requester     string `json:"requester"`
	Worker        string `json:"worker"`
}
