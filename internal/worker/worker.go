package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/metal-toolbox/flasher/internal/inventory"
	"github.com/metal-toolbox/flasher/internal/model"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
	"github.com/metal-toolbox/flasher/internal/store"
	"github.com/sirupsen/logrus"
)

type Worker struct {
	id                string
	dryrun            bool
	concurrency       int
	firmwareURLPrefix string
	syncWG            *sync.WaitGroup
	// map of task IDs to task state machines
	taskMachines sync.Map
	taskEventCh  chan sm.TaskEvent
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
		taskEventCh:       make(chan sm.TaskEvent),
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

	var drain bool

Loop:
	for {
		select {
		case <-tickQueueRun:
			o.queue(ctx)
			o.run(ctx)
		case e, ok := <-o.taskEventCh:
			if !ok {
				continue
			}

			o.persistTaskStatus(e.TaskID, e.Info)

		case <-ctx.Done():
			// StopWait is called in a go routine since StopWait blocks
			// and we don't want to hold up the select by blocking.
			//
			// the next conditional checks the active count is zero before returning.
			if !drain {
				go o.limiter.StopWait()
				drain = true
			}

			if o.limiter.ActiveCount() == 0 {
				break Loop
			}
		}
	}
}

func (o *Worker) concurrencyLimit() bool {
	return o.limiter.ActiveCount() >= o.concurrency
}

func (o *Worker) queue(ctx context.Context) {
	idevices, err := o.inv.DevicesForFwInstall(ctx, o.concurrency)
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
			}).Trace("skipped queuing task for device, reached concurrency limit")
		}

		acquired, err := o.inv.AcquireDevice(ctx, idevice.Device.ID.String(), o.id)
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

func (o *Worker) enqueueTask(ctx context.Context, inventoryDevice *inventory.DeviceInventory) (taskID string, err error) {
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
		// define state machine task handler
		handler := &taskHandler{}

		// define state machine task handler context
		taskHandlerCtx := o.newtaskHandlerContext(
			ctx,
			task.ID.String(),
			&task.Parameters.Device,
			task.Parameters.ForceInstall,
		)

		startTS := time.Now()

		// set task status to active
		//
		// The task has to be set to active as soon as possible
		// so that the for select loop above does not pick it up again.
		task.Status = string(model.StateActive)
		if err := taskHandlerCtx.Store.UpdateTask(ctx, *task); err != nil {
			sm.SendEvent(
				ctx,
				o.taskEventCh,
				sm.TaskEvent{
					TaskID: task.ID.String(),
					Info: fmt.Sprintf(
						"task failed, elapsed: %s, cause: %s ",
						time.Since(startTS).String(),
						err.Error()),
				},
			)

			o.logger.WithFields(
				logrus.Fields{
					"deviceID": task.Parameters.Device.ID,
					"taskID":   task.ID.String(),
					"err":      err.Error(),
				},
			).Warn("task for device failed")

			return
		}

		sm.SendEvent(
			ctx,
			o.taskEventCh,
			sm.TaskEvent{
				TaskID: task.ID.String(),
				Info:   "running task for device",
			},
		)

		o.logger.WithFields(logrus.Fields{
			"deviceID": task.Parameters.Device.ID.String(),
			"taskID":   task.ID,
			"dry-run":  o.dryrun,
		}).Info("running task for device")

		// init state machine for task
		machine, err := sm.NewTaskStateMachine(ctx, task, handler)
		if err != nil {
			o.logger.Error(err)

			return
		}

		// add to task machine list
		o.taskMachines.Store(task.ID.String(), *machine)

		// purge task state machine when this method returns
		defer o.taskMachines.Delete(task.ID.String())

		// run task state machine
		if err := machine.Run(ctx, task, handler, taskHandlerCtx); err != nil {
			// event cause task state to be persisted
			sm.SendEvent(
				ctx,
				o.taskEventCh,
				sm.TaskEvent{
					TaskID: task.ID.String(),
					Info: fmt.Sprintf(
						"task failed, elapsed: %s, cause: %s ",
						time.Since(startTS).String(),
						err.Error()),
				},
			)

			o.logger.WithFields(
				logrus.Fields{
					"deviceID": task.Parameters.Device.ID,
					"taskID":   task.ID.String(),
					"err":      err.Error(),
				},
			).Warn("task for device failed")

			return
		}

		sm.SendEvent(
			ctx,
			o.taskEventCh,
			sm.TaskEvent{
				TaskID: task.ID.String(),
				Info:   "task completed, elapsed " + time.Since(startTS).String(),
			},
		)

		o.logger.WithFields(logrus.Fields{
			"deviceID": task.Parameters.Device.ID.String(),
			"taskID":   task.ID,
			"elapsed":  time.Since(startTS).String(),
		}).Info("task for device completed")
	}
}
