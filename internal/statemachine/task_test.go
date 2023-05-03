//nolint:all // don't lint test files
package statemachine

// this package gets its own fixtures to prevent import cycles.

import (
	"context"
	"net"
	"testing"

	sw "github.com/filanov/stateswitch"
	"github.com/google/uuid"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/stretchr/testify/assert"
)

var (
	firmware = []model.Firmware{
		{
			Version:       "2.6.6",
			URL:           "https://dl.dell.com/FOLDER08105057M/1/BIOS_C4FT0_WN64_2.6.6.EXE",
			FileName:      "BIOS_C4FT0_WN64_2.6.6.EXE",
			Model:         "r6515",
			Checksum:      "1ddcb3c3d0fc5925ef03a3dde768e9e245c579039dd958fc0f3a9c6368b6c5f4",
			ComponentSlug: "bios",
		},
		{
			Version:       "DL6R",
			URL:           "https://downloads.dell.com/FOLDER06303849M/1/Serial-ATA_Firmware_Y1P10_WN32_DL6R_A00.EXE",
			FileName:      "Serial-ATA_Firmware_Y1P10_WN32_DL6R_A00.EXE",
			Model:         "r6515",
			Checksum:      "4189d3cb123a781d09a4f568bb686b23c6d8e6b82038eba8222b91c380a25281",
			ComponentSlug: "drive",
		},
	}

	device1 = uuid.New()
	device2 = uuid.New()

	devices = map[string]model.Device{
		device1.String(): {
			ID:          device1,
			Vendor:      "dell",
			Model:       "r6515",
			BmcAddress:  net.ParseIP("127.0.0.1"),
			BmcUsername: "root",
			BmcPassword: "hunter2",
		},

		device2.String(): {
			ID:          device2,
			Vendor:      "dell",
			Model:       "r6515",
			BmcAddress:  net.ParseIP("127.0.0.2"),
			BmcUsername: "root",
			BmcPassword: "hunter2",
		},
	}
)

func newTaskFixture(status string) *model.Task {
	task, _ := model.NewTask("", nil)
	task.Status = string(status)
	task.FirmwaresPlanned = firmware
	task.Parameters.Device = devices[device1.String()]

	return &task
}

func Test_NewTaskStateMachine(t *testing.T) {
	task, _ := model.NewTask("", nil)
	task.Status = string(model.StateQueued)

	tests := []struct {
		name string
		task *model.Task
	}{
		{
			"new task statemachine is created",
			newTaskFixture(string(model.StateQueued)),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			// transition handler implements the taskTransitioner methods to complete tasks
			handler := &mockTaskHandler{}
			m, err := NewTaskStateMachine(ctx, tc.task, handler)
			if err != nil {
				t.Fatal(err)
			}

			assert.NotNil(t, m)
		})
	}
}

func Test_Transitions(t *testing.T) {
	tests := []struct {
		name                        string
		task                        *model.Task
		runTransition               []sw.TransitionType
		expectedState               string
		expectError                 bool
		expectNoTransitionRuleError string
	}{
		{
			"Queued to Active",
			newTaskFixture(string(model.StateQueued)),
			[]sw.TransitionType{TransitionTypePlan},
			string(model.StateActive),
			false,
			"",
		},
		{
			"Active to Success",
			newTaskFixture(string(model.StateActive)),
			[]sw.TransitionType{TransitionTypeRun},
			string(model.StateSuccess),
			false,
			"",
		},
		{
			"Queued to Success - run all transitions",
			newTaskFixture(string(model.StateQueued)),
			[]sw.TransitionType{}, // with this not defined, the statemachine defaults to the configured transitions.
			string(model.StateSuccess),
			false,
			"",
		},
		{
			"Queued to Failed",
			newTaskFixture(string(model.StateActive)),
			[]sw.TransitionType{TransitionTypeTaskFail},
			string(model.StateFailed),
			true,
			"",
		},
		{
			"Active to Failed",
			newTaskFixture(string(model.StateQueued)),
			[]sw.TransitionType{TransitionTypeTaskFail},
			string(model.StateFailed),
			true,
			"",
		},
		{
			"Success to Active fails - invalid transition",
			newTaskFixture(string(model.StateQueued)),
			[]sw.TransitionType{TransitionTypeTaskSuccess},
			string(model.StateFailed),
			true,
			"",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			// init task handler context
			tctx := &HandlerContext{TaskID: tc.task.ID.String()}
			handler := &mockTaskHandler{}

			// init new state machine
			m, err := NewTaskStateMachine(ctx, tc.task, handler)
			if err != nil {
				t.Fatal(err)
			}

			// set transition to perform based on test case
			if len(tc.runTransition) > 0 {
				m.SetTransitionOrder(tc.runTransition)
			}

			// run transition
			err = m.Run(ctx, tc.task, handler, tctx)
			if err != nil {
				if !tc.expectError {
					t.Fatal(err)
				}
			}

			if tc.expectNoTransitionRuleError != "" {
				assert.Equal(t, tc.task.Info, "no transition rule found for transition type 'plan' and state 'success': error in task transition")
			}

			assert.Equal(t, tc.expectedState, tc.task.Status)
		})
	}
}

// mockTaskHandler implements the TaskTransitioner interface
type mockTaskHandler struct{}

func (h *mockTaskHandler) Query(t sw.StateSwitch, args sw.TransitionArgs) error {
	return nil
}

func (h *mockTaskHandler) Plan(t sw.StateSwitch, args sw.TransitionArgs) error {
	return nil
}

// planFromFirmwareSet
func (h *mockTaskHandler) planFromFirmwareSet(tctx *HandlerContext, task *model.Task, device model.Device) error {
	return nil
}

func (h *mockTaskHandler) ValidatePlan(t sw.StateSwitch, args sw.TransitionArgs) (bool, error) {
	return true, nil
}

func (h *mockTaskHandler) Run(t sw.StateSwitch, args sw.TransitionArgs) error {
	return nil
}

func (h *mockTaskHandler) TaskFailed(task sw.StateSwitch, args sw.TransitionArgs) error {
	return nil
}

func (h *mockTaskHandler) TaskSuccessful(task sw.StateSwitch, args sw.TransitionArgs) error {
	return nil
}

func (h *mockTaskHandler) PersistState(t sw.StateSwitch, args sw.TransitionArgs) error {
	return nil
}
