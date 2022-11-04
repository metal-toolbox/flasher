package outofband

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/bmc-toolbox/common"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/pkg/errors"
)

var (
	ErrInstalledVersionUnknown = errors.New("installed version unknown")
	ErrComponentNotFound       = errors.New("component not found for firmware install")
	ErrComponentNotSupported   = errors.New("component not supported")
)

func sleepWithContext(ctx context.Context, t time.Duration) error {
	// skip sleep in tests
	if os.Getenv(envTesting) == "1" {
		return nil
	}

	select {
	case <-time.After(t):
		return nil
	case <-ctx.Done():
		return ErrContextCancelled
	}
}

func (h *actionHandler) installedFirmwareVersionEqualsPlanned(device *common.Device, planned *model.Firmware) (equals bool, installedVersion string, err error) {
	// TODO: (joel) fix bmc-toolbox/common slug consts to be lower case
	switch strings.ToLower(planned.ComponentSlug) {
	case strings.ToLower(common.SlugBIOS):
		if device.BIOS == nil || device.BIOS.Firmware == nil || device.BIOS.Firmware.Installed == "" {
			return false, "", errors.Wrap(ErrInstalledVersionUnknown, planned.ComponentSlug)
		}

		return strings.EqualFold(device.BIOS.Firmware.Installed, planned.Version), device.BIOS.Firmware.Installed, nil

	case strings.ToLower(common.SlugBMC):
		if device.BMC == nil || device.BMC.Firmware == nil || device.BMC.Firmware.Installed == "" {
			return false, "", errors.Wrap(ErrInstalledVersionUnknown, planned.ComponentSlug)
		}

		return strings.EqualFold(device.BMC.Firmware.Installed, planned.Version), device.BMC.Firmware.Installed, nil

	case strings.ToLower(common.SlugMainboard):
		if device.Mainboard == nil || device.Mainboard.Firmware == nil || device.Mainboard.Firmware.Installed == "" {
			return false, "", errors.Wrap(ErrInstalledVersionUnknown, planned.ComponentSlug)
		}

		return strings.EqualFold(device.Mainboard.Firmware.Installed, planned.Version), device.BMC.Firmware.Installed, nil

	case strings.ToLower(common.SlugNIC):
		if device.NICs == nil {
			return false, "", errors.Wrap(ErrInstalledVersionUnknown, planned.ComponentSlug)
		}

		return nicsInstalledFirmwareEqualsNew(device.NICs, planned)

	case strings.ToLower(common.SlugCPLD):
		if device.CPLDs == nil {
			return false, "", errors.Wrap(ErrInstalledVersionUnknown, planned.ComponentSlug)
		}

		return cpldsInstalledFirmwareEqualsNew(device.CPLDs, planned)

	case strings.ToLower(common.SlugDrive):
		if device.Drives == nil {
			return false, "", errors.Wrap(ErrInstalledVersionUnknown, planned.ComponentSlug)
		}

		return drivesInstalledFirmwareEqualsNew(device.Drives, planned)

	case strings.ToLower(common.SlugPSU):
		if device.PSUs == nil {
			return false, "", errors.Wrap(ErrInstalledVersionUnknown, planned.ComponentSlug)
		}

		return psusInstalledFirmwareEqualsNew(device.PSUs, planned)

	case strings.ToLower(common.SlugTPM):
		if device.TPMs == nil {
			return false, "", errors.Wrap(ErrInstalledVersionUnknown, planned.ComponentSlug)
		}

		return tpmsInstalledFirmwareEqualsNew(device.TPMs, planned)

	case strings.ToLower(common.SlugGPU):
		if device.GPUs == nil {
			return false, "", errors.Wrap(ErrInstalledVersionUnknown, planned.ComponentSlug)
		}

		return gpusInstalledFirmwareEqualsNew(device.GPUs, planned)

	case strings.ToLower(common.SlugStorageController):
		if device.StorageControllers == nil {
			return false, "", errors.Wrap(ErrInstalledVersionUnknown, planned.ComponentSlug)
		}

		return storageControllersInstalledFirmwareEqualsNew(device.StorageControllers, planned)

	case strings.ToLower(common.SlugEnclosure):
		if device.Enclosures == nil {
			return false, "", errors.Wrap(ErrInstalledVersionUnknown, planned.ComponentSlug)
		}

		return enclosuresInstalledFirmwareEqualsNew(device.Enclosures, planned)

	default:
		return false, "", errors.Wrap(ErrComponentNotSupported, strings.ToLower(planned.ComponentSlug))
	}
}

func componentMatchesFirmwarePlan(componentVendor, componentModel, planVendor, planModel string) bool {
	planModels := []string{planModel}

	if strings.Contains(planModel, ",") {
		planModels = strings.Split(planModel, ",")
	}

	for _, pModel := range planModels {
		if strings.EqualFold(componentVendor, planVendor) &&
			strings.EqualFold(componentModel, strings.TrimSpace(pModel)) {
			return true
		}
	}

	return false
}

// TODO(joel): when generics allow struct member access, rewrite methods below https://github.com/golang/go/issues/48522
// OR: generate the code below

func nicsInstalledFirmwareEqualsNew(components []*common.NIC, planned *model.Firmware) (equals bool, installedVersion string, err error) {
	for _, component := range components {
		if component == nil || component.Firmware == nil || component.Firmware.Installed == "" {
			continue
		}

		// component matches firmware plan component vendor, model
		if componentMatchesFirmwarePlan(component.Vendor, component.Model, planned.Vendor, planned.Model) {
			if strings.EqualFold(strings.TrimSpace(component.Firmware.Installed), planned.Version) {
				return true, strings.TrimSpace(component.Firmware.Installed), nil
			}

			return false, strings.TrimSpace(component.Firmware.Installed), nil
		}
	}

	// at this point none of the components matched the planned firmware attributes
	return false, "", errors.Wrap(ErrComponentNotFound, planned.ComponentSlug)
}

func cpldsInstalledFirmwareEqualsNew(components []*common.CPLD, planned *model.Firmware) (equals bool, installedVersion string, err error) {
	for _, component := range components {
		if component == nil || component.Firmware == nil || component.Firmware.Installed == "" {
			continue
		}

		// component matches firmware plan component vendor, model
		if componentMatchesFirmwarePlan(component.Vendor, component.Model, planned.Vendor, planned.Model) {
			if strings.EqualFold(strings.TrimSpace(component.Firmware.Installed), planned.Version) {
				return true, strings.TrimSpace(component.Firmware.Installed), nil
			}

			return false, strings.TrimSpace(component.Firmware.Installed), nil
		}
	}

	// at this point none of the components matched the planned firmware attributes
	return false, "", errors.Wrap(ErrComponentNotFound, planned.ComponentSlug)
}

func drivesInstalledFirmwareEqualsNew(components []*common.Drive, planned *model.Firmware) (equals bool, installedVersion string, err error) {
	for _, component := range components {
		if component == nil || component.Firmware == nil || component.Firmware.Installed == "" {
			continue
		}

		// component matches firmware plan component vendor, model
		if componentMatchesFirmwarePlan(component.Vendor, component.Model, planned.Vendor, planned.Model) {
			if strings.EqualFold(strings.TrimSpace(component.Firmware.Installed), planned.Version) {
				return true, strings.TrimSpace(component.Firmware.Installed), nil
			}

			return false, strings.TrimSpace(component.Firmware.Installed), nil
		}
	}

	// at this point none of the components matched the planned firmware attributes
	return false, "", errors.Wrap(ErrComponentNotFound, planned.ComponentSlug)
}

func psusInstalledFirmwareEqualsNew(components []*common.PSU, planned *model.Firmware) (equals bool, installedVersion string, err error) {
	for _, component := range components {
		if component == nil || component.Firmware == nil || component.Firmware.Installed == "" {
			continue
		}

		// component matches firmware plan component vendor, model
		if componentMatchesFirmwarePlan(component.Vendor, component.Model, planned.Vendor, planned.Model) {
			if strings.EqualFold(strings.TrimSpace(component.Firmware.Installed), planned.Version) {
				return true, strings.TrimSpace(component.Firmware.Installed), nil
			}

			return false, strings.TrimSpace(component.Firmware.Installed), nil
		}
	}

	// at this point none of the components matched the planned firmware attributes
	return false, "", errors.Wrap(ErrComponentNotFound, planned.ComponentSlug)
}

func tpmsInstalledFirmwareEqualsNew(components []*common.TPM, planned *model.Firmware) (equals bool, installedVersion string, err error) {
	for _, component := range components {
		if component == nil || component.Firmware == nil || component.Firmware.Installed == "" {
			continue
		}

		// component matches firmware plan component vendor, model
		if componentMatchesFirmwarePlan(component.Vendor, component.Model, planned.Vendor, planned.Model) {
			if strings.EqualFold(strings.TrimSpace(component.Firmware.Installed), planned.Version) {
				return true, strings.TrimSpace(component.Firmware.Installed), nil
			}

			return false, strings.TrimSpace(component.Firmware.Installed), nil
		}
	}

	// at this point none of the components matched the planned firmware attributes
	return false, "", errors.Wrap(ErrComponentNotFound, planned.ComponentSlug)
}

func gpusInstalledFirmwareEqualsNew(components []*common.GPU, planned *model.Firmware) (equals bool, installedVersion string, err error) {
	for _, component := range components {
		if component == nil || component.Firmware == nil || component.Firmware.Installed == "" {
			continue
		}

		// component matches firmware plan component vendor, model
		if componentMatchesFirmwarePlan(component.Vendor, component.Model, planned.Vendor, planned.Model) {
			if strings.EqualFold(strings.TrimSpace(component.Firmware.Installed), planned.Version) {
				return true, strings.TrimSpace(component.Firmware.Installed), nil
			}

			return false, strings.TrimSpace(component.Firmware.Installed), nil
		}
	}

	// at this point none of the components matched the planned firmware attributes
	return false, "", errors.Wrap(ErrComponentNotFound, planned.ComponentSlug)
}

func storageControllersInstalledFirmwareEqualsNew(components []*common.StorageController, planned *model.Firmware) (equals bool, installedVersion string, err error) {
	for _, component := range components {
		if component == nil || component.Firmware == nil || component.Firmware.Installed == "" {
			continue
		}

		// component matches firmware plan component vendor, model
		if componentMatchesFirmwarePlan(component.Vendor, component.Model, planned.Vendor, planned.Model) {
			if strings.EqualFold(strings.TrimSpace(component.Firmware.Installed), planned.Version) {
				return true, strings.TrimSpace(component.Firmware.Installed), nil
			}

			return false, strings.TrimSpace(component.Firmware.Installed), nil
		}
	}

	// at this point none of the components matched the planned firmware attributes
	return false, "", errors.Wrap(ErrComponentNotFound, planned.ComponentSlug)
}

func enclosuresInstalledFirmwareEqualsNew(components []*common.Enclosure, planned *model.Firmware) (equals bool, installedVersion string, err error) {
	for _, component := range components {
		if component == nil || component.Firmware == nil || component.Firmware.Installed == "" {
			continue
		}

		// component matches firmware plan component vendor, model
		if componentMatchesFirmwarePlan(component.Vendor, component.Model, planned.Vendor, planned.Model) {
			if strings.EqualFold(strings.TrimSpace(component.Firmware.Installed), planned.Version) {
				return true, strings.TrimSpace(component.Firmware.Installed), nil
			}

			return false, strings.TrimSpace(component.Firmware.Installed), nil
		}
	}

	// at this point none of the components matched the planned firmware attributes
	return false, "", errors.Wrap(ErrComponentNotFound, planned.ComponentSlug)
}
