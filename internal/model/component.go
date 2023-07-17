package model

import (
	"strconv"
	"strings"

	"github.com/bmc-toolbox/common"
	"github.com/pkg/errors"
)

// Component is a device component
type Component struct {
	Slug              string
	Serial            string
	Vendor            string
	Model             string
	FirmwareInstalled string
}

// Components is a slice of Component on which one or more methods may be available.
type Components []*Component

// BySlug returns a component that matches the slug value.
func (c Components) BySlugVendorModel(cSlug, cVendor string, cModels []string) *Component {
	for idx, component := range c {
		// skip non matching component slug
		if !strings.EqualFold(cSlug, component.Slug) {
			continue
		}

		// skip non matching component vendor
		if !strings.EqualFold(component.Vendor, cVendor) {
			continue
		}

		// match component model with contains
		for _, findModel := range cModels {
			if strings.Contains(strings.ToLower(component.Model), strings.TrimSpace(findModel)) {
				return c[idx]
			}
		}
	}

	return nil
}

// ComponentFirmwareInstallStatus is the device component specific firmware install statuses
// returned by the FirmwareInstallStatus method, which is part of the DeviceQueryor interface.
//
// As an example, the BMCs return various firmware install statuses based on the vendor implementation
// and so these statuses defined reduce all of those differences into a few generic status values
//
// Note: these statuses are not related to the Flasher task status.
type ComponentFirmwareInstallStatus string

var (
	// StatusInstallRunning is returned by the FirmwareInstallStatus when the device indicates the install is running.
	StatusInstallRunning ComponentFirmwareInstallStatus = "running"

	// StatusInstallRunning is returned by the FirmwareInstallStatus when the device indicates the install is running.
	StatusInstallComplete ComponentFirmwareInstallStatus = "complete"

	// StatusInstallUnknown is returned by the FirmwareInstallStatus when the firmware install status is not known.
	StatusInstallUnknown ComponentFirmwareInstallStatus = "unknown"

	// StatusInstallFailed is returned by the FirmwareInstallStatus when the device indicates the install has failed.
	StatusInstallFailed ComponentFirmwareInstallStatus = "failed"

	// StatusInstallPowerCycleHostRequired is returned by the FirmwareInstallStatus when the device indicates the install requires a host power cycle.
	StatusInstallPowerCycleHostRequired ComponentFirmwareInstallStatus = "powerCycleHostRequired"

	// StatusInstallPowerCycleBMCRequired is returned by the FirmwareInstallStatus when the device indicates the BMC requires a power cycle.
	StatusInstallPowerCycleBMCRequired ComponentFirmwareInstallStatus = "powerCycleBMCRequired"
)

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
func (cc *ComponentConverter) CommonDeviceToComponents(device *common.Device) (Components, error) {
	if device == nil {
		return nil, errors.Wrap(ErrComponentConverter, "device object is nil")
	}

	cc.deviceModel = common.FormatProductName(device.Model)
	cc.deviceVendor = device.Vendor

	componentsTmp := []*Component{}
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

	components := []*Component{}

	for _, component := range componentsTmp {
		if component == nil {
			continue
		}

		components = append(components, component)
	}

	return components, nil
}

func (cc *ComponentConverter) newComponent(slug, cvendor, cmodel, cserial string) (*Component, error) {
	slug = strings.ToLower(slug)

	if cvendor == "" {
		cvendor = cc.deviceVendor
	}

	if cmodel == "" {
		cmodel = cc.deviceModel
	}

	return &Component{
		Vendor: common.FormatVendorName(cvendor),
		Model:  common.FormatProductName(cmodel),
		Serial: cserial,
		Slug:   slug,
	}, nil
}

func (cc *ComponentConverter) firmwareInstalled(firmware *common.Firmware) string {
	if firmware == nil {
		return ""
	}

	return strings.TrimSpace(firmware.Installed)
}

func (cc *ComponentConverter) gpus(gpus []*common.GPU) []*Component {
	if gpus == nil {
		return nil
	}

	components := make([]*Component, 0, len(gpus))

	for idx, c := range gpus {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := cc.newComponent(common.SlugGPU, c.Vendor, c.Model, c.Serial)
		if err != nil {
			return nil
		}

		sc.FirmwareInstalled = cc.firmwareInstalled(c.Firmware)
		components = append(components, sc)
	}

	return components
}

func (cc *ComponentConverter) cplds(cplds []*common.CPLD) []*Component {
	if cplds == nil {
		return nil
	}

	components := make([]*Component, 0, len(cplds))

	for idx, c := range cplds {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := cc.newComponent(common.SlugCPLD, c.Vendor, c.Model, c.Serial)
		if err != nil {
			return nil
		}

		sc.FirmwareInstalled = cc.firmwareInstalled(c.Firmware)
		components = append(components, sc)
	}

	return components
}

func (cc *ComponentConverter) tpms(tpms []*common.TPM) []*Component {
	if tpms == nil {
		return nil
	}

	components := make([]*Component, 0, len(tpms))

	for idx, c := range tpms {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := cc.newComponent(common.SlugTPM, c.Vendor, c.Model, c.Serial)
		if err != nil {
			return nil
		}

		sc.FirmwareInstalled = cc.firmwareInstalled(c.Firmware)
		components = append(components, sc)
	}

	return components
}

func (cc *ComponentConverter) cpus(cpus []*common.CPU) []*Component {
	if cpus == nil {
		return nil
	}

	components := make([]*Component, 0, len(cpus))

	for idx, c := range cpus {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := cc.newComponent(common.SlugCPU, c.Vendor, c.Model, c.Serial)
		if err != nil {
			return nil
		}

		sc.FirmwareInstalled = cc.firmwareInstalled(c.Firmware)
		components = append(components, sc)
	}

	return components
}

func (cc *ComponentConverter) storageControllers(controllers []*common.StorageController) []*Component {
	if controllers == nil {
		return nil
	}

	components := make([]*Component, 0, len(controllers))

	for idx, c := range controllers {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := cc.newComponent(common.SlugStorageController, c.Vendor, c.Model, c.Serial)
		if err != nil {
			return nil
		}

		sc.FirmwareInstalled = cc.firmwareInstalled(c.Firmware)
		components = append(components, sc)
	}

	return components
}

func (cc *ComponentConverter) psus(psus []*common.PSU) []*Component {
	if psus == nil {
		return nil
	}

	components := make([]*Component, 0, len(psus))

	for idx, c := range psus {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := cc.newComponent(common.SlugPSU, c.Vendor, c.Model, c.Serial)
		if err != nil {
			return nil
		}

		sc.FirmwareInstalled = cc.firmwareInstalled(c.Firmware)
		components = append(components, sc)
	}

	return components
}

func (cc *ComponentConverter) drives(drives []*common.Drive) []*Component {
	if drives == nil {
		return nil
	}

	components := make([]*Component, 0, len(drives))

	for idx, c := range drives {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := cc.newComponent(common.SlugDrive, c.Vendor, c.Model, c.Serial)
		if err != nil {
			return nil
		}

		sc.FirmwareInstalled = cc.firmwareInstalled(c.Firmware)
		components = append(components, sc)
	}

	return components
}

func (cc *ComponentConverter) nics(nics []*common.NIC) []*Component {
	if nics == nil {
		return nil
	}

	components := make([]*Component, 0, len(nics))

	for idx, c := range nics {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := cc.newComponent(common.SlugNIC, c.Vendor, c.Model, c.Serial)
		if err != nil {
			return nil
		}

		sc.FirmwareInstalled = cc.firmwareInstalled(c.Firmware)
		components = append(components, sc)
	}

	return components
}

func (cc *ComponentConverter) dimms(dimms []*common.Memory) []*Component {
	if dimms == nil {
		return nil
	}

	components := make([]*Component, 0, len(dimms))

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

		sc.FirmwareInstalled = cc.firmwareInstalled(c.Firmware)
		components = append(components, sc)
	}

	return components
}

func (cc *ComponentConverter) enclosures(enclosures []*common.Enclosure) []*Component {
	if enclosures == nil {
		return nil
	}

	components := make([]*Component, 0, len(enclosures))

	for idx, c := range enclosures {
		if strings.TrimSpace(c.Serial) == "" {
			c.Serial = strconv.Itoa(idx)
		}

		sc, err := cc.newComponent(common.SlugEnclosure, c.Vendor, c.Model, c.Serial)
		if err != nil {
			return nil
		}

		sc.FirmwareInstalled = cc.firmwareInstalled(c.Firmware)
		components = append(components, sc)
	}

	return components
}

func (cc *ComponentConverter) bmc(c *common.BMC) *Component {
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

	sc.FirmwareInstalled = cc.firmwareInstalled(c.Firmware)

	return sc
}

func (cc *ComponentConverter) bios(c *common.BIOS) *Component {
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

	sc.FirmwareInstalled = cc.firmwareInstalled(c.Firmware)

	return sc
}

func (cc *ComponentConverter) mainboard(c *common.Mainboard) *Component {
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

	sc.FirmwareInstalled = cc.firmwareInstalled(c.Firmware)

	return sc
}
