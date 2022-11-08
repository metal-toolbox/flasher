package worker

import (
	"fmt"
	"strings"

	"github.com/metal-toolbox/flasher/internal/inventory"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/outofband"
	"github.com/pkg/errors"

	sm "github.com/metal-toolbox/flasher/internal/statemachine"
)

func (h *taskHandler) inventoryFirmwareEqualsNew(components model.Components, firmware *model.Firmware) bool {
	component := components.BySlugVendorModel(firmware.ComponentSlug, firmware.Vendor, firmware.Model)
	if component == nil {
		return false
	}

	return strings.EqualFold(component.FirmwareInstalled, firmware.Version)
}

func (h *taskHandler) queryInstalledFirmware(ctx) {
	
}


// planInstall sets up the firmware install plan
//
// This returns a list of actions to added to the task and a list of action state machines for those actions.
func (h *taskHandler) planInstall(tctx *sm.HandlerContext, task *model.Task, firmwareURLPrefix string) (sm.ActionStateMachines, model.Actions, error) {
	// lookup inventory for installed firmware
	var device *inventory.DeviceInventory

	if task.Parameters.LookupInventoryFirmware {
		var err error

		// deviceByID returns outofband inventory, once
		//
		// TODO: The firmware is to define the preferred install method
		// based on that the action plan is setup, then this will return both inband, outofband inventory.
		device, err = tctx.Inv.DeviceByID(tctx.Ctx, task.Parameters.Device.ID.String())
		if err != nil {
			return nil, nil, errors.Wrap(errTaskPlanActions, err.Error())
		}
	}

	plans := make(sm.ActionStateMachines, 0)
	actions := make(model.Actions, 0)

	// sort the firmware for install
	task.FirmwaresPlanned.SortByInstallOrder()

	var final bool
	// each firmware planned results in an ActionPlan and an Action
	for idx, firmware := range task.FirmwaresPlanned {
		// lookup firmware
		if task.Parameters.LookupInventoryFirmware && device != nil {
			if h.inventoryFirmwareEqualsNew(device.Components, &task.FirmwaresPlanned[idx]) {
				continue
			}
		}

		actionID := sm.ActionID(task.ID.String(), firmware.ComponentSlug, idx)

		// TODO: The firmware is to define the preferred install method
		// based on that the action plan is setup.
		//
		// For now this is hardcoded to outofband.
		m, err := outofband.NewActionStateMachine(tctx.Ctx, actionID)
		if err != nil {
			return nil, nil, err
		}

		plans = append(plans, m)

		if len(task.FirmwaresPlanned) > 1 {
			final = (idx == len(task.FirmwaresPlanned)-1)
		} else {
			final = true
		}

		// set download url based on device vendor, model attributes
		// example : https://firmware.hosted/firmware/dell/r640/bmc/iDRAC-with-Lifecycle-Controller_Firmware_P8HC9_WN64_5.10.00.00_A00.EXE
		task.FirmwaresPlanned[idx].URL = fmt.Sprintf(
			"%s/%s/%s/%s/%s",
			firmwareURLPrefix,
			task.Parameters.Device.Vendor,
			task.Parameters.Device.Model,
			strings.ToLower(firmware.ComponentSlug),
			firmware.FileName,
		)

		actions = append(actions, model.Action{
			ID:     actionID,
			TaskID: task.ID.String(),

			// TODO: The firmware is to define the preferred install method
			// based on that the action plan is setup.
			//
			// For now this is hardcoded to outofband.
			InstallMethod: model.InstallMethodOutofband,
			Status:        string(model.StateQueued),
			Firmware:      task.FirmwaresPlanned[idx],

			// VerifyCurrentFirmware is disabled when ForceInstall is true.
			VerifyCurrentFirmware: !task.Parameters.ForceInstall,

			// Final is set to true when its the last action in the list.
			Final: final,
		})
	}

	return plans, actions, nil
}
