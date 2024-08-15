package install

import (
	"context"
	"log"
	"net"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/runner"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	rctypes "github.com/metal-toolbox/rivets/condition"
	rtypes "github.com/metal-toolbox/rivets/types"
)

type Installer struct {
	logger *logrus.Logger
}

func New(logger *logrus.Logger) *Installer {
	return &Installer{logger: logger}
}

type Params struct {
	BmcAddr   string
	User      string
	Pass      string
	Component string
	File      string
	Version   string
	Vendor    string
	Model     string
	DryRun    bool
	Force     bool
	OnlyPlan  bool
}

func (i *Installer) Install(ctx context.Context, params *Params) {
	_, err := os.Stat(params.File)
	if err != nil {
		log.Fatal(errors.Wrap(err, "unable to read firmware file"))
	}

	taskParams := &rctypes.FirmwareInstallTaskParameters{
		ForceInstall: params.Force,
		DryRun:       params.DryRun,
		Firmwares: []rctypes.Firmware{
			{
				Component: params.Component,
				Version:   params.Version,
				Models:    []string{params.Model},
				Vendor:    params.Vendor,
			},
		},
	}

	task, err := model.NewTask(uuid.New(), rctypes.FirmwareInstall, taskParams)
	if err != nil {
		i.logger.Fatal(err)
	}

	task.Parameters.DryRun = params.DryRun
	task.Server = &rtypes.Server{
		BMCAddress:  net.ParseIP(params.BmcAddr).String(),
		BMCUser:     params.User,
		BMCPassword: params.Pass,
		Model:       params.Model,
		Vendor:      params.Vendor,
	}

	task.Status = rctypes.NewTaskStatusRecord("initialized task")

	le := i.logger.WithFields(
		logrus.Fields{
			"dry-run":   params.DryRun,
			"bmc":       params.BmcAddr,
			"component": params.Component,
		})

	i.runTask(ctx, params, &task, le)
}

func (i *Installer) runTask(ctx context.Context, params *Params, task *model.Task, le *logrus.Entry) {
	h := &handler{
		fwFile:   params.File,
		onlyPlan: params.OnlyPlan,
		taskCtx: &runner.TaskHandlerContext{
			Task:      task,
			Publisher: nil,
			Logger:    le,
		},
	}

	r := runner.New(le)

	startTS := time.Now()

	i.logger.Info("running task for device")

	if err := r.RunTask(ctx, task, h); err != nil {
		i.logger.WithFields(
			logrus.Fields{
				"bmc-ip": task.Server.BMCAddress,
				"err":    err.Error(),
			},
		).Warn("task for device failed")

		return
	}

	i.logger.WithFields(logrus.Fields{
		"bmc-ip":  task.Server.BMCAddress,
		"elapsed": time.Since(startTS).String(),
	}).Info("task for device completed")
}
