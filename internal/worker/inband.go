package worker

import (
	"context"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/store"
	"github.com/metal-toolbox/flasher/internal/version"
	rctypes "github.com/metal-toolbox/rivets/condition"
	"github.com/metal-toolbox/rivets/events/controller"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
)

// implements the controller.ConditionHandler interface
type InbandConditionTaskHandler struct {
	store          store.Repository
	logger         *logrus.Logger
	facilityCode   string
	controllerID   string
	dryrun         bool
	faultInjection bool
	// indicates the handler has resumed work after a restart
	resumedWork bool
}

// RunInband initializes the inband installer
func RunInband(
	ctx context.Context,
	dryrun,
	faultInjection bool,
	facilityCode string,
	repository store.Repository,
	nc *controller.NatsHttpController,
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
		controllerID:   nc.ID(),
	}

	if err := nc.Run(ctx, &inbHandler); err != nil {
		logger.Fatal(err)
	}
}

// Handle implements the controller.ConditionHandler interface
func (h *InbandConditionTaskHandler) Handle(
	ctx context.Context,
	condition *rctypes.Condition,
	genericTask *rctypes.Task[any, any],
	publisher controller.ConditionStatusPublisher,
	taskRepository controller.ConditionTaskRepository,
) error {
	if condition == nil {
		h.resumedWork = true
	}

	if genericTask == nil {
		return errors.Wrap(errInitTask, "expected a generic Task object, got nil")
	}

	task, err := model.ConvToFwInstallTask(genericTask)
	if err != nil {
		return errors.Wrap(errInitTask, err.Error())
	}

	// prepare logger
	l := logrus.New()
	l.Formatter = h.logger.Formatter
	l.Level = h.logger.Level
	hLogger := l.WithFields(
		logrus.Fields{
			"conditionID":  condition.ID.String(),
			"controllerID": h.controllerID,
			"assetID":      task.Asset.ID.String(),
		},
	)

	spew.Dump(condition)
	spew.Dump(task)

	time.Sleep(600 * time.Second)

	hLogger.Info("task for device completed")
	return nil
}
