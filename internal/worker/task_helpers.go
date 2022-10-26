package worker

import (
	"context"

	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/outofband"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
)

// planInstall sets up the firmware install plan
//
// This returns a list of actions to added to the task and a list of action state machines for those actions.
func planInstall(ctx context.Context, task *model.Task) (sm.ActionStateMachines, model.Actions, error) {
	plans := make(sm.ActionStateMachines, 0)
	actions := make(model.Actions, 0)

	// sort the firmware for install
	task.FirmwaresPlanned.SortForInstall()

	var final bool
	// each firmware planned results in an ActionPlan and an Action
	for idx, firmware := range task.FirmwaresPlanned {
		actionID := sm.ActionID(task.ID.String(), firmware.ComponentSlug, idx)

		// TODO: The firmware is to define the preferred install method
		// based on that the action plan is setup.
		//
		// For now this is hardcoded to outofband.
		m, err := outofband.NewOutofbandActionStateMachine(ctx, actionID)
		if err != nil {
			return nil, nil, err
		}

		plans = append(plans, m)

		if len(task.FirmwaresPlanned) > 1 {
			final = (idx == len(task.FirmwaresPlanned)-1)
		} else {
			final = true
		}

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
			// Final is set to true when its the last action in the list.
			Final: final,
		})

	}

	return plans, actions, nil
}
