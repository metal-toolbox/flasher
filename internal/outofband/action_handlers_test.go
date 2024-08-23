package outofband

import (
	"context"
	"os"
	"testing"

	bconsts "github.com/bmc-toolbox/bmclib/v2/constants"
	"github.com/bmc-toolbox/common"
	"github.com/metal-toolbox/flasher/internal/device"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/runner"
	rctypes "github.com/metal-toolbox/rivets/condition"
	rtypes "github.com/metal-toolbox/rivets/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func newTestActionCtx() *runner.ActionHandlerContext {
	return &runner.ActionHandlerContext{
		TaskHandlerContext: &runner.TaskHandlerContext{
			Task: &model.Task{
				Parameters: &rctypes.FirmwareInstallTaskParameters{},
				Server:     &rtypes.Server{},
				State:      model.StateActive,
			},
			Logger: logrus.NewEntry(logrus.New()),
		},
		Firmware: &rctypes.Firmware{
			Vendor:    "Dell-icious",
			Version:   "DL6R",
			URL:       "https://downloads.dell.com/FOLDER06303849M/1/Serial-ATA_Firmware_Y1P10_WN32_DL6R_A00.EXE",
			FileName:  "Serial-ATA_Firmware_Y1P10_WN32_DL6R_A00.EXE",
			Models:    []string{"r6515"},
			Checksum:  "4189d3cb123a781d09a4f568bb686b23c6d8e6b82038eba8222b91c380a25281",
			Component: "drive",
		},
	}
}

func TestCheckCurrentFirmware(t *testing.T) {
	t.Parallel()

	// helper func to initialize handler, mock device queryor
	init := func(t *testing.T) (*handler, *device.MockOutofbandQueryor) {
		t.Helper()

		actionCtx := newTestActionCtx()
		m := new(device.MockOutofbandQueryor)
		m.On("FirmwareInstallSteps", mock.Anything, "drive").Once().Return(
			[]bconsts.FirmwareInstallStep{
				bconsts.FirmwareInstallStepUploadInitiateInstall,
				bconsts.FirmwareInstallStepInstallStatus,
			},
			nil,
		)
		actionCtx.DeviceQueryor = m

		ah := &ActionHandler{}
		_, err := ah.ComposeAction(context.Background(), actionCtx)
		assert.Nil(t, err)
		ah.handler.action.FirmwareInstallStep = string(bconsts.FirmwareInstallStepUploadInitiateInstall)

		return ah.handler, m
	}

	// helper func to debug device conversion
	conversionDebug := func(t *testing.T, dev *common.Device) {
		t.Helper()
		exp, cErr := model.NewComponentConverter().CommonDeviceToComponents(dev)
		require.NoError(t, cErr)
		t.Logf("expected components length: %d\n", len(exp))
		for i, c := range exp {
			t.Logf("component %d => %#v\n", i, c)
		}
	}

	ctx := context.Background()

	t.Run("inventory error", func(t *testing.T) {
		t.Parallel()
		handler, dq := init(t)

		dq.EXPECT().Inventory(mock.Anything).Times(1).Return(nil, errors.New("pound sand"))
		err := handler.checkCurrentFirmware(ctx)
		require.Error(t, err)
		require.Equal(t, "pound sand", err.Error())
	})

	t.Run("nil inventory", func(t *testing.T) {
		t.Parallel()
		handler, dq := init(t)

		dq.EXPECT().Inventory(mock.Anything).Times(1).Return(nil, nil)
		err := handler.checkCurrentFirmware(ctx)
		require.Error(t, err)
		require.ErrorIs(t, err, model.ErrComponentConverter)
	})

	t.Run("bad component slug", func(t *testing.T) {
		t.Parallel()
		handler, dq := init(t)

		dev := common.NewDevice()
		dev.Model = "PowerEdge R6515"
		dev.Vendor = "Dell-icious"
		dev.BIOS = &common.BIOS{}

		dq.EXPECT().Inventory(mock.Anything).Times(1).Return(&dev, nil)
		err := handler.checkCurrentFirmware(ctx)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrComponentNotFound)
	})

	t.Run("blank installed version", func(t *testing.T) {
		t.Parallel()
		debug := false
		handler, dq := init(t)

		dev := common.NewDevice()
		dev.Model = "PowerEdge R6515"
		dev.Vendor = "Dell-icious"
		dev.BIOS.Vendor = "Dell-icious"
		dev.BIOS.Model = "r6515"
		dev.BIOS.Serial = "12345"
		dev.Drives = []*common.Drive{
			{
				Common: common.Common{
					Vendor: "Dell-icious",
					Model:  "r6515",
					Firmware: &common.Firmware{
						Installed: "", // no firmware version for component
					},
				},
			},
		}

		if debug {
			conversionDebug(t, &dev)
		}

		dq.EXPECT().Inventory(mock.Anything).Times(1).Return(&dev, nil)
		err := handler.checkCurrentFirmware(ctx)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrInstalledVersionUnknown)
	})
	t.Run("equal installed version", func(t *testing.T) {
		t.Parallel()
		debug := false
		handler, dq := init(t)

		dev := common.NewDevice()
		dev.Model = "PowerEdge R6515"
		dev.Vendor = "Dell-icious"
		dev.Drives = []*common.Drive{
			{
				Common: common.Common{
					Vendor: "Dell-icious",
					Model:  "r6515",
					Firmware: &common.Firmware{
						Installed: "DL6R", // match whats returned by init()
					},
				},
			},
		}

		if debug {
			conversionDebug(t, &dev)
		}

		dq.EXPECT().Inventory(mock.Anything).Times(1).Return(&dev, nil)
		err := handler.checkCurrentFirmware(ctx)
		require.Error(t, err)
		require.ErrorIs(t, err, model.ErrInstalledFirmwareEqual)
	})
	t.Run("installed version does not match", func(t *testing.T) {
		t.Parallel()
		debug := false
		handler, dq := init(t)

		dev := common.NewDevice()
		dev.Model = "PowerEdge R6515"
		dev.Vendor = "Dell-icious"
		dev.Drives = []*common.Drive{
			{
				Common: common.Common{
					Vendor: "Dell-icious",
					Model:  "r6515",
					Firmware: &common.Firmware{
						Installed: "OLDversion",
					},
				},
			},
		}

		if debug {
			conversionDebug(t, &dev)
		}

		dq.EXPECT().Inventory(mock.Anything).Times(1).Return(&dev, nil)
		err := handler.checkCurrentFirmware(ctx)
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

	init := func(t *testing.T) (*handler, *device.MockOutofbandQueryor) {
		t.Helper()

		actionCtx := newTestActionCtx()
		m := new(device.MockOutofbandQueryor)
		m.On("FirmwareInstallSteps", mock.Anything, "drive").Once().Return(
			[]bconsts.FirmwareInstallStep{
				bconsts.FirmwareInstallStepUploadInitiateInstall,
				bconsts.FirmwareInstallStepInstallStatus,
			},
			nil,
		)
		actionCtx.DeviceQueryor = m

		ah := &ActionHandler{}
		_, err := ah.ComposeAction(context.Background(), actionCtx)
		assert.Nil(t, err)
		ah.handler.action.FirmwareInstallStep = string(bconsts.FirmwareInstallStepUploadInitiateInstall)

		return ah.handler, m
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			handler, m := init(t)

			os.Setenv(envTesting, "1")
			defer os.Unsetenv(envTesting)

			m.EXPECT().FirmwareTaskStatus(
				mock.Anything,
				bconsts.FirmwareInstallStepUploadInitiateInstall,
				"drive",
				mock.Anything,
				"DL6R",
			).Return(bconsts.TaskState(tc.state), "some status", tc.errorContains)

			if tc.state == "powercycle-host" {
				m.EXPECT().SetPowerState(mock.Anything, mock.Anything).Times(1).Return(nil)
			}

			if err := handler.pollFirmwareTaskStatus(context.Background()); err != nil {
				if tc.errorContains != nil {
					assert.ErrorContains(t, err, tc.errorContains.Error())
				} else {
					if tc.state == "powercycle-host" {
						assert.True(t, handler.action.HostPowerCycled)
						return
					}

					t.Fatal(err)
				}
			}
		})
	}
}
