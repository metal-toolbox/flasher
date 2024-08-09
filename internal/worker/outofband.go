package worker

import (
	"context"
	"sync"

	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/runner"
	"github.com/metal-toolbox/flasher/internal/store"
	"github.com/metal-toolbox/flasher/internal/version"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"

	"github.com/metal-toolbox/ctrl"
	rctypes "github.com/metal-toolbox/rivets/condition"
)

const (
	pkgName = "internal/worker"
)

var (
	errInitTask = errors.New("error initializing new task from condition")
)

type OobConditionTaskHandler struct {
	store          store.Repository
	syncWG         *sync.WaitGroup
	logger         *logrus.Logger
	facilityCode   string
	controllerID   string
	dryrun         bool
	faultInjection bool
}

// RunOutofband initializes the Out of band Condition handler and listens for events
func RunOutofband(
	ctx context.Context,
	dryrun,
	faultInjection bool,
	repository store.Repository,
	nc *ctrl.NatsController,
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
	).Info("flasher out-of-band installer running")

	handlerFactory := func() ctrl.TaskHandler {
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

// HandleTask implements the ctrl.TaskHandler interface
func (h *OobConditionTaskHandler) HandleTask(
	ctx context.Context,
	genericTask *rctypes.Task[any, any],
	statusPublisher ctrl.Publisher,
) error {
	task, err := model.CopyAsFwInstallTask(genericTask)
	if err != nil {
		return errors.Wrap(errInitTask, err.Error())
	}

	// first try to fetch asset inventory from inventory store
	asset, err := h.store.AssetByID(ctx, task.Parameters.AssetID.String())
	if err != nil {
		h.logger.WithFields(logrus.Fields{
			"assetID":      task.Parameters.AssetID.String(),
			"conditionID":  task.ID,
			"controllerID": h.controllerID,
			"err":          err.Error(),
		}).Error("asset lookup error")

		return ctrl.ErrRetryHandler
	}

	task.Server = asset
	task.FacilityCode = h.facilityCode
	task.WorkerID = h.controllerID

	// prepare logger
	l := logrus.New()
	l.Formatter = h.logger.Formatter
	l.Level = h.logger.Level
	hLogger := l.WithFields(
		logrus.Fields{
			"conditionID":  task.ID.String(),
			"controllerID": h.controllerID,
			"assetID":      asset.ID,
			"bmc":          asset.BMCAddress,
		},
	)

	// init handler
	handler := newHandler(
		model.RunOutofband,
		task,
		h.store,
		model.NewTaskStatusPublisher(hLogger, statusPublisher),
		hLogger,
	)

	// init runner
	r := runner.New(hLogger)

	hLogger.WithField("mode", model.RunOutofband).Info("running task for device")
	if err := r.RunTask(ctx, task, handler); err != nil {
		hLogger.WithError(err).Error("task for device failed")
		return err
	}

	hLogger.Info("task for device completed")
	return nil
}
