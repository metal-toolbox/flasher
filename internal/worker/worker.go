package worker

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/metal-toolbox/flasher/internal/firmware"
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
	concurrency int
	syncWG      *sync.WaitGroup
	// map of task IDs to task state machines
	taskMachines sync.Map
	store        store.Storage
	inv          inventory.Inventory
	logger       *logrus.Logger
}

// NewOutofbandWorker returns a out of band firmware install worker instance
func New(concurrency int, syncWG *sync.WaitGroup, taskStore store.Storage, inv inventory.Inventory, logger *logrus.Logger) *Worker {
	return &Worker{
		concurrency:  concurrency,
		syncWG:       syncWG,
		taskMachines: sync.Map{},
		store:        taskStore,
		inv:          inv,
		logger:       logger,
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

	for {
		select {
		case <-tickQueueRun:
			o.queue(ctx)
			o.run(ctx)
		case <-ctx.Done():
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
	return &sm.HandlerContext{
		TaskID:    taskID,
		Ctx:       ctx,
		FwPlanner: firmware.NewPlanner(skipCompareInstalled, device.Vendor, device.Model),

		Store:  o.store,
		Inv:    o.inv,
		Logger: o.logger,
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
		sm, err := sm.NewTaskStateMachine(ctx, &tasks[idx], handler)
		if err != nil {
			o.logger.Error(err)
		}

		// add to task machines list
		o.taskMachines.Store(task.ID.String(), *sm)

		// TODO: spawn block in a go routine with a limiter
		//
		if err := sm.Run(ctx, &tasks[idx], handler, taskHandlerCtx); err != nil {
			o.logger.Error(err)

			// remove from task machines list
			o.taskMachines.Delete(task.ID.String())

			continue
		}
	}
}

func (o *Worker) queue(ctx context.Context) {
	devices, err := o.inv.ListDevicesForFwInstall(ctx, aquireDeviceLimit)
	if err != nil {
		o.logger.Warn(err)
		return
	}

	for _, device := range devices {
		if o.concurrencyLimit() {
			continue
		}

		acquired, err := o.inv.AquireDevice(ctx, device.ID.String())
		if err != nil {
			o.logger.Warn(err)

			continue
		}

		if err := o.createTaskForDevice(ctx, acquired); err != nil {
			o.logger.Warn(err)

			continue
		}
	}
}

func (o *Worker) createTaskForDevice(ctx context.Context, device model.Device) error {
	task, err := model.NewTask("", nil)
	if err != nil {
		return err
	}

	task.Status = string(model.StateQueued)
	task.Parameters.Device = device

	if _, err := o.store.AddTask(ctx, task); err != nil {
		return err
	}

	return nil
}
