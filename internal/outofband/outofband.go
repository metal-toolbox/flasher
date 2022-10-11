package outofband

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/metal-toolbox/flasher/internal/bmc"
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

type OutofbandWorker struct {
	concurrency int
	// map of task IDs to task state machines
	taskMachines sync.Map
	cache        store.Storage
	inv          inventory.Inventory
	logger       *logrus.Logger
}

// NewOutofbandWorker returns a out of band firmware install worker instance
func NewOutofbandWorker(concurrency int, cache store.Storage, inv inventory.Inventory, logger *logrus.Logger) *OutofbandWorker {
	return &OutofbandWorker{
		concurrency:  concurrency,
		taskMachines: sync.Map{},
		cache:        cache,
		inv:          inv,
		logger:       logger,
	}
}

// RunWorker runs the fimware install worker.
//
// A firmware install worker runs in a loop, querying the inventory
// for devices that require updates.
//
// It proceeds to queue and install updates on those devices.
func (o *OutofbandWorker) Run(ctx context.Context) {
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

func (o *OutofbandWorker) concurrencyLimit() bool {
	var count int

	o.taskMachines.Range(func(key any, value any) bool {
		count++
		return true
	})

	return count >= o.concurrency
}

func (o *OutofbandWorker) newtaskHandlerContext(ctx context.Context, taskID string, device *model.Device, skipCompareInstalled bool) *sm.HandlerContext {
	return &sm.HandlerContext{
		TaskID:    taskID,
		Ctx:       ctx,
		FwPlanner: firmware.NewPlanner(skipCompareInstalled, device.Vendor, device.Model),
		Bmc:       bmc.NewQueryor(ctx, device, o.logger),
		Cache:     o.cache,
		Inv:       o.inv,
		Logger:    o.logger,
	}
}

func (o *OutofbandWorker) run(ctx context.Context) {
	tasks, err := o.cache.TasksByStatus(ctx, string(sm.StateQueued))
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
		taskHandlerCtx := o.newtaskHandlerContext(ctx, task.ID.String(), &task.Device, task.Parameters.ForceInstall)

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

func (o *OutofbandWorker) queue(ctx context.Context) {
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

func (o *OutofbandWorker) createTaskForDevice(ctx context.Context, device model.Device) error {
	task, err := model.NewTask(model.InstallMethodOutofband, "", nil)
	if err != nil {
		return err
	}

	task.Status = string(sm.StateQueued)
	task.Device = device

	if _, err := o.cache.AddTask(ctx, task); err != nil {
		return err
	}

	return nil
}
