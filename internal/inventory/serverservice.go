package inventory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	sservice "go.hollow.sh/serverservice/pkg/api/v1"
	"golang.org/x/exp/slices"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/pkg/errors"
)

const (
	// serverservice attribute namespace for flasher task information
	serverAttributeNSFlasherTask = "sh.hollow.flasher.task"

	// serverservice attribute namespace for device vendor, model, serial attributes
	serverAttributeNSVendor = "sh.hollow.alloy.server_vendor_attributes"

	// serverservice attribute namespace for the BMC address
	serverAttributeNSBmcAddress = "sh.hollow.bmc_info"

	// serverservice attribute namespace for firmware set labels
	firmwareAttributeNSFirmwareSetLabels = "sh.hollow.firmware_set.labels"

	component = "inventory.serverservice"
)

var (
	ErrNoAttributes                  = errors.New("no flasher attribute found")
	ErrAttributeList                 = errors.New("error in serverservice flasher attribute list")
	ErrAttributeCreate               = errors.New("error in serverservice flasher attribute create")
	ErrAttributeUpdate               = errors.New("error in serverservice flasher attribute update")
	ErrVendorModelAttributesNotFound = errors.New("vendor, model attributes not found in serverservice")

	ErrDeviceID = errors.New("device UUID error")

	// ErrBMCAddress is returned when an error occurs in the BMC address lookup.
	ErrBMCAddress = errors.New("error in server BMC Address")

	// ErrDeviceState is returned when an error occurs in the device state  lookup.
	ErrDeviceState = errors.New("error in device state")

	// ErrServerserviceAttrObj is retuned when an error occurred in unpacking the attribute.
	ErrServerserviceAttrObj = errors.New("serverservice attribute error")

	// ErrServerserviceVersionedAttrObj is retuned when an error occurred in unpacking the versioned attribute.
	ErrServerserviceVersionedAttrObj = errors.New("serverservice versioned attribute error")

	// ErrServerserviceQuery is returned when a server service query fails.
	ErrServerserviceQuery = errors.New("serverservice query returned error")

	ErrFirmwareSetLookup = errors.New("firmware set error")
)

type Serverservice struct {
	config *model.Config
	// componentSlugs map[string]string
	client *sservice.Client
	logger *logrus.Logger
}

func NewServerserviceInventory(ctx context.Context, config *model.Config, logger *logrus.Logger) (Inventory, error) {
	// TODO: add helper method for OIDC auth
	client, err := sservice.NewClientWithToken("fake", config.Endpoint, nil)
	if err != nil {
		return nil, err
	}

	serverservice := &Serverservice{
		client: client,
		config: config,
		//	componentSlugs: map[string]string{},
		logger: logger,
	}

	// cache component type slugs
	//	componentTypes, _, err := client.ListServerComponentTypes(ctx, nil)
	//	if err != nil {
	//		return nil, errors.Wrap(ErrServerserviceQuery, err.Error())
	//	}
	//
	//	for _, ct := range componentTypes {
	//		serverservice.componentSlugs[ct.ID] = ct.Slug
	//	}

	return serverservice, nil
}

// DeviceByID returns device attributes by its identifier
func (s *Serverservice) DeviceByID(ctx context.Context, id string) (*DeviceInventory, error) {
	deviceUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, errors.Wrap(ErrDeviceID, err.Error()+id)
	}

	device, installAttributes, err := s.deviceWithFwInstallAttributes(ctx, id)
	if err != nil {
		return nil, err
	}

	if device == nil {
		return nil, errors.Wrap(ErrServerserviceQuery, "got nil device object")
	}

	components, _, err := s.client.GetComponents(ctx, deviceUUID, nil)
	if err != nil {
		return nil, errors.Wrap(ErrServerserviceQuery, "device component query error: "+err.Error())
	}

	inventoryDevice := &DeviceInventory{
		Device:     *device,
		Components: s.fromServerserviceComponents(components),
	}

	if installAttributes != nil {
		inventoryDevice.FwInstallAttributes = *installAttributes
	}

	return inventoryDevice, nil
}

func (s *Serverservice) DevicesForFwInstall(ctx context.Context, limit int) ([]DeviceInventory, error) {
	params := &sservice.ServerListParams{
		FacilityCode: s.config.FacilityCode,
		AttributeListParams: []sservice.AttributeListParams{
			{
				Namespace: serverAttributeNSFlasherTask,
				Keys:      []string{"status"},
				Operator:  sservice.OperatorEqual,
				Value:     string(model.StateRequested),
			},
		},
	}

	found, _, err := s.client.List(ctx, params)
	if err != nil {
		return nil, err
	}

	if len(found) == 0 {
		return nil, nil
	}

	return s.convertServersToInventoryDeviceObjs(ctx, found)
}

func (s *Serverservice) convertServersToInventoryDeviceObjs(ctx context.Context, servers []sservice.Server) ([]DeviceInventory, error) {
	devices := make([]DeviceInventory, 0, len(servers))

	for _, server := range servers {
		device, fwInstallAttributes, err := s.deviceWithFwInstallAttributes(ctx, server.UUID.String())
		if err != nil {
			s.logger.WithFields(
				logrus.Fields{
					"component": component,
					"deviceID":  server.UUID.String(),
					"err":       err.Error(),
				},
			).Warn("error in device attribute lookup")

			continue
		}

		// check device state is acceptable for firmware install
		if device.State != "" && !slices.Contains(s.config.DeviceStates, device.State) {
			s.logger.WithFields(
				logrus.Fields{
					"component": component,
					"deviceID":  server.UUID.String(),
				},
			).Trace("device skipped, inventory state is not one of: ", strings.Join(s.config.DeviceStates, ", "))

			continue
		}

		device.ID = server.UUID

		devices = append(
			devices,
			DeviceInventory{Device: *device, FwInstallAttributes: *fwInstallAttributes},
		)
	}

	return devices, nil
}

func (s *Serverservice) AquireDevice(ctx context.Context, deviceID, workerID string) (DeviceInventory, error) {
	// updates the server service attribute
	// - the device should not have any active flasher tasks
	// - the device state should be maintenance
	device, fwInstallAttributes, err := s.deviceWithFwInstallAttributes(ctx, deviceID)
	if err != nil {
		return DeviceInventory{}, errors.Wrap(ErrAttributeList, err.Error())
	}

	fwInstallAttributes.Status = string(model.StateQueued)
	fwInstallAttributes.WorkerID = workerID

	if err := s.SetFlasherAttributes(ctx, deviceID, fwInstallAttributes); err != nil {
		return DeviceInventory{}, errors.Wrap(ErrAttributeUpdate, err.Error())
	}

	return DeviceInventory{Device: *device, FwInstallAttributes: *fwInstallAttributes}, nil
}

// ReleaseDevice looks up a device by its identifier and releases any locks held on the device.
// The lock release mechnism is left to the implementation.
func (s *Serverservice) ReleaseDevice(ctx context.Context, id string) error {
	return nil
}

func (s *Serverservice) FirmwareSetByDeviceVendorModel(ctx context.Context, deviceVendor, deviceModel string) ([]model.Firmware, error) {
	// looks up device inventory
	// looks up firmware

	return nil, nil
}

// FlasherAttributes - gets the firmware install attributes for the device.
func (s *Serverservice) FlasherAttributes(ctx context.Context, deviceID string) (FwInstallAttributes, error) {
	params := FwInstallAttributes{}

	deviceUUID, err := uuid.Parse(deviceID)
	if err != nil {
		return params, errors.Wrap(ErrDeviceID, err.Error()+deviceID)
	}

	// lookup flasher task attribute
	attributes, _, err := s.client.ListAttributes(ctx, deviceUUID, nil)
	if err != nil {
		return params, errors.Wrap(err, ErrAttributeList.Error())
	}

	// update existing task attribute
	foundAttributes := findAttribute(serverAttributeNSFlasherTask, attributes)
	if foundAttributes == nil {
		return params, ErrNoAttributes
	}

	installAttributes := FwInstallAttributes{}

	if err := json.Unmarshal(foundAttributes.Data, &installAttributes); err != nil {
		return params, errors.Wrap(ErrAttributeList, err.Error())
	}

	return installAttributes, nil
}

// SetDeviceFwInstallTaskAttributes - sets the firmware install attributes to the given values on a device.
func (s *Serverservice) SetFlasherAttributes(ctx context.Context, deviceID string, newTaskAttrs *FwInstallAttributes) error {
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

	// create firmware install attributes when theres none.
	if taskAttrs == nil {
		// create new task attribute
		return s.createFlasherAttributes(ctx, deviceUUID, newTaskAttrs)
	}

	// device already flagged for install
	if taskAttrs.Status == string(model.StateRequested) && newTaskAttrs.Status == string(model.StateRequested) {
		return errors.Wrap(
			ErrAttributeUpdate,
			"device already flagged for firmware install",
		)
	}

	// update firmware install attributes
	return s.updateFlasherAttributes(ctx, deviceUUID, taskAttrs, newTaskAttrs)
}

// DeleteFlasherAttributes - removes the firmware install attributes from a device.
func (s *Serverservice) DeleteFlasherAttributes(ctx context.Context, deviceID string) error {
	deviceUUID, err := uuid.Parse(deviceID)
	if err != nil {
		return err
	}

	_, err = s.client.DeleteAttributes(ctx, deviceUUID, serverAttributeNSFlasherTask)
	return err
}

// FirmwareInstalled returns the component installed firmware versions
func (s *Serverservice) FirmwareInstalled(ctx context.Context, deviceID string) (model.Components, error) {
	deviceUUID, err := uuid.Parse(deviceID)
	if err != nil {
		return nil, errors.Wrap(ErrDeviceID, err.Error()+": "+deviceID)
	}

	components, _, err := s.client.GetComponents(ctx, deviceUUID, nil)
	if err != nil {
		return nil, err
	}

	converted := model.Components{}

	for _, component := range components {
		if len(component.VersionedAttributes) == 0 {
			s.logger.WithFields(logrus.Fields{
				"slug":     component.ComponentTypeSlug,
				"deviceID": deviceID,
			}).Trace("component skipped - no versioned attributes")

			continue
		}

		installed, err := installedFirmwareFromVA(component.VersionedAttributes[1])
		if err != nil {
			s.logger.WithFields(logrus.Fields{
				"slug":     component.ComponentTypeSlug,
				"deviceID": deviceID,
				"err":      err.Error(),
			}).Trace("component skipped - versioned attribute error")
		}

		c := &model.Component{
			Slug:              component.ComponentTypeSlug,
			Vendor:            component.Vendor,
			Model:             component.Model,
			Serial:            component.Serial,
			FirmwareInstalled: installed,
		}

		converted = append(converted, c)
	}

	return converted, nil
}

//func cacheServerComponentTypes(ctx context.Context) error {
//	s.componentSlugs = map[string]string{}
//
//	return nil
//}
//

// FirmwareByDeviceVendorModel returns the firmware for the device vendor, model.
func (s *Serverservice) FirmwareByDeviceVendorModel(ctx context.Context, deviceVendor, deviceModel string) ([]model.Firmware, error) {
	// lookup flasher task attribute
	params := &sservice.ComponentFirmwareSetListParams{
		AttributeListParams: []sservice.AttributeListParams{
			{
				Namespace: firmwareAttributeNSFirmwareSetLabels,
				Keys:      []string{"model"},
				Operator:  "eq",
				Value:     deviceModel,
			},
			{
				Namespace: firmwareAttributeNSFirmwareSetLabels,
				Keys:      []string{"vendor"},
				Operator:  "eq",
				Value:     deviceVendor,
			},
		},
	}

	firmwaresets, _, err := s.client.ListServerComponentFirmwareSet(ctx, params)
	if err != nil {
		return nil, errors.Wrap(ErrServerserviceQuery, err.Error())
	}

	if len(firmwaresets) == 0 {
		return nil, errors.Wrap(
			ErrFirmwareSetLookup,
			fmt.Sprintf(
				"lookup by device vendor: %s, model: %s return no firmware set",
				deviceVendor,
				deviceModel,
			),
		)
	}

	if len(firmwaresets) > 1 {
		return nil, errors.Wrap(
			ErrFirmwareSetLookup,
			fmt.Sprintf(
				"lookup by device vendor: %s, model: %s returned multiple firmware sets, expected one",
				deviceVendor,
				deviceModel,
			),
		)
	}

	if len(firmwaresets[0].ComponentFirmware) == 0 {
		return nil, errors.Wrap(
			ErrFirmwareSetLookup,
			fmt.Sprintf(
				"lookup by device vendor: %s, model: %s returned firmware set with no component firmware",
				deviceVendor,
				deviceModel,
			),
		)
	}

	found := []model.Firmware{}
	for _, set := range firmwaresets {
		for _, firmware := range set.ComponentFirmware {
			found = append(found, model.Firmware{
				Vendor:        strings.ToLower(firmware.Vendor),
				Model:         strings.ToLower(firmware.Model),
				Version:       firmware.Version,
				FileName:      firmware.Filename,
				ComponentSlug: strings.ToLower(firmware.Component),
				Checksum:      firmware.Checksum,
			})
		}
	}

	return found, nil
}
