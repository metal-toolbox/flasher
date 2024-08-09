package worker

import (
	"context"
	"encoding/json"

	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/runner"
	"github.com/metal-toolbox/flasher/internal/store"
	"github.com/metal-toolbox/flasher/internal/version"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"

	rctypes "github.com/metal-toolbox/rivets/condition"
	"github.com/metal-toolbox/rivets/events/controller"
)

const (
	pkgName = "internal/worker"
)

var (
	errInitTask = errors.New("error initializing new task from condition")
)

type ConditionTaskHandler struct {
	store          store.Repository
	logger         *logrus.Logger
	facilityCode   string
	controllerID   string
	dryrun         bool
	faultInjection bool
}

// NewOutofbandWorker returns a out of band firmware install worker instance
func Run(
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
	).Info("flasher worker running")

	handlerFactory := func() controller.ConditionHandler {
		return &ConditionTaskHandler{
			store:          repository,
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
func (h *ConditionTaskHandler) Handle(ctx context.Context, condition *rctypes.Condition, publisher controller.ConditionStatusPublisher) error {
	task, err := newTaskFromCondition(condition, h.dryrun, h.faultInjection)
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

		return controller.ErrRetryHandler
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

// newTaskFromMsg returns a new task object with the given parameters
func newTaskFromCondition(condition *rctypes.Condition, dryRun, faultInjection bool) (*model.Task, error) {
	parameters := &rctypes.FirmwareInstallTaskParameters{}
	if err := json.Unmarshal(condition.Parameters, parameters); err != nil {
		return nil, errors.Wrap(errInitTask, "Firmware install task parameters error: "+err.Error())
	}

	t, err := model.NewTask(condition.ID, parameters)
	if err != nil {
		return nil, err
	}

	if faultInjection && condition.Fault != nil {
		t.Fault = condition.Fault
	}

	if dryRun {
		t.Parameters.DryRun = true
	}

	return &t, nil
}
