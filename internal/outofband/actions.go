package outofband

import (
	"context"

	"github.com/metal-toolbox/flasher/internal/device"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/runner"
	"github.com/pkg/errors"

	bconsts "github.com/bmc-toolbox/bmclib/v2/constants"
)

const (
	// transition types implemented and defined further below
	powerOnServer                 model.StepName = "powerOnServer"
	powerOffServer                model.StepName = "powerOffServer"
	checkInstalledFirmware        model.StepName = "checkInstalledFirmware"
	downloadFirmware              model.StepName = "downloadFirmware"
	preInstallResetBMC            model.StepName = "preInstallResetBMC"
	uploadFirmware                model.StepName = "uploadFirmware"
	pollUploadStatus              model.StepName = "pollUploadStatus"
	uploadFirmwareInitiateInstall model.StepName = "uploadFirmwareInitiateInstall"
	installUploadedFirmware       model.StepName = "installUploadedFirmware"
	pollInstallStatus             model.StepName = "pollInstallStatus"
)

const (
	PreInstall model.StepGroup = "PreInstall"
	Install    model.StepGroup = "Install"
	PowerState model.StepGroup = "PowerState"
)

var (
	errInstallStepsQuery = errors.New("error returned when querying firmware install steps")
	errNoInstallSteps    = errors.New("no firmware install steps identified")
	errCompose           = errors.New("error in composing steps for firmware install")
)

type ActionHandler struct {
	handler *handler
}

func initHandler(actionCtx *runner.ActionHandlerContext, queryor device.OutofbandQueryor) *handler {
	return &handler{
		task:          actionCtx.Task,
		firmware:      actionCtx.Firmware,
		publisher:     actionCtx.Publisher,
		logger:        actionCtx.Logger,
		deviceQueryor: queryor,
	}
}

func (o *ActionHandler) ComposeAction(ctx context.Context, actionCtx *runner.ActionHandlerContext) (*model.Action, error) {
	var deviceQueryor device.OutofbandQueryor
	if actionCtx.DeviceQueryor == nil {
		deviceQueryor = NewDeviceQueryor(ctx, actionCtx.Task.Asset, actionCtx.Logger)
	} else {
		deviceQueryor = actionCtx.DeviceQueryor.(device.OutofbandQueryor)
	}

	o.handler = initHandler(actionCtx, deviceQueryor)

	required, err := deviceQueryor.FirmwareInstallSteps(ctx, actionCtx.Firmware.Component)
	if err != nil {
		return nil, errors.Wrap(errInstallStepsQuery, err.Error())
	}

	if len(required) == 0 {
		return nil, errNoInstallSteps
	}

	// first action and a BMC reset is required before install
	bmcResetBeforeInstall := actionCtx.First && actionCtx.Task.Parameters.ResetBMCBeforeInstall

	bmcResetOnInstallFailure, bmcResetPostInstall := bmcResetParams(required)

	steps, err := o.composeSteps(required, bmcResetBeforeInstall)
	if err != nil {
		return nil, errors.Wrap(errCompose, err.Error())
	}

	action := &model.Action{
		InstallMethod:            model.InstallMethodOutofband,
		Firmware:                 *actionCtx.Firmware,
		BMCResetPreInstall:       bmcResetBeforeInstall,
		BMCResetPostInstall:      bmcResetPostInstall,
		BMCResetOnInstallFailure: bmcResetOnInstallFailure,
		HostPowerOffPreInstall:   hostPowerOffRequired(required),
		ForceInstall:             actionCtx.Task.Parameters.ForceInstall,
		Steps:                    steps,
		First:                    actionCtx.First,
		Last:                     actionCtx.Last,
	}

	o.handler.action = action

	return action, nil
}

func (o *ActionHandler) composeSteps(required []bconsts.FirmwareInstallStep, preInstallBMCReset bool) (model.Steps, error) {
	var final model.Steps

	// pre-install steps
	preInstallSteps, err := o.definitions().ByGroup(PreInstall)
	if err != nil {
		return nil, err
	}

	// install steps
	installSteps, err := o.convFirmwareInstallSteps(required)
	if err != nil {
		return nil, err
	}

	// skip bmc reset transition before install based on parameter
	if !preInstallBMCReset {
		preInstallSteps = preInstallSteps.Remove(preInstallResetBMC)
	}

	// populate steps in order of execution
	//
	// Power on server unless it explicitly requires a power off as the first step
	if installSteps[0].Name != powerOffServer && installSteps[0].Name != powerOnServer {
		powerOnServerStep, err := o.definitions().ByName(powerOnServer)
		if err != nil {
			return nil, err
		}

		final = append(final, &powerOnServerStep)
	}

	final = append(final, preInstallSteps...)
	final = append(final, installSteps...)

	return final, nil
}

// maps bmclib firmware install steps to transitions
func (o *ActionHandler) convFirmwareInstallSteps(required []bconsts.FirmwareInstallStep) (model.Steps, error) {
	errUnsupported := errors.New("bmclib.FirmwareInstallStep constant not supported")

	m := map[bconsts.FirmwareInstallStep]model.StepName{
		bconsts.FirmwareInstallStepPowerOffHost:          powerOffServer,
		bconsts.FirmwareInstallStepUpload:                uploadFirmware,
		bconsts.FirmwareInstallStepUploadInitiateInstall: uploadFirmwareInitiateInstall,
		bconsts.FirmwareInstallStepUploadStatus:          pollUploadStatus,
		bconsts.FirmwareInstallStepInstallUploaded:       installUploadedFirmware,
		bconsts.FirmwareInstallStepInstallStatus:         pollInstallStatus,
	}

	// items to be returned
	final := model.Steps{}

	for _, s := range required {
		// TODO: turn FirmwareInstalSteps into FirmwareInstallProperties with fields for these non step parameters
		if s == bconsts.FirmwareInstallStepResetBMCOnInstallFailure ||
			s == bconsts.FirmwareInstallStepResetBMCPostInstall {
			continue
		}

		stepType, exists := m[s]
		if !exists {
			return nil, errors.Wrap(errUnsupported, string(s))
		}

		step, err := o.definitions().ByName(stepType)
		if err != nil {
			return nil, err
		}

		final = append(final, &step)
	}

	if len(final) == 0 {
		return nil, errNoInstallSteps
	}

	return final, nil
}

func (o *ActionHandler) definitions() model.Steps {
	return model.Steps{
		{
			Name:        powerOnServer,
			Group:       PowerState,
			Handler:     o.handler.powerOnServer,
			Description: "Power on server - if its currently powered off.",
			State:       model.StatePending,
		},
		{
			Name:        powerOffServer,
			Group:       PowerState,
			Handler:     o.handler.powerOffServer,
			Description: "Powercycle Device, if this is the final firmware to be installed and the device was powered off earlier.",
			State:       model.StatePending,
		},
		{
			Name:        checkInstalledFirmware,
			Group:       PreInstall,
			Handler:     o.handler.checkCurrentFirmware,
			Description: "Check firmware currently installed on component",
			State:       model.StatePending,
		},
		{
			Name:        downloadFirmware,
			Group:       PreInstall,
			Handler:     o.handler.downloadFirmware,
			Description: "Download and verify firmware file checksum.",
			State:       model.StatePending,
		},
		{
			Name:        preInstallResetBMC,
			Group:       PreInstall,
			Handler:     o.handler.resetBMC,
			Description: "Powercycle BMC before installing any firmware - for better chances of success.",
			State:       model.StatePending,
		},
		{
			Name:        uploadFirmwareInitiateInstall,
			Group:       Install,
			Handler:     o.handler.uploadFirmwareInitiateInstall,
			Description: "Initiate firmware install for component.",
			State:       model.StatePending,
		},
		{
			Name:        installUploadedFirmware,
			Group:       Install,
			Handler:     o.handler.installUploadedFirmware,
			Description: "Initiate firmware install for firmware uploaded.",
			State:       model.StatePending,
		},
		{
			Name:        pollInstallStatus,
			Group:       Install,
			Handler:     o.handler.pollFirmwareTaskStatus,
			Description: "Poll BMC for firmware install status until its identified to be in a finalized state.",
		},
		{
			Name:        uploadFirmware,
			Group:       Install,
			Handler:     o.handler.uploadFirmware,
			Description: "Upload firmware to the device.",
			State:       model.StatePending,
		},
		{
			Name:        pollUploadStatus,
			Group:       Install,
			Handler:     o.handler.pollFirmwareTaskStatus,
			Description: "Poll device with exponential backoff for firmware upload status until it's confirmed.",
			State:       model.StatePending,
		},
	}
}
