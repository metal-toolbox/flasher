package store

import (
	"encoding/json"
	"net"

	fleetdbapi "github.com/metal-toolbox/fleetdb/pkg/api/v1"
	rfleetdb "github.com/metal-toolbox/rivets/fleetdb"

	rtypes "github.com/metal-toolbox/rivets/types"

	"github.com/bmc-toolbox/common"
	"github.com/pkg/errors"
)

func findAttribute(ns string, attributes []fleetdbapi.Attributes) *fleetdbapi.Attributes {
	for _, attribute := range attributes {
		if attribute.Namespace == ns {
			return &attribute
		}
	}

	return nil
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

func (s *FleetDBAPI) fromServerserviceComponents(scomponents fleetdbapi.ServerComponentSlice) []*rtypes.Component {
	components := make([]*rtypes.Component, 0, len(scomponents))

	// nolint:gocritic // rangeValCopy - this type is returned in the current form by fleetdb API.
	for _, sc := range scomponents {
		sc := sc
		c, err := rfleetdb.RecordToComponent(&sc)
		if err != nil {
			s.logger.WithError(err).Warn("failed to convert component from fleetdb record: " + sc.Name)
		}

		components = append(components, c)
	}

	return components
}
