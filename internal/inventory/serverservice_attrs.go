package inventory

import (
	"context"
	"encoding/json"
	"net"

	"github.com/bmc-toolbox/common"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/pkg/errors"
	sservice "go.hollow.sh/serverservice/pkg/api/v1"

	"github.com/google/uuid"
)

// firmwareVersionedAttribute is the firmware data format
type firmwareVersionedAttributes struct {
	Firmware *common.Firmware `json:"firmware,omitempty"`
}

func installedFirmwareFromVA(va sservice.VersionedAttributes) (string, error) {
	data := &common.Firmware{}

	if err := json.Unmarshal(va.Data, data); err != nil {
		return "", errors.Wrap(ErrServerserviceVersionedAttrObj, "failed to unpack Firmware data: "+err.Error())
	}

	if data.Installed == "" {
		return "", errors.Wrap(ErrServerserviceVersionedAttrObj, "installed firmware version unknown")
	}

	return data.Installed, nil
}

func findAttribute(ns string, attributes []sservice.Attributes) *sservice.Attributes {
	for _, attribute := range attributes {
		if attribute.Namespace == ns {
			return &attribute
		}
	}

	return nil
}

func (s *Serverservice) flasherTaskAttribute(attributes []sservice.Attributes) (*FwInstallAttributes, error) {
	// update existing task attribute
	found := findAttribute(serverAttributeNSFlasherTask, attributes)
	if found == nil {
		return nil, nil
	}

	taskAttrs := &FwInstallAttributes{}
	if err := json.Unmarshal(found.Data, taskAttrs); err != nil {
		return nil, err
	}

	return taskAttrs, nil
}

// deviceState returns the server state attribute value from the configured DeviceStateAttributeNS
func (s *Serverservice) deviceStateAttribute(attributes []sservice.Attributes) (string, error) {
	var deviceState string

	deviceStateAttribute := findAttribute(s.config.DeviceStateAttributeNS, attributes)
	if deviceStateAttribute == nil {
		return deviceState, nil
	}

	data := map[string]string{}
	if err := json.Unmarshal(deviceStateAttribute.Data, &data); err != nil {
		return deviceState, errors.Wrap(ErrDeviceState, err.Error())
	}

	return data[s.config.DeviceStateAttributeKey], nil
}

func (s *Serverservice) bmcAddressFromAttributes(attributes []sservice.Attributes) (net.IP, error) {
	ip := net.IP{}

	bmcAttribute := findAttribute(serverAttributeNSBmcAddress, attributes)
	if bmcAttribute == nil {
		return ip, errors.Wrap(ErrBMCAddress, "not found: "+serverAttributeNSBmcAddress)
	}

	data := map[string]string{}
	if err := json.Unmarshal(bmcAttribute.Data, &data); err != nil {
		return ip, errors.Wrap(ErrBMCAddress, err.Error())
	}

	if data["address"] == "" {
		return ip, errors.Wrap(ErrBMCAddress, "value undefined: "+serverAttributeNSBmcAddress)
	}

	return net.ParseIP(data["address"]), nil
}
func (s *Serverservice) vendorModelFromAttributes(attributes []sservice.Attributes) (deviceVendor, deviceModel, deviceSerial string, err error) {
	vendorAttrs := map[string]string{}

	vendorAttribute := findAttribute(serverAttributeNSVendor, attributes)
	if vendorAttribute == nil {
		return
	}

	if err := json.Unmarshal(vendorAttribute.Data, &vendorAttrs); err != nil {
		return deviceVendor,
			deviceModel,
			deviceSerial,
			errors.Wrap(ErrServerserviceAttrObj, "server vendor attribute: "+err.Error())
	}

	deviceVendor = common.FormatVendorName(vendorAttrs["vendor"])
	deviceModel = common.FormatProductName(vendorAttrs["model"])
	deviceSerial = vendorAttrs["serial"]

	return
}

func (s *Serverservice) updateFlasherAttributes(ctx context.Context, deviceUUID uuid.UUID, currentTaskAttrs, newTaskAttrs *FwInstallAttributes) error {
	if newTaskAttrs.Requester == "" && currentTaskAttrs.Requester != "" {
		newTaskAttrs.Requester = currentTaskAttrs.Requester
	}

	if newTaskAttrs.FlasherTaskID == "" && currentTaskAttrs.FlasherTaskID != "" {
		newTaskAttrs.FlasherTaskID = currentTaskAttrs.FlasherTaskID
	}

	payload, err := json.Marshal(newTaskAttrs)
	if err != nil {
		return errors.Wrap(ErrAttributeUpdate, err.Error())
	}

	_, err = s.client.UpdateAttributes(ctx, deviceUUID, serverAttributeNSFlasherTask, payload)
	if err != nil {
		return errors.Wrap(ErrAttributeUpdate, err.Error())
	}

	return nil
}

func (s *Serverservice) createFlasherAttributes(ctx context.Context, deviceUUID uuid.UUID, attrs *FwInstallAttributes) error {
	payload, err := json.Marshal(attrs)
	if err != nil {
		return errors.Wrap(ErrAttributeCreate, err.Error())
	}

	data := sservice.Attributes{Namespace: serverAttributeNSFlasherTask, Data: payload}

	_, err = s.client.CreateAttributes(ctx, deviceUUID, data)
	if err != nil {
		return err
	}

	return nil
}

// deviceWithFwInstallAttributes returns a device, firmware install parameters object with its fields populated with data from server service.
//
// The attributes looked up are,
// - BMC address
// - BMC credentials
// - Device vendor, model, serial attributes
// - Device state
// - Flasher install attributes
func (s *Serverservice) deviceWithFwInstallAttributes(ctx context.Context, deviceID string) (*model.Device, *FwInstallAttributes, error) {
	deviceUUID, err := uuid.Parse(deviceID)
	if err != nil {
		return nil, nil, err
	}

	device := &model.Device{ID: deviceUUID}

	// lookup attributes
	attributes, _, err := s.client.ListAttributes(ctx, deviceUUID, nil)
	if err != nil {
		return nil, nil, errors.Wrap(ErrServerserviceQuery, err.Error())
	}

	// bmc address from attribute
	device.BmcAddress, err = s.bmcAddressFromAttributes(attributes)
	if err != nil {
		return nil, nil, err
	}

	// credentials from credential store
	credential, _, err := s.client.GetCredential(ctx, deviceUUID, sservice.ServerCredentialTypeBMC)
	if err != nil {
		return nil, nil, errors.Wrap(ErrServerserviceQuery, err.Error())
	}

	device.BmcUsername = credential.Username
	device.BmcPassword = credential.Password

	// vendor attributes
	device.Vendor, device.Model, device.Serial, err = s.vendorModelFromAttributes(attributes)
	if err != nil {
		return nil, nil, err
	}

	// device state attribute
	device.State, err = s.deviceStateAttribute(attributes)
	if err != nil {
		return nil, nil, err
	}

	installParams, err := s.flasherTaskAttribute(attributes)
	if err != nil {
		return nil, nil, err
	}

	return device, installParams, nil
}
