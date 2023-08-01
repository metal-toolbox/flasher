package worker

import (
	"testing"

	"github.com/google/uuid"
	"github.com/metal-toolbox/flasher/internal/model"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.hollow.sh/toolbox/events/registry"
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
	require.Equal(t, 1, len(got))
	require.Equal(t, expected[0], got[0])
}
