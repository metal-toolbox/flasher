package outofband

import (
	"context"
	"testing"

	"github.com/filanov/stateswitch"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/store"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"github.com/google/uuid"
)

func newTaskFixture(status string) *model.Task {
	return &model.Task{
		ID:     uuid.New(),
		Status: string(status),
		Parameters: model.TaskParameters{
			InstallParameters: []model.InstallParameter{
				{
					ComponentSlug: "bmc",
					Vendor:        "dell",
					Model:         "r6515",
				},
				{
					ComponentSlug: "bios",
					Vendor:        "dell",
					Model:         "r6515",
				},
			},
			Configuration: []model.Firmware{
				{
					ComponentSlug: "bmc",
					Version:       "0.1",
					Vendor:        "dell",
					Model:         "r6515",
				},
				{
					ComponentSlug: "bios",
					Version:       "0.1",
					Vendor:        "dell",
					Model:         "r6515",
				},
				{
					ComponentSlug: "nic",
					Version:       "0.1",
					Vendor:        "dell",
					Model:         "x710",
				},
			},
		},
	}
}

func Test_StateMachine_Transitions(t *testing.T) {
	tests := []struct {
		name           string
		task           *model.Task
		transitionType stateswitch.TransitionType
		expectedState  string
	}{
		{
			"task transition - Queued to Active",
			newTaskFixture(string(stateQueued)),
			transitionTypeTaskResolveFwCfg,
			string(stateActive),
		},
		//	{
		//		"task transition - Active to Success",
		//		newTaskFixture(string(stateActive)),
		//		transitionTypeTaskRunSubTasks,
		//		string(stateSuccess),
		//	},
	}

	args := &Args{
		logger: logrus.New(),
		cache:  store.NewCacheStore(),
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			// transition handler implements methods to complete tasks
			th := &taskHandler{}
			sm := NewTaskStateMachine(ctx, th)

			err := sm.Run(tc.transitionType, tc.task, args)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, string(tc.expectedState), tc.task.Status)

			task, _ := args.cache.TaskByID(ctx, tc.task.ID.String())
			assert.Equal(t, string(tc.expectedState), task.Status)

		})
	}
}

func Test_StateMachine_RunTasks(t *testing.T) {
	tests := []struct {
		name           string
		task           *model.Task
		transitionType stateswitch.TransitionType
		expectedState  string
	}{
		{
			"task transition - Queued to Active",
			newTaskFixture(string(stateQueued)),
			transitionTypeTaskResolveFwCfg,
			string(stateActive),
		},
		//	{
		//		"task transition - Active to Success",
		//		newTaskFixture(string(stateActive)),
		//		transitionTypeTaskRunSubTasks,
		//		string(stateSuccess),
		//	},
	}

	args := &Args{
		logger: logrus.New(),
		cache:  store.NewCacheStore(),
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			tc.task.SM = NewTaskStateMachine(ctx, nil)

			err := RunTasks(ctx, tc.task, args)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, string(tc.expectedState), tc.task.Status)

			task, _ := args.cache.TaskByID(ctx, tc.task.ID.String())
			assert.Equal(t, string(tc.expectedState), task.Status)

		})
	}
}
