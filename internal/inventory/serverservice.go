package inventory

import (
	"context"

	"github.com/metal-toolbox/flasher/internal/model"
)

type Serverservice struct {
}

func NewServerserviceInventory() (*Serverservice, error) {
	return &Serverservice{}, nil

}

func (s *Serverservice) ListDevicesForFwInstall(ctx context.Context, limit int) ([]model.Device, error) {
	return nil, nil
}

func (s *Serverservice) AquireDeviceByID(ctx context.Context, ID string) (model.Device, error) {
	// updates the server service attribute
	return model.Device{}, nil
}

func (s *Serverservice) FirmwareConfiguration(ctx context.Context, device *model.Device) ([]*model.Firmware, error) {

	return nil, nil
}

// taskAttribute is the server service attribute stored in serverservice
type taskAttribute struct {
	model.TaskParameters

	FlasherTaskID string `json:"flasher_task_id"`
	Status        string `json:"status"`
	Info          string `json:"info"`
	Requester     string `json:"requester"`
	Worker        string `json:"worker"`
}

// DeviceFwInstallTaskAttributes - gets the firmware install attributes for the device.
func (s *Serverservice) DeviceFwInstallTaskAttributes(ctx context.Context, deviceID string) (error, model.TaskParameters) {
	return nil, model.TaskParameters{}
}

// SetDeviceFwInstallTaskAttributes - sets the firmware install attributes to the given values on a device.
func (s *Serverservice) SetDeviceFwInstallTaskAttributes(ctx context.Context, taskID, status, info, worker string) error {
	return nil
}
