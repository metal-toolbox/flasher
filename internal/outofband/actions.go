package outofband

import (
	sw "github.com/filanov/stateswitch"
	"github.com/metal-toolbox/flasher/internal/model"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
	"github.com/pkg/errors"

	bconsts "github.com/bmc-toolbox/bmclib/v2/constants"
)

const (
	// transition types implemented and defined further below
	powerOnDevice                 sw.TransitionType = "powerOnDevice"
	powerOffDevice                sw.TransitionType = "powerOffDevice"
	checkInstalledFirmware        sw.TransitionType = "checkInstalledFirmware"
	downloadFirmware              sw.TransitionType = "downloadFirmware"
	preInstallResetBMC            sw.TransitionType = "preInstallResetBMC"
	uploadFirmware                sw.TransitionType = "uploadFirmware"
	pollUploadStatus              sw.TransitionType = "pollUploadStatus"
	uploadFirmwareInitiateInstall sw.TransitionType = "uploadFirmwareInitiateInstall"
	installUploadedFirmware       sw.TransitionType = "installUploadedFirmware"
	pollInstallStatus             sw.TransitionType = "pollInstallStatus"
	postInstallResetBMC           sw.TransitionType = "postInstallResetBMC"
	resetDevice                   sw.TransitionType = "resetDevice"
)

// TransitionKind type groups firmware install transitions.
type TransitionKind string

const (
	PreInstall    TransitionKind = "PreInstall"
	Install       TransitionKind = "Install"
	PostInstall   TransitionKind = "PostInstall"
	PowerStateOff TransitionKind = "PowerStateOff"
	PowerStateOn  TransitionKind = "PowerStateOn"
)

// Transition is an internal type to hold all attributes required to build a stateswitch.TransitionRule.
type Transition struct {
	Name           sw.TransitionType
	DestState      sw.State
	Handler        sw.Transition
	Kind           TransitionKind
	PostTransition sw.Transition
	TransitionDoc  sw.TransitionRuleDoc
	DestStateDoc   sw.StateDoc
}

type Transitions []Transition

func (ts Transitions) ByName(name sw.TransitionType) (t Transition, err error) {
	errNotFound := errors.New("transition not found by Name")
	for _, t := range ts { // nolint:gocritic // we're fine with 128 bytes being copied
		if t.Name == name {
			return t, nil
		}
	}

	return t, errors.Wrap(errNotFound, string(name))
}

func (ts Transitions) ByKind(kind TransitionKind) (t []Transition, err error) {
	errNotFound := errors.New("transition not found by Kind")

	for _, elem := range ts { // nolint:gocritic // we're fine with 128 bytes being copied
		if elem.Kind == kind {
			t = append(t, elem)
		}
	}

	if len(t) == 0 {
		return t, errors.Wrap(errNotFound, string(kind))
	}

	return t, nil
}

func NewActionStateMachine(actionID string, steps []bconsts.FirmwareInstallStep) (*sm.ActionStateMachine, error) {
	// defined transitions
	defined := definitions()

	transitions, err := composeTransitions(defined, steps)
	if err != nil {
		return nil, err
	}

	tr := transitions.prepare()
	return sm.NewActionStateMachine(actionID, tr)
}

func composeTransitions(defined Transitions, installSteps []bconsts.FirmwareInstallStep) (Transitions, error) {
	// . errTransitionDef := errors.New("error in transition definition")
	var final Transitions

	// transition to power on host
	powerOnTransiton, err := defined.ByKind(PowerStateOn)
	if err != nil {
		return nil, err
	}

	// transitions for install
	installTransitions, err := convFirmwareInstallSteps(installSteps, defined)
	if err != nil {
		return nil, err
	}

	// transitions before install
	preInstallTransitions, err := defined.ByKind(PreInstall)
	if err != nil {
		return nil, err
	}

	// transitions post install
	postInstallTransitions, err := defined.ByKind(PostInstall)
	if err != nil {
		return nil, err
	}

	// populate transitions in order of execution

	// When the first install transition indicates the host must be powered off
	// exclude the initial power on host transition.
	if installTransitions[0].Kind != PowerStateOff {
		final = append(final, powerOnTransiton...)
	}

	final = append(final, preInstallTransitions...)
	final = append(final, installTransitions...)
	final = append(final, postInstallTransitions...)

	return final, nil
}

// maps bmclib firmware install steps to transitions
func convFirmwareInstallSteps(required []bconsts.FirmwareInstallStep, defined Transitions) (Transitions, error) {
	errUnsupported := errors.New("bmclib.FirmwareInstallStep constant not supported")
	errNoInstallTransitions := errors.New("no required install transitions")

	m := map[bconsts.FirmwareInstallStep]sw.TransitionType{
		bconsts.FirmwareInstallStepPowerOffHost:          powerOffDevice,
		bconsts.FirmwareInstallStepUpload:                uploadFirmware,
		bconsts.FirmwareInstallStepUploadInitiateInstall: uploadFirmwareInitiateInstall,
		bconsts.FirmwareInstallStepUploadStatus:          pollUploadStatus,
		bconsts.FirmwareInstallStepInstallUploaded:       installUploadedFirmware,
		bconsts.FirmwareInstallStepInstallStatus:         pollInstallStatus,
	}

	// items to be returned
	transitions := Transitions{}

	for _, s := range required {
		transitionName, exists := m[s]
		if !exists {
			return nil, errors.Wrap(errUnsupported, string(s))
		}

		t, err := defined.ByName(transitionName)
		if err != nil {
			return nil, err
		}

		transitions = append(transitions, t)
	}

	if len(transitions) == 0 {
		return nil, errNoInstallTransitions
	}

	return transitions, nil
}

// prepare returns sw.TransitionRule from the Transitions defined.
func (ts Transitions) prepare() []sw.TransitionRule {
	rules := []sw.TransitionRule{}

	for idx, t := range ts { // nolint:gocritic // we're fine with 128 bytes being copied
		tr := sw.TransitionRule{
			TransitionType:   t.Name,
			DestinationState: t.DestState,
			Transition:       t.Handler,
			PostTransition:   sw.PostTransition(t.PostTransition),
			Documentation:    t.TransitionDoc,
		}

		// transitions begin in the active state
		if idx == 0 {
			tr.SourceStates = sw.States{model.StateActive}
		} else {
			tr.SourceStates = sw.States{ts[idx-1].DestState}
		}

		rules = append(rules, tr)
	}

	return rules
}

func definitions() Transitions {
	handler := &actionHandler{}

	// Note: transitions are defined in order of execution
	return []Transition{
		{
			Name:           powerOnDevice, // rename powerOnDevice -> powerOnHost
			Kind:           PowerStateOn,
			DestState:      "devicePoweredOn",
			Handler:        handler.powerOnDevice,
			PostTransition: handler.publishStatus,
			TransitionDoc: sw.TransitionRuleDoc{
				Name:        "Power on device",
				Description: "Power on device - if its currently powered off.",
			},
			DestStateDoc: sw.StateDoc{
				Name:        "devicePoweredOn",
				Description: "This action state indicates the device has been (conditionally) powered on for a component firmware install.",
			},
		},
		{
			Name:           checkInstalledFirmware,
			Kind:           PreInstall,
			DestState:      "installedFirmwareChecked",
			Handler:        handler.checkCurrentFirmware,
			PostTransition: handler.publishStatus,
			TransitionDoc: sw.TransitionRuleDoc{
				Name:        "Check installed firmware",
				Description: "Check firmware installed on component",
			},
			DestStateDoc: sw.StateDoc{
				Name:        "installedFirmwareChecked",
				Description: "This action state indicates the installed firmware on the component has been checked.",
			},
		},
		{
			Name:           downloadFirmware,
			Kind:           PreInstall,
			DestState:      "firmwareDownloaded",
			Handler:        handler.downloadFirmware,
			PostTransition: handler.publishStatus,
			TransitionDoc: sw.TransitionRuleDoc{
				Name:        "Download and verify firmware",
				Description: "Download and verify firmware file checksum.",
			},
			DestStateDoc: sw.StateDoc{
				Name:        "firmwareDownloaded",
				Description: "This action state indicates the component firmware to be installed has been downloaded and verified.",
			},
		},
		{
			Name:           preInstallResetBMC,
			Kind:           PreInstall,
			DestState:      "preInstallBMCReset",
			Handler:        handler.resetBMC,
			PostTransition: handler.publishStatus,
			TransitionDoc: sw.TransitionRuleDoc{
				Name:        "Powercycle BMC before install",
				Description: "Powercycle BMC before installing any firmware as a precaution.",
			},
			DestStateDoc: sw.StateDoc{
				Name:        "preInstallBMCReset",
				Description: "This action state indicates the BMC has been power cycled as a pre-install step to make sure the BMC is in good health before proceeding.",
			},
		},
		{
			Name:           uploadFirmwareInitiateInstall,
			Kind:           Install,
			DestState:      "firmwareUploadedInstallInitiated",
			Handler:        handler.uploadFirmwareInitiateInstall,
			PostTransition: handler.publishStatus,
			TransitionDoc: sw.TransitionRuleDoc{
				Name:        "Initiate firmware install",
				Description: "Initiate firmware install for component.",
			},
			DestStateDoc: sw.StateDoc{
				Name:        "firmwareUploadedInstallInitiated",
				Description: "This action state indicates the component firmware has been uploaded to the target device for install, and the firmware install on the device has been initiated.",
			},
		},
		{
			Name:           installUploadedFirmware,
			Kind:           Install,
			DestState:      "installedUploadedFirmware",
			Handler:        handler.installUploadedFirmware,
			PostTransition: handler.publishStatus,
			TransitionDoc: sw.TransitionRuleDoc{
				Name:        "Initiate firmware install for firmware uploaded already uploaded",
				Description: "Initiate firmware install for firmware uploaded.",
			},
			DestStateDoc: sw.StateDoc{
				Name:        "installedUploadedFirmware",
				Description: "This action state indicates the install process was initiated for a firmware that was uploaded through uploadFirmware",
			},
		},
		{
			Name:           pollInstallStatus,
			Kind:           Install,
			DestState:      "firmwareInstallStatusPolled",
			Handler:        handler.pollFirmwareTaskStatus,
			PostTransition: handler.publishStatus,
			TransitionDoc: sw.TransitionRuleDoc{
				Name:        "Poll firmware install status",
				Description: "Poll BMC with exponential backoff for firmware install status until its in a finalized state (completed/powercyclehost/powercyclebmc/failed).",
			},
			DestStateDoc: sw.StateDoc{
				Name:        "firmwareInstallStatusPolled",
				Description: "This action state indicates the component firmware install status is in a finalized state (powerCycleDevice, powerCycleBMC, successful, failed).",
			},
		},
		{
			Name:           uploadFirmware,
			Kind:           Install,
			DestState:      "firmwareUploaded",
			Handler:        handler.uploadFirmware,
			PostTransition: handler.publishStatus,
			TransitionDoc: sw.TransitionRuleDoc{
				Name:        "Upload firmware",
				Description: "Upload firmware to the device.",
			},
			DestStateDoc: sw.StateDoc{
				Name:        "firmwareUploaded",
				Description: "This action state indicates the component firmware has been uploaded to the target device.",
			},
		},
		{
			Name:           pollUploadStatus,
			Kind:           Install,
			DestState:      "uploadFirmwareStatusPolled",
			Handler:        handler.pollFirmwareTaskStatus,
			PostTransition: handler.publishStatus,
			TransitionDoc: sw.TransitionRuleDoc{
				Name:        "Poll firmware upload status",
				Description: "Poll device with exponential backoff for firmware upload status until it's confirmed.",
			},
			DestStateDoc: sw.StateDoc{
				Name:        "uploadFirmwareStatusPolled",
				Description: "This action state indicates the component firmware upload status is confirmed.",
			},
		},
		{
			Name:           postInstallResetBMC,
			Kind:           PostInstall,
			DestState:      "postInstallBMCReset",
			Handler:        handler.resetBMC,
			PostTransition: handler.publishStatus,
			TransitionDoc: sw.TransitionRuleDoc{
				Name:        "Powercycle BMC",
				Description: "Powercycle BMC - only when pollFirmwareInstallStatus() identifies a BMC reset is required.",
			},
			DestStateDoc: sw.StateDoc{
				Name:        "postInstallBMCReset",
				Description: "This action state indicates the BMC has been power cycled as a post-install step to complete a component firmware install.",
			},
		},
		{
			Name:           resetDevice,
			Kind:           PostInstall,
			DestState:      "deviceReset", // rename to powerCycleHost
			Handler:        handler.resetDevice,
			PostTransition: handler.publishStatus,
			TransitionDoc: sw.TransitionRuleDoc{
				Name:        "Powercycle Device",
				Description: "Powercycle Device - only when pollFirmwareInstallStatus() identifies a Device power cycle is required.",
			},
			DestStateDoc: sw.StateDoc{
				Name:        "deviceReset",
				Description: "This action state indicates the Device has been (conditionally) power cycled to complete a component firmware install.",
			},
		},
		{
			Name:           powerOffDevice,
			Kind:           PowerStateOff,
			DestState:      "devicePoweredOff",
			Handler:        handler.powerOffDevice,
			PostTransition: handler.publishStatus,
			TransitionDoc: sw.TransitionRuleDoc{
				Name:        "Power off Device",
				Description: "Powercycle Device - only if this is the final firmware (action statemachine) to be installed and the device was powered off earlier.",
			},
			DestStateDoc: sw.StateDoc{
				Name:        "devicePoweredOff",
				Description: "This action state indicates the Device has been (conditionally) power off to complete a component firmware install.",
			},
		},
	}
}
