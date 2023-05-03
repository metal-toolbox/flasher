package worker

import (
	"fmt"
	"strings"

	"github.com/bmc-toolbox/common"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/outofband"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// fetches device components inventory from the configured inventory source.d
func (h *taskHandler) queryFromInventorySource(tctx *sm.HandlerContext, deviceID string) (model.Components, error) {
	device, err := tctx.Inv.DeviceByID(tctx.Ctx, deviceID)
	if err != nil {
		return nil, err
	}

	// this is a bit
	if tctx.Device.Vendor == "" {
		tctx.Device.Vendor = device.Device.Vendor
	}

	if tctx.Device.Model == "" {
		tctx.Device.Vendor = device.Device.Model
	}

	return device.Components, nil
}

// query device components inventory from the device itself.
func (h *taskHandler) queryFromDevice(tctx *sm.HandlerContext) (model.Components, error) {
	if tctx.DeviceQueryor == nil {
		// TODO(joel): DeviceQueryor is to be instantiated based on the method(s) for the firmwares to be installed
		// if its a mix of inband, out of band firmware to be installed, then both are to be queried and
		// so this DeviceQueryor may have to be extended
		//
		// For this to work with both inband and out of band, the firmware must include the install method.
		tctx.DeviceQueryor = outofband.NewDeviceQueryor(tctx.Ctx, tctx.Device, tctx.Logger)
	}

	if err := tctx.DeviceQueryor.Open(tctx.Ctx); err != nil {
		return nil, err
	}

	deviceCommon, err := tctx.DeviceQueryor.Inventory(tctx.Ctx)
	if err != nil {
		return nil, err
	}

	if tctx.Device.Vendor == "" {
		tctx.Device.Vendor = deviceCommon.Vendor
	}

	if tctx.Device.Model == "" {
		tctx.Device.Model = common.FormatProductName(deviceCommon.Model)
	}

	return model.NewComponentConverter().CommonDeviceToComponents(deviceCommon)
}

// returns a bool value based on if the firmware install (for a component) should be skipped
func (h *taskHandler) skipFirmwareInstall(tctx *sm.HandlerContext, task *model.Task, firmware *model.Firmware) bool {
	component := tctx.Device.Components.BySlugVendorModel(firmware.ComponentSlug, firmware.Vendor, firmware.Model)
	if component == nil {
		tctx.Logger.WithFields(
			logrus.Fields{
				"component": firmware.ComponentSlug,
				"model":     firmware.Model,
				"vendor":    firmware.Vendor,
				"requested": firmware.Version,
			},
		).Trace("install skipped - component not present on device")

		return true
	}

	// when force install is set, firmware version comparison is skipped.
	if task.Parameters.ForceInstall {
		return false
	}

	skip := strings.EqualFold(component.FirmwareInstalled, firmware.Version)
	if skip {
		tctx.Logger.WithFields(
			logrus.Fields{
				"component": firmware.ComponentSlug,
				"requested": firmware.Version,
			},
		).Info("install skipped - installed firmware equals requested")
	}

	return skip
}

// planInstall sets up the firmware install plan
//
// This returns a list of actions to added to the task and a list of action state machines for those actions.
func (h *taskHandler) planInstall(tctx *sm.HandlerContext, task *model.Task, firmwaresApplicable model.Firmwares) (sm.ActionStateMachines, model.Actions, error) {
	actionMachines := make(sm.ActionStateMachines, 0)
	actions := make(model.Actions, 0)

	// final is set to true in the final action
	var final bool

	// sort firmware applicable by install order.
	firmwaresApplicable.SortByInstallOrder()

	// each firmware applicable results in an ActionPlan and an Action
	for idx, firmware := range firmwaresApplicable {
		// skip firmware install based on a few clauses
		if h.skipFirmwareInstall(tctx, task, &firmwaresApplicable[idx]) {
			continue
		}

		// set final bool when its the last firmware in the slice
		if len(firmwaresApplicable) > 1 {
			final = (idx == len(firmwaresApplicable)-1)
		} else {
			final = true
		}

		// generate an action ID
		actionID := sm.ActionID(task.ID.String(), firmware.ComponentSlug, idx)

		// TODO: The firmware is to define the preferred install method
		// based on that the action plan is setup.
		//
		// For now this is hardcoded to outofband.
		m, err := outofband.NewActionStateMachine(tctx.Ctx, actionID)
		if err != nil {
			return nil, nil, err
		}

		// include action state machines that will be executed.
		actionMachines = append(actionMachines, m)

		// set download url based on device vendor, model attributes
		// example : https://firmware.hosted/firmware/dell/r640/bmc/iDRAC-with-Lifecycle-Controller_Firmware_P8HC9_WN64_5.10.00.00_A00.EXE
		firmwaresApplicable[idx].URL = fmt.Sprintf(
			"%s/%s/%s/%s/%s",
			tctx.FirmwareURLPrefix,
			task.Parameters.Device.Vendor,
			task.Parameters.Device.Model,
			strings.ToLower(firmware.ComponentSlug),
			firmware.FileName,
		)

		// include applicable firmware in planned
		task.FirmwaresPlanned = append(task.FirmwaresPlanned, firmwaresApplicable[idx])

		// create action thats added to the task
		actions = append(actions, model.Action{
			ID:     actionID,
			TaskID: task.ID.String(),

			// TODO: The firmware is to define the preferred install method
			// based on that the action plan is setup.
			//
			// For now this is hardcoded to outofband.
			InstallMethod: model.InstallMethodOutofband,
			Status:        string(model.StateQueued),
			Firmware:      firmwaresApplicable[idx],

			// VerifyCurrentFirmware is disabled when ForceInstall is true.
			VerifyCurrentFirmware: !task.Parameters.ForceInstall,

			// Final is set to true when its the last action in the list.
			Final: final,
		})
	}

	return actionMachines, actions, nil
}

// planFromFirmwareSet
func (h *taskHandler) planFromFirmwareSet(tctx *sm.HandlerContext, task *model.Task, device model.Device) error {
	var err error

	var firmwaresApplicable []model.Firmware

	// When theres no firmware set ID, lookup firmware by the device vendor, model.
	if task.Parameters.FirmwareSetID == "" {
		if device.Vendor == "" {
			return errors.Wrap(errTaskPlanActions, "device vendor attribute was not identified")
		}

		if device.Model == "" {
			return errors.Wrap(errTaskPlanActions, "device model attribute was not identified")
		}

		firmwaresApplicable, err = tctx.Inv.FirmwareByDeviceVendorModel(tctx.Ctx, device.Vendor, device.Model)
		if err != nil {
			return errors.Wrap(errTaskPlanActions, err.Error())
		}
	} else {
		// TODO: implement inventory methods for firmware by set id
		return errors.Wrap(errTaskPlanActions, "firmware set by ID not implemented")
	}

	if len(firmwaresApplicable) == 0 {
		return errors.Wrap(errTaskPlanActions, "planFromFirmwareSet(): no applicable firmware identified")
	}

	// plan actions based and update task action list
	tctx.ActionStateMachines, task.ActionsPlanned, err = h.planInstall(tctx, task, firmwaresApplicable)
	if err != nil {
		return err
	}

	// 	update task in cache
	return tctx.Store.UpdateTask(tctx.Ctx, *task)
}
