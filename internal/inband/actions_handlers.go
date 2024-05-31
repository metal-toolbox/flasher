package inband

import (
	"context"
	"os"
	"path/filepath"

	"github.com/metal-toolbox/flasher/internal/device"
	"github.com/metal-toolbox/flasher/internal/download"
	"github.com/metal-toolbox/flasher/internal/metrics"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

const (
	// firmware files are downloaded into this directory
	downloadDir = "/tmp"
)

type handler struct {
	firmware      *model.Firmware
	task          *model.Task
	action        *model.Action
	deviceQueryor device.InbandQueryor
	publisher     model.Publisher
	logger        *logrus.Entry
}

func (h *handler) checkCurrentFirmware(ctx context.Context) error {

	//
	return nil
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
	err = download.FromUrlToFile(ctx, h.firmware.URL, file)
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
