package outofband

import (
	"context"
	"strconv"

	sw "github.com/filanov/stateswitch"
	"github.com/metal-toolbox/flasher/internal/model"
)

const (
	// action states
	//
	// the SM transitions through these states for each component being updated.
	stateLoginBMC        sw.State = "loginBMC"
	stateUploadFirmware  sw.State = "uploadFirmware"
	stateInstallFirmware sw.State = "installFirmware"
	stateResetBMC        sw.State = "resetBMC"
	stateResetHost       sw.State = "resetHost"

	transitionTypeLoginBMC        sw.TransitionType = "loginBMC"
	transitionTypeInstallFirmware sw.TransitionType = "installFirmware"
	transitionTypeUploadFirmware  sw.TransitionType = "uploadFirmware"
	transitionTypeResetBMC        sw.TransitionType = "resetBMC"
	transitionTypeResetHost       sw.TransitionType = "resetHost"

	// state, transition for failed actions
	stateInstallFailed         sw.State          = "installFailed"
	transitionTypeActionFailed sw.TransitionType = "actionFailed"
)

type ActionStateMachine struct {
	sm sw.StateMachine
}

func NewActionStateMachine(ctx context.Context) (*ActionStateMachine, error) {
	m := &ActionStateMachine{sm: sw.NewStateMachine()}

	handler := &actionHandler{}

	// The SM has transition rules define the transitionHandler methods
	// each transitionHandler method is passed as values to the transition rule.
	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   transitionTypeLoginBMC,
		SourceStates:     sw.States{stateQueued},
		DestinationState: stateActive,

		// Condition for the transition, transition will be executed only if this function return true
		// Can be nil, in this case it's considered as return true, nil
		Condition: nil,

		// Transition is users business logic, should not set the state or return next state
		// If condition returns true this function will be executed
		Transition: handler.loginBMC,

		// PostTransition will be called if condition and transition are successful.
		PostTransition: handler.saveState,
	})

	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   transitionTypeUploadFirmware,
		SourceStates:     sw.States{stateLoginBMC},
		DestinationState: stateUploadFirmware,
		Condition:        nil,
		Transition:       handler.uploadFirmware,
		PostTransition:   handler.saveState,
	})

	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   transitionTypeInstallFirmware,
		SourceStates:     sw.States{stateUploadFirmware},
		DestinationState: stateUploadFirmware,
		Condition:        nil,
		Transition:       handler.uploadFirmware,
		PostTransition:   handler.saveState,
	})

	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   transitionTypeResetBMC,
		SourceStates:     sw.States{stateInstallFirmware},
		DestinationState: stateResetBMC,
		Condition:        handler.conditionalResetBMC,
		Transition:       handler.resetBMC,
		PostTransition:   handler.saveState,
	})

	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   transitionTypeResetHost,
		SourceStates:     sw.States{stateInstallFirmware},
		DestinationState: stateResetHost,
		Condition:        handler.conditionalResetHost,
		Transition:       handler.resetHost,
		PostTransition:   handler.saveState,
	})

	m.sm.AddTransition(sw.TransitionRule{
		TransitionType: transitionTypeActionFailed,
		SourceStates: sw.States{
			stateLoginBMC,
			stateUploadFirmware,
			stateInstallFirmware,
			stateResetBMC,
			stateResetHost,
		},
		DestinationState: stateInstallFailed,
		Condition:        nil,
		Transition:       handler.saveState,
		PostTransition:   nil,
	})

	return m, nil
}

func (a *ActionStateMachine) TransitionFailed(ctx context.Context, action *model.Action, smCtx *taskHandlerContext) error {
	return a.sm.Run(transitionTypeActionFailed, action, smCtx)
}

func (a *ActionStateMachine) run(ctx context.Context, action *model.Action, smCtx *taskHandlerContext) error {
	order := []sw.TransitionType{
		transitionTypeLoginBMC,
		transitionTypeInstallFirmware,
		transitionTypeUploadFirmware,
		transitionTypeResetBMC,
		transitionTypeResetHost,
	}

	for _, transitionType := range order {
		err := a.sm.Run(transitionType, action, smCtx)
		if err != nil {
			if err := a.TransitionFailed(ctx, action, smCtx); err != nil {
				return err
			}

			return err
		}

	}

	return nil
}

// actionStateMachinesForTask returns a slice of state machines for each of the firmware versions to be installed in a task.
func actionStateMachinesForTask(ctx context.Context, task *model.Task) (ActionStateMachineList, error) {
	// actionStateMachines is an ordered map of taskIDs -> Action IDs -> Action state machine
	actionStateMachineList := make(ActionStateMachineList, 0)

	// each firmware install parameter results in an action
	for idx, firmware := range task.FirmwarePlanned {
		actionSM, err := NewActionStateMachine(ctx)
		if err != nil {
			return nil, err
		}

		action := model.Action{
			ID:     firmware.ComponentSlug + "-" + strconv.Itoa(idx),
			Status: string(stateQueued),
			// Firmware is populated by the task firmware resolve transition
			Firmware: task.FirmwarePlanned[idx],
		}

		m := map[string]map[string]*ActionStateMachine{
			task.ID.String(): map[string]*ActionStateMachine{
				action.ID: actionSM,
			},
		}

		actionStateMachineList = append(actionStateMachineList, m)
	}

	return actionStateMachineList, nil
}
