package install

import (
	"context"
	"fmt"

	"github.com/bmc-toolbox/common"
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

// handler implements the Runner.Handler interface
//
// The handler is instantiated to run a single task
type handler struct {
	ctx                *sm.HandlerContext
	fwFile             string
	fwComponent        string
	fwVersion          string
	model              string
	vendor             string
	onlyPlan           bool
	bmcResetPreInstall bool
}

func (t *handler) Initialize(ctx context.Context) error {
	if t.ctx.DeviceQueryor == nil {
		// TODO(joel): DeviceQueryor is to be instantiated based on the method(s) for the firmwares to be installed
		// if its a mix of inband, out of band firmware to be installed, then both are to be queried and
		// so this DeviceQueryor would have to be extended
		//
		// For this to work with both inband and out of band, the firmware set data should include the install method.
		t.ctx.DeviceQueryor = outofband.NewDeviceQueryor(ctx, t.ctx.Asset, t.ctx.Logger)
	}

	return nil
}

// Query looks up the device component inventory and sets it in the task handler context.
func (t *handler) Query(ctx context.Context) error {
	t.ctx.Logger.Debug("run query step")

	// attempt to fetch component inventory from the device
	components, err := t.queryFromDevice(ctx)
	if err != nil {
		return errors.Wrap(errTaskQueryInventory, err.Error())
	}

	// component inventory was identified
	if len(components) > 0 {
		t.ctx.Asset.Components = components

		return nil
	}

	return errors.Wrap(errTaskQueryInventory, "failed to query device component inventory")
}

func (t *handler) PlanActions(ctx context.Context) error {
	t.ctx.Logger.Debug("create the plan")

	actionSMs, actions, err := t.planInstallFile(ctx)
	if err != nil {
		return err
	}

	t.ctx.ActionStateMachines = actionSMs
	t.ctx.Task.ActionsPlanned = actions

	return nil
}

func (t *handler) listPlan(tctx *sm.HandlerContext) error {
	tctx.Logger.WithField("plan.actions", len(tctx.ActionStateMachines)).Info("only listing the plan")
	for _, actionSM := range tctx.ActionStateMachines {
		for _, tx := range actionSM.TransitionOrder() {
			fmt.Println(tx)
		}
	}

	return nil
}

func (t *handler) RunActions(ctx context.Context) error {
	if t.onlyPlan {
		return t.listPlan(t.ctx)
	}

	t.ctx.Logger.WithField("plan.actions", len(t.ctx.ActionStateMachines)).Debug("running the plan")

	// each actionSM (state machine) corresponds to a firmware to be installed
	for _, actionSM := range t.ctx.ActionStateMachines {
		// fetch action attributes from task
		action := t.ctx.Task.ActionsPlanned.ByID(actionSM.ActionID())
		if err := action.SetState(model.StateActive); err != nil {
			return err
		}

		// return on context cancellation
		if ctx.Err() != nil {
			return ctx.Err()
		}

		t.ctx.Logger.WithFields(logrus.Fields{
			"statemachineID": actionSM.ActionID(),
			"final":          action.Final,
		}).Debug("action state machine start")

		// run the action state machine
		err := actionSM.Run(ctx, action, t.ctx)
		if err != nil {
			return errors.Wrap(
				err,
				"while running action to install firmware on component "+action.Firmware.Component,
			)
		}

		t.ctx.Logger.WithFields(logrus.Fields{
			"action":    action.ID,
			"condition": action.TaskID,
			"component": action.Firmware.Component,
			"version":   action.Firmware.Version,
		}).Info("action for component completed successfully")

		if !action.Final {
			continue
		}

		t.ctx.Logger.WithFields(logrus.Fields{
			"statemachineID": actionSM.ActionID(),
		}).Debug("state machine end")
	}

	t.ctx.Logger.Debug("plan finished")
	return nil
}

func (t *handler) Publish() {
	t.ctx.Publisher.Publish(t.ctx)
}

func (t *handler) planInstallFile(ctx context.Context) (sm.ActionStateMachines, model.Actions, error) {
	firmware := &model.Firmware{
		Component: t.fwComponent,
		Version:   t.fwVersion,
		Models:    []string{t.model},
		Vendor:    t.vendor,
	}

	steps, err := t.ctx.DeviceQueryor.FirmwareInstallSteps(ctx, firmware.Component)
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

	actionID := sm.ActionID(t.ctx.Task.ID.String(), firmware.Component, 1)
	m, err := outofband.NewActionStateMachine(actionID, steps, t.bmcResetPreInstall)
	if err != nil {
		return nil, nil, err
	}

	// include action state machines that will be executed.
	actionMachines = append(actionMachines, m)

	newAction := model.Action{
		ID:     actionID,
		TaskID: t.ctx.Task.ID.String(),

		// TODO: The firmware is to define the preferred install method
		// based on that the action plan is setup.
		//
		// For now this is hardcoded to outofband.
		InstallMethod: model.InstallMethodOutofband,

		// Firmware is the firmware to be installed
		Firmware: *firmware,

		// VerifyCurrentFirmware is disabled when ForceInstall is true.
		VerifyCurrentFirmware: !t.ctx.Task.Parameters.ForceInstall,

		// Setting this causes the action SM to not download the file
		FirmwareTempFile: t.fwFile,

		// Final is set to true when its the last action in the list.
		Final: true,

		BMCResetPreInstall:       t.bmcResetPreInstall,
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
func (t *handler) queryFromDevice(ctx context.Context) (model.Components, error) {
	if t.ctx.DeviceQueryor == nil {
		// TODO(joel): DeviceQueryor is to be instantiated based on the method(s) for the firmwares to be installed
		// if its a mix of inband, out of band firmware to be installed, then both are to be queried and
		// so this DeviceQueryor would have to be extended
		//
		// For this to work with both inband and out of band, the firmware set data should include the install method.
		t.ctx.DeviceQueryor = outofband.NewDeviceQueryor(ctx, t.ctx.Asset, t.ctx.Logger)
	}

	t.ctx.Task.Status.Append("connecting to device BMC")
	t.ctx.Publisher.Publish(t.ctx)

	if err := t.ctx.DeviceQueryor.Open(ctx); err != nil {
		return nil, err
	}

	t.ctx.Task.Status.Append("collecting inventory from device BMC")
	t.ctx.Publisher.Publish(t.ctx)

	deviceCommon, err := t.ctx.DeviceQueryor.Inventory(ctx)
	if err != nil {
		return nil, err
	}

	if t.ctx.Asset.Vendor == "" {
		t.ctx.Asset.Vendor = deviceCommon.Vendor
	}

	if t.ctx.Asset.Model == "" {
		t.ctx.Asset.Model = common.FormatProductName(deviceCommon.Model)
	}

	return model.NewComponentConverter().CommonDeviceToComponents(deviceCommon)
}

func (t *handler) OnSuccess(ctx context.Context, _ *model.Task) {
	if t.ctx.DeviceQueryor == nil {
		return
	}

	if err := t.ctx.DeviceQueryor.Close(ctx); err != nil {
		t.ctx.Logger.WithFields(logrus.Fields{"err": err.Error()}).Warn("device logout error")
	}
}

func (t *handler) OnFailure(ctx context.Context, _ *model.Task) {
	if t.ctx.DeviceQueryor == nil {
		return
	}

	if err := t.ctx.DeviceQueryor.Close(ctx); err != nil {
		t.ctx.Logger.WithFields(logrus.Fields{"err": err.Error()}).Warn("device logout error")
	}
}
