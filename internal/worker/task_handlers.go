package worker

import (
	"context"

	sw "github.com/filanov/stateswitch"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/outofband"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
	"github.com/pkg/errors"
)

var (
	ErrSaveTask          = errors.New("error in saveTask transition handler")
	ErrTaskTypeAssertion = errors.New("error asserting Task type")
	errTaskPlanActions   = errors.New("error in task action planning")
	errTaskPlanValidate  = errors.New("error in task plan validation")
)

// taskHandler implements the taskTransitionHandler methods
type taskHandler struct{}

func (h *taskHandler) Plan(t sw.StateSwitch, args sw.TransitionArgs) error {
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return sm.ErrInvalidtaskHandlerContext
	}

	task, ok := t.(*model.Task)
	if !ok {
		return errors.Wrap(ErrSaveTask, ErrTaskTypeAssertion.Error())
	}

	switch task.Parameters.FirmwarePlanMethod {
	case model.PlanFromFirmwareSet:
		return h.planFromFirmwareSet(tctx, task, task.Parameters.Device)
	case model.PlanRequestedFirmware:
		return errors.Wrap(errTaskPlanActions, "firmware plan method not implemented: "+string(model.PlanRequestedFirmware))
	default:
		return errors.Wrap(errTaskPlanActions, "firmware plan method invalid: "+string(task.Parameters.FirmwarePlanMethod))
	}
}

// planFromFirmwareSet
func (h *taskHandler) planFromFirmwareSet(tctx *sm.HandlerContext, task *model.Task, device model.Device) error {
	var err error

	// When theres no firmware set ID, lookup firmware by the device vendor, model.
	if task.Parameters.FirmwareSetID == "" {
		if device.Vendor == "" {
			return errors.Wrap(errTaskPlanActions, "device vendor attribute not defined")
		}

		if device.Model == "" {
			return errors.Wrap(errTaskPlanActions, "device model attribute not defined")
		}

		task.FirmwaresPlanned, err = tctx.Inv.FirmwareByDeviceVendorModel(tctx.Ctx, device.Vendor, device.Model)
		if err != nil {
			return errors.Wrap(errTaskPlanActions, err.Error())
		}
	} else {
		// TODO: implement inventory methods for firmware by set id
		panic("firmware by set id not implemented")
	}

	// plan actions based and update task action list
	tctx.ActionStateMachines, task.ActionsPlanned, err = planInstall(tctx.Ctx, task)
	if err != nil {
		return err
	}

	// 	update task in cache
	if err := tctx.Store.UpdateTask(tctx.Ctx, *task); err != nil {
		return err
	}

	return nil
}

func (h *taskHandler) Validate(t sw.StateSwitch, args sw.TransitionArgs) (bool, error) {
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return false, sm.ErrInvalidtaskHandlerContext
	}

	task, ok := t.(*model.Task)
	if !ok {
		return false, errors.Wrap(ErrSaveTask, ErrTaskTypeAssertion.Error())
	}

	if len(task.FirmwaresPlanned) == 0 {
		return false, errors.Wrap(errTaskPlanValidate, "task firmware plan empty")
	}

	// validate task has action plans listed
	if len(task.ActionsPlanned) == 0 {
		return false, errors.Wrap(errTaskPlanValidate, "task actions planned empty")
	}

	// validate task context has action statemachines for execution
	if len(tctx.ActionStateMachines) == 0 {
		return false, errors.Wrap(errTaskPlanValidate, "task action plan empty")
	}

	return true, nil
}

func (h *taskHandler) Run(t sw.StateSwitch, args sw.TransitionArgs) error {
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return sm.ErrInvalidTransitionHandler
	}

	task, ok := t.(*model.Task)
	if !ok {
		return errors.Wrap(ErrSaveTask, ErrTaskTypeAssertion.Error())
	}

	// each actionSM (state machine) corresponds to a firmware to be installed
	for _, actionSM := range tctx.ActionStateMachines {
		// fetch action attributes from task
		action := task.ActionsPlanned.ByID(actionSM.ActionID())

		// run the action state machine
		err := actionSM.Run(tctx.Ctx, action, tctx)
		if err != nil {
			// save action on failure
			//
			// The action save state handler is not invoked when an action fails
			// and so the action state saving is done here.
			//
			// the returned erorr is ignored, since theres no error expected from the SetState method
			_ = action.SetState(model.StateFailed)

			saveTaskErr := tctx.Store.UpdateTaskAction(tctx.Ctx, tctx.TaskID, *action)
			if saveTaskErr != nil {
				return errors.Wrap(saveTaskErr, err.Error())
			}

			return errors.Wrap(err, action.Firmware.ComponentSlug)
		}
		// save action on success
		//
		// The action save state handler is not invoked when an action fails
		// and so the action state saving is done here.
		//
		// the returned erorr is ignored, since theres no error expected from the SetState method
		_ = action.SetState(model.StateSuccess)

		saveTaskErr := tctx.Store.UpdateTaskAction(tctx.Ctx, tctx.TaskID, *action)
		if saveTaskErr != nil {
			return errors.Wrap(saveTaskErr, err.Error())
		}
	}

	return nil
}

func (h *taskHandler) FailedState(task sw.StateSwitch, args sw.TransitionArgs) error {
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return sm.ErrInvalidtaskHandlerContext
	}

	t, ok := task.(*model.Task)
	if !ok {
		// TODO: fix error
		return errors.Wrap(ErrSaveTask, ErrTaskTypeAssertion.Error())
	}

	// include error in task information
	if tctx.Err != nil {
		t.Info = tctx.Err.Error()
	}

	return nil
}

func (h *taskHandler) SaveState(task sw.StateSwitch, args sw.TransitionArgs) error {
	// check currently queued count of tasks
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return sm.ErrInvalidTransitionHandler
	}

	t, ok := task.(*model.Task)
	if !ok {
		return errors.Wrap(ErrSaveTask, ErrTaskTypeAssertion.Error())
	}

	if err := tctx.Store.UpdateTask(tctx.Ctx, *t); err != nil {
		return errors.Wrap(ErrSaveTask, err.Error())
	}

	return nil
}

// planInstall sets up an install plan along with actions
//
// This returns a list of actions to added to the task and a list of action state machines for those actions.
func planInstall(ctx context.Context, task *model.Task) (sm.ActionStateMachines, model.Actions, error) {
	plans := make(sm.ActionStateMachines, 0)
	actions := make(model.Actions, 0)

	// each firmware planned results in an ActionPlan and an Action
	for idx, firmware := range task.FirmwaresPlanned {
		actionID := sm.ActionID(task.ID.String(), firmware.ComponentSlug, idx)

		// TODO: The firmware is to define the preferred install method
		// based on that the action plan is setup.
		//
		// For now this is hardcoded to outofband.
		m, err := outofband.NewActionStateMachines(ctx, actionID)
		if err != nil {
			return nil, nil, err
		}

		plans = append(plans, m)

		actions = append(actions, model.Action{
			ID:     actionID,
			TaskID: task.ID.String(),
			// TODO: The firmware is to define the preferred install method
			// based on that the action plan is setup.
			//
			// For now this is hardcoded to outofband.
			InstallMethod: model.InstallMethodOutofband,
			Status:        string(model.StateQueued),
			Firmware:      task.FirmwaresPlanned[idx],
		})

	}

	return plans, actions, nil
}
