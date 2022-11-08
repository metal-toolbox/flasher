package worker

import (
	"context"

	"github.com/metal-toolbox/flasher/internal/model"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
	"github.com/sirupsen/logrus"
)

type Runner interface {
	NewActionStateMachine(ctx context.Context, actionID string) (*sm.ActionStateMachine, error)
	NewDeviceQueryor(ctx context.Context, device *model.Device, logger *logrus.Entry) model.DeviceQueryor
}
