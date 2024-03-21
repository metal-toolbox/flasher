package worker

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/runner"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
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
	fetchEventsInterval = 10 * time.Second

	// taskTimeout defines the time after which a task will be canceled.
	taskTimeout = 180 * time.Minute

	// taskInprogressTicker is the interval at which tasks in progress
	// will ack themselves as in progress on the event stream.
	//
	// This value should be set to less than the event stream Ack timeout value.
	taskInprogressTick = 3 * time.Minute

	errTaskFirmwareParam = errors.New("error in task firmware parameters")
	errInitTask          = errors.New("error initializing new task from condition")
)

type ConditionTaskHandler struct {
	store          store.Repository
	syncWG         *sync.WaitGroup
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
func (h *ConditionTaskHandler) Handle(ctx context.Context, condition *rctypes.Condition, publisher controller.ConditionStatusPublisher) error {
	task, err := newTaskFromCondition(condition, h.faultInjection)
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
		ctx,
		h.dryrun,
		h.controllerID,
		h.facilityCode,
		task,
		asset,
		h.store,
		sm.NewNatsConditionStatusPublisher(publisher),
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

func newTask(conditionID uuid.UUID, params *rctypes.FirmwareInstallTaskParameters) (model.Task, error) {
	task := model.Task{
		ID:         conditionID,
		Parameters: *params,
		Status:     model.NewTaskStatusRecord("initialized task"),
	}

	//nolint:errcheck // this method returns nil unconditionally
	task.SetState(model.StatePending)

	if len(params.Firmwares) > 0 {
		task.Parameters.Firmwares = params.Firmwares
		task.FirmwarePlanMethod = model.FromRequestedFirmware

		return task, nil
	}

	if params.FirmwareSetID != uuid.Nil {
		task.Parameters.FirmwareSetID = params.FirmwareSetID
		task.FirmwarePlanMethod = model.FromFirmwareSet

		return task, nil
	}

	return task, errors.Wrap(errTaskFirmwareParam, "no firmware list or firmwareSetID specified")
}

// newTaskFromMsg returns a new task object with the given parameters
func newTaskFromCondition(condition *rctypes.Condition, faultInjection bool) (*model.Task, error) {
	parameters := &rctypes.FirmwareInstallTaskParameters{}
	if err := json.Unmarshal(condition.Parameters, parameters); err != nil {
		return nil, errors.Wrap(errInitTask, "Firmware install task parameters error: "+err.Error())
	}

	task, err := newTask(condition.ID, parameters)
	if err != nil {
		return nil, err
	}

	if faultInjection && condition.Fault != nil {
		task.Fault = condition.Fault
	}

	return &task, nil
}
