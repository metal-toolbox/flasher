package runner

import (
	"context"
	"testing"

	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
)

func TestRunTask(t *testing.T) {
	tests := []struct {
		name          string
		mockSetup     func(*MockTaskHandler)
		expectedState string
		expectedError error
	}{
		{
			name: "Successful execution",
			mockSetup: func(m *MockTaskHandler) {
				m.On("Initialize", mock.Anything).Return(nil)
				m.On("Query", mock.Anything).Return(nil)
				m.On("PlanActions", mock.Anything).Return(nil)
				m.On("OnSuccess", mock.Anything, mock.Anything).Once()
				m.On("Publish", mock.Anything).Maybe()
			},
			expectedState: string(model.StateSucceeded),
			expectedError: nil,
		},
		{
			name: "Failure during Initialize",
			mockSetup: func(m *MockTaskHandler) {
				m.On("Initialize", mock.Anything).Return(errors.New("Initialize failed"))
				m.On("OnFailure", mock.Anything, mock.Anything).Once()
				m.On("Publish", mock.Anything, mock.Anything).Twice()
			},
			expectedState: string(model.StateFailed),
			expectedError: errors.New("Initialize failed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockHandler := new(MockTaskHandler)
			tt.mockSetup(mockHandler) // Set up the mock expectations

			r := New(logrus.NewEntry(logrus.New()))
			task := &model.Task{
				Data: &model.TaskData{},
			}

			err := r.RunTask(context.Background(), task, mockHandler)

			// Assert task state and error expectations
			assert.Equal(t, tt.expectedState, string(task.State))
			if tt.expectedError != nil {
				assert.EqualError(t, err, tt.expectedError.Error())
			} else {
				assert.NoError(t, err)
			}

			mockHandler.AssertExpectations(t)
		})
	}
}
