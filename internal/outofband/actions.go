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
	// when a action has transitioned into a state, that action is considered complete
	//
	// These states should be named in the format "state+verb past tense+subject"
	statePoweredOnDevice             sw.State = "poweredOnDevice"
	stateDownloadedFirmware          sw.State = "downloadedFirmware"
	stateInitiatedInstallFirmware    sw.State = "initiatedInstallFirmware"
	statePolledFirmwareInstallStatus sw.State = "polledFirmwareInstallStatus"
	stateResetBMC                    sw.State = "resetBMC"
	stateResetHost                   sw.State = "resetHost"
	statePoweredOffDevice            sw.State = "poweredOffDevice"

	//
	// The transition types var names should be in the format - transitionType + "state"
	// the values should be in the continuous present tense
	transitionTypePowerOnDevice             sw.TransitionType = "poweringOnDevice"
	transitionTypeDownloadFirmware          sw.TransitionType = "downloadingFirmware"
	transitionTypeInitiatingInstallFirmware sw.TransitionType = "initiatingInstallFirmware"
	transitionTypePollInstallStatus         sw.TransitionType = "pollingInstallStatus"
	transitionTypeResetBMC                  sw.TransitionType = "resettingBMC"
	transitionTypeResetHost                 sw.TransitionType = "resettingHost"
	transitionTypePowerOffDevice            sw.TransitionType = "poweringOffDevice"
)

func transitionOrder() []sw.TransitionType {
	return []sw.TransitionType{
		transitionTypePowerOnDevice,
		transitionTypeDownloadFirmware,
		transitionTypeInitiatingInstallFirmware,
		transitionTypePollInstallStatus,
		transitionTypeResetBMC,
		transitionTypeResetHost,
		transitionTypePowerOffDevice,
	}
}

func NewStateMachine(ctx context.Context, actionID string) (*sm.ActionStateMachine, error) {
	return sm.NewActionStateMachine(ctx, actionID, transitionOrder(), transitionRules())
}

func transitionRules() []sw.TransitionRule {
	handler := &actionHandler{}

	return []sw.TransitionRule{
		{
			TransitionType:   transitionTypePowerOnDevice,
			SourceStates:     sw.States{model.StateQueued},
			DestinationState: statePoweredOnDevice,

			// Condition for the transition, transition will be executed only if this function return true
			// Can be nil, in this case it's considered as return true, nil
			//
			// Note: theres no fall through if a condition fails
			//       and so this code does not use it.
			Condition: nil,

			// Transition is users business logic, should not set the state or return next state
			// If condition returns true this function will be executed
			Transition: handler.powerOnDevice,

			// PostTransition will be called if condition and transition are successful.
			PostTransition: handler.SaveState,
		},
		{
			TransitionType:   transitionTypeDownloadFirmware,
			SourceStates:     sw.States{statePoweredOnDevice},
			DestinationState: stateDownloadedFirmware,
			Condition:        nil,
			Transition:       handler.downloadFirmware,
			PostTransition:   handler.SaveState,
		},
		{
			TransitionType:   transitionTypeInitiatingInstallFirmware,
			SourceStates:     sw.States{stateDownloadedFirmware},
			DestinationState: stateInitiatedInstallFirmware, // poll is missing
			Condition:        nil,
			Transition:       handler.initiateInstallFirmware,
			PostTransition:   handler.SaveState,
		},
		{
			TransitionType:   transitionTypePollInstallStatus,
			SourceStates:     sw.States{stateInitiatedInstallFirmware},
			DestinationState: statePolledFirmwareInstallStatus,
			Condition:        nil,
			Transition:       handler.pollFirmwareInstallStatus,
			PostTransition:   handler.SaveState,
		},
		{
			TransitionType:   transitionTypeResetBMC,
			SourceStates:     sw.States{statePolledFirmwareInstallStatus},
			DestinationState: stateResetBMC,
			Condition:        nil,
			Transition:       handler.resetBMC,
			PostTransition:   handler.SaveState,
		},
		{
			TransitionType:   transitionTypeResetHost,
			SourceStates:     sw.States{stateResetBMC},
			DestinationState: stateResetHost,
			Condition:        nil,
			Transition:       handler.resetHost,
			PostTransition:   handler.SaveState,
		},
		{
			TransitionType:   transitionTypePowerOffDevice,
			SourceStates:     sw.States{stateResetHost},
			DestinationState: statePoweredOffDevice,
			Condition:        nil,
			Transition:       handler.powerOffDevice,
			PostTransition:   handler.SaveState,
		},
		// This transition is executed when the action completes successfully
		{
			TransitionType:   sm.TransitionTypeActionSuccess,
			SourceStates:     sw.States{statePoweredOffDevice},
			DestinationState: sm.StateActionSuccessful,
			Condition:        nil,
			Transition:       handler.installSuccessful,
			PostTransition:   handler.SaveState,
		},

		// This transition is executed when the transition fails.
		{
			TransitionType: sm.TransitionTypeActionFailed,
			SourceStates: sw.States{
				statePoweredOnDevice,
				stateDownloadedFirmware,
				stateInitiatedInstallFirmware,
				statePolledFirmwareInstallStatus,
				stateResetBMC,
				stateResetHost,
				statePoweredOffDevice,
			},
			DestinationState: sm.StateActionFailed,
			Condition:        nil,
			Transition:       handler.installFailed,
			PostTransition:   handler.SaveState,
		},
	}
}
