package outofband

import (
	"context"
	"os"
	"testing"

	bconsts "github.com/bmc-toolbox/bmclib/v2/constants"
	"github.com/bmc-toolbox/common"
	sw "github.com/filanov/stateswitch"
	"github.com/metal-toolbox/flasher/internal/fixtures"
	"github.com/metal-toolbox/flasher/internal/model"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

type fakeStateSwitch struct{}

func (_ fakeStateSwitch) State() sw.State {
	return sw.State("bogus")
}
func (_ fakeStateSwitch) SetState(sw.State) error {
	return nil
}

func TestCheckCurrentFirmware(t *testing.T) {
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
		dq := fixtures.NewMockDeviceQueryor(ctrl)
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
		dq := fixtures.NewMockDeviceQueryor(ctrl)
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
		dq := fixtures.NewMockDeviceQueryor(ctrl)
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
	})
	t.Run("blank installed version", func(t *testing.T) {
		t.Parallel()
		conversionDebug := false
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		dq := fixtures.NewMockDeviceQueryor(ctrl)
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
		dq := fixtures.NewMockDeviceQueryor(ctrl)
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
		require.ErrorIs(t, err, ErrInstalledFirmwareEqual)
	})
	t.Run("installed version does not match", func(t *testing.T) {
		t.Parallel()
		conversionDebug := false
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		dq := fixtures.NewMockDeviceQueryor(ctrl)
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
		require.Nil(t, err)
	})

}

func TestPollFirmwareInstallStatus(t *testing.T) {
	testcases := []struct {
		name          string
		state         string
		errorContains error
	}{
		{
			"too many failures, returns error",
			"unknown",
			ErrMaxBMCQueryAttempts,
		},
		{
			"install requires a BMC power cycle",
			"powercycle-bmc",
			nil,
		},
		{
			"install requires a Host power cycle",
			"powercycle-host",
			nil,
		},
		{
			"install state running exceeds max BMC query attempts",
			"running",
			ErrMaxBMCQueryAttempts,
		},
		{
			"install state failed returns error",
			"failed",
			ErrFirmwareInstallFailed,
		},
		{
			"install state complete returns",
			"complete",
			nil,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {

			task := newTaskFixture(string(model.StateActive))
			asset := fixtures.Assets[fixtures.Asset1ID.String()]

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			q := fixtures.NewMockDeviceQueryor(ctrl)

			handlerCtx := &sm.HandlerContext{
				Task:          task,
				Ctx:           context.Background(),
				Logger:        logrus.NewEntry(&logrus.Logger{}),
				DeviceQueryor: q,
				Asset:         &asset,
			}

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

			q.EXPECT().FirmwareTaskStatus(
				gomock.Any(),
				gomock.Any(),
				gomock.Any(),
				gomock.Any(),
				gomock.Any(),
				gomock.Any(),
			).AnyTimes().Return(bconsts.TaskState(tc.state), "some status", tc.errorContains)

			if tc.state == "powercycle-host" {
				q.EXPECT().SetPowerState(gomock.Any(), gomock.Any()).Times(1).Return(nil)
			}

			if err := handler.pollFirmwareTaskStatus(&action, handlerCtx); err != nil {
				if tc.errorContains != nil {
					assert.ErrorContains(t, err, tc.errorContains.Error())
				} else {
					if tc.state == "powercycle-host" {
						assert.True(t, action.HostPowerCycled)
						return
					}

					t.Fatal(err)
				}
			}

			// assert action fields are set when bmc/host power cycle is required.
			if tc.state == "powercycle-bmc" {
				assert.True(t, action.BMCPowerCycleRequired)
			}
		})
	}
}
