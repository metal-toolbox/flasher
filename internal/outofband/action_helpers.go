package outofband

import (
	"context"
	"fmt"
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

func (h *actionHandler) installedFirmwareVersionEqualsNew(device *common.Device, planned *model.Firmware) (bool, error) {
	switch strings.ToUpper(planned.ComponentSlug) {
	case common.SlugBIOS:
		if device.BIOS == nil || device.BIOS.Firmware == nil || device.BIOS.Firmware.Installed == "" {
			return false, errors.Wrap(ErrInstalledVersionUnknown, planned.ComponentSlug)
		}

		fmt.Println(device.BIOS.Firmware.Installed)
		fmt.Println(planned.Version)

		return strings.EqualFold(device.BIOS.Firmware.Installed, planned.Version), nil

	case common.SlugBMC:
		if device.BMC == nil || device.BMC.Firmware == nil || device.BMC.Firmware.Installed == "" {
			return false, errors.Wrap(ErrInstalledVersionUnknown, planned.ComponentSlug)
		}

		fmt.Println(device.BMC.Firmware.Installed)
		fmt.Println(planned.Version)

		return strings.EqualFold(device.BMC.Firmware.Installed, planned.Version), nil

	case common.SlugMainboard:
		if device.Mainboard == nil || device.Mainboard.Firmware == nil || device.Mainboard.Firmware.Installed == "" {
			return false, errors.Wrap(ErrInstalledVersionUnknown, planned.ComponentSlug)
		}

		return strings.EqualFold(device.Mainboard.Firmware.Installed, planned.Version), nil

	case common.SlugNIC:
		if device.NICs == nil {
			return false, errors.Wrap(ErrInstalledVersionUnknown, planned.ComponentSlug)
		}

		return nicsInstalledFirmwareEqualsNew(device.NICs, planned)

	case common.SlugCPLD:
		if device.CPLDs == nil {
			return false, errors.Wrap(ErrInstalledVersionUnknown, planned.ComponentSlug)
		}

		return cpldsInstalledFirmwareEqualsNew(device.CPLDs, planned)

	case common.SlugDrive:
		if device.Drives == nil {
			return false, errors.Wrap(ErrInstalledVersionUnknown, planned.ComponentSlug)
		}

		return drivesInstalledFirmwareEqualsNew(device.Drives, planned)

	case common.SlugPSU:
		if device.PSUs == nil {
			return false, errors.Wrap(ErrInstalledVersionUnknown, planned.ComponentSlug)
		}

		return psusInstalledFirmwareEqualsNew(device.PSUs, planned)

	case common.SlugTPM:
		if device.TPMs == nil {
			return false, errors.Wrap(ErrInstalledVersionUnknown, planned.ComponentSlug)
		}

		return tpmsInstalledFirmwareEqualsNew(device.TPMs, planned)
	case common.SlugGPU:
		if device.GPUs == nil {
			return false, errors.Wrap(ErrInstalledVersionUnknown, planned.ComponentSlug)
		}

		return gpusInstalledFirmwareEqualsNew(device.GPUs, planned)

	case common.SlugStorageController:
		if device.StorageControllers == nil {
			return false, errors.Wrap(ErrInstalledVersionUnknown, planned.ComponentSlug)
		}

		return storageControllersInstalledFirmwareEqualsNew(device.StorageControllers, planned)

	case common.SlugEnclosure:
		if device.Enclosures == nil {
			return false, errors.Wrap(ErrInstalledVersionUnknown, planned.ComponentSlug)
		}

		return enclosuresInstalledFirmwareEqualsNew(device.Enclosures, planned)

	default:
		return false, errors.Wrap(ErrComponentNotSupported, planned.ComponentSlug)
	}
}

// TODO(joel): when generics allow struct member access, rewrite methods below
// https://github.com/golang/go/issues/48522

func nicsInstalledFirmwareEqualsNew(components []*common.NIC, planned *model.Firmware) (bool, error) {
	for _, component := range components {
		if component == nil || component.Firmware == nil || component.Firmware.Installed == "" {
			return false, errors.Wrap(ErrInstalledVersionUnknown, planned.ComponentSlug)
		}

		if strings.EqualFold(component.Model, planned.Model) && strings.EqualFold(component.Vendor, planned.Vendor) {
			return strings.EqualFold(strings.TrimSpace(component.Firmware.Installed), planned.Version), nil
		}
	}

	// at this point none of the component matched the planned firmware attributes
	return false, errors.Wrap(ErrComponentNotFound, planned.ComponentSlug)
}

func cpldsInstalledFirmwareEqualsNew(components []*common.CPLD, planned *model.Firmware) (bool, error) {
	for _, component := range components {
		if component == nil || component.Firmware == nil || component.Firmware.Installed == "" {
			return false, errors.Wrap(ErrInstalledVersionUnknown, planned.ComponentSlug)
		}

		if strings.EqualFold(component.Model, planned.Model) && strings.EqualFold(component.Vendor, planned.Vendor) {
			return strings.EqualFold(strings.TrimSpace(component.Firmware.Installed), planned.Version), nil
		}
	}

	// at this point none of the components matched the planned firmware attributes
	return false, errors.Wrap(ErrComponentNotFound, planned.ComponentSlug)
}

func drivesInstalledFirmwareEqualsNew(components []*common.Drive, planned *model.Firmware) (bool, error) {
	for _, component := range components {
		if component == nil || component.Firmware == nil || component.Firmware.Installed == "" {
			return false, errors.Wrap(ErrInstalledVersionUnknown, planned.ComponentSlug)
		}

		if strings.EqualFold(component.Model, planned.Model) && strings.EqualFold(component.Vendor, planned.Vendor) {
			return strings.EqualFold(strings.TrimSpace(component.Firmware.Installed), planned.Version), nil
		}
	}

	// at this point none of the components matched the planned firmware attributes
	return false, errors.Wrap(ErrComponentNotFound, planned.ComponentSlug)
}

func psusInstalledFirmwareEqualsNew(components []*common.PSU, planned *model.Firmware) (bool, error) {
	for _, component := range components {
		if component == nil || component.Firmware == nil || component.Firmware.Installed == "" {
			return false, errors.Wrap(ErrInstalledVersionUnknown, planned.ComponentSlug)
		}

		if strings.EqualFold(component.Model, planned.Model) && strings.EqualFold(component.Vendor, planned.Vendor) {
			return strings.EqualFold(strings.TrimSpace(component.Firmware.Installed), planned.Version), nil
		}
	}

	// at this point none of the components matched the planned firmware attributes
	return false, errors.Wrap(ErrComponentNotFound, planned.ComponentSlug)
}

func tpmsInstalledFirmwareEqualsNew(components []*common.TPM, planned *model.Firmware) (bool, error) {
	for _, component := range components {
		if component == nil || component.Firmware == nil || component.Firmware.Installed == "" {
			return false, errors.Wrap(ErrInstalledVersionUnknown, planned.ComponentSlug)
		}

		if strings.EqualFold(component.Model, planned.Model) && strings.EqualFold(component.Vendor, planned.Vendor) {
			return strings.EqualFold(strings.TrimSpace(component.Firmware.Installed), planned.Version), nil
		}
	}

	// at this point none of the components matched the planned firmware attributes
	return false, errors.Wrap(ErrComponentNotFound, planned.ComponentSlug)
}

func gpusInstalledFirmwareEqualsNew(components []*common.GPU, planned *model.Firmware) (bool, error) {
	for _, component := range components {
		if component == nil || component.Firmware == nil || component.Firmware.Installed == "" {
			return false, errors.Wrap(ErrInstalledVersionUnknown, planned.ComponentSlug)
		}

		if strings.EqualFold(component.Model, planned.Model) && strings.EqualFold(component.Vendor, planned.Vendor) {
			return strings.EqualFold(strings.TrimSpace(component.Firmware.Installed), planned.Version), nil
		}
	}

	// at this point none of the components matched the planned firmware attributes
	return false, errors.Wrap(ErrComponentNotFound, planned.ComponentSlug)
}

func storageControllersInstalledFirmwareEqualsNew(components []*common.StorageController, planned *model.Firmware) (bool, error) {
	for _, component := range components {
		if component == nil || component.Firmware == nil || component.Firmware.Installed == "" {
			return false, errors.Wrap(ErrInstalledVersionUnknown, planned.ComponentSlug)
		}

		if strings.EqualFold(component.Model, planned.Model) && strings.EqualFold(component.Vendor, planned.Vendor) {
			return strings.EqualFold(strings.TrimSpace(component.Firmware.Installed), planned.Version), nil
		}
	}

	// at this point none of the components matched the planned firmware attributes
	return false, errors.Wrap(ErrComponentNotFound, planned.ComponentSlug)
}

func enclosuresInstalledFirmwareEqualsNew(components []*common.Enclosure, planned *model.Firmware) (bool, error) {
	for _, component := range components {
		if component == nil || component.Firmware == nil || component.Firmware.Installed == "" {
			return false, errors.Wrap(ErrInstalledVersionUnknown, planned.ComponentSlug)
		}

		if strings.EqualFold(component.Model, planned.Model) && strings.EqualFold(component.Vendor, planned.Vendor) {
			return strings.EqualFold(strings.TrimSpace(component.Firmware.Installed), planned.Version), nil
		}
	}

	// at this point none of the components matched the planned firmware attributes
	return false, errors.Wrap(ErrComponentNotFound, planned.ComponentSlug)
}
