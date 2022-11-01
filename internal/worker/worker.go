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
	limiter      *Limiter
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
		limiter:           NewLimiter(concurrency),
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

	o.logger.WithFields(
		logrus.Fields{
			"concurrency": o.concurrency,
			"dry-run":     o.dryrun,
		},
	).Info("flasher worker running")

	for {
		select {
		case <-tickQueueRun:
			o.queue(ctx)
			o.run(ctx)
		case <-ctx.Done():
			o.limiter.StopWait()
			return
		}
	}
}

func (o *Worker) concurrencyLimit() bool {
	return o.limiter.ActiveCount() >= o.concurrency
}

func (o *Worker) queue(ctx context.Context) {
	idevices, err := o.inv.ListDevicesForFwInstall(ctx, aquireDeviceLimit)
	if err != nil {
		o.logger.Warn(err)
		return
	}

	for _, idevice := range idevices {
		if o.concurrencyLimit() {
			o.logger.WithFields(logrus.Fields{
				"deviceID":    idevice.Device.ID.String(),
				"concurrency": o.concurrency,
				"active":      o.limiter.ActiveCount(),
			}).Trace("skipped queuing task for device, concurrency limit")

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

func (o *Worker) run(ctx context.Context) {
	tasks, err := o.store.TasksByStatus(ctx, string(model.StateQueued))
	if err != nil {
		if errors.Is(err, store.ErrNoTasksFound) {
			return
		}

		o.logger.Warn(err)
	}

	for idx := range tasks {
		if o.concurrencyLimit() {
			o.logger.WithFields(logrus.Fields{
				"deviceID":    tasks[idx].Parameters.Device.ID.String(),
				"concurrency": o.concurrency,
				"active":      o.limiter.ActiveCount(),
			}).Trace("skipped running task for device, concurrency limit")

			return
		}

		// create work from task
		work := o.initializeWork(ctx, &tasks[idx])

		// dispatch work for execution
		if err := o.limiter.Dispatch(work); err != nil {
			o.logger.WithFields(logrus.Fields{"err": err.Error()}).Warn("limiter dispatch error")
		}
	}
}

// initializeWork initializes a statemachine to execute a task and returns a func.
func (o *Worker) initializeWork(ctx context.Context, task *model.Task) func() {
	return func() {
		o.logger.WithFields(logrus.Fields{
			"deviceID": task.Parameters.Device.ID.String(),
			"taskID":   task.ID,
		}).Info("running task for device")

		// define state machine task handler
		handler := &taskHandler{}

		// task handler context
		taskHandlerCtx := o.newtaskHandlerContext(ctx, task.ID.String(), &task.Parameters.Device, task.Parameters.ForceInstall)

		// init state machine for task
		m, err := sm.NewTaskStateMachine(ctx, task, handler)
		if err != nil {
			o.logger.Error(err)
		}

		// add to task machines list
		o.taskMachines.Store(task.ID.String(), *m)

		// purge task state machine when this method returns
		defer o.taskMachines.Delete(task.ID.String())

		o.logger.WithField("deviceID", task.Parameters.Device.ID.String()).Trace("run task for device")

		if taskHandlerCtx.Dryrun {
			o.logger.WithField("deviceID", task.Parameters.Device.ID.String()).Trace("task to be run in dry-run mode")
		}

		// run task state machine
		if err := m.Run(ctx, task, handler, taskHandlerCtx); err != nil {
			o.taskFailed(ctx, task)

			o.logger.Error(err)

			return
		}

		o.logger.WithFields(logrus.Fields{
			"deviceID": task.Parameters.Device.ID.String(),
			"taskID":   task.ID,
		}).Info("task for device completed")
	}
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

	o.logger.WithFields(
		logrus.Fields{
			"deviceID": task.Parameters.Device.ID,
			"taskID":   task.ID.String(),
		},
	).Trace("task for device failed")
}
