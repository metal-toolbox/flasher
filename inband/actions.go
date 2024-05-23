package inband

import "github.com/metal-toolbox/flasher/internal/model"

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

func (o *ActionHandler) definitions() model.Steps {
	return model.Steps{
		//{
		//	Name:        powerOffServer,
		//	Group:       PowerState,
		//	Handler:     o.handler.powerOffServer,
		//	Description: "Powercycle Device, if this is the final firmware to be installed and the device was powered off earlier.",
		//},
		{
			Name:        checkInstalledFirmware,
			Group:       PreInstall,
			Handler:     o.handler.checkCurrentFirmware,
			Description: "Check firmware currently installed on component",
		},
		//{
		//	Name:        downloadFirmware,
		//	Group:       PreInstall,
		//	Handler:     o.handler.downloadFirmware,
		//	Description: "Download and verify firmware file checksum.",
		//},

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
