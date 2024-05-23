package worker

import (
	"context"
	"sync"
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

func convToFwInstallTask(task *rctypes.Task[any, any]) (*model.Task, error) {
	fwInstallParams, ok := task.Parameters.(rctypes.FirmwareInstallTaskParameters)
	if !ok {
		return nil, errors.New("parameters are not of type FirmwareInstallTaskParameters")
	}

	return &model.Task{
		StructVersion: task.StructVersion,
		ID:            task.ID,
		Kind:          task.Kind,
		State:         task.State,
		Status:        task.Status,
		Data:          &model.TaskData{},
		Parameters:    fwInstallParams,
		Fault:         task.Fault,
		FacilityCode:  task.FacilityCode,
		Asset:         task.Asset,
		WorkerID:      task.WorkerID,
		TraceID:       task.TraceID,
		SpanID:        task.SpanID,
		CreatedAt:     task.CreatedAt,
		UpdatedAt:     task.UpdatedAt,
		CompletedAt:   task.CompletedAt,
	}, nil
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

	task, err := convToFwInstallTask(genericTask)
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
