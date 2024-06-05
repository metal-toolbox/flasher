package inband

import (
	"context"

	"github.com/metal-toolbox/flasher/internal/device"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/runner"
	"github.com/pkg/errors"

	imodel "github.com/metal-toolbox/ironlib/model"
)

var (
	errInstallReqQuery = errors.New("error returned when querying firmware install requirements")
	errCompose         = errors.New("error in composing steps for firmware install")
)

const (
	// transition types implemented and defined further below
	powerOffServer         model.StepName = "powerOffServer"
	powerCycleServer       model.StepName = "powerCycleServer"
	checkInstalledFirmware model.StepName = "checkInstalledFirmware"
	downloadFirmware       model.StepName = "downloadFirmware"
	installFirmware        model.StepName = "installFirmware"
	pollInstallStatus      model.StepName = "pollInstallStatus"
)

const (
	PreInstall  model.StepGroup = "PreInstall"
	PostInstall model.StepGroup = "PostInstall"
	Install     model.StepGroup = "Install"
	PowerState  model.StepGroup = "PowerState"
)

type ActionHandler struct {
	handler *handler
}

func (i *ActionHandler) ComposeAction(ctx context.Context, actionCtx *runner.ActionHandlerContext) (*model.Action, error) {
	var deviceQueryor device.InbandQueryor
	if actionCtx.DeviceQueryor == nil {
		deviceQueryor = NewDeviceQueryor(actionCtx.Logger)
	} else {
		deviceQueryor = actionCtx.DeviceQueryor.(device.InbandQueryor)
	}

	i.handler = initHandler(actionCtx, deviceQueryor)

	required, err := deviceQueryor.FirmwareInstallRequirements(
		ctx,
		actionCtx.Firmware.Component,
		actionCtx.Firmware.Vendor,
		actionCtx.Firmware.Models[0], // TODO; figure how we want to deal with multiple models in the list
	)
	if err != nil {
		return nil, errors.Wrap(errInstallReqQuery, err.Error())
	}

	if required == nil {
		return nil, errors.Wrap(errInstallReqQuery, "nil object returned")
	}

	steps, err := i.composeSteps(required)
	if err != nil {
		return nil, errors.Wrap(errCompose, err.Error())
	}

	action := &model.Action{
		InstallMethod: model.InstallMethodInband,
		Firmware:      *actionCtx.Firmware,
		ForceInstall:  actionCtx.Task.Parameters.ForceInstall,
		Steps:         steps,
		First:         actionCtx.First,
		Last:          actionCtx.Last,
	}

	i.handler.action = action

	return action, nil
}

func initHandler(actionCtx *runner.ActionHandlerContext, queryor device.InbandQueryor) *handler {
	return &handler{
		task:          actionCtx.Task,
		firmware:      actionCtx.Firmware,
		publisher:     actionCtx.Publisher,
		logger:        actionCtx.Logger,
		deviceQueryor: queryor,
	}
}

func (i *ActionHandler) composeSteps(required *imodel.UpdateRequirements) (model.Steps, error) {
	var final model.Steps

	// pre-install steps
	preinstall, err := i.definitions().ByGroup(PreInstall)
	if err != nil {
		return nil, err
	}

	final = append(final, preinstall...)

	// install steps
	install, err := i.definitions().ByGroup(Install)
	if err != nil {
		return nil, err
	}

	final = append(final, install...)

	if required.PostInstallHostPowercycle {
		powerCycle, errDef := i.definitions().ByName(powerCycleServer)
		if errDef != nil {
			return nil, err
		}

		final = append(final, &powerCycle)
	}

	postinstall, err := i.definitions().ByGroup(PostInstall)
	if err != nil {
		return nil, err
	}

	final = append(final, postinstall...)

	// TODO: add a validation for step state since that is required by the runner

	//	powerOff, err := i.definitions().ByName(powerOffServer)
	//	if err != nil {
	//		return nil, err
	//	}
	//
	//	final = append(final, &powerOff)
	return final, nil
}

func (i *ActionHandler) definitions() model.Steps {
	return model.Steps{
		{
			Name:        checkInstalledFirmware,
			Group:       PreInstall,
			Handler:     i.handler.checkCurrentFirmware,
			Description: "Check firmware currently installed on component",
			State:       model.StatePending,
		},
		{
			Name:        downloadFirmware,
			Group:       PreInstall,
			Handler:     i.handler.downloadFirmware,
			Description: "Download and verify firmware file checksum.",
			State:       model.StatePending,
		},
		{
			Name:        installFirmware,
			Group:       Install,
			Handler:     i.handler.installFirmware,
			Description: "Install firmware.",
			State:       model.StatePending,
		},
		{
			Name:        powerCycleServer,
			Group:       PowerState,
			Handler:     i.handler.powerCycleServer,
			Description: "Turn the computer off and on again.",
			State:       model.StatePending,
		},
		{
			Name:        checkInstalledFirmware,
			Group:       PostInstall,
			Handler:     i.handler.checkCurrentFirmware,
			Description: "Check firmware currently installed on components",
			State:       model.StatePending,
		},
	}
}
