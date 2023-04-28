package worker

import (
	"strings"

	"github.com/bmc-toolbox/common"
	sw "github.com/filanov/stateswitch"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/outofband"
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

func (h *taskHandler) Init(_ sw.StateSwitch, _ sw.TransitionArgs) error {
	return nil
}

// Query looks up the device component inventory and sets it in the task handler context.
func (h *taskHandler) Query(t sw.StateSwitch, args sw.TransitionArgs) error {
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return sm.ErrInvalidtaskHandlerContext
	}

	_, ok = t.(*model.Task)
	if !ok {
		return errors.Wrap(errTaskQueryInventory, ErrTaskTypeAssertion.Error())
	}

	// asset has component inventory
	if len(tctx.Asset.Components) > 0 {
		return nil
	}

	// attempt to fetch component inventory from the device
	components, err := h.queryFromDevice(tctx)
	if err != nil {
		return errors.Wrap(errTaskQueryInventory, err.Error())
	}

	// component inventory was identified
	if len(components) > 0 {
		tctx.Asset.Components = components

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

	switch task.FirmwarePlanMethod {
	case model.FromFirmwareSet:
		return h.planFromFirmwareSet(tctx, task)
	case model.FromRequestedFirmware:
		return errors.Wrap(errTaskPlanActions, "firmware plan method not implemented"+string(model.FromRequestedFirmware))
	default:
		return errors.Wrap(errTaskPlanActions, "firmware plan method invalid: "+string(task.FirmwarePlanMethod))
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

	if len(task.InstallFirmwares) == 0 {
		return false, errors.Wrap(errTaskPlanValidate, "no firmwares planned for install")
	}

	// validate task has action plans listed
	if len(task.ActionsPlanned) == 0 {
		return false, errors.Wrap(errTaskPlanValidate, "no task actions planned")
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
		// return on context cancellation
		if tctx.Ctx.Err() != nil {
			return tctx.Ctx.Err()
		}

		// fetch action attributes from task
		action := task.ActionsPlanned.ByID(actionSM.ActionID())

		// run the action state machine
		err := actionSM.Run(tctx.Ctx, action, tctx)
		if err != nil {
			return errors.Wrap(
				err,
				"while running action to install firmware on component "+action.Firmware.Component,
			)
		}
	}

	return nil
}

func (h *taskHandler) TaskFailed(_ sw.StateSwitch, args sw.TransitionArgs) error {
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

func (h *taskHandler) TaskSuccessful(_ sw.StateSwitch, args sw.TransitionArgs) error {
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

func (h *taskHandler) PublishStatus(t sw.StateSwitch, args sw.TransitionArgs) error {
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return sm.ErrInvalidTransitionHandler
	}

	task, ok := t.(*model.Task)
	if !ok {
		return errors.Wrap(ErrSaveTask, ErrTaskTypeAssertion.Error())
	}

	tctx.Publisher.Publish(tctx.Ctx, task)

	return nil
}

// planFromFirmwareSet
func (h *taskHandler) planFromFirmwareSet(tctx *sm.HandlerContext, task *model.Task) error {
	applicable, err := tctx.Store.FirmwareSetByID(tctx.Ctx, task.Parameters.FirmwareSetID)
	if err != nil {
		return errors.Wrap(errTaskPlanActions, err.Error())
	}

	if len(applicable) == 0 {
		return errors.Wrap(errTaskPlanActions, "planFromFirmwareSet(): no applicable firmware identified")
	}

	// plan actions based and update task action list
	tctx.ActionStateMachines, task.ActionsPlanned, err = h.planInstall(tctx, task, applicable)
	if err != nil {
		return err
	}

	return nil
}

// query device components inventory from the device itself.
func (h *taskHandler) queryFromDevice(tctx *sm.HandlerContext) (model.Components, error) {
	if tctx.DeviceQueryor == nil {
		// TODO(joel): DeviceQueryor is to be instantiated based on the method(s) for the firmwares to be installed
		// if its a mix of inband, out of band firmware to be installed, then both are to be queried and
		// so this DeviceQueryor would have to be extended
		//
		// For this to work with both inband and out of band, the firmware set data should include the install method.
		tctx.DeviceQueryor = outofband.NewDeviceQueryor(tctx.Ctx, tctx.Asset, tctx.Logger)
	}

	tctx.Task.Status = "connecting to device BMC"
	tctx.Publisher.Publish(tctx.Ctx, tctx.Task)

	if err := tctx.DeviceQueryor.Open(tctx.Ctx); err != nil {
		return nil, err
	}

	tctx.Task.Status = "collecting inventory from device BMC"
	tctx.Publisher.Publish(tctx.Ctx, tctx.Task)

	deviceCommon, err := tctx.DeviceQueryor.Inventory(tctx.Ctx)
	if err != nil {
		return nil, err
	}

	if tctx.Asset.Vendor == "" {
		tctx.Asset.Vendor = deviceCommon.Vendor
	}

	if tctx.Asset.Model == "" {
		tctx.Asset.Model = common.FormatProductName(deviceCommon.Model)
	}

	return model.NewComponentConverter().CommonDeviceToComponents(deviceCommon)
}

// returns a bool value based on if the firmware install (for a component) should be skipped
func (h *taskHandler) skipFirmwareInstall(tctx *sm.HandlerContext, task *model.Task, firmware *model.Firmware) bool {
	component := tctx.Asset.Components.BySlugVendorModel(firmware.Component, firmware.Vendor, firmware.Models)
	if component == nil {
		tctx.Logger.WithFields(
			logrus.Fields{
				"component": firmware.Component,
				"models":    firmware.Models,
				"vendor":    firmware.Vendor,
				"requested": firmware.Version,
			},
		).Trace("install skipped - component not present on device")

		return true
	}

	// when force install is set, installed firmware version comparison is skipped.
	if task.Parameters.ForceInstall {
		return false
	}

	skip := strings.EqualFold(component.FirmwareInstalled, firmware.Version)
	if skip {
		tctx.Logger.WithFields(
			logrus.Fields{
				"component": firmware.Component,
				"requested": firmware.Version,
			},
		).Info("install skipped - installed firmware equals requested")
	}

	return skip
}

// planInstall sets up the firmware install plan
//
// This returns a list of actions to added to the task and a list of action state machines for those actions.
func (h *taskHandler) planInstall(tctx *sm.HandlerContext, task *model.Task, firmwares []*model.Firmware) (sm.ActionStateMachines, model.Actions, error) {
	actionMachines := make(sm.ActionStateMachines, 0)
	actions := make(model.Actions, 0)

	// final is set to true in the final action
	var final bool

	// each firmware applicable results in an ActionPlan and an Action
	for idx, firmware := range firmwares {
		// skip firmware install based on a few clauses
		if h.skipFirmwareInstall(tctx, task, firmwares[idx]) {
			continue
		}

		// set final bool when its the last firmware in the slice
		if len(firmwares) > 1 {
			final = (idx == len(firmwares)-1)
		} else {
			final = true
		}

		// generate an action ID
		actionID := sm.ActionID(task.ID.String(), firmware.Component, idx)

		// TODO: The firmware is to define the preferred install method
		// based on that the action plan is setup.
		//
		// For now this is hardcoded to outofband.
		m, err := outofband.NewActionStateMachine(actionID)
		if err != nil {
			return nil, nil, err
		}

		// include action state machines that will be executed.
		actionMachines = append(actionMachines, m)

		// include applicable firmware in planned
		task.InstallFirmwares = append(task.InstallFirmwares, firmwares[idx])

		newAction := model.Action{
			ID:     actionID,
			TaskID: task.ID.String(),

			// TODO: The firmware is to define the preferred install method
			// based on that the action plan is setup.
			//
			// For now this is hardcoded to outofband.
			InstallMethod: model.InstallMethodOutofband,

			// Firmware is the firmware to be installed
			Firmware: *firmwares[idx],

			// VerifyCurrentFirmware is disabled when ForceInstall is true.
			VerifyCurrentFirmware: !task.Parameters.ForceInstall,

			// Final is set to true when its the last action in the list.
			Final: final,
		}

		if err := newAction.SetState(model.StatePending); err != nil {
			return nil, nil, err
		}

		// create action thats added to the task
		actions = append(actions, &newAction)
	}

	return actionMachines, actions, nil
}
