package install

import (
	"fmt"

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
)

// taskHandler implements the taskTransitionHandler methods
type taskHandler struct {
	fwFile             string
	fwComponent        string
	fwVersion          string
	model              string
	vendor             string
	onlyPlan           bool
	bmcResetPreInstall bool
}

func (h *taskHandler) Init(_ sw.StateSwitch, args sw.TransitionArgs) error {
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return sm.ErrInvalidtaskHandlerContext
	}

	if tctx.DeviceQueryor == nil {
		// TODO(joel): DeviceQueryor is to be instantiated based on the method(s) for the firmwares to be installed
		// if its a mix of inband, out of band firmware to be installed, then both are to be queried and
		// so this DeviceQueryor would have to be extended
		//
		// For this to work with both inband and out of band, the firmware set data should include the install method.
		tctx.DeviceQueryor = outofband.NewDeviceQueryor(tctx.Ctx, tctx.Asset, tctx.Logger)
	}

	return nil
}

// Query looks up the device component inventory and sets it in the task handler context.
func (h *taskHandler) Query(_ sw.StateSwitch, args sw.TransitionArgs) error {
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return sm.ErrInvalidtaskHandlerContext
	}

	tctx.Logger.Debug("run query step")

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

	tctx.Logger.Debug("create the plan")

	actionSMs, actions, err := h.planInstallFile(tctx, task.ID.String(), task.Parameters.ForceInstall)
	if err != nil {
		return err
	}

	tctx.ActionStateMachines = actionSMs
	task.ActionsPlanned = actions

	return nil
}

func (h *taskHandler) listPlan(tctx *sm.HandlerContext) error {
	tctx.Logger.WithField("plan.actions", len(tctx.ActionStateMachines)).Info("only listing the plan")
	for _, actionSM := range tctx.ActionStateMachines {
		for _, tx := range actionSM.TransitionOrder() {
			fmt.Println(tx)
		}
	}

	return nil
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

	if h.onlyPlan {
		return h.listPlan(tctx)
	}

	tctx.Logger.WithField("plan.actions", len(tctx.ActionStateMachines)).Debug("running the plan")

	// each actionSM (state machine) corresponds to a firmware to be installed
	for _, actionSM := range tctx.ActionStateMachines {
		// fetch action attributes from task
		action := task.ActionsPlanned.ByID(actionSM.ActionID())
		if err := action.SetState(model.StateActive); err != nil {
			return err
		}

		// return on context cancellation
		if tctx.Ctx.Err() != nil {
			return tctx.Ctx.Err()
		}

		tctx.Logger.WithFields(logrus.Fields{
			"statemachineID": actionSM.ActionID(),
			"final":          action.Final,
		}).Debug("action state machine start")

		// run the action state machine
		err := actionSM.Run(tctx.Ctx, action, tctx)
		if err != nil {
			return errors.Wrap(
				err,
				"while running action to install firmware on component "+action.Firmware.Component,
			)
		}

		tctx.Logger.WithFields(logrus.Fields{
			"action":    action.ID,
			"condition": action.TaskID,
			"component": action.Firmware.Component,
			"version":   action.Firmware.Version,
		}).Info("action for component completed successfully")

		if !action.Final {
			continue
		}

		tctx.Logger.WithFields(logrus.Fields{
			"statemachineID": actionSM.ActionID(),
		}).Debug("state machine end")
	}

	tctx.Logger.Debug("plan finished")
	return nil
}

func (h *taskHandler) TaskFailed(_ sw.StateSwitch, args sw.TransitionArgs) error {
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return sm.ErrInvalidTransitionHandler
	}

	tctx.Task.Status.Append("task failed")

	if tctx.DeviceQueryor != nil {
		if err := tctx.DeviceQueryor.Close(tctx.Ctx); err != nil {
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

	tctx.Task.Status.Append("task completed successfully")

	if tctx.DeviceQueryor != nil {
		if err := tctx.DeviceQueryor.Close(tctx.Ctx); err != nil {
			tctx.Logger.WithFields(logrus.Fields{"err": err.Error()}).Warn("device logout error")
		}
	}

	return nil
}

func (h *taskHandler) PublishStatus(_ sw.StateSwitch, args sw.TransitionArgs) error {
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return sm.ErrInvalidTransitionHandler
	}

	tctx.Publisher.Publish(tctx)

	return nil
}

func (h *taskHandler) planInstallFile(tctx *sm.HandlerContext, taskID string, forceInstall bool) (sm.ActionStateMachines, model.Actions, error) {
	firmware := &model.Firmware{
		Component: h.fwComponent,
		Version:   h.fwVersion,
		Models:    []string{h.model},
		Vendor:    h.vendor,
	}

	steps, err := tctx.DeviceQueryor.FirmwareInstallSteps(tctx.Ctx, firmware.Component)
	if err != nil {
		return nil, nil, err
	}

	errFirmwareInstallSteps := errors.New("no firmware install steps identified for component")
	if len(steps) == 0 {
		return nil, nil, errors.Wrap(errFirmwareInstallSteps, firmware.Component)
	}

	bmcResetOnInstallFailure, bmcResetPostInstall := outofband.BmcResetParams(steps)

	actionMachines := make(sm.ActionStateMachines, 0)
	actions := make(model.Actions, 0)

	actionID := sm.ActionID(taskID, firmware.Component, 1)
	m, err := outofband.NewActionStateMachine(actionID, steps, h.bmcResetPreInstall)
	if err != nil {
		return nil, nil, err
	}

	// include action state machines that will be executed.
	actionMachines = append(actionMachines, m)

	newAction := model.Action{
		ID:     actionID,
		TaskID: taskID,

		// TODO: The firmware is to define the preferred install method
		// based on that the action plan is setup.
		//
		// For now this is hardcoded to outofband.
		InstallMethod: model.InstallMethodOutofband,

		// Firmware is the firmware to be installed
		Firmware: *firmware,

		// VerifyCurrentFirmware is disabled when ForceInstall is true.
		VerifyCurrentFirmware: !forceInstall,

		// Setting this causes the action SM to not download the file
		FirmwareTempFile: h.fwFile,

		// Final is set to true when its the last action in the list.
		Final: true,

		BMCResetPreInstall:       h.bmcResetPreInstall,
		BMCResetPostInstall:      bmcResetPostInstall,
		BMCResetOnInstallFailure: bmcResetOnInstallFailure,
	}

	//nolint:errcheck  // SetState never returns an error
	newAction.SetState(model.StatePending)

	// create action thats added to the task
	actions = append(actions, &newAction)

	return actionMachines, actions, nil
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

	tctx.Task.Status.Append("connecting to device BMC")
	tctx.Publisher.Publish(tctx)

	if err := tctx.DeviceQueryor.Open(tctx.Ctx); err != nil {
		return nil, err
	}

	tctx.Task.Status.Append("collecting inventory from device BMC")
	tctx.Publisher.Publish(tctx)

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
