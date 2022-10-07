package outofband

import (
	"context"
	"testing"

	"github.com/filanov/stateswitch"
	"github.com/metal-toolbox/flasher/internal/inventory"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/store"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func newTaskFixture(status string) *model.Task {
	task, _ := model.NewTask(model.InstallMethodOutofband, "", nil)
	task.Status = string(status)

	return &task
}

func newtaskHandlerContextFixture(taskID string) *taskHandlerContext {
	inv, _ := inventory.NewMockInventory()
	return &taskHandlerContext{
		taskID: taskID,
		ctx:    context.Background(),
		cache:  store.NewCacheStore(),
		inv:    inv,
		logger: logrus.New(),
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
		name           string
		task           *model.Task
		transitionType stateswitch.TransitionType
		expectedState  string
		expectError    bool
	}{
		{
			"Queued to Active",
			newTaskFixture(string(stateQueued)),
			planActions,
			string(stateActive),
			false,
		},
		{
			"Active to Success",
			newTaskFixture(string(stateActive)),
			runActions,
			string(stateSuccess),
			false,
		},

		{
			"task transition - Active to Failed",
			newTaskFixture(string(stateActive)),
			stateswitch.TransitionType("invalid"),
			string(stateFailed),
			true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			// init task handler context
			tctx := newtaskHandlerContextFixture(tc.task.ID.String())

			m, err := NewTaskStateMachine(ctx, tc.task, &taskHandler{})
			if err != nil {
				t.Fatal(err)
			}

			if err := m.sm.Run(tc.transitionType, tc.task, tctx); err != nil {
				if !tc.expectError {
					t.Fatal(err)
				}
			}

			task, _ := tctx.cache.TaskByID(ctx, tc.task.ID.String())
			assert.Equal(t, string(tc.expectedState), task.Status)
		})
	}
}

// taskHandler implements the taskTransitionHandler methods
//type mockTaskHandler struct{}
//
//func (h *mockTaskHandler) planActions(sw sw.StateSwitch, args sw.TransitionArgs) error {
//	ctx, ok := args.(*taskHandlerContext)
//	if !ok {
//		return errInvalidtaskHandlerContext
//	}
//
//	task, ok := sw.(*model.Task)
//	if !ok {
//		return errors.Wrap(ErrSaveTask, ErrTaskTypeAssertions.Error())
//	}
//
//	var plan []model.Action
//	var err error
//
//	switch task.Parameters.FirmwarePlanMethod {
//	case model.PlanFromFirmwareSet:
//		return errors.Wrap(errTaskPlanActions, "not implemented plan method: "+string(model.PlanFromFirmwareSet))
//	case model.PlanUseDefinedFirmware:
//		return errors.Wrap(errTaskPlanActions, "not implemented plan method: "+string(model.PlanUseDefinedFirmware))
//	case model.PlanFromInstalledFirmware:
//		plan, err = h.planFromInstalledFirmware(ctx, task.Device)
//	}
//
//	_ = plan
//	_ = err
//	// 1. query inventory for inventory, firmwares
//	// 2. resolve firmware to be installed
//	// 3. plan actions for task
//	// TODO: add actions to task
//	//	actionSMs, err := actionsFromTask(ctx, task.Parameters.Install)
//	//	if err != nil {
//	//		return nil, errors.Wrap(errTaskActionsInit, err.Error())
//	//	}
//	//
//	//	if len(actionSMs) == 0 {
//	//		return nil, nil, errors.Wrap(errTaskActionsInit, "no actions identified for firmware install")
//	//	}
//	return nil
//}
//
//// TODO: move plan methods into firmware package
//func (h *mockTaskHandler) planFromInstalledFirmware(ctx *taskHandlerContext, device model.Device) ([]model.Action, error) {
//	// 1. query current device inventory - from the BMC
//	// 2. query firmware set that match the device vendor, model
//	// 3. compare installed version with the versions returned in the firmware set
//	// 4. prepare actions based on the firmware versions planned
//
//}
//
//func (h *mockTaskHandler) validatePlanAction(sw sw.StateSwitch, args sw.TransitionArgs) (bool, error) {
//	taskCtx, ok := args.(*taskHandlerContext)
//	if !ok {
//		return false, errInvalidtaskHandlerContext
//	}
//
//	task, ok := sw.(*model.Task)
//	if !ok {
//		return false, errors.Wrap(ErrSaveTask, ErrTaskTypeAssertions.Error())
//	}
//
//	// validate task has firmware resolved
//	if len(task.FirmwareResolved) == 0 {
//		return false, errTaskInstallParametersUndefined
//	}
//
//	// validate task context has actions planned
//	if len(taskCtx.actionStateMachineList) == 0 {
//		return false, err
//	}
//
//	return true, nil
//}
//
//func (h *mockTaskHandler) runActions(sw sw.StateSwitch, args sw.TransitionArgs) error {
//	mctx, ok := args.(*StateMachineContext)
//	if !ok {
//		return errInvalidTransitionHandler
//	}
//
//	for _, action := range mCtx.actionsSM {
//		//action.run(mCtx.ctx, <need model.Action here>, mctx)
//	}
//
//	fmt.Println("here")
//	return nil
//}
//
//func (h *mockTaskHandler) saveState(sw sw.StateSwitch, args sw.TransitionArgs) error {
//	// check currently queued count of tasks
//	a, ok := args.(*StateMachineContext)
//	if !ok {
//		return errInvalidTransitionHandler
//	}
//
//	task, ok := sw.(*model.Task)
//	if !ok {
//		return errors.Wrap(ErrSaveTask, ErrTaskTypeAssertions.Error())
//	}
//
//	if err := a.cache.UpdateTask(a.ctx, *task); err != nil {
//		return errors.Wrap(ErrSaveTask, err.Error())
//	}
//
//	return nil
//}
//
