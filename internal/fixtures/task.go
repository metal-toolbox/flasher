package fixtures

import rctypes "github.com/metal-toolbox/rivets/condition"

var (
	TaskParametersA = rctypes.FirmwareInstallTaskParameters{
		ResetBMCBeforeInstall: false,
		ForceInstall:          false,
	}
)
