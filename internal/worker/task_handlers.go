package worker

import (
	sw "github.com/filanov/stateswitch"
	"github.com/metal-toolbox/flasher/internal/inventory"
	"github.com/metal-toolbox/flasher/internal/model"
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
		return errors.Wrap(errTaskPlanActions, "firmware set by ID not implemented")
	}

	// plan actions based and update task action list
	tctx.ActionStateMachines, task.ActionsPlanned, err = planInstall(tctx.Ctx, task, tctx.FirmwareURLPrefix)
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
			return errors.Wrap(
				err,
				"while running action to install firmware on component "+action.Firmware.ComponentSlug,
			)
		}
	}

	return nil
}

func (h *taskHandler) TaskFailed(task sw.StateSwitch, args sw.TransitionArgs) error {
	return nil
}

func (h *taskHandler) TaskSuccessful(task sw.StateSwitch, args sw.TransitionArgs) error {
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

	// update task state in inventory
	//
	// TODO(joel) - figure if this can be moved in a different package
	attr := &inventory.FwInstallAttributes{
		TaskParameters: task.Parameters,
		FlasherTaskID:  task.ID.String(),
		WorkerID:       tctx.WorkerID,
		Status:         task.Status,
	}

	if err := tctx.Inv.SetFlasherAttributes(tctx.Ctx, task.Parameters.Device.ID.String(), attr); err != nil {
		return err
	}

	return nil
}
