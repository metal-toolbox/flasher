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
	//
	// These states should be named in the format "verb+subject"
	statePowerOnDevice     sw.State = "powerOnDevice"
	stateDownloadFirmware  sw.State = "downloadFirmware"
	stateInstallFirmware   sw.State = "installFirmware"
	statePollInstallStatus sw.State = "pollInstallStatus"
	stateResetBMC          sw.State = "resetBMC"
	stateResetHost         sw.State = "resetHost"
	statePowerOffDevice    sw.State = "powerOffDevice"

	//
	// The transition types var names should be in the format - transitionType + "state"
	// the values should be in the continuous present tense
	transitionTypePowerOnDevice     sw.TransitionType = "poweringOnDevice"
	transitionTypeDownloadFirmware  sw.TransitionType = "downloadingFirmware"
	transitionTypeInstallFirmware   sw.TransitionType = "installingFirmware"
	transitionTypePollInstallStatus sw.TransitionType = "pollingInstallStatus"
	transitionTypeResetBMC          sw.TransitionType = "resettingBMC"
	transitionTypeResetHost         sw.TransitionType = "resettingHost"
	transitionTypePowerOffDevice    sw.TransitionType = "poweringOffDevice"

	// state, transition for FailedState actions
	stateInstallFailed sw.State = "installFailed"
)

func NewActionStateMachine(ctx context.Context, actionID string) (*sm.ActionStateMachine, error) {
	transitions := []sw.TransitionType{
		transitionTypePowerOnDevice,
		transitionTypeDownloadFirmware,
		transitionTypeInstallFirmware,
		transitionTypeResetBMC,
		transitionTypeResetHost,
		transitionTypePowerOffDevice,
	}

	handler := &actionHandler{}

	// The SM has transition rules define the transitionHandler methods
	// each transitionHandler method is passed as values to the transition rule.
	transitionsRules := []sw.TransitionRule{
		{
			TransitionType:   transitionTypePowerOnDevice,
			SourceStates:     sw.States{model.StateQueued},
			DestinationState: stateDownloadFirmware,

			// Condition for the transition, transition will be executed only if this function return true
			// Can be nil, in this case it's considered as return true, nil
			Condition: handler.conditionPowerOnDevice,

			// Transition is users business logic, should not set the state or return next state
			// If condition returns true this function will be executed
			Transition: handler.powerOnDevice,

			// PostTransition will be called if condition and transition are successful.
			PostTransition: handler.SaveState,
		},
		{
			TransitionType:        transitionTypeDownloadFirmware,
			SourceStates:          sw.States{statePowerOnDevice},
			DestinationState:      stateDownloadFirmware,
			Condition:             handler.conditionInstallFirmware,
			Transition:            handler.downloadFirmware,
			PostTransitionFailure: handler.SaveState,
			PostTransition:        handler.SaveState,
		},
		{
			TransitionType:   transitionTypeInstallFirmware,
			SourceStates:     sw.States{stateDownloadFirmware},
			DestinationState: stateInstallFirmware,
			Condition:        nil,
			Transition:       handler.installFirmware,
			PostTransition:   handler.SaveState,
		},
		{
			TransitionType:   transitionTypePollInstallStatus,
			SourceStates:     sw.States{stateInstallFirmware},
			DestinationState: stateResetBMC,
			Condition:        nil,
			Transition:       handler.pollInstallStatus,
			PostTransition:   handler.SaveState,
		},
		{
			TransitionType:   transitionTypeResetBMC,
			SourceStates:     sw.States{statePollInstallStatus},
			DestinationState: stateResetHost,
			Condition:        handler.conditionResetBMC,
			Transition:       handler.resetBMC,
			PostTransition:   handler.SaveState,
		},
		{
			TransitionType:   transitionTypeResetHost,
			SourceStates:     sw.States{stateResetBMC},
			DestinationState: statePowerOffDevice,
			Condition:        handler.conditionResetHost,
			Transition:       handler.resetHost,
			PostTransition:   handler.SaveState,
		},
		{
			TransitionType:   transitionTypePowerOffDevice,
			SourceStates:     sw.States{stateResetHost},
			DestinationState: model.StateSuccess,
			Condition:        handler.conditionPowerOffDevice,
			Transition:       handler.powerOffDevice,
			PostTransition:   handler.SaveState,
		},
		{
			TransitionType: sm.TransitionTypeActionFailed,
			SourceStates: sw.States{
				statePowerOnDevice,
				stateDownloadFirmware,
				stateInstallFirmware,
				stateResetBMC,
				stateResetHost,
				statePowerOffDevice,
			},
			DestinationState: stateInstallFailed,
			Condition:        nil,
			Transition:       handler.SaveState,
			PostTransition:   nil,
		},
	}

	return sm.NewActionStateMachine(ctx, actionID, transitions, transitionsRules)
}
