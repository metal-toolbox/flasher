package runner

import (
	"context"
	"testing"

	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestRunTask(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHandler := NewMockHandler(ctrl)

	tests := []struct {
		name          string
		setupMock     func()
		expectedState string
		expectedError error
	}{
		{
			name: "Successful execution",
			setupMock: func() {
				mockHandler.EXPECT().Initialize(gomock.Any()).Return(nil)
				mockHandler.EXPECT().Query(gomock.Any()).Return(nil)
				mockHandler.EXPECT().PlanActions(gomock.Any()).Return(nil)
				mockHandler.EXPECT().RunActions(gomock.Any()).Return(nil)
				mockHandler.EXPECT().OnSuccess(gomock.Any(), gomock.Any())
				mockHandler.EXPECT().Publish().AnyTimes()
			},
			expectedState: string(model.StateSucceeded),
			expectedError: nil,
		},
		{
			name: "Failure during Initialize",
			setupMock: func() {
				mockHandler.EXPECT().Initialize(gomock.Any()).Return(errors.New("Initialize failed"))
				mockHandler.EXPECT().OnFailure(gomock.Any(), gomock.Any())
			},
			expectedState: string(model.StateFailed),
			expectedError: errors.New("Initialize failed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			r := New(logrus.NewEntry(logrus.New()))
			task := &model.Task{}
			err := r.RunTask(context.Background(), task, mockHandler)

			if string(task.State()) != tt.expectedState {
				t.Errorf("Expected task state %s, but got %s", tt.expectedState, task.State())
			}

			if tt.expectedError != nil {
				assert.EqualError(t, tt.expectedError, err.Error())
			}

			if tt.expectedState == string(model.StateSucceeded) {
				assert.Nil(t, err)
			}
		})
	}
}
