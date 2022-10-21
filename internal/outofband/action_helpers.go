package outofband

import (
	"context"
	"strings"

	"github.com/bmc-toolbox/common"
	"github.com/metal-toolbox/flasher/internal/model"
)


type differFirmware struct {
	// install indicates
	componentSlug string
	install bool
	info string
}

// install is a map of key values
// where the key is the component slug and the bool when true 
type install map[string]struct{}

func (f firmwareDiff) compare(componentSlug, installedV, newV string) {
	if strings.EqualFold(installedV, newV) {
		f[componentSlug] = false
	}

	f[componentSlug] = true
}

func (h *actionHandler) differ(ctx context.Context, firmwarePlanned []model.Firmware, device *common.Device) firmwareDiff {

	diff := make(firmwareDiff, 0)

	for _, planned := range firmwarePlanned {
		switch planned.ComponentSlug {
		case common.SlugBIOS:
			if device.BIOS == nil || device.BIOS.Firmware == nil {
				continue
			}

			diff.compare(planned.ComponentSlug, device.BIOS.Firmware.Installed, planned.Version)

		case common.SlugBMC:
			if device.BMC == nil || device.BMC.Firmware == nil {
				continue
			}

			diff.compare(planned.ComponentSlug, device.BMC.Firmware.Installed, planned.Version)

		case common.SlugMainboard:
			if device.Mainboard == nil || device.Mainboard.Firmware == nil {
				continue
			}

			diff.compare(planned.ComponentSlug, device.Mainboard.Firmware.Installed, planned.Version)

		case common.SlugNIC:
			differNICFirmware(device.NICs, firmwarePlanned, &diff)

			diff.compare(planned.ComponentSlug, device.Mainboard.Firmware.Installed, planned.Version)

		case common.SlugCPLD:
		case common.SlugDrive:
		case common.SlugPSU:
		case common.SlugTPM:
		case common.SlugGPU:
		case common.SlugStorageController:
		case common.SlugEnclosure:
		}
	}

	return diff
}

func differNICFirmware(nics []*common.NIC, firmwarePlanned model.FirmwarePlanned, differ *firmwareDiff) {
	for _, nic := range nics {
		if nic == nil || nic.Firmware == nil {
			continue
		}

		for _, planned := range firmwarePlanned {
			if nic.Model == "" || nic.Vendor == "" {
				continue
			}

			if planned.Model == nic.Model && planned.
			differ.compare(common.SlugNIC, nic.Firmware.Installed)
		}
	}
}
