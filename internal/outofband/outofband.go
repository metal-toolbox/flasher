package outofband

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/metal-toolbox/flasher/internal/inventory"
	"github.com/metal-toolbox/flasher/internal/model"
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

	if count >= o.concurrency {
		return true
	}

	return false
}

func (o *OutofbandWorker) stateMachineContext(ctx context.Context, taskID string, actionsSM []*StateMachineAction) *StateMachineContext {
	return &taskStateMachineContext{
		taskID:    taskID,
		ctx:       ctx,
		actionsSM: actionsSM,
		cache:     o.cache,
		inv:       o.inv,
		logger:    o.logger,
	}
}

func (o *OutofbandWorker) run(ctx context.Context) {
	tasks, err := o.cache.TasksByStatus(ctx, string(stateQueued))
	if err != nil {
		if errors.Is(err, store.ErrNoTasksFound) {
			return
		}

		o.logger.Warn(err)
	}

	for _, task := range tasks {
		if o.concurrencyLimit() {
			return
		}

		// define state machine task handler
		handler := &taskHandler{}

		// TODO: handle case where no task

		// init state machine for task
		sm, actions, err := NewTaskStateMachine(ctx, &task, handler)
		if err != nil {
			o.logger.Error(err)
		}

		// context passed to state machine handlers
		stateMachineCtx := o.newTaskstateMachineContext(task.ID.String(), actions)

		// add to task machines list
		o.taskMachines.Store(task.ID.String(), *sm)

		// TODO: spawn block in a go routine with a limiter
		//
		// TODO: create channel for actions state machine to trigger state saves
		if err := sm.run(ctx, &task, stateMachineCtx); err != nil {
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

		acquired, err := o.inv.AquireDeviceByID(ctx, device.ID.String())
		if err != nil {
			o.logger.Warn(err)

			continue
		}

		if err := o.createTask(ctx, acquired); err != nil {
			o.logger.Warn(err)

			continue
		}
	}
}

func (o *OutofbandWorker) createTask(ctx context.Context, device model.Device) error {
	configuration, err := o.inv.FirmwareConfiguration(ctx, device)
	if err != nil {
		return err
	}

	task := model.Task{
		Status: string(stateQueued),
		Device: device,
		Parameters: model.TaskParameters{
			Configuration: configuration,
			Install:       []model.InstallParameter{},
		},
	}

	if _, err = o.cache.AddTask(ctx, task); err != nil {
		return err
	}

	return nil
}
