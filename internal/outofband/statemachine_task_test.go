package outofband

import (
	"context"
	"fmt"
	"testing"

	sw "github.com/filanov/stateswitch"
	"github.com/metal-toolbox/flasher/internal/firmware"
	"github.com/metal-toolbox/flasher/internal/fixtures"
	"github.com/metal-toolbox/flasher/internal/inventory"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/store"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func newTaskFixture(status string) *model.Task {
	task, _ := model.NewTask(model.InstallMethodOutofband, "", nil)
	task.Status = string(status)
	task.FirmwaresPlanned = fixtures.Firmware

	return &task
}

func newtaskHandlerContextFixture(taskID string, device *model.Device) *taskHandlerContext {
	inv, _ := inventory.NewMockInventory()
	return &taskHandlerContext{
		taskID:    taskID,
		bmc:       NewBmcMockQueryor(context.Background(), device, logrus.New()),
		ctx:       context.Background(),
		cache:     store.NewCacheStore(),
		inv:       inv,
		fwPlanner: firmware.NewMockPlanner(),
		logger:    logrus.New(),
	}
}

func Test_NewTaskStateMachine(t *testing.T) {
	task, _ := model.NewTask(model.InstallMethodOutofband, "", nil)
	task.Status = string(stateQueued)

	tests := []struct {
		name string
		task *model.Task
	}{
		{
			"new task statemachine is created",
			newTaskFixture(string(stateQueued)),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			// transition handler implements the taskTransitioner methods to complete tasks
			handler := &taskHandler{}
			m, err := NewTaskStateMachine(ctx, tc.task, handler)
			if err != nil {
				t.Fatal(err)
			}

			assert.NotNil(t, m.sm)
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
			newTaskFixture(string(stateQueued)),
			[]sw.TransitionType{planActions},
			string(stateActive),
			false,
			false,
		},
		{
			"Active to Success",
			newTaskFixture(string(stateActive)),
			[]sw.TransitionType{runActions},
			string(stateSuccess),
			false,
			false,
		},
		{
			"Queued to Failed",
			newTaskFixture(string(stateActive)),
			[]sw.TransitionType{taskFailed},
			string(stateFailed),
			true,
			false,
		},
		{
			"Active to Failed",
			newTaskFixture(string(stateQueued)),
			[]sw.TransitionType{taskFailed},
			string(stateFailed),
			true,
			false,
		},
		{
			"Success to Active fails - invalid transition",
			newTaskFixture(string(stateSuccess)),
			[]sw.TransitionType{runActions},
			string(stateFailed),
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
			m, err := NewTaskStateMachine(ctx, tc.task, handler)
			if err != nil {
				t.Fatal(err)
			}

			// set transition to perform based on test case
			m.setTransitionOrder(tc.transitionTypes)

			switch tc.transitionTypes[0] {
			// set a error for failed task
			case taskFailed:
				tctx.err = errors.New("cosmic rays")
			// set the action plan for runActions
			case runActions:
				tctx.actionPlan, err = planInstallActions(context.Background(), tc.task)
				if err != nil {
					panic(err)
				}
			}

			// run transition
			err = m.run(ctx, tc.task, handler, tctx)
			if err != nil {
				if !tc.expectError {
					t.Fatal(err)
				}
			}

			// lookup task from cache
			task, _ := tctx.cache.TaskByID(ctx, tc.task.ID.String())

			assert.Equal(t, string(tc.expectedState), task.Status)

			// set a error for failed task
			if tc.transitionTypes[0] == taskFailed {
				assert.Equal(t, "cosmic rays", task.Info)
			}

			// a transition attempt with no transition rule defined
			// should have,
			// - an error returned
			// - the task info includes the error
			// - the task state is failed
			if tc.noTransitionRule {
				fmt.Println(err)
				s := "no transition rule found for transition type 'runActions' and state 'success': error in task transition"
				assert.Equal(t, s, err.Error())
				assert.Equal(t, s, task.Info)
				assert.Equal(t, string(stateFailed), task.Status)
			}
		})
	}
}
