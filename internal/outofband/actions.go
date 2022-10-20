package outofband

import (
	"context"

	sw "github.com/filanov/stateswitch"
	"github.com/metal-toolbox/flasher/internal/model"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
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

	transitionTypeLoginBMC        sw.TransitionType = "logginBMC"
	transitionTypeInstallFirmware sw.TransitionType = "installingFirmware"
	transitionTypeUploadFirmware  sw.TransitionType = "uploadingFirmware"
	transitionTypeResetBMC        sw.TransitionType = "resettingBMC"
	transitionTypeResetHost       sw.TransitionType = "resettingHost"

	// state, transition for FailedState actions
	stateInstallFailed sw.State = "installFailed"
)

func NewActionStateMachines(ctx context.Context, actionID string) (*sm.ActionStateMachine, error) {
	transitions := []sw.TransitionType{
		transitionTypeLoginBMC,
		transitionTypeUploadFirmware,
		transitionTypeInstallFirmware,
		transitionTypeResetBMC,
		transitionTypeResetHost,
	}

	handler := &actionHandler{}

	// The SM has transition rules define the transitionHandler methods
	// each transitionHandler method is passed as values to the transition rule.
	transitionsRules := []sw.TransitionRule{
		{
			TransitionType:   transitionTypeLoginBMC,
			SourceStates:     sw.States{model.StateQueued},
			DestinationState: stateLoginBMC,

			// Condition for the transition, transition will be executed only if this function return true
			// Can be nil, in this case it's considered as return true, nil
			Condition: nil,

			// Transition is users business logic, should not set the state or return next state
			// If condition returns true this function will be executed
			Transition: handler.loginBMC,

			// PostTransition will be called if condition and transition are successful.
			PostTransition: handler.SaveState,
		},
		{
			TransitionType:        transitionTypeUploadFirmware,
			SourceStates:          sw.States{stateLoginBMC},
			DestinationState:      stateUploadFirmware,
			Condition:             handler.conditionInstallFirmware,
			Transition:            handler.uploadFirmware,
			PostTransitionFailure: handler.SaveState,
			PostTransition:        handler.SaveState,
		},
		{
			TransitionType:   transitionTypeInstallFirmware,
			SourceStates:     sw.States{stateUploadFirmware},
			DestinationState: stateInstallFirmware,
			Condition:        nil,
			Transition:       handler.installFirmware,
			PostTransition:   handler.SaveState,
		},
		{
			TransitionType:   transitionTypeResetBMC,
			SourceStates:     sw.States{stateInstallFirmware},
			DestinationState: stateResetHost,
			Condition:        handler.conditionalResetBMC,
			Transition:       handler.resetBMC,
			PostTransition:   handler.SaveState,
		},
		{
			TransitionType:   transitionTypeResetHost,
			SourceStates:     sw.States{stateResetHost},
			DestinationState: model.StateSuccess,
			Condition:        handler.conditionalResetHost,
			Transition:       handler.resetHost,
			PostTransition:   handler.SaveState,
		},
		{
			TransitionType: sm.TransitionTypeActionFailed,
			SourceStates: sw.States{
				stateLoginBMC,
				stateUploadFirmware,
				stateInstallFirmware,
				stateResetBMC,
				stateResetHost,
			},
			DestinationState: stateInstallFailed,
			Condition:        nil,
			Transition:       handler.SaveState,
			PostTransition:   nil,
		},
	}

	return sm.NewActionStateMachine(ctx, actionID, transitions, transitionsRules)
}
