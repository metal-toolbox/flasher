package fixtures

import "github.com/metal-toolbox/flasher/internal/model"

var (
	TaskParametersA = model.TaskParameters{
		Priority:              0,
		InstallMethod:         model.InstallMethodOutofband,
		ResetBMCBeforeInstall: false,
		ForceInstall:          false,
	}
)
