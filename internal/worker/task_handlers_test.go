package worker

import (
	"context"
	"testing"

	"github.com/metal-toolbox/flasher/internal/firmware"
	"github.com/metal-toolbox/flasher/internal/fixtures"
	"github.com/metal-toolbox/flasher/internal/inventory"
	"github.com/metal-toolbox/flasher/internal/model"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
	"github.com/metal-toolbox/flasher/internal/store"

	sw "github.com/filanov/stateswitch"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func newTaskFixture(status string) *model.Task {
	task, _ := model.NewTask("", nil)
	task.Status = string(status)
	task.FirmwaresPlanned = fixtures.Firmware

	return &task
}

func newtaskHandlerContextFixture(taskID string, device *model.Device) *sm.HandlerContext {
	inv, _ := inventory.NewMockInventory()
	return &sm.HandlerContext{
		TaskID:    taskID,
		Device:    fixtures.NewMockDeviceQueryor(context.Background(), device, logrus.New()),
		Ctx:       context.Background(),
		Store:     store.NewMemStore(),
		Inv:       inv,
		FwPlanner: firmware.NewMockPlanner(),
		Logger:    logrus.New(),
	}
}

func Test_NewTaskStateMachine(t *testing.T) {
	task, _ := model.NewTask("", nil)
	task.Status = string(sm.StateQueued)

	tests := []struct {
		name string
		task *model.Task
	}{
		{
			"new task statemachine is created",
			newTaskFixture(string(sm.StateQueued)),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			// transition handler implements the taskTransitioner methods to complete tasks
			handler := &taskHandler{}
			m, err := sm.NewTaskStateMachine(ctx, tc.task, handler)
			if err != nil {
				t.Fatal(err)
			}

			assert.NotNil(t, m)
		})
	}
}

func Test_Transitions(t *testing.T) {
	tests := []struct {
		name             string
		task             *model.Task
		transitionTypes  []sw.TransitionType
		expectedState    string
		expectError      bool
		noTransitionRule bool
	}{
		{
			"Queued to Active",
			newTaskFixture(string(sm.StateQueued)),
			[]sw.TransitionType{sm.Plan},
			string(sm.StateActive),
			false,
			false,
		},
		{
			"Active to Success",
			newTaskFixture(string(sm.StateActive)),
			[]sw.TransitionType{sm.Run},
			string(sm.StateSuccess),
			false,
			false,
		},
		{
			"Queued to Failed",
			newTaskFixture(string(sm.StateActive)),
			[]sw.TransitionType{sm.TaskFailed},
			string(sm.StateFailed),
			true,
			false,
		},
		{
			"Active to Failed",
			newTaskFixture(string(sm.StateQueued)),
			[]sw.TransitionType{sm.TaskFailed},
			string(sm.StateFailed),
			true,
			false,
		},
		{
			"Success to Active fails - invalid transition",
			newTaskFixture(string(sm.StateSuccess)),
			[]sw.TransitionType{sm.Run},
			string(sm.StateFailed),
			true,
			true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			// init task handler context
			tctx := newtaskHandlerContextFixture(tc.task.ID.String(), &model.Device{})
			handler := &taskHandler{}

			// init new state machine
			m, err := sm.NewTaskStateMachine(ctx, tc.task, handler)
			if err != nil {
				t.Fatal(err)
			}

			// set transition to perform based on test case
			m.SetTransitionOrder(tc.transitionTypes)

			switch tc.transitionTypes[0] {
			// set a error for FailedState task
			case sm.TaskFailed:
				tctx.Err = errors.New("cosmic rays")
			// set the action plan for Run
			case sm.Run:
				tctx.ActionPlan, err = planInstallActions(context.Background(), tc.task)
				if err != nil {
					panic(err)
				}
			}

			// run transition
			err = m.Run(ctx, tc.task, handler, tctx)
			if err != nil {
				if !tc.expectError {
					t.Fatal(err)
				}
			}

			// lookup task from cache
			task, _ := tctx.Store.TaskByID(ctx, tc.task.ID.String())

			assert.Equal(t, string(tc.expectedState), task.Status)

			// set a error for FailedState task
			if tc.transitionTypes[0] == sm.TaskFailed {
				assert.Equal(t, "cosmic rays", task.Info)
			}

			// a transition attempt with no transition rule defined
			// should have,
			// - an error returned
			// - the task info includes the error
			// - the task state is FailedState
			if tc.noTransitionRule {
				s := "no transition rule found for transition type 'run' and state 'success': error in task transition"
				assert.Equal(t, s, err.Error())
				assert.Equal(t, s, task.Info)
				assert.Equal(t, string(sm.StateFailed), task.Status)
			}
		})
	}
}
