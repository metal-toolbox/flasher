package worker

import (
	"testing"

	"github.com/google/uuid"
	"github.com/metal-toolbox/flasher/internal/fixtures"
	"github.com/metal-toolbox/flasher/internal/model"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.hollow.sh/toolbox/events/registry"
	"go.uber.org/mock/gomock"

	bconsts "github.com/bmc-toolbox/bmclib/v2/constants"
	rctypes "github.com/metal-toolbox/rivets/condition"
)

func Test_sortFirmwareByInstallOrde(t *testing.T) {
	have := []*model.Firmware{
		{
			Version:   "DL6R",
			URL:       "https://downloads.dell.com/FOLDER06303849M/1/Serial-ATA_Firmware_Y1P10_WN32_DL6R_A00.EXE",
			FileName:  "Serial-ATA_Firmware_Y1P10_WN32_DL6R_A00.EXE",
			Models:    []string{"r6515"},
			Checksum:  "4189d3cb123a781d09a4f568bb686b23c6d8e6b82038eba8222b91c380a25281",
			Component: "drive",
		},
		{
			Version:   "2.6.6",
			URL:       "https://dl.dell.com/FOLDER08105057M/1/BIOS_C4FT0_WN64_2.6.6.EXE",
			FileName:  "BIOS_C4FT0_WN64_2.6.6.EXE",
			Models:    []string{"r6515"},
			Checksum:  "1ddcb3c3d0fc5925ef03a3dde768e9e245c579039dd958fc0f3a9c6368b6c5f4",
			Component: "bios",
		},
		{
			Version:   "20.5.13",
			URL:       "https://dl.dell.com/FOLDER08105057M/1/Network_Firmware_NVXX9_WN64_20.5.13_A00.EXE",
			FileName:  "Network_Firmware_NVXX9_WN64_20.5.13_A00.EXE",
			Models:    []string{"r6515"},
			Checksum:  "b445417d7869bdbdffe7bad69ce32dc19fa29adc61f8e82a324545cabb53f30a",
			Component: "nic",
		},
	}

	expected := []*model.Firmware{
		{
			Version:   "2.6.6",
			URL:       "https://dl.dell.com/FOLDER08105057M/1/BIOS_C4FT0_WN64_2.6.6.EXE",
			FileName:  "BIOS_C4FT0_WN64_2.6.6.EXE",
			Models:    []string{"r6515"},
			Checksum:  "1ddcb3c3d0fc5925ef03a3dde768e9e245c579039dd958fc0f3a9c6368b6c5f4",
			Component: "bios",
		},
		{
			Version:   "DL6R",
			URL:       "https://downloads.dell.com/FOLDER06303849M/1/Serial-ATA_Firmware_Y1P10_WN32_DL6R_A00.EXE",
			FileName:  "Serial-ATA_Firmware_Y1P10_WN32_DL6R_A00.EXE",
			Models:    []string{"r6515"},
			Checksum:  "4189d3cb123a781d09a4f568bb686b23c6d8e6b82038eba8222b91c380a25281",
			Component: "drive",
		},
		{
			Version:   "20.5.13",
			URL:       "https://dl.dell.com/FOLDER08105057M/1/Network_Firmware_NVXX9_WN64_20.5.13_A00.EXE",
			FileName:  "Network_Firmware_NVXX9_WN64_20.5.13_A00.EXE",
			Models:    []string{"r6515"},
			Checksum:  "b445417d7869bdbdffe7bad69ce32dc19fa29adc61f8e82a324545cabb53f30a",
			Component: "nic",
		},
	}

	sortFirmwareByInstallOrder(have)

	assert.Equal(t, expected, have)
}

func TestRemoveFirmwareAlreadyAtDesiredVersion(t *testing.T) {
	t.Parallel()
	fwSet := []*model.Firmware{
		{
			Version:   "2.6.6",
			URL:       "https://dl.dell.com/FOLDER08105057M/1/BIOS_C4FT0_WN64_2.6.6.EXE",
			FileName:  "BIOS_C4FT0_WN64_2.6.6.EXE",
			Models:    []string{"r6515"},
			Checksum:  "1ddcb3c3d0fc5925ef03a3dde768e9e245c579039dd958fc0f3a9c6368b6c5f4",
			Component: "bios",
		},
		{
			Version:   "DL6R",
			URL:       "https://downloads.dell.com/FOLDER06303849M/1/Serial-ATA_Firmware_Y1P10_WN32_DL6R_A00.EXE",
			FileName:  "Serial-ATA_Firmware_Y1P10_WN32_DL6R_A00.EXE",
			Models:    []string{"r6515"},
			Checksum:  "4189d3cb123a781d09a4f568bb686b23c6d8e6b82038eba8222b91c380a25281",
			Component: "drive",
		},
		{
			Version:   "20.5.13",
			URL:       "https://dl.dell.com/FOLDER08105057M/1/Network_Firmware_NVXX9_WN64_20.5.13_A00.EXE",
			FileName:  "Network_Firmware_NVXX9_WN64_20.5.13_A00.EXE",
			Models:    []string{"r6515"},
			Checksum:  "b445417d7869bdbdffe7bad69ce32dc19fa29adc61f8e82a324545cabb53f30a",
			Component: "nic",
		},
	}
	serverID := uuid.MustParse("fa125199-e9dd-47d4-8667-ce1d26f58c4a")
	ctx := &sm.HandlerContext{
		Logger: logrus.NewEntry(logrus.New()),
		Asset: &model.Asset{
			ID: serverID,
			Components: model.Components{
				{
					Slug:              "BiOs",
					FirmwareInstalled: "2.6.6",
				},
				{
					Slug:              "nic",
					FirmwareInstalled: "some-different-version",
				},
			},
		},
		Task: &model.Task{
			ID: serverID, // it just needs to be a UUID
		},
		WorkerID: registry.GetID("test-app"),
	}
	expected := []*model.Firmware{
		{
			Version:   "20.5.13",
			URL:       "https://dl.dell.com/FOLDER08105057M/1/Network_Firmware_NVXX9_WN64_20.5.13_A00.EXE",
			FileName:  "Network_Firmware_NVXX9_WN64_20.5.13_A00.EXE",
			Models:    []string{"r6515"},
			Checksum:  "b445417d7869bdbdffe7bad69ce32dc19fa29adc61f8e82a324545cabb53f30a",
			Component: "nic",
		},
	}

	got := removeFirmwareAlreadyAtDesiredVersion(ctx, fwSet)
	require.Equal(t, 3, len(ctx.Task.Status.StatusMsgs))
	require.Equal(t, 1, len(got))
	require.Equal(t, expected[0], got[0])
}

func TestPlanInstall1(t *testing.T) {
	t.Parallel()
	fwSet := []*model.Firmware{
		{
			Version:   "5.10.00.00",
			URL:       "https://downloads.dell.com/FOLDER06303849M/1/BMC_5_10_00_00.EXE",
			FileName:  "BMC_5_10_00_00.EXE",
			Models:    []string{"r6515"},
			Checksum:  "4189d3cb123a781d09a4f568bb686b23c6d8e6b82038eba8222b91c380a25281",
			Component: "bmc",
		},
		{
			Version:   "2.19.6",
			URL:       "https://dl.dell.com/FOLDER08105057M/1/BIOS_C4FT0_WN64_2.19.6.EXE",
			FileName:  "BIOS_C4FT0_WN64_2.19.6.EXE",
			Models:    []string{"r6515"},
			Checksum:  "1ddcb3c3d0fc5925ef03a3dde768e9e245c579039dd958fc0f3a9c6368b6c5f4",
			Component: "bios",
		},
		{
			Version:   "1.2.3",
			URL:       "https://foo/BLOB.exx",
			FileName:  "NIC_1.2.3.EXE",
			Models:    []string{"r6515"},
			Checksum:  "1ddcb3c3d0fc5925ef03a3dde768e9e245c579039dd958fc0f3a9c63aaaaaaa",
			Component: "nic",
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	q := fixtures.NewMockDeviceQueryor(ctrl)

	serverID := uuid.MustParse("fa125199-e9dd-47d4-8667-ce1d26f58c4a")
	taskID := uuid.MustParse("05c3296d-be5d-473a-b90c-4ce66cfdec65")
	ctx := &sm.HandlerContext{
		Logger: logrus.NewEntry(logrus.New()),
		Asset: &model.Asset{
			ID: serverID,
			Components: model.Components{
				{
					Slug:              "BiOs",
					FirmwareInstalled: "2.6.6",
				},
				{
					Slug:              "bmc",
					FirmwareInstalled: "5.10.00.00",
				},
				{
					Slug:              "nic",
					FirmwareInstalled: "1.2.2",
				},
			},
		},
		Task: &model.Task{
			ID: taskID,
		},
		WorkerID:      registry.GetID("test-app"),
		DeviceQueryor: q,
	}

	h := &taskHandler{}

	taskParam := &model.Task{
		ID: uuid.MustParse("95ccb1c5-d807-4078-bb22-facc3045a49a"),
		Parameters: rctypes.FirmwareInstallTaskParameters{
			AssetID:               serverID,
			ResetBMCBeforeInstall: true,
		},
	}

	q.EXPECT().FirmwareInstallSteps(gomock.Any(), gomock.Any()).
		Times(2).
		Return([]bconsts.FirmwareInstallStep{
			bconsts.FirmwareInstallStepUploadInitiateInstall,
			bconsts.FirmwareInstallStepInstallStatus,
		}, nil)

	sms, actions, err := h.planInstall(ctx, taskParam, fwSet)
	require.NoError(t, err, "no errors returned")
	require.Equal(t, 2, len(sms), "expect two action state machines")
	require.Equal(t, 2, len(actions), "expect two actions to be performed")
	require.True(t, actions[0].BMCResetPreInstall, "expect BMCResetPreInstall is true on the first action")
	require.False(t, actions[1].BMCResetPreInstall, "expect BMCResetPreInstall is false on subsequent actions")
	require.False(t, actions[0].BMCResetOnInstallFailure, "expect BMCResetOnInstallFailure is false for action")
	require.False(t, actions[1].BMCResetOnInstallFailure, "expect BMCResetOnInstallFailure is false for action")
	require.True(t, actions[1].Final, "expect final bool is true the last action")
	require.Equal(t, "bios", actions[0].Firmware.Component, "expect bios component action")
	require.Equal(t, "nic", actions[1].Firmware.Component, "expect nic component action")
}

func TestPlanInstall2(t *testing.T) {
	t.Parallel()
	fwSet := []*model.Firmware{
		{
			Version:   "5.10.00.00",
			URL:       "https://downloads.dell.com/FOLDER06303849M/1/BMC_5_10_00_00.EXE",
			FileName:  "BMC_5_10_00_00.EXE",
			Models:    []string{"r6515"},
			Checksum:  "4189d3cb123a781d09a4f568bb686b23c6d8e6b82038eba8222b91c380a25281",
			Component: "bmc",
		},
		{
			Version:   "2.19.6",
			URL:       "https://dl.dell.com/FOLDER08105057M/1/BIOS_C4FT0_WN64_2.19.6.EXE",
			FileName:  "BIOS_C4FT0_WN64_2.19.6.EXE",
			Models:    []string{"r6515"},
			Checksum:  "1ddcb3c3d0fc5925ef03a3dde768e9e245c579039dd958fc0f3a9c6368b6c5f4",
			Component: "bios",
		},
		{
			Version:   "1.2.3",
			URL:       "https://foo/BLOB.exx",
			FileName:  "NIC_1.2.3.EXE",
			Models:    []string{"r6515"},
			Checksum:  "1ddcb3c3d0fc5925ef03a3dde768e9e245c579039dd958fc0f3a9c63aaaaaaa",
			Component: "nic",
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	q := fixtures.NewMockDeviceQueryor(ctrl)

	serverID := uuid.MustParse("fa125199-e9dd-47d4-8667-ce1d26f58c4a")
	taskID := uuid.MustParse("05c3296d-be5d-473a-b90c-4ce66cfdec65")
	ctx := &sm.HandlerContext{
		Logger: logrus.NewEntry(logrus.New()),
		Asset: &model.Asset{
			ID: serverID,
			Components: model.Components{
				{
					Slug:              "BiOs",
					FirmwareInstalled: "2.6.6",
				},
				{
					Slug:              "bmc",
					FirmwareInstalled: "5.10.00.00",
				},
				{
					Slug:              "nic",
					FirmwareInstalled: "1.2.2",
				},
			},
		},
		Task: &model.Task{
			ID: taskID,
		},
		WorkerID:      registry.GetID("test-app"),
		DeviceQueryor: q,
	}

	h := &taskHandler{}

	taskParam := &model.Task{
		ID: uuid.MustParse("95ccb1c5-d807-4078-bb22-facc3045a49a"),
		Parameters: rctypes.FirmwareInstallTaskParameters{
			AssetID:      serverID,
			ForceInstall: true,
		},
	}

	q.EXPECT().FirmwareInstallSteps(gomock.Any(), gomock.Any()).
		Times(3).
		Return([]bconsts.FirmwareInstallStep{
			bconsts.FirmwareInstallStepResetBMCOnInstallFailure,
			bconsts.FirmwareInstallStepUploadInitiateInstall,
			bconsts.FirmwareInstallStepInstallStatus,
		}, nil)

	sms, actions, err := h.planInstall(ctx, taskParam, fwSet)
	require.NoError(t, err, "no errors returned")
	require.Equal(t, 3, len(sms), "expect three action state machines")
	require.Equal(t, 3, len(actions), "expect three actions to be performed")
	require.False(t, actions[0].BMCResetPreInstall, "expect BMCResetPreInstall is false on the first action")
	require.False(t, actions[1].BMCResetPreInstall, "expect BMCResetPreInstall is false on subsequent actions")
	require.False(t, actions[2].BMCResetPreInstall, "expect BMCResetPreInstall is false on subsequent actions")
	require.True(t, actions[0].BMCResetOnInstallFailure, "expect BMCResetOnInstallFailure is true for action")
	require.True(t, actions[1].BMCResetOnInstallFailure, "expect BMCResetOnInstallFailure is true for action")
	require.True(t, actions[2].BMCResetOnInstallFailure, "expect BMCResetOnInstallFailure is true for action")
	require.True(t, actions[2].Final, "expect final bool is true on the last action")
	require.False(t, actions[0].VerifyCurrentFirmware, "expect VerifyCurrentFirmware set to false when task is forced")
	require.False(t, actions[1].VerifyCurrentFirmware, "expect VerifyCurrentFirmware set to false when task is forced")
	require.False(t, actions[2].VerifyCurrentFirmware, "expect VerifyCurrentFirmware set to false when task is forced")
	require.Equal(t, "bmc", actions[0].Firmware.Component, "expect bmc component action")
	require.Equal(t, "bios", actions[1].Firmware.Component, "expect bios component action")
	require.Equal(t, "nic", actions[2].Firmware.Component, "expect nic component action")
}
