package runner

import (
	"context"
	"testing"

	"github.com/metal-toolbox/flasher/internal/model"
	rctypes "github.com/metal-toolbox/rivets/v2/condition"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
)

func TestRunTask(t *testing.T) {
	tests := []struct {
		name          string
		task          *model.Task
		mockSetup     func(*MockTaskHandler)
		expectedState rctypes.State
		expectedError error
	}{
		{
			name: "Successful execution - new task",
			task: &model.Task{
				State: model.StatePending,
				Data:  &model.TaskData{},
			},
			mockSetup: func(m *MockTaskHandler) {
				m.On("Initialize", mock.Anything).Return(nil)
				m.On("Query", mock.Anything).Return(nil)
				m.On("PlanActions", mock.Anything).Return(nil)
				m.On("Publish", mock.Anything).Return(nil)
				m.On("OnSuccess", mock.Anything, mock.Anything).Once()
			},
			expectedState: model.StateSucceeded,
			expectedError: nil,
		},
		{
			name: "Failure during Initialize",
			task: &model.Task{
				State: model.StatePending,
				Data:  &model.TaskData{},
			},
			mockSetup: func(m *MockTaskHandler) {
				m.On("Initialize", mock.Anything).Return(errors.New("Initialize failed"))
				m.On("Publish", mock.Anything).Return(nil)
				m.On("OnFailure", mock.Anything, mock.Anything).Once()
			},
			expectedState: model.StateFailed,
			expectedError: errors.New("Initialize failed"),
		},
		{
			name: "Resume active task",
			task: &model.Task{
				State: model.StateActive,
				Data:  &model.TaskData{},
			},
			mockSetup: func(m *MockTaskHandler) {
				m.On("Initialize", mock.Anything).Return(nil)
				m.On("Query", mock.Anything).Return(nil)
				m.On("PlanActions", mock.Anything).Return(nil)
				m.On("Publish", mock.Anything).Return(nil)
				m.On("OnSuccess", mock.Anything, mock.Anything).Once()
			},
			expectedState: model.StateSucceeded,
			expectedError: nil,
		},
		{
			name: "Task already in final state",
			task: &model.Task{
				State: model.StateSucceeded,
				Data:  &model.TaskData{},
			},
			mockSetup: func(m *MockTaskHandler) {
				m.On("Publish", mock.Anything).Return(nil)
				m.On("OnSuccess", mock.Anything, mock.Anything).Once()
			},
			expectedState: model.StateSucceeded,
			expectedError: nil,
		},
		{
			name: "Failure during runActions",
			task: &model.Task{
				State: model.StatePending,
				Data: &model.TaskData{
					ActionsPlanned: []*model.Action{
						{
							ID:    "action1",
							State: model.StatePending,
							Steps: []*model.Step{
								{
									Name:    "step1",
									State:   model.StatePending,
									Handler: func(context.Context) error { return errors.New("Step failed") },
								},
							},
						},
					},
				},
			},
			mockSetup: func(m *MockTaskHandler) {
				m.On("Initialize", mock.Anything).Return(nil)
				m.On("Query", mock.Anything).Return(nil)
				m.On("PlanActions", mock.Anything).Return(nil)
				m.On("Publish", mock.Anything).Return(nil)
				m.On("OnFailure", mock.Anything, mock.Anything).Once()
			},
			expectedState: model.StateFailed,
			expectedError: errors.New("error while running step=step1 to install firmware on component=: Step failed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockHandler := new(MockTaskHandler)
			tt.mockSetup(mockHandler)

			r := New(logrus.NewEntry(logrus.New()))
			err := r.RunTask(context.Background(), tt.task, mockHandler)

			assert.Equal(t, tt.expectedState, tt.task.State)
			if tt.expectedError != nil {
				assert.EqualError(t, err, tt.expectedError.Error())
			} else {
				assert.NoError(t, err)
			}

			mockHandler.AssertExpectations(t)
		})
	}
}

func TestRunActions(t *testing.T) {
	tests := []struct {
		name                string
		task                *model.Task
		mockSetup           func(*MockTaskHandler)
		expectedError       error
		expectedActionState rctypes.State
	}{
		{
			name: "Successful execution of all actions",
			task: &model.Task{
				Data: &model.TaskData{
					ActionsPlanned: []*model.Action{
						{
							ID: "action1",
							Firmware: rctypes.Firmware{
								Component: "component1",
								Version:   "1.0",
							},
							State: model.StatePending,
							Steps: []*model.Step{
								{
									Name:    "step1",
									State:   model.StatePending,
									Handler: func(context.Context) error { return nil },
								},
								{
									Name:    "step2",
									State:   model.StatePending,
									Handler: func(context.Context) error { return nil },
								},
							},
						},
					},
				},
			},
			mockSetup: func(m *MockTaskHandler) {
				m.On("Publish", mock.Anything).Return(nil)
			},
			expectedError:       nil,
			expectedActionState: rctypes.Succeeded,
		},
		{
			name: "Action fails on second step",
			task: &model.Task{
				Data: &model.TaskData{
					ActionsPlanned: []*model.Action{
						{
							ID: "action1",
							Firmware: rctypes.Firmware{
								Component: "component1",
								Version:   "1.0",
							},
							State: model.StatePending,
							Steps: []*model.Step{
								{
									Name:    "step1",
									State:   model.StatePending,
									Handler: func(context.Context) error { return nil },
								},
								{
									Name:    "step2",
									State:   model.StatePending,
									Handler: func(context.Context) error { return errors.New("step failed") },
								},
							},
						},
					},
				},
			},
			mockSetup: func(m *MockTaskHandler) {
				m.On("Publish", mock.Anything).Return(nil)
			},
			expectedError:       errors.New("error while running step=step2 to install firmware on component=component1: step failed"),
			expectedActionState: rctypes.Failed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockHandler := new(MockTaskHandler)
			tt.mockSetup(mockHandler)

			r := New(logrus.NewEntry(logrus.New()))
			err := r.runActions(context.Background(), tt.task, mockHandler)

			if tt.expectedError != nil {
				assert.EqualError(t, err, tt.expectedError.Error())
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectedActionState, tt.task.Data.ActionsPlanned[0].State)
		})
	}
}

func TestResumeAction(t *testing.T) {
	tests := []struct {
		name           string
		action         *model.Action
		mockSetup      func(*MockTaskHandler)
		expectedResume bool
		expectedError  error
	}{
		{
			name: "Run pending action",
			action: &model.Action{
				State: model.StatePending,
			},
			mockSetup:      func(m *MockTaskHandler) {},
			expectedResume: true,
			expectedError:  nil,
		},
		{
			name: "Skip succeeded action",
			action: &model.Action{
				State: model.StateSucceeded,
			},
			mockSetup:      func(m *MockTaskHandler) {},
			expectedResume: false,
			expectedError:  nil,
		},
		{
			name: "Resume active action",
			action: &model.Action{
				State:    model.StateActive,
				Attempts: 1,
			},
			mockSetup:      func(m *MockTaskHandler) {},
			expectedResume: true,
			expectedError:  nil,
		},
		{
			name: "Fail action with max attempts",
			action: &model.Action{
				State:    model.StateActive,
				Attempts: model.ActionMaxAttempts + 1,
			},
			mockSetup: func(m *MockTaskHandler) {
				m.On("Publish", mock.Anything).Return(nil)
			},
			expectedResume: false,
			expectedError:  errors.New("reached maximum attempts on action: 4: error in resuming action"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockHandler := new(MockTaskHandler)
			tt.mockSetup(mockHandler)

			r := New(logrus.NewEntry(logrus.New()))
			resume, err := r.resumeAction(context.Background(), tt.action, mockHandler)

			assert.Equal(t, tt.expectedResume, resume)
			if tt.expectedError != nil {
				assert.EqualError(t, err, tt.expectedError.Error())
			} else {
				assert.NoError(t, err)
			}

		})
	}
}

func TestRunActionSteps(t *testing.T) {
	tests := []struct {
		name            string
		task            *model.Task
		action          *model.Action
		mockSetup       func(*MockTaskHandler)
		expectedProceed bool
		expectedError   error
	}{
		{
			name: "All steps succeed",
			task: &model.Task{Data: &model.TaskData{}},
			action: &model.Action{
				Firmware: rctypes.Firmware{Component: "test", Version: "1.0"},
				Steps: []*model.Step{
					{
						Name:    "step1",
						State:   model.StatePending,
						Handler: func(context.Context) error { return nil },
					},
					{
						Name:    "step2",
						State:   model.StatePending,
						Handler: func(context.Context) error { return nil },
					},
				},
			},
			mockSetup: func(m *MockTaskHandler) {
				m.On("Publish", mock.Anything).Return(nil)
			},
			expectedProceed: true,
			expectedError:   nil,
		},
		{
			name: "Step fails",
			task: &model.Task{Data: &model.TaskData{}},
			action: &model.Action{
				Firmware: rctypes.Firmware{Component: "test", Version: "1.0"},
				Steps: []*model.Step{
					{
						Name:    "step1",
						State:   model.StatePending,
						Handler: func(context.Context) error { return errors.New("step failed") },
					},
				},
			},
			mockSetup: func(m *MockTaskHandler) {
				m.On("Publish", mock.Anything).Return(nil)
			},
			expectedProceed: false,
			expectedError:   errors.New("error while running step=step1 to install firmware on component=test: step failed"),
		},
		{
			name: "Installed firmware equals expected",
			task: &model.Task{Data: &model.TaskData{}},
			action: &model.Action{
				Firmware: rctypes.Firmware{Component: "test", Version: "1.0"},
				Steps: []*model.Step{
					{
						Name:    "step1",
						State:   model.StatePending,
						Handler: func(context.Context) error { return model.ErrInstalledFirmwareEqual },
					},
				},
			},
			mockSetup: func(m *MockTaskHandler) {
				m.On("Publish", mock.Anything).Return(nil)
			},
			expectedProceed: false,
			expectedError:   nil,
		},
		{
			name: "Host power cycle required",
			task: &model.Task{Data: &model.TaskData{}},
			action: &model.Action{
				Firmware: rctypes.Firmware{Component: "test", Version: "1.0"},
				Steps: []*model.Step{
					{
						Name:    "step1",
						State:   model.StatePending,
						Handler: func(context.Context) error { return model.ErrHostPowerCycleRequired },
					},
				},
			},
			mockSetup: func(m *MockTaskHandler) {
				m.On("Publish", mock.Anything).Return(nil)
			},
			expectedProceed: false,
			expectedError:   model.ErrHostPowerCycleRequired,
		},
		{
			name: "Nil step handler",
			task: &model.Task{Data: &model.TaskData{}},
			action: &model.Action{
				Firmware: rctypes.Firmware{Component: "test", Version: "1.0"},
				Steps: []*model.Step{
					{
						Name:    "borky step2",
						State:   model.StatePending,
						Handler: nil,
					},
				},
			},
			mockSetup: func(m *MockTaskHandler) {
				m.On("Publish", mock.Anything).Return(nil)
			},
			expectedProceed: false,
			expectedError:   errors.New("error while running step=borky step2 to install firmware on component=test, handler was nil"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockHandler := new(MockTaskHandler)
			tt.mockSetup(mockHandler)

			r := New(logrus.NewEntry(logrus.New()))
			proceed, err := r.runActionSteps(context.Background(), tt.task, tt.action, mockHandler, r.logger)

			assert.Equal(t, tt.expectedProceed, proceed)
			if tt.expectedError != nil {
				assert.EqualError(t, err, tt.expectedError.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestResumeStep(t *testing.T) {
	tests := []struct {
		name           string
		step           *model.Step
		expectedResume bool
		expectedError  error
	}{
		{
			name: "Resume pending step",
			step: &model.Step{
				Name:  "step1",
				State: model.StatePending,
			},
			expectedResume: true,
			expectedError:  nil,
		},
		{
			name: "Skip succeeded step",
			step: &model.Step{
				Name:  "step1",
				State: model.StateSucceeded,
			},
			expectedResume: false,
			expectedError:  nil,
		},
		{
			name: "Resume active step",
			step: &model.Step{
				Name:     "step1",
				State:    model.StateActive,
				Attempts: 1,
			},
			expectedResume: true,
			expectedError:  nil,
		},
		{
			name: "Fail step with max attempts",
			step: &model.Step{
				Name:     "step1",
				State:    model.StateActive,
				Attempts: model.StepMaxAttempts + 1,
			},
			expectedResume: false,
			expectedError:  errors.New("reached maximum attempts on step: 3: error in resuming step"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := New(logrus.NewEntry(logrus.New()))
			resume, err := r.resumeStep(tt.step, r.logger)

			assert.Equal(t, tt.expectedResume, resume)
			if tt.expectedError != nil {
				assert.EqualError(t, err, tt.expectedError.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
