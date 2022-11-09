package worker

import (
	sw "github.com/filanov/stateswitch"
	"github.com/metal-toolbox/flasher/internal/model"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var (
	ErrSaveTask           = errors.New("error in saveTask transition handler")
	ErrTaskTypeAssertion  = errors.New("error asserting Task type")
	errTaskQueryInventory = errors.New("error in task query inventory for installed firmware")
	errTaskPlanActions    = errors.New("error in task action planning")
	errTaskPlanValidate   = errors.New("error in task plan validation")
)

// taskHandler implements the taskTransitionHandler methods
type taskHandler struct{}

// Query looks up the device component inventory and sets it in the task handler context.
func (h *taskHandler) Query(t sw.StateSwitch, args sw.TransitionArgs) error {
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return sm.ErrInvalidtaskHandlerContext
	}

	task, ok := t.(*model.Task)
	if !ok {
		return errors.Wrap(errTaskQueryInventory, ErrTaskTypeAssertion.Error())
	}

	deviceID := task.Parameters.Device.ID.String()

	// first attempt to fetch component inventory from inventory source
	//
	// error ignored on purpose
	components, _ := h.queryFromInventorySource(tctx, deviceID)

	// component inventory was identified
	if len(components) > 0 {
		tctx.Device.Components = components

		return nil
	}

	var err error

	// second attempt to fetch component inventory from the device
	if components, err = h.queryFromDevice(tctx); err != nil {
		return errors.Wrap(errTaskQueryInventory, err.Error())
	}

	// component inventory was identified
	if len(components) > 0 {
		tctx.Device.Components = components

		return nil
	}

	return errors.Wrap(errTaskQueryInventory, "failed to query device component inventory")
}

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

func (h *taskHandler) ValidatePlan(t sw.StateSwitch, args sw.TransitionArgs) (bool, error) {
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
			return errors.Wrap(
				err,
				"while running action to install firmware on component "+action.Firmware.ComponentSlug,
			)
		}
	}

	return nil
}

func (h *taskHandler) TaskFailed(task sw.StateSwitch, args sw.TransitionArgs) error {
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return sm.ErrInvalidTransitionHandler
	}

	if tctx.DeviceQueryor != nil {
		if err := tctx.DeviceQueryor.Close(); err != nil {
			tctx.Logger.WithFields(logrus.Fields{"err": err.Error()}).Warn("device logout error")
		}
	}

	return nil
}

func (h *taskHandler) TaskSuccessful(task sw.StateSwitch, args sw.TransitionArgs) error {
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return sm.ErrInvalidTransitionHandler
	}

	if tctx.DeviceQueryor != nil {
		if err := tctx.DeviceQueryor.Close(); err != nil {
			tctx.Logger.WithFields(logrus.Fields{"err": err.Error()}).Warn("device logout error")
		}
	}

	return nil
}

func (h *taskHandler) PersistState(t sw.StateSwitch, args sw.TransitionArgs) error {
	// check currently queued count of tasks
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return sm.ErrInvalidTransitionHandler
	}

	task, ok := t.(*model.Task)
	if !ok {
		return errors.Wrap(ErrSaveTask, ErrTaskTypeAssertion.Error())
	}

	if err := tctx.Store.UpdateTask(tctx.Ctx, *task); err != nil {
		return errors.Wrap(ErrSaveTask, err.Error())
	}

	return nil
}
