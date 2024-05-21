package model

import (
	"strconv"
	"strings"

	"github.com/bmc-toolbox/common"
	"github.com/pkg/errors"

	rtypes "github.com/metal-toolbox/rivets/types"
)

// Component is a device component
//type Component struct {
//	Slug              string
//	Serial            string
//	Vendor            string
//	Model             string
//	FirmwareInstalled string
//}

// ComponentConvertor provides methods to convert a common.Device to its Component equivalents.
type ComponentConverter struct {
	deviceVendor string
	deviceModel  string
}

var (
	// ErrComponentConverter is returned when an error occurs in the component data conversion.
	ErrComponentConverter = errors.New("error in component converter")
)

// NewComponentConverter returns a new ComponentConvertor
func NewComponentConverter() *ComponentConverter { return &ComponentConverter{} }

// CommonDeviceToComponents converts a bmc-toolbox/common Device object to its flasher Components type
//
// TODO(joel): the bmc-toolbox/common Device component types could implement an interface with
// methods to retrieve component - firmware installed, vendor, model, serial, slug attributes
// this method can then call the interface methods instead of multiple small methods for each device component type.
func (cc *ComponentConverter) CommonDeviceToComponents(device *common.Device) (rtypes.Components, error) {
	if device == nil {
		return nil, errors.Wrap(ErrComponentConverter, "device object is nil")
	}

	cc.deviceModel = common.FormatProductName(device.Model)
	cc.deviceVendor = device.Vendor

	componentsTmp := []*rtypes.Component{}
	componentsTmp = append(componentsTmp,
		cc.bios(device.BIOS),
		cc.bmc(device.BMC),
		cc.mainboard(device.Mainboard),
	)

	componentsTmp = append(componentsTmp, cc.dimms(device.Memory)...)
	componentsTmp = append(componentsTmp, cc.nics(device.NICs)...)
	componentsTmp = append(componentsTmp, cc.drives(device.Drives)...)
	componentsTmp = append(componentsTmp, cc.psus(device.PSUs)...)
	componentsTmp = append(componentsTmp, cc.cpus(device.CPUs)...)
	componentsTmp = append(componentsTmp, cc.tpms(device.TPMs)...)
	componentsTmp = append(componentsTmp, cc.cplds(device.CPLDs)...)
	componentsTmp = append(componentsTmp, cc.gpus(device.GPUs)...)
	componentsTmp = append(componentsTmp, cc.storageControllers(device.StorageControllers)...)
	componentsTmp = append(componentsTmp, cc.enclosures(device.Enclosures)...)

	components := []*rtypes.Component{}

	for _, component := range componentsTmp {
		if component == nil {
			continue
		}

		components = append(components, component)
	}

	return components, nil
}

func (cc *ComponentConverter) newComponent(slug, cvendor, cmodel, cserial string) (*rtypes.Component, error) {
	slug = strings.ToLower(slug)

	if cvendor == "" {
		cvendor = cc.deviceVendor
	}

	if cmodel == "" {
		cmodel = cc.deviceModel
	}

	return &rtypes.Component{
		Vendor: common.FormatVendorName(cvendor),
		Model:  common.FormatProductName(cmodel),
		Serial: cserial,
		Name:   slug,
	}, nil
}

func (cc *ComponentConverter) firmwareInstalled(firmware *common.Firmware) string {
	if firmware == nil {
		return ""
	}

	return strings.TrimSpace(firmware.Installed)
}

func (cc *ComponentConverter) gpus(gpus []*common.GPU) []*rtypes.Component {
	if gpus == nil {
		return nil
	}

	components := make([]*rtypes.Component, 0, len(gpus))

	for idx, c := range gpus {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := cc.newComponent(common.SlugGPU, c.Vendor, c.Model, c.Serial)
		if err != nil {
			return nil
		}

		sc.Firmware.Installed = cc.firmwareInstalled(c.Firmware)
		components = append(components, sc)
	}

	return components
}

func (cc *ComponentConverter) cplds(cplds []*common.CPLD) []*rtypes.Component {
	if cplds == nil {
		return nil
	}

	components := make([]*rtypes.Component, 0, len(cplds))

	for idx, c := range cplds {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := cc.newComponent(common.SlugCPLD, c.Vendor, c.Model, c.Serial)
		if err != nil {
			return nil
		}

		sc.Firmware.Installed = cc.firmwareInstalled(c.Firmware)
		components = append(components, sc)
	}

	return components
}

func (cc *ComponentConverter) tpms(tpms []*common.TPM) []*rtypes.Component {
	if tpms == nil {
		return nil
	}

	components := make([]*rtypes.Component, 0, len(tpms))

	for idx, c := range tpms {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := cc.newComponent(common.SlugTPM, c.Vendor, c.Model, c.Serial)
		if err != nil {
			return nil
		}

		sc.Firmware.Installed = cc.firmwareInstalled(c.Firmware)
		components = append(components, sc)
	}

	return components
}

func (cc *ComponentConverter) cpus(cpus []*common.CPU) []*rtypes.Component {
	if cpus == nil {
		return nil
	}

	components := make([]*rtypes.Component, 0, len(cpus))

	for idx, c := range cpus {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := cc.newComponent(common.SlugCPU, c.Vendor, c.Model, c.Serial)
		if err != nil {
			return nil
		}

		sc.Firmware.Installed = cc.firmwareInstalled(c.Firmware)
		components = append(components, sc)
	}

	return components
}

func (cc *ComponentConverter) storageControllers(controllers []*common.StorageController) []*rtypes.Component {
	if controllers == nil {
		return nil
	}

	components := make([]*rtypes.Component, 0, len(controllers))

	for idx, c := range controllers {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := cc.newComponent(common.SlugStorageController, c.Vendor, c.Model, c.Serial)
		if err != nil {
			return nil
		}

		sc.Firmware.Installed = cc.firmwareInstalled(c.Firmware)
		components = append(components, sc)
	}

	return components
}

func (cc *ComponentConverter) psus(psus []*common.PSU) []*rtypes.Component {
	if psus == nil {
		return nil
	}

	components := make([]*rtypes.Component, 0, len(psus))

	for idx, c := range psus {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := cc.newComponent(common.SlugPSU, c.Vendor, c.Model, c.Serial)
		if err != nil {
			return nil
		}

		sc.Firmware.Installed = cc.firmwareInstalled(c.Firmware)
		components = append(components, sc)
	}

	return components
}

func (cc *ComponentConverter) drives(drives []*common.Drive) []*rtypes.Component {
	if drives == nil {
		return nil
	}

	components := make([]*rtypes.Component, 0, len(drives))

	for idx, c := range drives {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := cc.newComponent(common.SlugDrive, c.Vendor, c.Model, c.Serial)
		if err != nil {
			return nil
		}

		sc.Firmware.Installed = cc.firmwareInstalled(c.Firmware)
		components = append(components, sc)
	}

	return components
}

func (cc *ComponentConverter) nics(nics []*common.NIC) []*rtypes.Component {
	if nics == nil {
		return nil
	}

	components := make([]*rtypes.Component, 0, len(nics))

	for idx, c := range nics {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := cc.newComponent(common.SlugNIC, c.Vendor, c.Model, c.Serial)
		if err != nil {
			return nil
		}

		sc.Firmware.Installed = cc.firmwareInstalled(c.Firmware)
		components = append(components, sc)
	}

	return components
}

func (cc *ComponentConverter) dimms(dimms []*common.Memory) []*rtypes.Component {
	if dimms == nil {
		return nil
	}

	components := make([]*rtypes.Component, 0, len(dimms))

	for idx, c := range dimms {
		// skip empty dimm slots
		if c.Vendor == "" && c.ProductName == "" && c.SizeBytes == 0 && c.ClockSpeedHz == 0 {
			continue
		}

		// set incrementing serial when one isn't found
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		// trim redundant prefix
		c.Slot = strings.TrimPrefix(c.Slot, "DIMM.Socket.")

		sc, err := cc.newComponent(common.SlugPhysicalMem, c.Vendor, c.Model, c.Serial)
		if err != nil {
			return nil
		}

		sc.Firmware.Installed = cc.firmwareInstalled(c.Firmware)
		components = append(components, sc)
	}

	return components
}

func (cc *ComponentConverter) enclosures(enclosures []*common.Enclosure) []*rtypes.Component {
	if enclosures == nil {
		return nil
	}

	components := make([]*rtypes.Component, 0, len(enclosures))

	for idx, c := range enclosures {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := cc.newComponent(common.SlugEnclosure, c.Vendor, c.Model, c.Serial)
		if err != nil {
			return nil
		}

		sc.Firmware.Installed = cc.firmwareInstalled(c.Firmware)
		components = append(components, sc)
	}

	return components
}

func (cc *ComponentConverter) bmc(c *common.BMC) *rtypes.Component {
	if c == nil {
		return nil
	}

	if strings.TrimSpace(c.Serial) == "" {
		c.Serial = "0"
	}

	sc, err := cc.newComponent(common.SlugBMC, c.Vendor, c.Model, c.Serial)
	if err != nil {
		return nil
	}

	sc.Firmware.Installed = cc.firmwareInstalled(c.Firmware)

	return sc
}

func (cc *ComponentConverter) bios(c *common.BIOS) *rtypes.Component {
	if c == nil {
		return nil
	}

	if strings.TrimSpace(c.Serial) == "" {
		c.Serial = "0"
	}

	sc, err := cc.newComponent(common.SlugBIOS, c.Vendor, c.Model, c.Serial)
	if err != nil {
		return nil
	}

	sc.Firmware.Installed = cc.firmwareInstalled(c.Firmware)

	return sc
}

func (cc *ComponentConverter) mainboard(c *common.Mainboard) *rtypes.Component {
	if c == nil {
		return nil
	}

	if strings.TrimSpace(c.Serial) == "" {
		c.Serial = "0"
	}

	sc, err := cc.newComponent(common.SlugMainboard, c.Vendor, c.Model, c.Serial)
	if err != nil {
		return nil
	}

	sc.Firmware.Installed = cc.firmwareInstalled(c.Firmware)

	return sc
}
