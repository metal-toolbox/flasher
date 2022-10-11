package inventory

import (
	"context"

	sservice "go.hollow.sh/serverservice/pkg/api/v1"

	"github.com/metal-toolbox/flasher/internal/model"
)

const (
	namespaceAttributeFlasherTask = "sh.hollow.flasher.task"
)

type Serverservice struct {
	client *sservice.Client
}

func NewServerserviceInventory() (*Serverservice, error) {
	// TODO: add helper method for OIDC auth
	client, err := sservice.NewClientWithToken("", "", nil)
	if err != nil {
		return nil, err
	}

	return &Serverservice{client: client}, nil
}

func (s *Serverservice) ListDevicesForFwInstall(ctx context.Context, limit int) ([]model.Device, error) {
	devices := []model.Device{}

	params := &sservice.ServerListParams{
		AttributeListParams: []sservice.AttributeListParams{
			{
				Namespace: namespaceAttributeFlasherTask,
				Keys:      []string{"status"},
				Operator:  sservice.OperatorEqual,
				Value:     "",
			},
		},
	}

	found, _, err := s.client.List(ctx, params)
	if err != nil {
		return devices, err
	}

	if len(found) == 0 {
		return devices, nil
	}

	return s.convertServersToDevices(ctx, found)
}

func (s *Serverservice) convertServersToDevices(ctx context.Context, servers []sservice.Server) ([]model.Device, error) {
	return []model.Device{}, nil
}

func (s *Serverservice) AquireDevice(ctx context.Context, id string) (model.Device, error) {
	// updates the server service attribute
	// - the device should not have any active flasher tasks
	// - the device state should be maintenance

	return model.Device{}, nil
}

func (s *Serverservice) FirmwareByDeviceVendorModel(ctx context.Context, deviceVendor, deviceModel string) ([]model.Firmware, error) {

	// looks up device inventory
	// looks up firmware
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
