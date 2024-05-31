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

func (i *ActionHandler) ComposeAction(ctx context.Context, actionCtx *runner.ActionHandlerContext) (*model.Action, error) {
	steps := i.definitions()

	return &model.Action{
		InstallMethod: model.InstallMethodOutofband,
		Firmware:      *actionCtx.Firmware,
		ForceInstall:  actionCtx.Task.Parameters.ForceInstall,
		Steps:         steps,
		First:         actionCtx.First,
		Last:          actionCtx.Last,
	}, nil
}

func (o *ActionHandler) definitions() model.Steps {
	return model.Steps{
		{
			Name:        checkInstalledFirmware,
			Group:       PreInstall,
			Handler:     o.handler.checkCurrentFirmware,
			Description: "Check firmware currently installed on component",
		},
		{
			Name:        downloadFirmware,
			Group:       PreInstall,
			Handler:     o.handler.downloadFirmware,
			Description: "Download and verify firmware file checksum.",
		},

		//		{
		//			Name:        pollInstallStatus,
		//			Group:       Install,
		//			Handler:     o.handler.pollFirmwareTaskStatus,
		//			Description: "Poll BMC for firmware install status until its identified to be in a finalized state.",
		//		},
		//{
		//	Name:        pollUploadStatus,
		//	Group:       Install,
		//	Handler:     o.handler.pollFirmwareTaskStatus,
		//	Description: "Poll device with exponential backoff for firmware upload status until it's confirmed.",
		//},
	}
}
