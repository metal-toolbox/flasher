// nolint
package statemachine

// this package gets its own fixtures to prevent import cycles.

import (
	"net"
	"testing"
	"time"

	sw "github.com/filanov/stateswitch"
	"github.com/google/uuid"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	rctypes "github.com/metal-toolbox/rivets/condition"
)

var (
	firmwares = []*model.Firmware{
		{
			Version:   "2.6.6",
			URL:       "https://dl.dell.com/FOLDER08105057M/1/BIOS_C4FT0_WN64_2.6.6.EXE",
			FileName:  "BIOS_C4FT0_WN64_2.6.6.EXE",
			Models:    []string{"r6515"},
			Checksum:  "1ddcb3c3d0fc5925ef03a3dde768e9e245c579039dd958fc0f3a9c6368b6c5f4",
			Component: "bios",
		},
		{
			Version:   "DL6R",
			URL:       "https://downloads.dell.com/FOLDER06303849M/1/Serial-ATA_Firmware_Y1P10_WN32_DL6R_A00.EXE",
			FileName:  "Serial-ATA_Firmware_Y1P10_WN32_DL6R_A00.EXE",
			Models:    []string{"r6515"},
			Checksum:  "4189d3cb123a781d09a4f568bb686b23c6d8e6b82038eba8222b91c380a25281",
			Component: "drive",
		},
	}

	asset1 = uuid.New()

	asset2 = uuid.New()

	assets = map[string]model.Asset{
		asset1.String(): {
			ID:          asset1,
			Vendor:      "dell",
			Model:       "r6515",
			BmcAddress:  net.ParseIP("127.0.0.1"),
			BmcUsername: "root",
			BmcPassword: "hunter2",
		},

		asset2.String(): {
			ID:          asset2,
			Vendor:      "dell",
			Model:       "r6515",
			BmcAddress:  net.ParseIP("127.0.0.2"),
			BmcUsername: "root",
			BmcPassword: "hunter2",
		},
	}
)

func newTaskFixture(t *testing.T, state string, fault *rctypes.Fault) *model.Task {
	t.Helper()

	task := &model.Task{Fault: fault}
	if err := task.SetState(sw.State(state)); err != nil {
		t.Fatal(err)
	}

	task.Parameters.AssetID = asset1

	return task
}

func Test_NewTaskStateMachine(t *testing.T) {
	task := &model.Task{}
	task.Status = string(model.StatePending)

	tests := []struct {
		name string
		task *model.Task
	}{
		{
			"new task statemachine is created",
			newTaskFixture(t, string(model.StatePending), nil),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// transition handler implements the taskTransitioner methods to complete tasks
			handler := &MockTaskHandler{}
			m, err := NewTaskStateMachine(handler)
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
			"Pending to Active",
			newTaskFixture(t, string(model.StatePending), nil),
			[]sw.TransitionType{TransitionTypeActive},
			string(model.StateActive),
			false,
			"",
		},
		{
			"Active to Success",
			newTaskFixture(t, string(model.StateActive), nil),
			[]sw.TransitionType{TransitionTypeRun},
			string(model.StateSucceeded),
			false,
			"",
		},
		{
			"Queued to Success - run all transitions",
			newTaskFixture(t, string(model.StatePending), nil),
			[]sw.TransitionType{}, // with this not defined, the statemachine defaults to the configured transitions.
			string(model.StateSucceeded),
			false,
			"",
		},
		{
			"Queued to Failed",
			newTaskFixture(t, string(model.StateActive), nil),
			[]sw.TransitionType{TransitionTypeTaskFail},
			string(model.StateFailed),
			true,
			"",
		},
		{
			"Active to Failed",
			newTaskFixture(t, string(model.StatePending), nil),
			[]sw.TransitionType{TransitionTypeTaskFail},
			string(model.StateFailed),
			true,
			"",
		},
		{
			"Success to Active fails - invalid transition",
			newTaskFixture(t, string(model.StatePending), nil),
			[]sw.TransitionType{TransitionTypeTaskSuccess},
			string(model.StateFailed),
			true,
			"",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// init task handler context
			tctx := &HandlerContext{Task: tc.task}
			handler := &MockTaskHandler{}

			// init new state machine
			m, err := NewTaskStateMachine(handler)
			if err != nil {
				t.Fatal(err)
			}

			// set transition to perform based on test case
			if len(tc.runTransition) > 0 {
				m.SetTransitionOrder(tc.runTransition)
			}

			// run transition
			err = m.Run(tc.task, tctx)
			if err != nil {
				if !tc.expectError {
					t.Fatal(err)
				}
			}

			if tc.expectNoTransitionRuleError != "" {
				assert.Equal(t, tc.task.Status, "no transition rule found for transition type 'plan' and state 'success': error in task transition")
			}

			assert.Equal(t, tc.expectedState, string(tc.task.State()))

		})
	}
}

func Test_ConditionalFaultWithTransitions(t *testing.T) {
	tests := []struct {
		name          string
		task          *model.Task
		runTransition []sw.TransitionType
		expectedState string
		expectError   bool
		expectPanic   bool
		expectDelay   time.Duration
	}{
		{
			"condition induced error",
			newTaskFixture(t, string(model.StateActive), &rctypes.Fault{FailAt: "plan"}),
			[]sw.TransitionType{TransitionTypePlan},
			string(model.StateFailed),
			true,
			false,
			0,
		},
		{
			"condition induced panic",
			newTaskFixture(t, string(model.StateActive), &rctypes.Fault{Panic: true}),
			[]sw.TransitionType{TransitionTypePlan},
			string(model.StateFailed),
			true,
			true,
			0,
		},
		{
			"condition induced delay",
			newTaskFixture(t, string(model.StateActive), &rctypes.Fault{DelayDuration: "33ms"}),
			[]sw.TransitionType{TransitionTypePlan},
			string(model.StateActive),
			false,
			false,
			33 * time.Millisecond,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// init task handler context
			tctx := &HandlerContext{Task: tc.task, Logger: logrus.NewEntry(logrus.New())}
			handler := &MockTaskHandler{}

			// init new state machine
			m, err := NewTaskStateMachine(handler)
			if err != nil {
				t.Fatal(err)
			}

			// set transition to perform based on test case
			if len(tc.runTransition) > 0 {
				m.SetTransitionOrder(tc.runTransition)
			}

			if tc.expectPanic {
				assert.Panics(t, func() {
					_ = m.Run(tc.task, tctx)
				})

			} else {
				start := time.Now()

				// run transition
				err = m.Run(tc.task, tctx)
				if err != nil {
					if !tc.expectError {
						t.Fatal(err)
					}
				}

				assert.GreaterOrEqual(t, time.Since(start), tc.expectDelay)

				assert.Equal(t, tc.expectedState, string(tc.task.State()))
			}
		})
	}
}
