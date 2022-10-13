package inventory

import (
	"encoding/json"
	"net"

	"github.com/pkg/errors"
	sservice "go.hollow.sh/serverservice/pkg/api/v1"
)

func findAttribute(ns string, attributes []sservice.Attributes) *sservice.Attributes {
	for _, attribute := range attributes {
		if attribute.Namespace == serverAttributeNSFlasherTask {
			return &attribute
		}
	}

	return nil
}

func (s *Serverservice) flasherTaskAttribute(attributes []sservice.Attributes) (*InstallAttributes, error) {
	// update existing task attribute
	found := findAttribute(serverAttributeNSFlasherTask, attributes)
	if found == nil {
		return nil, nil
	}

	taskAttrs := &InstallAttributes{}
	if err := json.Unmarshal(found.Data, taskAttrs); err != nil {
		return nil, err
	}

	return taskAttrs, nil
}

// deviceState returns the server state attribute value from the configured NodeStateAttributeNS
func (s *Serverservice) deviceStateAttribute(attributes []sservice.Attributes) (string, error) {
	var deviceState string

	deviceStateAttribute := findAttribute(s.config.NodeStateAttributeNS, attributes)
	if deviceStateAttribute == nil {
		return deviceState, nil
	}

	data := map[string]string{}
	if err := json.Unmarshal(deviceStateAttribute.Data, &data); err != nil {
		return deviceState, errors.Wrap(ErrDeviceState, err.Error())
	}

	return data[s.config.NodeStateAttributeNSDataKey], nil
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

	address := data["address"]
	if address == "" {
		return ip, errors.Wrap(ErrBMCAddress, "value undefined: "+serverAttributeNSBmcAddress)
	}

	return net.ParseIP(address), nil
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

	deviceVendor = vendorAttrs["vendor"]
	deviceModel = vendorAttrs["model"]
	deviceSerial = vendorAttrs["serial"]

	return
}
