package store

import (
	"encoding/json"
	"net"

	fleetdbapi "github.com/metal-toolbox/fleetdb/pkg/api/v1"
	rfleetdb "github.com/metal-toolbox/rivets/fleetdb"

	"github.com/bmc-toolbox/common"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/pkg/errors"
)

// versionedAttributeFirmware is the format in which the firmware data is present in fleetdb API.
type versionedAttributeFirmware struct {
	Firmware *common.Firmware `json:"firmware,omitempty"`
}

func findAttribute(ns string, attributes []fleetdbapi.Attributes) *fleetdbapi.Attributes {
	for _, attribute := range attributes {
		if attribute.Namespace == ns {
			return &attribute
		}
	}

	return nil
}

func findVersionedAttribute(ns string, attributes []fleetdbapi.VersionedAttributes) *fleetdbapi.VersionedAttributes {
	for _, attribute := range attributes {
		if attribute.Namespace == ns {
			return &attribute
		}
	}

	return nil
}

// assetState returns the asset state attribute value from the configured AssetStateAttributeNS
func (s *FleetDBAPI) assetStateAttribute(attributes []fleetdbapi.Attributes) (string, error) {
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

func (s *FleetDBAPI) bmcAddressFromAttributes(attributes []fleetdbapi.Attributes) (net.IP, error) {
	ip := net.IP{}

	bmcAttribute := findAttribute(rfleetdb.ServerAttributeNSBmcAddress, attributes)
	if bmcAttribute == nil {
		return ip, errors.Wrap(ErrBMCAddress, "not found: "+rfleetdb.ServerAttributeNSBmcAddress)
	}

	data := map[string]string{}
	if err := json.Unmarshal(bmcAttribute.Data, &data); err != nil {
		return ip, errors.Wrap(ErrBMCAddress, err.Error())
	}

	if data["address"] == "" {
		return ip, errors.Wrap(ErrBMCAddress, "value undefined: "+rfleetdb.ServerAttributeNSBmcAddress)
	}

	return net.ParseIP(data["address"]), nil
}
func (s *FleetDBAPI) vendorModelFromAttributes(attributes []fleetdbapi.Attributes) (deviceVendor, deviceModel, deviceSerial string, err error) {
	vendorAttrs := map[string]string{}

	vendorAttribute := findAttribute(rfleetdb.ServerVendorAttributeNS, attributes)
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

func (s *FleetDBAPI) fromServerserviceComponents(scomponents fleetdbapi.ServerComponentSlice) model.Components {
	components := make(model.Components, 0, len(scomponents))

	// nolint:gocritic // rangeValCopy - this type is returned in the current form by fleetdb API.
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

func (s *FleetDBAPI) firmwareFromVersionedAttributes(va []fleetdbapi.VersionedAttributes) string {
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
