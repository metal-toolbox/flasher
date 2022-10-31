package worker

import (
	"context"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/metal-toolbox/flasher/internal/inventory"
	"github.com/metal-toolbox/flasher/internal/model"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
	"github.com/metal-toolbox/flasher/internal/store"
	"github.com/sirupsen/logrus"
)

const (
	// number of devices to acquire for update per tick.
	aquireDeviceLimit = 1
)

type Worker struct {
	id                string
	dryrun            bool
	concurrency       int
	firmwareURLPrefix string
	syncWG            *sync.WaitGroup
	// map of task IDs to task state machines
	taskMachines sync.Map
	store        store.Storage
	inv          inventory.Inventory
	logger       *logrus.Logger
}

// NewOutofbandWorker returns a out of band firmware install worker instance
func New(
	firmwareURLPrefix,
	facilityCode string,
	dryrun bool,
	concurrency int,
	syncWG *sync.WaitGroup,
	taskStore store.Storage,
	inv inventory.Inventory,
	logger *logrus.Logger,
) *Worker {
	id, _ := os.Hostname()

	return &Worker{
		id:                id,
		dryrun:            dryrun,
		concurrency:       concurrency,
		firmwareURLPrefix: firmwareURLPrefix,
		syncWG:            syncWG,
		taskMachines:      sync.Map{},
		store:             taskStore,
		inv:               inv,
		logger:            logger,
	}
}

// Run runs the fimware install worker.
//
// A firmware install worker runs in a loop, querying the inventory
// for devices that require updates.
//
// It proceeds to queue and install updates on those devices.
func (o *Worker) Run(ctx context.Context) {
	tickQueueRun := time.NewTicker(time.Duration(10) * time.Second).C

	o.logger.Info("worker started in dry-run mode")

	for {
		select {
		case <-tickQueueRun:
			o.queue(ctx)
			o.run(ctx)
		case <-ctx.Done():
			//o.cleanup(ctx)
			return
		}
	}
}

func (o *Worker) concurrencyLimit() bool {
	var count int

	o.taskMachines.Range(func(key any, value any) bool {
		count++
		return true
	})

	return count >= o.concurrency
}

func (o *Worker) newtaskHandlerContext(ctx context.Context, taskID string, device *model.Device, skipCompareInstalled bool) *sm.HandlerContext {
	l := logrus.New()
	l.Formatter = o.logger.Formatter
	l.Level = o.logger.Level

	return &sm.HandlerContext{
		WorkerID:          o.id,
		Dryrun:            o.dryrun,
		TaskID:            taskID,
		Ctx:               ctx,
		Store:             o.store,
		Inv:               o.inv,
		FirmwareURLPrefix: o.firmwareURLPrefix,
		Data:              make(map[string]string),
		Logger: l.WithFields(
			logrus.Fields{
				"workerID": o.id,
				"taskID":   taskID,
				"deviceID": device.ID.String(),
				"bmc":      device.BmcAddress.String(),
			},
		),
	}
}

func (o *Worker) run(ctx context.Context) {
	tasks, err := o.store.TasksByStatus(ctx, string(model.StateQueued))
	if err != nil {
		if errors.Is(err, store.ErrNoTasksFound) {
			return
		}

		o.logger.Warn(err)
	}

	for idx, task := range tasks {
		if o.concurrencyLimit() {
			return
		}

		// define state machine task handler
		handler := &taskHandler{}

		// task handler context
		taskHandlerCtx := o.newtaskHandlerContext(ctx, task.ID.String(), &task.Parameters.Device, task.Parameters.ForceInstall)

		// init state machine for task
		m, err := sm.NewTaskStateMachine(ctx, &tasks[idx], handler)
		if err != nil {
			o.logger.Error(err)
		}

		// add to task machines list
		o.taskMachines.Store(task.ID.String(), *m)

		o.logger.WithField("deviceID", task.Parameters.Device.ID.String()).Trace("run task for device")

		if taskHandlerCtx.Dryrun {
			o.logger.WithField("deviceID", task.Parameters.Device.ID.String()).Trace("task to be run in dry-run mode")
		}
		// TODO: spawn block in a go routine with a limiter

		// run task state machine
		if err := m.Run(ctx, &tasks[idx], handler, taskHandlerCtx); err != nil {
			// mark task failed and cleanup
			//
			// since the current implementation of the state machine
			// has no transition failure handler, the failed task actions
			// ands handled in this method
			//
			// TODO(joel) - extend stateswitch library to have an OnFailure method
			o.taskFailed(ctx, &tasks[idx])

			o.logger.Error(err)

			continue
		}

		o.logger.WithFields(logrus.Fields{
			"deviceID": task.Parameters.Device.ID.String(),
			"taskID":   task.ID,
		}).Info("task for device completed")
	}
}

func (o *Worker) queue(ctx context.Context) {
	idevices, err := o.inv.ListDevicesForFwInstall(ctx, aquireDeviceLimit)
	if err != nil {
		o.logger.Warn(err)
		return
	}

	for _, idevice := range idevices {
		if o.concurrencyLimit() {
			continue
		}

		acquired, err := o.inv.AquireDevice(ctx, idevice.Device.ID.String(), o.id)
		if err != nil {
			o.logger.Warn(err)

			continue
		}

		taskID, err := o.enqueueTask(ctx, &acquired)
		if err != nil {
			o.logger.Warn(err)

			continue
		}

		o.logger.WithFields(logrus.Fields{
			"deviceID": idevice.Device.ID.String(),
			"taskID":   taskID,
		}).Trace("queued task for device")
	}
}

func (o *Worker) enqueueTask(ctx context.Context, inventoryDevice *inventory.InventoryDevice) (taskID string, err error) {
	task, err := model.NewTask("", nil)
	if err != nil {
		return taskID, err
	}

	// set task parameters based on inventory device flasher fw install attributes
	task.Status = string(model.StateQueued)
	task.Parameters.Device = inventoryDevice.Device
	task.Parameters.ForceInstall = inventoryDevice.FwInstallAttributes.ForceInstall
	task.Parameters.ResetBMCBeforeInstall = inventoryDevice.FwInstallAttributes.ResetBMCBeforeInstall
	task.Parameters.Priority = inventoryDevice.FwInstallAttributes.Priority

	id, err := o.store.AddTask(ctx, task)
	if err != nil {
		return taskID, err
	}

	return id.String(), nil
}

func (o *Worker) taskFailed(ctx context.Context, task *model.Task) {
	// update task state in inventory - move to task handler context?
	attr := &inventory.FwInstallAttributes{
		TaskParameters: task.Parameters,
		FlasherTaskID:  task.ID.String(),
		WorkerID:       o.id,
		Status:         string(model.StateFailed),
		Info:           task.Info,
	}

	if err := o.inv.SetFlasherAttributes(ctx, task.Parameters.Device.ID.String(), attr); err != nil {
		o.logger.WithFields(
			logrus.Fields{
				"deviceID": task.Parameters.Device.ID,
				"taskID":   task.ID.String(),
				"err":      err.Error(),
			},
		).Warn("failed inventory device flasher attribute set")
	}

	// purge task state machine
	o.taskMachines.Delete(task.ID.String())

	o.logger.WithFields(
		logrus.Fields{
			"deviceID": task.Parameters.Device.ID,
			"taskID":   task.ID.String(),
		},
	).Trace("failed task purged")
}
