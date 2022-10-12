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

func (h *taskHandler) Plan(task sw.StateSwitch, args sw.TransitionArgs) error {
	ctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return sm.ErrInvalidtaskHandlerContext
	}

	t, ok := task.(*model.Task)
	if !ok {
		return errors.Wrap(ErrSaveTask, ErrTaskTypeAssertion.Error())
	}

	switch t.Parameters.FirmwarePlanMethod {
	case model.PlanFromFirmwareSet:
		return errors.Wrap(errTaskPlanActions, "firmware plan method not implemented: "+string(model.PlanFromFirmwareSet))
	case model.PlanUseDefinedFirmware:
		return errors.Wrap(errTaskPlanActions, "firmware plan method not implemented: "+string(model.PlanUseDefinedFirmware))
	case model.PlanFromInstalledFirmware:
		return h.planFromInstalledFirmware(ctx, t.Parameters.Device)
	default:
		return errors.Wrap(errTaskPlanActions, "firmware plan method invalid: "+string(t.Parameters.FirmwarePlanMethod))
	}
}

// planFromInstalledFirmawre
func (h *taskHandler) planFromInstalledFirmware(tctx *sm.HandlerContext, device model.Device) error {
	ctx := tctx.Ctx

	// retrieve task from cache
	task, err := tctx.Store.TaskByID(ctx, tctx.TaskID)
	if err != nil {
		return err
	}

	// plan firmware for install - based on firmware currently installed on the device
	// note: this requires the inventory have device inventory made available.
	found, err := tctx.FwPlanner.FromInstalled(ctx, task.Parameters.Device.ID.String())
	if err != nil {
		return err
	}

	task.FirmwaresPlanned = found

	// plan actions based and update task action list
	tctx.ActionPlan, err = planInstallActions(ctx, &task)
	if err != nil {
		return err
	}

	// 	update task in cache
	if err := tctx.Store.UpdateTask(ctx, task); err != nil {
		return err
	}

	return nil
}

func (h *taskHandler) Validate(task sw.StateSwitch, args sw.TransitionArgs) (bool, error) {
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return false, sm.ErrInvalidtaskHandlerContext
	}

	t, ok := task.(*model.Task)
	if !ok {
		return false, errors.Wrap(ErrSaveTask, ErrTaskTypeAssertion.Error())
	}

	if len(t.FirmwaresPlanned) == 0 {
		return false, errors.Wrap(errTaskPlanValidate, "task firmware plan empty")
	}

	// validate task context has actions planned
	if len(tctx.ActionPlan) == 0 {
		return false, errors.Wrap(errTaskPlanValidate, "task action plan empty")
	}

	return true, nil
}

func (h *taskHandler) Run(task sw.StateSwitch, args sw.TransitionArgs) error {
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return sm.ErrInvalidTransitionHandler
	}

	t, ok := task.(*model.Task)
	if !ok {
		return errors.Wrap(ErrSaveTask, ErrTaskTypeAssertion.Error())
	}

	for _, plan := range tctx.ActionPlan {
		err := plan.Run(tctx.Ctx, t.ActionsPlanned.ByID(plan.ActionID()), tctx)
		if err != nil {
			return err
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

// planInstallActions plans the firmware install actions
//
// The given task is updated with Actions based on the FirmwaresPlanned attribute
// and an actionPlan is returned which is to be executed.
func planInstallActions(ctx context.Context, task *model.Task) (sm.ActionPlan, error) {
	plans := make(sm.ActionPlan, 0)

	// each firmware install parameter results in an action
	for idx, firmware := range task.FirmwaresPlanned {
		actionID := sm.ActionID(task.ID.String(), firmware.ComponentSlug, idx)

		// TODO: The firmware is to define the preferred install method
		// based on that the action plan is setup.
		//
		// For now this is hardcoded to outofband.
		m, err := outofband.NewActionPlan(ctx, actionID)
		if err != nil {
			return nil, err
		}

		plans = append(plans, m)

		action := model.Action{
			ID:     actionID,
			TaskID: task.ID.String(),
			// TODO: The firmware is to define the preferred install method
			// based on that the action plan is setup.
			//
			// For now this is hardcoded to outofband.
			InstallMethod: model.InstallMethodOutofband,
			Status:        string(sm.StateQueued),
			Firmware:      task.FirmwaresPlanned[idx],
		}

		task.ActionsPlanned = append(task.ActionsPlanned, action)
	}

	return plans, nil
}
