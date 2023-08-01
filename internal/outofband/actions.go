package outofband

import (
	sw "github.com/filanov/stateswitch"
	"github.com/metal-toolbox/flasher/internal/model"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
)

const (
	// To add a new transition handler here
	//
	// 1. Add an action state
	// 2. Add a transition type
	// 3. Include transition type in transitionOrder()
	// 4. Add transition rule, update previous and next transition rules - for destination states.
	// 5. Add transition handler method
	// 6. List transition type in TransitionTypeActionFailed

	// action states
	//
	// the SM transitions through these states for each component being updated.
	// when a action has transitioned into a state, that action is considered complete
	//
	// These states should be named in the format "state+verb past tense+subject"
	statePoweredOnDevice             sw.State = "poweredOnDevice"
	stateCheckedCurrentFirmware      sw.State = "checkedCurrentFirmware"
	stateDownloadedFirmware          sw.State = "downloadedFirmware"
	statePreInstallResetBMC          sw.State = "preInstallresetBMC"
	stateInitiatedInstallFirmware    sw.State = "initiatedInstallFirmware"
	statePolledFirmwareInstallStatus sw.State = "polledFirmwareInstallStatus"
	statePostInstallResetBMC         sw.State = "postInstallResetBMC"
	stateResetDevice                 sw.State = "resetDevice"
	statePoweredOffDevice            sw.State = "poweredOffDevice"

	// transition types
	//
	// The transition types var names should be in the format - transitionType + "state"
	// the values should be in the continuous present tense
	transitionTypePowerOnDevice             sw.TransitionType = "poweringOnDevice"
	transitionTypeCheckInstalledFirmware    sw.TransitionType = "checkingInstalledFirmware"
	transitionTypeDownloadFirmware          sw.TransitionType = "downloadingFirmware"
	transitionTypePreInstallResetBMC        sw.TransitionType = "preInstallResettingBMC"
	transitionTypeInitiatingInstallFirmware sw.TransitionType = "initiatingInstallFirmware"
	transitionTypePollInstallStatus         sw.TransitionType = "pollingInstallStatus"
	transitionTypePostInstallResetBMC       sw.TransitionType = "postInstallResettingBMC"
	transitionTypeResetDevice               sw.TransitionType = "resettingHost"
	transitionTypePowerOffDevice            sw.TransitionType = "poweringOffDevice"
)

// transitionOrder defines the order of transition execution
func transitionOrder() []sw.TransitionType {
	return []sw.TransitionType{
		transitionTypePowerOnDevice,
		transitionTypeCheckInstalledFirmware,
		transitionTypeDownloadFirmware,
		transitionTypePreInstallResetBMC,
		transitionTypeInitiatingInstallFirmware,
		transitionTypePollInstallStatus,
		transitionTypePostInstallResetBMC,
		transitionTypeResetDevice,
		transitionTypePowerOffDevice,
	}
}

func NewActionStateMachine(actionID string) (*sm.ActionStateMachine, error) {
	machine, err := sm.NewActionStateMachine(actionID, transitionOrder(), transitionRules())
	if err != nil {
		return nil, err
	}

	machine.AddStateTransitionDocumentation(actionDocumentation())

	return machine, nil
}

func actionDocumentation() ([]sw.StateDoc, []sw.TransitionTypeDoc) {
	return []sw.StateDoc{
			{
				Name:        string(statePoweredOnDevice),
				Description: "This action state indicates the device has been (conditionally) powered on for a component firmware install.",
			},
			{
				Name:        string(stateCheckedCurrentFirmware),
				Description: "This action state indicates the installed firmware on the component has been checked.",
			},
			{
				Name:        string(stateDownloadedFirmware),
				Description: "This action state indicates the component firmware to be installed has been downloaded and verified.",
			},
			{
				Name:        string(statePreInstallResetBMC),
				Description: "This action state indicates the BMC has been power cycled as a pre-install step to make sure the BMC is in good health before proceeding.",
			},

			{
				Name:        string(stateInitiatedInstallFirmware),
				Description: "This action state indicates the component firmware has been uploaded to the target device for install, and the firmware install on the device has been initiated.",
			},
			{
				Name:        string(statePolledFirmwareInstallStatus),
				Description: "This action state indicates the component firmware install status is in a finalized state (powerCycleDevice, powerCycleBMC, successful, failed).",
			},
			{
				Name:        string(statePostInstallResetBMC),
				Description: "This action state indicates the BMC has been power cycled as a post-install step to complete a component firmware install.",
			},
			{
				Name:        string(stateResetDevice),
				Description: "This action state indicates the Device has been (conditionally) power cycled to complete a component firmware install.",
			},
			{
				Name:        string(statePoweredOffDevice),
				Description: "This action state indicates the Device has been (conditionally) power off to complete a component firmware install.",
			},
		},
		[]sw.TransitionTypeDoc{
			{
				Name:        string(transitionTypePowerOnDevice),
				Description: "In this action transition the device is being powered on for a component firmware install - if it was powered-off.",
			},
			{
				Name:        string(transitionTypeCheckInstalledFirmware),
				Description: "In this action transition the installed component firmware is being checked.",
			},
			{
				Name:        string(transitionTypeDownloadFirmware),
				Description: "In this action transition the component firmware to be installed is being downloaded and verified.",
			},
			{
				Name:        string(transitionTypePreInstallResetBMC),
				Description: "In this action transition the BMC is power cycled before attempting to install any firmware.",
			},
			{
				Name:        string(transitionTypeInitiatingInstallFirmware),
				Description: "In this action transition the component firmware to be installed is being uploaded to the device and the component firmware install is being initated.",
			},
			{
				Name:        string(transitionTypePollInstallStatus),
				Description: "In this action transition the component firmware install status is being polled until its in a finalized state (powerCycleDevice, powerCycleBMC, successful, failed).",
			},
			{
				Name:        string(transitionTypePostInstallResetBMC),
				Description: "In this action transition the BMC is being power-cycled - if the component firmware install status requires a BMC reset to proceed/complete.",
			},
			{
				Name:        string(transitionTypeResetDevice),
				Description: "In this action transition the Device will be power-cycled if the component firmware install status requires a Device reset to proceed/complete.",
			},
			{
				Name:        string(transitionTypePowerOffDevice),
				Description: "In this action transition the Device will be powered-off if the device was powered off when task started.",
			},
		}
}

func transitionRules() []sw.TransitionRule {
	handler := &actionHandler{}

	return []sw.TransitionRule{
		{
			TransitionType:   transitionTypePowerOnDevice,
			SourceStates:     sw.States{model.StateActive},
			DestinationState: statePoweredOnDevice,

			// Condition for the transition, transition will be executed only if this function return true
			// Can be nil, in this case it's considered as return true, nil
			//
			// Note: theres no fall through if a condition fails and so this code does not define conditions.
			Condition: nil,

			// Transition is users business logic, should not set the state or return next state
			// If condition returns true this function will be executed
			Transition: handler.powerOnDevice,

			// PostTransition will be called if condition and transition are successful.
			PostTransition: handler.PublishStatus,
			Documentation: sw.TransitionRuleDoc{
				Name:        "Power on device",
				Description: "Power on device - if its currently powered off.",
			},
		},
		{
			TransitionType:   transitionTypeCheckInstalledFirmware,
			SourceStates:     sw.States{statePoweredOnDevice},
			DestinationState: stateCheckedCurrentFirmware,
			Condition:        nil,
			Transition:       handler.checkCurrentFirmware,
			PostTransition:   handler.PublishStatus,
			Documentation: sw.TransitionRuleDoc{
				Name:        "Check installed firmware",
				Description: "Check firmware installed on component",
			},
		},
		{
			TransitionType:   transitionTypeDownloadFirmware,
			SourceStates:     sw.States{stateCheckedCurrentFirmware},
			DestinationState: stateDownloadedFirmware,
			Condition:        nil,
			Transition:       handler.downloadFirmware,
			PostTransition:   handler.PublishStatus,
			Documentation: sw.TransitionRuleDoc{
				Name:        "Download and verify firmware",
				Description: "Download and verify firmware file checksum.",
			},
		},
		{
			TransitionType:   transitionTypePreInstallResetBMC,
			SourceStates:     sw.States{stateDownloadedFirmware},
			DestinationState: statePreInstallResetBMC,
			Condition:        nil,
			Transition:       handler.resetBMC,
			PostTransition:   handler.PublishStatus,
			Documentation: sw.TransitionRuleDoc{
				Name:        "Powercycle BMC before install",
				Description: "Powercycle BMC before installing any firmware as a precaution.",
			},
		},
		{
			TransitionType:   transitionTypeInitiatingInstallFirmware,
			SourceStates:     sw.States{statePreInstallResetBMC},
			DestinationState: stateInitiatedInstallFirmware,
			Condition:        nil,
			Transition:       handler.initiateInstallFirmware,
			PostTransition:   handler.PublishStatus,
			Documentation: sw.TransitionRuleDoc{
				Name:        "Initiate firmware install",
				Description: "Initiate firmware install for component.",
			},
		},
		{
			TransitionType:   transitionTypePollInstallStatus,
			SourceStates:     sw.States{stateInitiatedInstallFirmware},
			DestinationState: statePolledFirmwareInstallStatus,
			Condition:        nil,
			Transition:       handler.pollFirmwareInstallStatus,
			PostTransition:   handler.PublishStatus,
			Documentation: sw.TransitionRuleDoc{
				Name:        "Poll firmware install status",
				Description: "Poll BMC with exponential backoff for firmware install status until firmware install status is in a finalized state (completed/powercyclehost/powercyclebmc/failed).",
			},
		},
		{
			TransitionType:   transitionTypePostInstallResetBMC,
			SourceStates:     sw.States{statePolledFirmwareInstallStatus},
			DestinationState: statePostInstallResetBMC,
			Condition:        nil,
			Transition:       handler.resetBMC,
			PostTransition:   handler.PublishStatus,
			Documentation: sw.TransitionRuleDoc{
				Name:        "Powercycle BMC",
				Description: "Powercycle BMC - only when pollFirmwareInstallStatus() identifies a BMC reset is required.",
			},
		},
		{
			TransitionType:   transitionTypeResetDevice,
			SourceStates:     sw.States{statePostInstallResetBMC},
			DestinationState: stateResetDevice,
			Condition:        nil,
			Transition:       handler.resetDevice,
			PostTransition:   handler.PublishStatus,
			Documentation: sw.TransitionRuleDoc{
				Name:        "Powercycle Device",
				Description: "Powercycle Device - only when pollFirmwareInstallStatus() identifies a Device power cycle is required.",
			},
		},
		{
			TransitionType:   transitionTypePowerOffDevice,
			SourceStates:     sw.States{stateResetDevice},
			DestinationState: statePoweredOffDevice,
			Condition:        nil,
			Transition:       handler.powerOffDevice,
			PostTransition:   handler.PublishStatus,
			Documentation: sw.TransitionRuleDoc{
				Name:        "Power off Device",
				Description: "Powercycle Device - only if this is the final firmware (action statemachine) to be installed and the device was powered off earlier.",
			},
		},
		// This transition is executed when the action completes successfully
		{
			TransitionType:   sm.TransitionTypeActionSuccess,
			SourceStates:     sw.States{statePoweredOffDevice},
			DestinationState: sm.StateActionSuccessful,
			Condition:        nil,
			Transition:       handler.actionSuccessful,
			PostTransition:   handler.PublishStatus,
			Documentation: sw.TransitionRuleDoc{
				Name:        "Success",
				Description: "Firmware install on component completed successfully.",
			},
		},

		// This transition is executed when the transition fails.
		{
			TransitionType: sm.TransitionTypeActionFailed,
			SourceStates: sw.States{
				model.StatePending,
				model.StateActive,
				statePoweredOnDevice,
				stateCheckedCurrentFirmware,
				stateDownloadedFirmware,
				statePreInstallResetBMC,
				stateInitiatedInstallFirmware,
				statePolledFirmwareInstallStatus,
				statePostInstallResetBMC,
				stateResetDevice,
				statePoweredOffDevice,
			},
			DestinationState: sm.StateActionFailed,
			Condition:        nil,
			Transition:       handler.actionFailed,
			PostTransition:   handler.PublishStatus,
			Documentation: sw.TransitionRuleDoc{
				Name:        "Failed",
				Description: "Firmware install on component failed.",
			},
		},
	}
}
