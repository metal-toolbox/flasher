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
)

type taskTransitioner interface {
	planActions(sw sw.StateSwitch, args sw.TransitionArgs) error
	runActions(sw sw.StateSwitch, args sw.TransitionArgs) error
	saveState(sw sw.StateSwitch, args sw.TransitionArgs) error

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

	var plan []model.Action
	var err error

	switch task.Parameters.FirmwarePlanMethod {
	case model.PlanFromFirmwareSet:
		return errors.Wrap(errTaskPlanActions, "not implemented plan method: "+string(model.PlanFromFirmwareSet))
	case model.PlanUseDefinedFirmware:
		return errors.Wrap(errTaskPlanActions, "not implemented plan method: "+string(model.PlanUseDefinedFirmware))
	case model.PlanFromInstalledFirmware:
		plan, err = h.planFromInstalledFirmware(ctx, task.Device)
	}

	_ = plan
	_ = err
	// 1. query inventory for inventory, firmwares
	// 2. resolve firmware to be installed
	// 3. plan actions for task
	// TODO: add actions to task
	//	actionSMs, err := actionsFromTask(ctx, task.Parameters.Install)
	//	if err != nil {
	//		return nil, errors.Wrap(errTaskActionsInit, err.Error())
	//	}
	//
	//	if len(actionSMs) == 0 {
	//		return nil, nil, errors.Wrap(errTaskActionsInit, "no actions identified for firmware install")
	//	}
	return nil
}

// TODO: move plan methods into firmware package
func (h *taskHandler) planFromInstalledFirmware(ctx *taskHandlerContext, device model.Device) ([]model.Action, error) {
	// 1. query current device inventory - from the BMC
	// 2. query firmware set that match the device vendor, model
	// 3. compare installed version with the versions returned in the firmware set
	// 4. prepare actions based on the firmware versions planned

	return nil, nil
}

func (h *taskHandler) validatePlanAction(sw sw.StateSwitch, args sw.TransitionArgs) (bool, error) {
	_, ok := args.(*taskHandlerContext)
	if !ok {
		return false, errInvalidtaskHandlerContext
	}

	_, ok = sw.(*model.Task)
	if !ok {
		return false, errors.Wrap(ErrSaveTask, ErrTaskTypeAssertions.Error())
	}

	// validate task has firmware resolved
	//	if len(task.FirmwareResolved) == 0 {
	//		return false, errTaskInstallParametersUndefined
	//	}
	//
	//	// validate task context has actions planned
	//	if len(taskCtx.actionStateMachineList) == 0 {
	//		return false, err
	//	}

	return true, nil
}

func (h *taskHandler) runActions(sw sw.StateSwitch, args sw.TransitionArgs) error {
	mctx, ok := args.(*taskHandlerContext)
	if !ok {
		return errInvalidTransitionHandler
	}
	_ = mctx

	//	for _, action := range mCtx.actionsSM {
	//		//action.run(mCtx.ctx, <need model.Action here>, mctx)
	//	}
	//
	fmt.Println("here")
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

	// handler error to be set in task
	if tctx.err != nil {
		task.Info = tctx.err.Error()
	}

	if err := tctx.cache.UpdateTask(tctx.ctx, *task); err != nil {
		return errors.Wrap(ErrSaveTask, err.Error())
	}

	return nil
}
