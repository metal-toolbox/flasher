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
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
	rctypes "github.com/metal-toolbox/rivets/condition"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
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

	task := &model.Task{
		ID: uuid.New(),
		Parameters: rctypes.FirmwareInstallTaskParameters{
			ForceInstall: params.Force,
			DryRun:       params.DryRun,
		},
		Status: model.NewTaskStatusRecord("initialized task"),
	}

	le := i.logger.WithFields(
		logrus.Fields{
			"dry-run":   params.DryRun,
			"bmc":       params.BmcAddr,
			"component": params.Component,
		})

	i.runTask(ctx, params, task, le)
}

type publisher struct{}

func (f *publisher) Publish(_ *sm.HandlerContext) {}

func (i *Installer) runTask(ctx context.Context, params *Params, task *model.Task, le *logrus.Entry) {
	h := &handler{
		fwFile:      params.File,
		fwComponent: params.Component,
		fwVersion:   params.Version,
		model:       params.Model,
		vendor:      params.Vendor,
		onlyPlan:    params.OnlyPlan,
		ctx: &sm.HandlerContext{
			Dryrun:    params.DryRun,
			Task:      task,
			Ctx:       ctx,
			Publisher: &publisher{},
			Data:      make(map[string]string),
			Asset: &model.Asset{
				BmcAddress:  net.ParseIP(params.BmcAddr),
				BmcUsername: params.User,
				BmcPassword: params.Pass,
				Model:       params.Model,
				Vendor:      params.Vendor,
			},
			Logger: le,
		},
	}

	r := runner.New(le)

	startTS := time.Now()

	i.logger.Info("running task for device")

	if err := r.RunTask(ctx, task, h); err != nil {
		i.logger.WithFields(
			logrus.Fields{
				"bmc-ip": h.ctx.Asset.BmcAddress.String(),
				"err":    err.Error(),
			},
		).Warn("task for device failed")

		return
	}

	i.logger.WithFields(logrus.Fields{
		"bmc-ip":  h.ctx.Asset.BmcAddress,
		"elapsed": time.Since(startTS).String(),
	}).Info("task for device completed")
}
