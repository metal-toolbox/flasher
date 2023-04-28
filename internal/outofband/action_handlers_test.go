package outofband

import (
	"os"
	"testing"

	"github.com/metal-toolbox/flasher/internal/fixtures"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/stretchr/testify/assert"
)

func Test_pollFirmwareInstallStatus(t *testing.T) {
	testcases := []struct {
		name        string
		mockStatus  model.ComponentFirmwareInstallStatus
		expectError string
	}{
		{
			"too many failures, returns error",
			model.StatusInstallUnknown,
			"attempts querying FirmwareInstallStatus",
		},
		{
			"install requires a BMC power cycle",
			model.StatusInstallPowerCycleBMCRequired,
			"",
		},
		{
			"install requires a Host power cycle",
			model.StatusInstallPowerCycleHostRequired,
			"",
		},
		{
			"install state running exceeds max BMC query attempts",
			model.StatusInstallRunning,
			"reached maximum BMC query attempts",
		},
		{
			"install state failed returns error",
			model.StatusInstallFailed,
			ErrFirmwareInstallFailed.Error(),
		},
		{
			"install state complete returns",
			model.StatusInstallComplete,
			"",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			task := newTaskFixture(string(model.StateActive))
			asset := fixtures.Assets[fixtures.Asset1ID.String()]
			tctx := newtaskHandlerContextFixture(task, &asset)

			action := model.Action{
				ID:       "foobar",
				TaskID:   task.ID.String(),
				Firmware: *fixtures.NewFirmware()[0],
			}

			_ = action.SetState(model.StateActive)
			task.ActionsPlanned = append(task.ActionsPlanned, &action)

			// init handler
			handler := &actionHandler{}

			os.Setenv(envTesting, "1")
			defer os.Unsetenv(envTesting)

			os.Setenv(fixtures.EnvMockBMCFirmwareInstallStatus, string(tc.mockStatus))
			defer os.Unsetenv(fixtures.EnvMockBMCFirmwareInstallStatus)

			if err := handler.pollFirmwareInstallStatus(&action, tctx); err != nil {
				if tc.expectError != "" {
					assert.Contains(t, err.Error(), tc.expectError)
				} else {
					t.Fatal(err)
				}
			}

			// assert action fields are set when bmc/host power cycle is required.
			switch tc.mockStatus {
			case model.StatusInstallPowerCycleBMCRequired:
				assert.True(t, action.BMCPowerCycleRequired)
			case model.StatusInstallPowerCycleHostRequired:
				assert.True(t, action.HostPowerCycleRequired)
			}
		})
	}
}
