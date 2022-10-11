package outofband

import (
	sw "github.com/filanov/stateswitch"
	"github.com/metal-toolbox/flasher/internal/model"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
	"github.com/pkg/errors"
)

var (
	ErrSaveTask           = errors.New("error in saveTask transition handler")
	ErrTaskTypeAssertions = errors.New("error asserting Task type")
	errTaskPlanActions    = errors.New("error in task action planning")
	errTaskPlanValidate   = errors.New("error in task plan validation")
)

// taskHandler implements the taskTransitionHandler methods
type taskHandler struct{}

func (h *taskHandler) Plan(sw sw.StateSwitch, args sw.TransitionArgs) error {
	ctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return sm.ErrInvalidtaskHandlerContext
	}

	task, ok := sw.(*model.Task)
	if !ok {
		return errors.Wrap(ErrSaveTask, ErrTaskTypeAssertions.Error())
	}

	switch task.Parameters.FirmwarePlanMethod {
	case model.PlanFromFirmwareSet:
		return errors.Wrap(errTaskPlanActions, "firmware plan method not implemented: "+string(model.PlanFromFirmwareSet))
	case model.PlanUseDefinedFirmware:
		return errors.Wrap(errTaskPlanActions, "firmware plan method not implemented: "+string(model.PlanUseDefinedFirmware))
	case model.PlanFromInstalledFirmware:
		return h.planFromInstalledFirmware(ctx, task.Device)
	default:
		return errors.Wrap(errTaskPlanActions, "firmware plan method invalid: "+string(task.Parameters.FirmwarePlanMethod))
	}
}

// TODO: move plan methods into firmware package
func (h *taskHandler) planFromInstalledFirmware(tctx *sm.HandlerContext, device model.Device) error {
	// 1. query current device inventory - from the BMC
	if err := tctx.Bmc.Open(tctx.Ctx); err != nil {
		return err
	}

	deviceInventory, err := tctx.Bmc.Inventory(tctx.Ctx)
	if err != nil {
		return err
	}

	ctx := tctx.Ctx

	// retrieve task from cache
	task, err := tctx.Cache.TaskByID(ctx, tctx.TaskID)
	if err != nil {
		return err
	}

	// plan firmware for install
	//
	// TODO(joel): extend to support other device, inventory attributes
	found, err := tctx.FwPlanner.FromInstalled(deviceInventory)
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
	if err := tctx.Cache.UpdateTask(ctx, task); err != nil {
		return err
	}

	return nil
}

func (h *taskHandler) Validate(sw sw.StateSwitch, args sw.TransitionArgs) (bool, error) {
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return false, sm.ErrInvalidtaskHandlerContext
	}

	task, ok := sw.(*model.Task)
	if !ok {
		return false, errors.Wrap(ErrSaveTask, ErrTaskTypeAssertions.Error())
	}

	if len(task.FirmwaresPlanned) == 0 {
		return false, errors.Wrap(errTaskPlanValidate, "task firmware plan empty")
	}

	// validate task context has actions planned
	if len(tctx.ActionPlan) == 0 {
		return false, errors.Wrap(errTaskPlanValidate, "task action plan empty")
	}

	return true, nil
}

func (h *taskHandler) Run(sw sw.StateSwitch, args sw.TransitionArgs) error {
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return sm.ErrInvalidTransitionHandler
	}

	task, ok := sw.(*model.Task)
	if !ok {
		return errors.Wrap(ErrSaveTask, ErrTaskTypeAssertions.Error())
	}

	for _, plan := range tctx.ActionPlan {
		err := plan.Run(tctx.Ctx, task.ActionsPlanned.ByID(plan.ActionID()), tctx)
		if err != nil {
			return err
		}
	}

	return nil
}

func (h *taskHandler) FailedState(sw sw.StateSwitch, args sw.TransitionArgs) error {
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return sm.ErrInvalidtaskHandlerContext
	}

	task, ok := sw.(*model.Task)
	if !ok {
		// TODO: fix error
		return errors.Wrap(ErrSaveTask, ErrTaskTypeAssertions.Error())
	}

	// include error in task information
	if tctx.Err != nil {
		task.Info = tctx.Err.Error()
	}

	return nil
}

func (h *taskHandler) SaveState(sw sw.StateSwitch, args sw.TransitionArgs) error {
	// check currently queued count of tasks
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return sm.ErrInvalidTransitionHandler
	}

	task, ok := sw.(*model.Task)
	if !ok {
		return errors.Wrap(ErrSaveTask, ErrTaskTypeAssertions.Error())
	}

	if err := tctx.Cache.UpdateTask(tctx.Ctx, *task); err != nil {
		return errors.Wrap(ErrSaveTask, err.Error())
	}

	return nil
}
