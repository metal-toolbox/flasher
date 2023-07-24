package outofband

import (
	"context"
	"os"
	"testing"

	"github.com/bmc-toolbox/common"
	sw "github.com/filanov/stateswitch"
	"github.com/golang/mock/gomock"
	"github.com/metal-toolbox/flasher/internal/fixtures"
	"github.com/metal-toolbox/flasher/internal/model"
	modeltest "github.com/metal-toolbox/flasher/internal/model/test"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeStateSwitch struct{}

func (_ fakeStateSwitch) State() sw.State {
	return sw.State("bogus")
}
func (_ fakeStateSwitch) SetState(sw.State) error {
	return nil
}

func Test_checkCurrentFirmware(t *testing.T) {
	t.Parallel()
	hPtr := &actionHandler{}
	t.Run("type-check error", func(t *testing.T) {
		t.Parallel()
		ctx := &sm.HandlerContext{}
		fss := &fakeStateSwitch{}
		err := hPtr.checkCurrentFirmware(fss, ctx)
		require.Error(t, err)
		require.ErrorIs(t, err, sm.ErrActionTypeAssertion)
	})
	t.Run("inventory error", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		dq := modeltest.NewMockDeviceQueryor(ctrl)
		ctx := &sm.HandlerContext{
			Ctx:           context.Background(),
			Logger:        logrus.NewEntry(&logrus.Logger{}),
			DeviceQueryor: dq,
		}
		act := &model.Action{
			VerifyCurrentFirmware: true,
			Firmware: model.Firmware{
				Component: "the-component",
			},
		}
		dq.EXPECT().Inventory(gomock.Any()).Times(1).Return(nil, errors.New("pound sand"))
		err := hPtr.checkCurrentFirmware(act, ctx)
		require.Error(t, err)
		require.Equal(t, "pound sand", err.Error())
	})
	t.Run("nil inventory", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		dq := modeltest.NewMockDeviceQueryor(ctrl)
		ctx := &sm.HandlerContext{
			Ctx:           context.Background(),
			Logger:        logrus.NewEntry(&logrus.Logger{}),
			DeviceQueryor: dq,
		}
		act := &model.Action{
			VerifyCurrentFirmware: true,
			Firmware: model.Firmware{
				Component: "the-component",
			},
		}
		dq.EXPECT().Inventory(gomock.Any()).Times(1).Return(nil, nil)
		err := hPtr.checkCurrentFirmware(act, ctx)
		require.Error(t, err)
		require.ErrorIs(t, err, model.ErrComponentConverter)
	})
	t.Run("bad component slug", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		dq := modeltest.NewMockDeviceQueryor(ctrl)
		ctx := &sm.HandlerContext{
			Ctx:           context.Background(),
			Logger:        logrus.NewEntry(&logrus.Logger{}),
			DeviceQueryor: dq,
		}
		act := &model.Action{
			VerifyCurrentFirmware: true,
			Firmware: model.Firmware{
				Component: "cool-bios",
				Vendor:    "Dell-icious",
				Models: []string{
					"PowerEdge R6515",
				},
			},
		}
		dev := common.NewDevice()
		dev.Model = "PowerEdge R6515"
		dev.Vendor = "Dell-icious"
		dev.BIOS.Vendor = "Dell-icious"
		dev.BIOS.Model = "Bios Model"
		dq.EXPECT().Inventory(gomock.Any()).Times(1).Return(&dev, nil)
		err := hPtr.checkCurrentFirmware(act, ctx)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrComponentNotFound)
		t.Logf("error: %s\n", err.Error())
	})
	t.Run("blank installed version", func(t *testing.T) {
		t.Parallel()
		conversionDebug := false
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		dq := modeltest.NewMockDeviceQueryor(ctrl)
		ctx := &sm.HandlerContext{
			Ctx:           context.Background(),
			Logger:        logrus.NewEntry(&logrus.Logger{}),
			DeviceQueryor: dq,
		}
		act := &model.Action{
			VerifyCurrentFirmware: true,
			Firmware: model.Firmware{
				Component: "bios",
				Vendor:    "dell",
				Models: []string{
					"r6515",
				},
			},
		}
		dev := common.NewDevice()
		dev.Model = "PowerEdge R6515"
		dev.Vendor = "Dell-icious" // NB: This and the bogus BIOS vendor are ignored.
		dev.BIOS.Vendor = "Dell-icious"
		dev.BIOS.Model = "r6515"
		dev.BIOS.Serial = "12345"

		if conversionDebug {
			exp, cErr := model.NewComponentConverter().CommonDeviceToComponents(&dev)
			require.NoError(t, cErr)
			t.Logf("expected components length: %d\n", len(exp))
			for i, c := range exp {
				t.Logf("component %d => %#v\n", i, c)
			}
		}

		dq.EXPECT().Inventory(gomock.Any()).Times(1).Return(&dev, nil)
		err := hPtr.checkCurrentFirmware(act, ctx)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrInstalledVersionUnknown)
	})
	t.Run("equal installed version", func(t *testing.T) {
		t.Parallel()
		conversionDebug := false
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		dq := modeltest.NewMockDeviceQueryor(ctrl)
		ctx := &sm.HandlerContext{
			Ctx:           context.Background(),
			Logger:        logrus.NewEntry(&logrus.Logger{}),
			DeviceQueryor: dq,
		}
		act := &model.Action{
			TaskID:                "my-task-uuid",
			VerifyCurrentFirmware: true,
			Firmware: model.Firmware{
				Component: "bios",
				Vendor:    "dell",
				Models: []string{
					"r6515",
				},
				Version: "the version",
			},
		}
		dev := common.NewDevice()
		dev.Model = "PowerEdge R6515"
		dev.Vendor = "Dell-icious" // NB: This and the bogus BIOS vendor are ignored.
		dev.BIOS.Vendor = "Dell-icious"
		dev.BIOS.Model = "r6515"
		dev.BIOS.Serial = "12345"
		dev.BIOS.Firmware = &common.Firmware{
			Installed: "the version",
		}

		if conversionDebug {
			exp, cErr := model.NewComponentConverter().CommonDeviceToComponents(&dev)
			require.NoError(t, cErr)
			t.Logf("expected components length: %d\n", len(exp))
			for i, c := range exp {
				t.Logf("component %d => %#v\n", i, c)
			}
		}

		dq.EXPECT().Inventory(gomock.Any()).Times(1).Return(&dev, nil)
		err := hPtr.checkCurrentFirmware(act, ctx)
		require.Error(t, err)
		require.ErrorIs(t, err, sm.ErrNoAction)
	})
	t.Run("installed version does not match", func(t *testing.T) {
		t.Parallel()
		conversionDebug := false
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		dq := modeltest.NewMockDeviceQueryor(ctrl)
		ctx := &sm.HandlerContext{
			Ctx:           context.Background(),
			Logger:        logrus.NewEntry(&logrus.Logger{}),
			DeviceQueryor: dq,
		}
		act := &model.Action{
			TaskID:                "my-task-uuid",
			VerifyCurrentFirmware: true,
			Firmware: model.Firmware{
				Component: "bios",
				Vendor:    "dell", // this case has to match the vendor derived from the library
				Models: []string{
					"r6515",
				},
				Version: "the new version",
			},
		}
		dev := common.NewDevice()
		dev.Model = "PowerEdge R6515"
		dev.Vendor = "Dell-icious" // NB: This and the bogus BIOS vendor are ignored.
		dev.BIOS.Vendor = "Dell-icious"
		dev.BIOS.Model = "r6515"
		dev.BIOS.Serial = "12345"
		dev.BIOS.Firmware = &common.Firmware{
			Installed: "the version",
		}

		if conversionDebug {
			exp, cErr := model.NewComponentConverter().CommonDeviceToComponents(&dev)
			require.NoError(t, cErr)
			t.Logf("expected components length: %d\n", len(exp))
			for i, c := range exp {
				t.Logf("component %d => %#v\n", i, c)
			}
		}

		dq.EXPECT().Inventory(gomock.Any()).Times(1).Return(&dev, nil)
		err := hPtr.checkCurrentFirmware(act, ctx)
		require.NoError(t, err)
	})

}

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
