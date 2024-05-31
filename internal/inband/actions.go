package inband

import (
	"context"

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
	return &model.Action{
		InstallMethod: model.InstallMethodOutofband,
		Firmware:      *actionCtx.Firmware,
		ForceInstall:  actionCtx.Task.Parameters.ForceInstall,
		Steps:         i.definitions(),
		First:         actionCtx.First,
		Last:          actionCtx.Last,
	}, nil
}

func (i *ActionHandler) definitions() model.Steps {
	return model.Steps{
		{
			Name:        checkInstalledFirmware,
			Group:       PreInstall,
			Handler:     i.handler.checkCurrentFirmware,
			Description: "Check firmware currently installed on component",
		},
		{
			Name:        downloadFirmware,
			Group:       PreInstall,
			Handler:     i.handler.downloadFirmware,
			Description: "Download and verify firmware file checksum.",
		},
		{
			Name:        installFirmware,
			Group:       Install,
			Handler:     i.handler.installFirmware,
			Description: "Install firmware.",
		},
	}
}
