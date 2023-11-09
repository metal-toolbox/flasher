package install

import (
	"context"
	"net"
	"time"

	"github.com/google/uuid"
	"github.com/metal-toolbox/flasher/internal/model"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
	rctypes "github.com/metal-toolbox/rivets/condition"
	"github.com/sirupsen/logrus"
)

type Installer struct {
	logger *logrus.Logger
}

func New(logger *logrus.Logger) *Installer {
	return &Installer{logger: logger}
}

func (i *Installer) Install(ctx context.Context, bmcAddr, user, pass, component, file, version string, dryRun bool) {
	task := &model.Task{
		ID:         uuid.New(),
		Parameters: rctypes.FirmwareInstallTaskParameters{},
		Status:     model.NewTaskStatusRecord("initialized task"),
	}

	// setup state machine task handler
	handler := &taskHandler{
		fwFile:      file,
		fwVersion:   version,
		fwComponent: component,
	}

	handlerCtx := &sm.HandlerContext{
		Dryrun: dryRun,
		Task:   task,
		Ctx:    ctx,
		Data:   make(map[string]string),
		Asset: &model.Asset{
			BmcAddress:  net.IP(bmcAddr),
			BmcUsername: user,
			BmcPassword: pass,
		},
		Logger: i.logger.WithFields(
			logrus.Fields{
				"bmc":       bmcAddr,
				"component": component,
			},
		),
	}

	i.runTaskStatemachine(handler, handlerCtx)
}

func (i *Installer) runTaskStatemachine(handler *taskHandler, handlerCtx *sm.HandlerContext) {
	startTS := time.Now()

	i.logger.WithFields(logrus.Fields{
		"bmc-ip": handlerCtx.Asset.BmcAddress,
	}).Info("running task for device")

	// init state machine for task
	stateMachine, err := sm.NewTaskStateMachine(handler)
	if err != nil {
		i.logger.Error(err)

		return
	}

	// run task state machine
	if err := stateMachine.Run(handlerCtx.Task, handlerCtx); err != nil {
		i.logger.WithFields(
			logrus.Fields{
				"bmc-ip": handlerCtx.Asset.BmcAddress,
				"err":    err.Error(),
			},
		).Warn("task for device failed")

		return
	}

	i.logger.WithFields(logrus.Fields{
		"bmc-ip":  handlerCtx.Asset.BmcAddress,
		"elapsed": time.Since(startTS).String(),
	}).Info("task for device completed")
}
