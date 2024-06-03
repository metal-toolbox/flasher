package inband

import (
	"context"

	"github.com/metal-toolbox/flasher/internal/device"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/runner"
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
	PreInstall model.StepGroup = "PreInstall"
	Install    model.StepGroup = "Install"
	PowerState model.StepGroup = "PowerState"
)

type ActionHandler struct {
	handler *handler
}

func (i *ActionHandler) ComposeAction(_ context.Context, actionCtx *runner.ActionHandlerContext) (*model.Action, error) {
	var deviceQueryor device.InbandQueryor
	if actionCtx.DeviceQueryor == nil {
		deviceQueryor = NewDeviceQueryor(actionCtx.Logger)
	} else {
		deviceQueryor = actionCtx.DeviceQueryor.(device.InbandQueryor)
	}

	i.handler = initHandler(actionCtx, deviceQueryor)

	action := &model.Action{
		InstallMethod: model.InstallMethodInband,
		Firmware:      *actionCtx.Firmware,
		ForceInstall:  actionCtx.Task.Parameters.ForceInstall,
		Steps:         i.definitions(),
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
	}
}
