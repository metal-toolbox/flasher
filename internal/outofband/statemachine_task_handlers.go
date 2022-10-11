package outofband

import (
	"fmt"

	sw "github.com/filanov/stateswitch"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/pkg/errors"
)

var (
	ErrSaveTask           = errors.New("error in saveTask transition handler")
	ErrTaskTypeAssertions = errors.New("error asserting Task type")
	errTaskPlanActions    = errors.New("error in task action planning")
	errTaskPlanValidate   = errors.New("error in task plan validation")
)

type taskTransitioner interface {
	planActions(sw sw.StateSwitch, args sw.TransitionArgs) error
	runActions(sw sw.StateSwitch, args sw.TransitionArgs) error
	saveState(sw sw.StateSwitch, args sw.TransitionArgs) error
	failed(sw sw.StateSwitch, args sw.TransitionArgs) error

	validatePlanAction(sw sw.StateSwitch, args sw.TransitionArgs) (bool, error)
}

// taskHandler implements the taskTransitionHandler methods
type taskHandler struct{}

func (h *taskHandler) planActions(sw sw.StateSwitch, args sw.TransitionArgs) error {
	ctx, ok := args.(*taskHandlerContext)
	if !ok {
		return errInvalidtaskHandlerContext
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
func (h *taskHandler) planFromInstalledFirmware(tctx *taskHandlerContext, device model.Device) error {
	// 1. query current device inventory - from the BMC
	if err := tctx.bmc.Open(tctx.ctx); err != nil {
		return err
	}

	deviceInventory, err := tctx.bmc.Inventory(tctx.ctx)
	if err != nil {
		return err
	}

	ctx := tctx.ctx

	// retrieve task from cache
	task, err := tctx.cache.TaskByID(ctx, tctx.taskID)
	if err != nil {
		return err
	}

	// plan firmware for install
	//
	// TODO(joel): extend to support other device, inventory attributes
	found, err := tctx.fwPlanner.FromInstalled(deviceInventory)
	if err != nil {
		return err
	}

	task.FirmwaresPlanned = found

	// plan actions based and update task action list
	tctx.actionPlan, err = planInstallActions(ctx, &task)
	if err != nil {
		return err
	}

	// 	update task in cache
	if err := tctx.cache.UpdateTask(ctx, task); err != nil {
		return err
	}

	return nil
}

func (h *taskHandler) validatePlanAction(sw sw.StateSwitch, args sw.TransitionArgs) (bool, error) {
	tctx, ok := args.(*taskHandlerContext)
	if !ok {
		return false, errInvalidtaskHandlerContext
	}

	task, ok := sw.(*model.Task)
	if !ok {
		return false, errors.Wrap(ErrSaveTask, ErrTaskTypeAssertions.Error())
	}

	if len(task.FirmwaresPlanned) == 0 {
		return false, errors.Wrap(errTaskPlanValidate, "task firmware plan empty")
	}

	// validate task context has actions planned
	if len(tctx.actionPlan) == 0 {
		return false, errors.Wrap(errTaskPlanValidate, "task action plan empty")
	}

	return true, nil
}

func (h *taskHandler) runActions(sw sw.StateSwitch, args sw.TransitionArgs) error {
	tctx, ok := args.(*taskHandlerContext)
	if !ok {
		return errInvalidTransitionHandler
	}

	task, ok := sw.(*model.Task)
	if !ok {
		return errors.Wrap(ErrSaveTask, ErrTaskTypeAssertions.Error())
	}

	for _, plan := range tctx.actionPlan {
		fmt.Println(plan.transitions)
		fmt.Println("xxx")
		err := plan.run(tctx.ctx, task.ActionsPlanned.ByID(plan.actionID), tctx)
		if err != nil {
			return err
		}
	}

	return nil
}

func (h *taskHandler) failed(sw sw.StateSwitch, args sw.TransitionArgs) error {
	tctx, ok := args.(*taskHandlerContext)
	if !ok {
		return errInvalidtaskHandlerContext
	}

	task, ok := sw.(*model.Task)
	if !ok {
		// TODO: fix error
		return errors.Wrap(ErrSaveTask, ErrTaskTypeAssertions.Error())
	}

	// include error in task information
	if tctx.err != nil {
		task.Info = tctx.err.Error()
	}

	return nil
}

func (h *taskHandler) saveState(sw sw.StateSwitch, args sw.TransitionArgs) error {
	// check currently queued count of tasks
	tctx, ok := args.(*taskHandlerContext)
	if !ok {
		return errInvalidTransitionHandler
	}

	task, ok := sw.(*model.Task)
	if !ok {
		return errors.Wrap(ErrSaveTask, ErrTaskTypeAssertions.Error())
	}

	if err := tctx.cache.UpdateTask(tctx.ctx, *task); err != nil {
		return errors.Wrap(ErrSaveTask, err.Error())
	}

	return nil
}
