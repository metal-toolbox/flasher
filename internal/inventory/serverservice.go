package inventory

import (
	"context"
	"encoding/json"
	"os"

	sservice "go.hollow.sh/serverservice/pkg/api/v1"

	"github.com/google/uuid"

	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/pkg/errors"
)

const (
	// serverservice attribute namespace for flasher task information
	serverAttributeNSFlasherTask = "sh.hollow.flasher.task"

	// serverservice attribute namespace for device vendor, model, serial attributes
	serverAttributeNSVendor = "sh.hollow.server_vendor_attributes"

	// serverservice attribute namespace for the BMC address
	serverAttributeNSBmcAddress = "sh.hollow.bmc_info"
)

var (
	ErrServerserviceTaskList   = errors.New("error in serverservice task list")
	ErrServerserviceTaskCreate = errors.New("error in serverservice task create")
	ErrServerserviceTaskUpdate = errors.New("error in serverservice task update")

	// ErrBMCAddress is returned when an error occurs in the BMC address lookup.
	ErrBMCAddress = errors.New("error in server BMC Address")

	// ErrDeviceState is returned when an error occurs in the device state  lookup.
	ErrDeviceState = errors.New("error in device state")

	// ErrServerserviceAttrObj is retuned when a serverservice attribute is not as expected.
	ErrServerserviceAttrObj = errors.New("serverservice attribute error")

	// ErrServerserviceQuery is returned when a server service query fails.
	ErrServerserviceQuery = errors.New("serverservice query returned error")
)

type Serverservice struct {
	config *model.Config
	client *sservice.Client
}

func NewServerserviceInventory(config *model.Config) (Inventory, error) {
	// TODO: add helper method for OIDC auth
	client, err := sservice.NewClientWithToken("", "", nil)
	if err != nil {
		return nil, err
	}

	return &Serverservice{client: client, config: config}, nil
}

func (s *Serverservice) ListDevicesForFwInstall(ctx context.Context, limit int) ([]model.Device, error) {
	devices := []model.Device{}

	params := &sservice.ServerListParams{
		AttributeListParams: []sservice.AttributeListParams{
			{
				Namespace: serverAttributeNSFlasherTask,
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

func (s *Serverservice) AquireDevice(ctx context.Context, deviceID string) (model.Device, error) {
	// updates the server service attribute
	// - the device should not have any active flasher tasks
	// - the device state should be maintenance
	device, err := s.deviceAttributes(ctx, deviceID)
	if err != nil {
		return *device, err
	}

	// attributes to set
	attrs := &InstallAttributes{
		Status: string(model.StateQueued),
		// TODO(joel): identify user from OIDC login
		Requester: os.Getenv("USER"),
	}

	if err := s.SetFwInstallAttributes(ctx, deviceID, attrs); err != nil {
		return *device, err
	}

	return model.Device{}, nil
}

// ReleaseDevice looks up a device by its identifier and releases any locks held on the device.
// The lock release mechnism is left to the implementation.
func (s *Serverservice) ReleaseDevice(ctx context.Context, id string) error {
	return nil
}

func (s *Serverservice) FirmwareByDeviceVendorModel(ctx context.Context, deviceVendor, deviceModel string) ([]model.Firmware, error) {

	// looks up device inventory
	// looks up firmware
	return nil, nil
}

// FwInstallAttributes - gets the firmware install attributes for the device.
func (s *Serverservice) FwInstallAttributes(ctx context.Context, deviceID string) (InstallAttributes, error) {
	params := InstallAttributes{}

	deviceUUID, err := uuid.Parse(deviceID)
	if err != nil {
		return params, err
	}

	// lookup flasher task attribute
	attributes, _, err := s.client.ListAttributes(ctx, deviceUUID, nil)
	if err != nil {
		return params, err
	}

	// update existing task attribute
	foundAttributes := findAttribute(serverAttributeNSFlasherTask, attributes)
	if foundAttributes == nil {
		return params, errors.Wrap(ErrServerserviceTaskList, "no flasher task present for device: "+deviceID)
	}

	installAttributes := InstallAttributes{}

	if err := json.Unmarshal(foundAttributes.Data, foundAttributes); err != nil {
		return params, errors.Wrap(ErrServerserviceTaskList, err.Error())
	}

	return installAttributes, nil
}

// SetDeviceFwInstallTaskAttributes - sets the firmware install attributes to the given values on a device.
func (s *Serverservice) SetFwInstallAttributes(ctx context.Context, deviceID string, newTaskAttrs *InstallAttributes) error {
	deviceUUID, err := uuid.Parse(deviceID)
	if err != nil {
		return err
	}

	// lookup flasher task attribute
	attributes, _, err := s.client.ListAttributes(ctx, deviceUUID, nil)
	if err != nil {
		return err
	}

	taskAttrs, err := s.flasherTaskAttribute(attributes)
	if err != nil {
		return err
	}

	// update when theres an existing attribute
	if taskAttrs != nil {
		if taskAttrs.Status == string(model.StateActive) ||
			taskAttrs.Status == string(model.StateQueued) {
			return errors.Wrap(
				ErrServerserviceTaskUpdate,
				"task present on device in non finalized state: "+taskAttrs.Status,
			)
		}

		return s.updateInstallAttributes(ctx, deviceUUID, taskAttrs, newTaskAttrs)
	}

	// create new task attribute
	return s.createInstallAttributes(ctx, deviceUUID, newTaskAttrs)
}

func (s *Serverservice) updateInstallAttributes(ctx context.Context, deviceUUID uuid.UUID, currentTaskAttrs, newTaskAttrs *InstallAttributes) error {

	payload, err := json.Marshal(newTaskAttrs)
	if err != nil {
		return errors.Wrap(ErrServerserviceTaskUpdate, err.Error())
	}

	_, err = s.client.UpdateAttributes(ctx, deviceUUID, serverAttributeNSFlasherTask, payload)
	if err != nil {
		return errors.Wrap(ErrServerserviceTaskUpdate, err.Error())
	}

	return nil
}

func (s *Serverservice) createInstallAttributes(ctx context.Context, deviceUUID uuid.UUID, attrs *InstallAttributes) error {
	payload, err := json.Marshal(attrs)
	if err != nil {
		return errors.Wrap(ErrServerserviceTaskCreate, err.Error())
	}

	data := sservice.Attributes{Namespace: serverAttributeNSFlasherTask, Data: payload}

	_, err = s.client.CreateAttributes(ctx, deviceUUID, data)
	if err != nil {
		return err
	}

	return nil
}

// deviceAttributes returns a device object with its fields populated.
//
// The attributes looked up are,
// - BMC address
// - BMC credentials
// - Device vendor, model, serial attributes
// - Device state
func (s *Serverservice) deviceAttributes(ctx context.Context, deviceID string) (*model.Device, error) {
	device := &model.Device{}

	deviceUUID, err := uuid.Parse(deviceID)
	if err != nil {
		return nil, err
	}

	// lookup attributes to acquire device for an update
	attributes, _, err := s.client.ListAttributes(ctx, deviceUUID, nil)
	if err != nil {
		return nil, errors.Wrap(ErrServerserviceQuery, err.Error())
	}

	// bmc address from attribute
	device.BmcAddress, err = s.bmcAddressFromAttributes(attributes)
	if err != nil {
		return nil, err
	}

	// credentials from credential store
	credential, _, err := s.client.GetCredential(ctx, deviceUUID, sservice.ServerCredentialTypeBMC)
	if err != nil {
		return nil, errors.Wrap(ErrServerserviceQuery, err.Error())
	}

	device.BmcUsername = credential.Username
	device.BmcPassword = credential.Password

	// vendor attributes
	device.Vendor, device.Model, device.Serial, err = s.vendorModelFromAttributes(attributes)
	if err != nil {
		return nil, err
	}

	// device state attribute
	device.State, err = s.deviceStateAttribute(attributes)
	if err != nil {
		return nil, err
	}

	return device, nil
}
