package outofband

import (
	"context"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/metal-toolbox/flasher/internal/fixtures"
	"github.com/metal-toolbox/flasher/internal/inventory"
	"github.com/metal-toolbox/flasher/internal/model"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
	"github.com/metal-toolbox/flasher/internal/store"

	sw "github.com/filanov/stateswitch"
	"github.com/sirupsen/logrus"
)

func newTaskFixture(status string) *model.Task {
	task, _ := model.NewTask("", nil)
	task.Status = string(status)
	task.FirmwaresPlanned = fixtures.Firmware
	task.Parameters.Device = fixtures.Devices[fixtures.Device1.String()]
	return &task
}

func newtaskHandlerContextFixture(taskID string, device *model.Device) *sm.HandlerContext {
	inv, _ := inventory.NewMockInventory()
	logger := logrus.New().WithField("test", "true")
	return &sm.HandlerContext{
		TaskID:        taskID,
		DeviceQueryor: fixtures.NewDeviceQueryor(context.Background(), device, logger),
		Ctx:           context.Background(),
		Store:         store.NewMemStore(),
		Inv:           inv,
		Logger:        logger,
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
			newTaskFixture(string(model.StateQueued)),
			[]sw.TransitionType{sm.Plan},
			string(model.StateActive),
			false,
			false,
		},
	}

	fixtureAction := model.Action{
		ID:            "testing",
		TaskID:        "testing",
		InstallMethod: model.InstallMethodOutofband,
		Status:        string(model.StateQueued),
		Firmware:      fixtures.Firmware[0],
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			// init task handler context
			tctx := newtaskHandlerContextFixture(tc.task.ID.String(), &model.Device{})

			// init new state machine
			m, err := NewActionStateMachine(ctx, "testing")
			if err != nil {
				t.Fatal(err)
			}

			// run transition
			err = m.Run(ctx, &fixtureAction, tctx)
			if err != nil {
				if !tc.expectError {
					t.Fatal(err)
				}
			}

			// lookup task from cache
			//task, _ := tctx.Store.TaskByID(ctx, tc.task.ID.String())

			spew.Dump(fixtureAction)

		})
	}
}
