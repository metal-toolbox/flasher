package store

import (
	"encoding/json"
	"net"

	"github.com/bmc-toolbox/common"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/pkg/errors"
	sservice "go.hollow.sh/serverservice/pkg/api/v1"
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

// assetState returns the asset state attribute value from the configured AssetStateAttributeNS
func (s *Serverservice) assetStateAttribute(attributes []sservice.Attributes) (string, error) {
	var assetState string

	assetStateAttribute := findAttribute(s.config.AssetStateAttributeNS, attributes)
	if assetStateAttribute == nil {
		return assetState, nil
	}

	data := map[string]string{}
	if err := json.Unmarshal(assetStateAttribute.Data, &data); err != nil {
		return assetState, errors.Wrap(ErrDeviceState, err.Error())
	}

	if data[s.config.AssetStateAttributeKey] == "" {
		return assetState, errors.Wrap(ErrDeviceState, "device state attribute is not set")
	}

	return data[s.config.AssetStateAttributeKey], nil
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

func (s *Serverservice) fromServerserviceComponents(scomponents sservice.ServerComponentSlice) model.Components {
	components := make(model.Components, 0, len(scomponents))

	for _, sc := range scomponents {
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
