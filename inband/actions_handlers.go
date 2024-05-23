package inband

import (
	"context"

	"github.com/metal-toolbox/flasher/internal/device"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/sirupsen/logrus"
)

type handler struct {
	firmware      *model.Firmware
	task          *model.Task
	action        *model.Action
	deviceQueryor device.Queryor
	publisher     model.Publisher
	logger        *logrus.Entry
}

func (h *handler) checkCurrentFirmware(ctx context.Context) error {

	//
	return nil
}
