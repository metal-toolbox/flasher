package inventory

import (
	"context"
	"encoding/json"
	"net"

	"github.com/bmc-toolbox/common"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	sservice "go.hollow.sh/serverservice/pkg/api/v1"

	"github.com/google/uuid"
)

// versionedAttributeFirmware is the format in which the firmware data is present in serverservice.
type versionedAttributeFirmware struct {
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

func findVersionedAttribute(ns string, attributes []sservice.VersionedAttributes) *sservice.VersionedAttributes {
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

	if data[s.config.DeviceStateAttributeKey] == "" {
		return deviceState, errors.Wrap(ErrDeviceState, "device state attribute is not set")
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
		return deviceVendor,
			deviceModel,
			deviceSerial,
			ErrVendorModelAttributes
	}

	if err := json.Unmarshal(vendorAttribute.Data, &vendorAttrs); err != nil {
		return deviceVendor,
			deviceModel,
			deviceSerial,
			errors.Wrap(ErrVendorModelAttributes, "server vendor attribute: "+err.Error())
	}

	deviceVendor = common.FormatVendorName(vendorAttrs["vendor"])
	deviceModel = common.FormatProductName(vendorAttrs["model"])
	deviceSerial = vendorAttrs["serial"]

	if deviceVendor == "" {
		return deviceVendor,
			deviceModel,
			deviceSerial,
			errors.Wrap(ErrVendorModelAttributes, "device vendor unknown")
	}

	if deviceModel == "" {
		return deviceVendor,
			deviceModel,
			deviceSerial,
			errors.Wrap(ErrVendorModelAttributes, "device model unknown")
	}

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

	return err
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

	// device state attribute
	device.State, err = s.deviceStateAttribute(attributes)
	if err != nil {
		return nil, nil, err
	}

	installParams, err := s.flasherTaskAttribute(attributes)
	if err != nil {
		return nil, nil, err
	}

	// vendor attributes
	device.Vendor, device.Model, device.Serial, err = s.vendorModelFromAttributes(attributes)
	if err != nil {
		if errors.Is(err, ErrVendorModelAttributes) {
			s.logger.WithFields(
				logrus.Fields{
					"component": component,
					"deviceID":  deviceID,
					"err":       err.Error(),
				},
			).Debug("device vendor/model is unknown")
		}

		return device, installParams, err
	}

	return device, installParams, nil
}

func (s *Serverservice) fromServerserviceComponents(deviceVendor, deviceModel string, scomponents sservice.ServerComponentSlice) model.Components {
	components := make(model.Components, 0, len(scomponents))

	for _, sc := range scomponents {
		if sc.Vendor == "" {
			sc.Vendor = deviceVendor
		}

		if sc.Model == "" {
			sc.Model = deviceModel
		}

		components = append(components, &model.Component{
			Slug:              sc.ComponentTypeSlug,
			Serial:            sc.Serial,
			Vendor:            sc.Vendor,
			Model:             sc.Model,
			FirmwareInstalled: s.firmwareFromVersionedAttributes(sc.VersionedAttributes),
		})
	}

	return components
}

func (s *Serverservice) firmwareFromVersionedAttributes(va []sservice.VersionedAttributes) string {
	if len(va) == 0 {
		return ""
	}

	found := findVersionedAttribute(s.config.OutofbandFirmwareNS, va)
	if found == nil {
		return ""
	}

	vaData := &versionedAttributeFirmware{}
	if err := json.Unmarshal(found.Data, vaData); err != nil {
		s.logger.Warn("failed to unmarshal firmware data")
		return ""
	}

	if vaData.Firmware == nil {
		return ""
	}

	return vaData.Firmware.Installed
}
