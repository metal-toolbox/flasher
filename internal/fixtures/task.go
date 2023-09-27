package fixtures

import rctypes "github.com/metal-toolbox/rivets/condition"

var (
	TaskParametersA = rctypes.FirmwareInstallTaskParameters{
		Priority:              0,
		ResetBMCBeforeInstall: false,
		ForceInstall:          false,
	}
)
