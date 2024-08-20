package inband

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/metal-toolbox/flasher/internal/device"
	"github.com/metal-toolbox/flasher/internal/download"
	"github.com/metal-toolbox/flasher/internal/metrics"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	iutils "github.com/metal-toolbox/ironlib/utils"
)

const (
	// firmware files are downloaded into this directory
	downloadDir = "/tmp"
	rebootFlag  = "/var/run/reboot"
)

var (
	ErrInstalledFirmwareNotEqual = errors.New("installed and expected firmware not equal")
	ErrInstalledFirmwareEqual    = errors.New("installed and expected firmware are equal, no action necessary")
	ErrInstalledVersionUnknown   = errors.New("installed version unknown")
	ErrComponentNotFound         = errors.New("component not identified for firmware install")
	ErrRequireHostPoweredOff     = errors.New("expected host to be powered off")
)

type handler struct {
	firmware      *model.Firmware
	task          *model.Task
	action        *model.Action
	deviceQueryor device.InbandQueryor
	publisher     model.Publisher
	logger        *logrus.Entry
}

func (h *handler) installedEqualsExpected(ctx context.Context, component, expectedFirmware, vendor string, models []string) error {
	inv, err := h.deviceQueryor.Inventory(ctx)
	if err != nil {
		return err
	}

	h.logger.WithFields(
		logrus.Fields{
			"component": component,
		}).Debug("Querying device inventory from BMC for current component firmware")

	components, err := model.NewComponentConverter().CommonDeviceToComponents(inv)
	if err != nil {
		return err
	}

	found := components.ByNameModel(component, models)
	if found == nil {
		h.logger.WithFields(
			logrus.Fields{
				"component": component,
				"vendor":    vendor,
				"models":    models,
				"err":       ErrComponentNotFound,
			}).Error("no component found for given component/vendor/model")

		return errors.Wrap(ErrComponentNotFound,
			fmt.Sprintf("component: %s, vendor: %s, model: %s", component,
				vendor,
				models,
			),
		)
	}

	h.logger.WithFields(
		logrus.Fields{
			"component": component,
			"model":     found.Model,
			"vendor":    found.Vendor,
		}).Debug("component for update identified")

	if strings.TrimSpace(found.Firmware.Installed) == "" {
		return ErrInstalledVersionUnknown
	}

	if !strings.EqualFold(expectedFirmware, found.Firmware.Installed) {
		return errors.Wrap(
			ErrInstalledFirmwareNotEqual,
			fmt.Sprintf("expected: %s, current: %s", expectedFirmware, found.Firmware.Installed),
		)
	}

	return nil
}

func (h *handler) checkCurrentFirmware(ctx context.Context) error {
	if h.task.Parameters.ForceInstall {
		h.logger.WithFields(
			logrus.Fields{
				"component": h.firmware.Component,
			}).Debug("Skipped installed version lookup - task.Parameters.ForceInstall=true")

		return nil
	}

	if err := h.installedEqualsExpected(
		ctx,
		h.firmware.Component,
		h.firmware.Version,
		h.firmware.Vendor,
		h.firmware.Models,
	); err != nil {
		if errors.Is(err, ErrInstalledVersionUnknown) {
			return errors.Wrap(err, "use task.Parameters.ForceInstall=true to disable this check")
		}

		if errors.Is(err, ErrInstalledFirmwareNotEqual) {
			return nil
		}

		return err
	}

	return ErrInstalledFirmwareEqual
}

func (h *handler) downloadFirmware(ctx context.Context) error {
	if h.action.FirmwareTempFile != "" {
		h.logger.WithFields(
			logrus.Fields{
				"component": h.firmware.Component,
				"file":      h.action.FirmwareTempFile,
			}).Info("firmware file path provided, skipped download")

		return nil
	}

	// create a temp download directory
	dir, err := os.MkdirTemp(downloadDir, "")
	if err != nil {
		return errors.Wrap(err, "error creating tmp directory to download firmware")
	}

	file := filepath.Join(dir, h.firmware.FileName)

	// download firmware file
	err = download.FromURLToFile(ctx, h.firmware.URL, file)
	if err != nil {
		return err
	}

	// collect download metrics
	fileInfo, err := os.Stat(file)
	if err == nil {
		metrics.DownloadBytes.With(
			prometheus.Labels{
				"component": h.firmware.Component,
				"vendor":    h.firmware.Vendor,
			},
		).Add(float64(fileInfo.Size()))
	}

	// validate checksum
	if err := download.ChecksumValidate(file, h.firmware.Checksum); err != nil {
		os.RemoveAll(filepath.Dir(file))
		return err
	}

	// store the firmware temp file location
	h.action.FirmwareTempFile = file

	h.logger.WithFields(
		logrus.Fields{
			"component": h.firmware.Component,
			"version":   h.firmware.Version,
			"url":       h.firmware.URL,
			"file":      file,
			"checksum":  h.firmware.Checksum,
		}).Info("downloaded and verified firmware file checksum")

	return nil
}

func (h *handler) installFirmware(ctx context.Context) error {
	if !h.task.Parameters.DryRun {
		// initiate firmware install
		if err := h.deviceQueryor.FirmwareInstall(
			ctx,
			h.firmware.Component,
			h.firmware.Vendor,
			h.action.Component.Model,
			h.firmware.Version,
			h.action.FirmwareTempFile,
			h.action.ForceInstall,
		); err != nil {
			// component update could not be applied because it requires a host power cycle
			if errors.Is(err, iutils.ErrRebootRequired) {
				h.logger.WithFields(
					logrus.Fields{
						"component": h.firmware.Component,
						"update":    h.firmware.FileName,
						"version":   h.firmware.Version,
						"msg":       err.Error(),
					}).Info("firmware install requires a server power cycle")

				// force power cycle if we're on the last action
				if h.action.Last {
					return h.powerCycleServer(ctx)
				}

				h.task.Data.HostPowercycleRequired = true
				return nil
			}

			return err
		}
	}

	h.logger.WithFields(
		logrus.Fields{
			"component": h.firmware.Component,
			"update":    h.firmware.FileName,
			"version":   h.firmware.Version,
		}).Info("firmware installed")

	return nil
}

func (h *handler) powerCycleServer(ctx context.Context) error {
	if h.task.Parameters.DryRun {
		h.logger.WithFields(
			logrus.Fields{
				"component": h.firmware.Component,
				"update":    h.firmware.FileName,
				"version":   h.firmware.Version,
			}).Info("power cycling server - dry-run")

		return nil
	}

	if h.action.HostPowerCycleInitiated {
		h.logger.WithFields(
			logrus.Fields{
				"component": h.firmware.Component,
				"update":    h.firmware.FileName,
				"version":   h.firmware.Version,
			}).Info("server previously power cycled, not attempting another.")

		return nil
	}

	h.logger.WithFields(
		logrus.Fields{
			"component": h.firmware.Component,
			"update":    h.firmware.FileName,
			"version":   h.firmware.Version,
		}).Info("power cycling server")

	f, err := os.OpenFile(rebootFlag, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	h.logger.Infof("%s reboot flag created, waiting for host power cycle..", rebootFlag)

	// we must be able to publish a status at this point
	h.action.HostPowerCycleInitiated = true
	h.task.Status.Append("server powercycle flag set, waiting for powercycle")
	if errPub := h.publisher.Publish(ctx, h.task); errPub != nil {
		h.logger.WithError(errPub).Info("publish failure")
		return errPub
	}

	return model.ErrHostPowerCycleRequired
}
