package worker

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/metal-toolbox/flasher/internal/device"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/runner"
	"github.com/metal-toolbox/rivets/events/registry"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	bconsts "github.com/bmc-toolbox/bmclib/v2/constants"
	"github.com/bmc-toolbox/common"
	rctypes "github.com/metal-toolbox/rivets/condition"
	rtypes "github.com/metal-toolbox/rivets/types"
)

func TestSortFirmwareByInstallOrder(t *testing.T) {
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

	h := handler{}
	h.sortFirmwareByInstallOrder(have)

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

	taskHandlerCtx := &runner.TaskHandlerContext{
		Logger: logrus.NewEntry(logrus.New()),
		Task: &model.Task{
			ID:       serverID, // it just needs to be a UUID
			WorkerID: registry.GetID("test-app").String(),
			Asset: &rctypes.Asset{
				ID: serverID,
				Components: rtypes.Components{
					{
						Name:     "BiOs",
						Firmware: &common.Firmware{Installed: "2.6.6"},
					},
					{
						Name:     "nic",
						Firmware: &common.Firmware{Installed: "some-different-version"},
					},
				},
			},
		},
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

	h := handler{taskHandlerCtx}
	got := h.removeFirmwareAlreadyAtDesiredVersion(fwSet)
	require.Equal(t, 3, len(h.Task.Status.StatusMsgs))
	require.Equal(t, 1, len(got))
	require.Equal(t, expected[0], got[0])
}

func TestPlanInstallOutofband(t *testing.T) {
	t.Parallel()
	fwSet := []*model.Firmware{
		{
			Version:       "5.10.00.00",
			URL:           "https://downloads.dell.com/FOLDER06303849M/1/BMC_5_10_00_00.EXE",
			FileName:      "BMC_5_10_00_00.EXE",
			Models:        []string{"r6515"},
			Checksum:      "4189d3cb123a781d09a4f568bb686b23c6d8e6b82038eba8222b91c380a25281",
			Component:     "bmc",
			InstallInband: false,
		},
		{
			Version:       "2.19.6",
			URL:           "https://dl.dell.com/FOLDER08105057M/1/BIOS_C4FT0_WN64_2.19.6.EXE",
			FileName:      "BIOS_C4FT0_WN64_2.19.6.EXE",
			Models:        []string{"r6515"},
			Checksum:      "1ddcb3c3d0fc5925ef03a3dde768e9e245c579039dd958fc0f3a9c6368b6c5f4",
			Component:     "bios",
			InstallInband: false,
		},
		{
			Version:       "1.2.3",
			URL:           "https://foo/BLOB.exx",
			FileName:      "NIC_1.2.3.EXE",
			Models:        []string{"r6515"},
			Checksum:      "1ddcb3c3d0fc5925ef03a3dde768e9e245c579039dd958fc0f3a9c63aaaaaaa",
			Component:     "nic",
			InstallInband: false,
		},
	}

	dq := new(device.MockQueryor)

	serverID := uuid.MustParse("fa125199-e9dd-47d4-8667-ce1d26f58c4a")
	taskID := uuid.MustParse("05c3296d-be5d-473a-b90c-4ce66cfdec65")
	taskHandlerCtx := &runner.TaskHandlerContext{
		Logger: logrus.NewEntry(logrus.New()),
		Task: &model.Task{
			ID:       taskID,
			WorkerID: registry.GetID("test-app").String(),
			Parameters: rctypes.FirmwareInstallTaskParameters{
				AssetID:               serverID,
				ResetBMCBeforeInstall: true,
			},
			Asset: &rctypes.Asset{
				ID: serverID,
				Components: rtypes.Components{
					{
						Name:     "BiOs",
						Firmware: &common.Firmware{Installed: "2.6.6"},
					},
					{
						Name:     "bmc",
						Firmware: &common.Firmware{Installed: "5.10.00.00"},
					},
					{
						Name:     "nic",
						Firmware: &common.Firmware{Installed: "1.2.2"},
					},
				},
			},
		},

		DeviceQueryor: dq,
	}

	dq.EXPECT().FirmwareInstallSteps(mock.Anything, mock.Anything).
		Times(2).
		Return([]bconsts.FirmwareInstallStep{
			bconsts.FirmwareInstallStepUploadInitiateInstall,
			bconsts.FirmwareInstallStepInstallStatus,
		}, nil)

	h := &handler{taskHandlerCtx}
	actions, err := h.planInstallOutofband(context.Background(), fwSet)
	require.NoError(t, err, "no errors returned")
	require.Equal(t, 2, len(actions), "expect two actions to be performed")
	require.True(t, actions[0].BMCResetPreInstall, "expect BMCResetPreInstall is true on the first action")
	require.False(t, actions[1].BMCResetPreInstall, "expect BMCResetPreInstall is false on subsequent actions")
	require.False(t, actions[0].BMCResetOnInstallFailure, "expect BMCResetOnInstallFailure is false for action")
	require.False(t, actions[1].BMCResetOnInstallFailure, "expect BMCResetOnInstallFailure is false for action")
	require.True(t, actions[1].Last, "expect Last bool is true the last action")
	require.Equal(t, "bios", actions[0].Firmware.Component, "expect bios component action")
	require.Equal(t, "nic", actions[1].Firmware.Component, "expect nic component action")
}

func TestPlanInstallOutofband2(t *testing.T) {
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

	dq := new(device.MockQueryor)

	serverID := uuid.MustParse("fa125199-e9dd-47d4-8667-ce1d26f58c4a")
	taskID := uuid.MustParse("05c3296d-be5d-473a-b90c-4ce66cfdec65")
	taskHandlerCtx := &runner.TaskHandlerContext{
		Logger: logrus.NewEntry(logrus.New()),
		Task: &model.Task{
			ID:       taskID,
			WorkerID: registry.GetID("test-app").String(),
			Parameters: rctypes.FirmwareInstallTaskParameters{
				AssetID:      serverID,
				ForceInstall: true,
			},
			Asset: &rctypes.Asset{
				ID: serverID,
				Components: rtypes.Components{
					{
						Name:     "BiOs",
						Firmware: &common.Firmware{Installed: "2.6.6"},
					},
					{
						Name:     "bmc",
						Firmware: &common.Firmware{Installed: "5.10.00.00"},
					},
					{
						Name:     "nic",
						Firmware: &common.Firmware{Installed: "1.2.2"},
					},
				},
			},
		},
		DeviceQueryor: dq,
	}

	dq.EXPECT().FirmwareInstallSteps(mock.Anything, mock.Anything).
		Times(3).
		Return([]bconsts.FirmwareInstallStep{
			bconsts.FirmwareInstallStepResetBMCOnInstallFailure,
			bconsts.FirmwareInstallStepUploadInitiateInstall,
			bconsts.FirmwareInstallStepInstallStatus,
		}, nil)

	h := &handler{taskHandlerCtx}
	actions, err := h.planInstallOutofband(context.Background(), fwSet)
	require.NoError(t, err, "no errors returned")
	require.Equal(t, 3, len(actions), "expect three actions to be performed")
	require.False(t, actions[0].BMCResetPreInstall, "expect BMCResetPreInstall is false on the first action")
	require.False(t, actions[1].BMCResetPreInstall, "expect BMCResetPreInstall is false on subsequent actions")
	require.False(t, actions[2].BMCResetPreInstall, "expect BMCResetPreInstall is false on subsequent actions")
	require.True(t, actions[0].BMCResetOnInstallFailure, "expect BMCResetOnInstallFailure is true for action")
	require.True(t, actions[1].BMCResetOnInstallFailure, "expect BMCResetOnInstallFailure is true for action")
	require.True(t, actions[2].BMCResetOnInstallFailure, "expect BMCResetOnInstallFailure is true for action")
	require.True(t, actions[2].Last, "expect Last bool is true on the last action")
	require.True(t, actions[0].ForceInstall, "expect ForceInstall set to true when task is forced")
	require.True(t, actions[1].ForceInstall, "expect ForceInstall set to true when task is forced")
	require.True(t, actions[2].ForceInstall, "expect ForceInstall set to true when task is forced")
	require.Equal(t, "bmc", actions[0].Firmware.Component, "expect bmc component action")
	require.Equal(t, "bios", actions[1].Firmware.Component, "expect bios component action")
	require.Equal(t, "nic", actions[2].Firmware.Component, "expect nic component action")
}
