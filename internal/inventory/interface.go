package inventory

import (
	"context"

	"github.com/metal-toolbox/flasher/internal/model"
)

type Inventory interface {
	// DevicesForFwInstall returns a list of devices eligible for firmware installation.
	DevicesForFwInstall(ctx context.Context, limit int) ([]DeviceInventory, error)

	// DeviceByID returns device attributes by its identifier
	DeviceByID(ctx context.Context, id string) (*DeviceInventory, error)

	// AcquireDevice looks up a device by its identifier and flags or locks it for an update.
	//
	// - The implementation is to check if the device is a eligible based its status or other non-firmware inventory attributes.
	// - The locking mechnism is left to the implementation.
	AquireDevice(ctx context.Context, deviceID, workerID string) (DeviceInventory, error)

	// ReleaseDevice looks up a device by its identifier and releases any locks held on the device.
	// The lock release mechnism is left to the implementation.
	ReleaseDevice(ctx context.Context, id string) error

	// SetFlasherAttributes - sets the firmware install attributes for a device.
	SetFlasherAttributes(ctx context.Context, deviceID string, attrs *FwInstallAttributes) error

	// FlasherAttributes - gets the firmware install attributes to the given value for a device.
	FlasherAttributes(ctx context.Context, deviceID string) (FwInstallAttributes, error)

	// DeleteFlasherAttributes - removes the firmware install attributes from a device.
	DeleteFlasherAttributes(ctx context.Context, deviceID string) error

	// FirmwareInstalled returns the component installed firmware versions
	FirmwareInstalled(ctx context.Context, deviceID string) (model.Components, error)

	// FirmwareByDeviceVendorModel returns the firmware for the device vendor, model.
	FirmwareByDeviceVendorModel(ctx context.Context, deviceVendor, deviceModel string) ([]model.Firmware, error)
}

// FwInstallAttributes is the flasher server service attribute stored in serverservice
type FwInstallAttributes struct {
	model.TaskParameters `json:"parameters,omitempty"`

	FlasherTaskID string `json:"flasher_task_id,omitempty"`
	Status        string `json:"status,omitempty"`
	Info          string `json:"info,omitempty"`
	Requester     string `json:"requester,omitempty"`
	WorkerID      string `json:"worker_id,omitempty"`
}

// DeviceInventory objects are returned by the inventory package
type DeviceInventory struct {
	Device              model.Device        `json:"device"`
	Components          model.Components    `json:"components"`
	FwInstallAttributes FwInstallAttributes `json:"fw_install_attributes,omitempty"`
	Firmware            []model.Firmware    `json:"firmware,omitempty"`
}
