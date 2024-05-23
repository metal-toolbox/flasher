package worker

import (
	"context"
	"sync"

	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/runner"
	"github.com/metal-toolbox/flasher/internal/store"
	"github.com/metal-toolbox/flasher/internal/version"
	rctypes "github.com/metal-toolbox/rivets/condition"
	"github.com/metal-toolbox/rivets/events/controller"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
)

type InbandConditionTaskHandler struct {
	store          store.Repository
	logger         *logrus.Logger
	facilityCode   string
	controllerID   string
	dryrun         bool
	faultInjection bool
}

// RunInband initializes the inband installer
func RunInband(
	ctx context.Context,
	dryrun,
	faultInjection bool,
	repository store.Repository,
	nc *controller.NatsController,
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

	handlerFactory := func() controller.ConditionHandler {
		return &OobConditionTaskHandler{
			store:          repository,
			syncWG:         &sync.WaitGroup{},
			logger:         logger,
			dryrun:         dryrun,
			faultInjection: faultInjection,
			facilityCode:   nc.FacilityCode(),
			controllerID:   nc.ID(),
		}
	}

	if err := nc.ListenEvents(ctx, handlerFactory); err != nil {
		logger.Fatal(err)
	}
}

// Handle implements the controller.ConditionHandler interface
func (h *InbandConditionTaskHandler) Handle(ctx context.Context, condition *rctypes.Condition, ) error {
	task, err := newTaskFromCondition(condition, h.dryrun, h.faultInjection)
	if err != nil {
		return errors.Wrap(errInitTask, err.Error())
	}

	// first try to fetch asset inventory from inventory store
	asset, err := h.store.AssetByID(ctx, task.Parameters.AssetID.String())
	if err != nil {
		h.logger.WithFields(logrus.Fields{
			"assetID":      task.Parameters.AssetID.String(),
			"conditionID":  condition.ID,
			"controllerID": h.controllerID,
			"err":          err.Error(),
		}).Error("asset lookup error")

		return controller.ErrRetryHandler
	}

	task.Asset = asset
	task.FacilityCode = h.facilityCode
	task.WorkerID = h.controllerID

	// prepare logger
	l := logrus.New()
	l.Formatter = h.logger.Formatter
	l.Level = h.logger.Level
	hLogger := l.WithFields(
		logrus.Fields{
			"conditionID":  condition.ID.String(),
			"controllerID": h.controllerID,
			"assetID":      asset.ID.String(),
			"bmc":          asset.BmcAddress.String(),
		},
	)

	// init handler
	handler := newHandler(
		task,
		h.store,
		model.NewTaskStatusPublisher(
			hLogger,
			helpers.ConditionStatusPublisher,
			helpers.ConditionTaskRepository,
		),
		helpers.ConditionRequestor,
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
