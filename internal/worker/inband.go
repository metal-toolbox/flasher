package worker

import (
	"context"

	"github.com/metal-toolbox/ctrl"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/runner"
	"github.com/metal-toolbox/flasher/internal/store"
	"github.com/metal-toolbox/flasher/internal/version"
	rctypes "github.com/metal-toolbox/rivets/v2/condition"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
)

// implements the controller.TaskHandler interface
type InbandConditionTaskHandler struct {
	store          store.Repository
	logger         *logrus.Logger
	facilityCode   string
	dryrun         bool
	faultInjection bool
}

// RunInband initializes the inband installer
func RunInband(
	ctx context.Context,
	dryrun,
	faultInjection bool,
	facilityCode string,
	repository store.Repository,
	nc *ctrl.HTTPController,
	logger *logrus.Logger,
) {
	ctx, span := otel.Tracer(pkgName).Start(
		ctx,
		"Run",
	)
	defer span.End()

	v := version.Current()
	logger.WithFields(
		logrus.Fields{
			"version":        v.AppVersion,
			"commit":         v.GitCommit,
			"branch":         v.GitBranch,
			"dry-run":        dryrun,
			"faultInjection": faultInjection,
		},
	).Info("flasher inband installer running")

	inbHandler := InbandConditionTaskHandler{
		store:          repository,
		logger:         logger,
		dryrun:         dryrun,
		faultInjection: faultInjection,
		facilityCode:   facilityCode,
	}

	if err := nc.Run(ctx, &inbHandler); err != nil {
		logger.Fatal(err)
	}
}

// Handle implements the controller.ConditionHandler interface
func (h *InbandConditionTaskHandler) HandleTask(
	ctx context.Context,
	genericTask *rctypes.Task[any, any],
	publisher ctrl.Publisher,
) error {
	if genericTask == nil {
		return errors.Wrap(errInitTask, "expected a generic Task object, got nil")
	}

	task, err := model.CopyAsFwInstallTask(genericTask)
	if err != nil {
		return errors.Wrap(errInitTask, err.Error())
	}

	// prepare logger
	l := logrus.New()
	l.Formatter = h.logger.Formatter
	l.Level = h.logger.Level
	hLogger := l.WithFields(
		logrus.Fields{
			"conditionID": genericTask.ID.String(),
			"serverID":    task.Server.ID,
			"mode":        model.RunInband,
		},
	)

	// init handler
	handler := newHandler(
		model.RunInband,
		task,
		h.store,
		model.NewTaskStatusPublisher(hLogger, publisher),
		hLogger,
	)

	// init runner
	r := runner.New(hLogger)

	hLogger.Info("running task for device")
	if err := r.RunTask(ctx, task, handler); err != nil {
		hLogger.WithError(err).Error("task for device failed")
		return err
	}

	hLogger.Info("task for device completed")
	return nil
}
