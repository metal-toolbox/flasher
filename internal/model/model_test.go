package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_SortByInstallOrder(t *testing.T) {
	have := FirmwarePlanned{
		{
			Version:       "DL6R",
			URL:           "https://downloads.dell.com/FOLDER06303849M/1/Serial-ATA_Firmware_Y1P10_WN32_DL6R_A00.EXE",
			FileName:      "Serial-ATA_Firmware_Y1P10_WN32_DL6R_A00.EXE",
			Model:         "r6515",
			Checksum:      "4189d3cb123a781d09a4f568bb686b23c6d8e6b82038eba8222b91c380a25281",
			ComponentSlug: "drive",
		},
		{
			Version:       "2.6.6",
			URL:           "https://dl.dell.com/FOLDER08105057M/1/BIOS_C4FT0_WN64_2.6.6.EXE",
			FileName:      "BIOS_C4FT0_WN64_2.6.6.EXE",
			Model:         "r6515",
			Checksum:      "1ddcb3c3d0fc5925ef03a3dde768e9e245c579039dd958fc0f3a9c6368b6c5f4",
			ComponentSlug: "bios",
		},
		{
			Version:       "20.5.13",
			URL:           "https://dl.dell.com/FOLDER08105057M/1/Network_Firmware_NVXX9_WN64_20.5.13_A00.EXE",
			FileName:      "Network_Firmware_NVXX9_WN64_20.5.13_A00.EXE",
			Model:         "r6515",
			Checksum:      "b445417d7869bdbdffe7bad69ce32dc19fa29adc61f8e82a324545cabb53f30a",
			ComponentSlug: "nic",
		},
	}

	expected := FirmwarePlanned{
		{
			Version:       "2.6.6",
			URL:           "https://dl.dell.com/FOLDER08105057M/1/BIOS_C4FT0_WN64_2.6.6.EXE",
			FileName:      "BIOS_C4FT0_WN64_2.6.6.EXE",
			Model:         "r6515",
			Checksum:      "1ddcb3c3d0fc5925ef03a3dde768e9e245c579039dd958fc0f3a9c6368b6c5f4",
			ComponentSlug: "bios",
		},
		{
			Version:       "DL6R",
			URL:           "https://downloads.dell.com/FOLDER06303849M/1/Serial-ATA_Firmware_Y1P10_WN32_DL6R_A00.EXE",
			FileName:      "Serial-ATA_Firmware_Y1P10_WN32_DL6R_A00.EXE",
			Model:         "r6515",
			Checksum:      "4189d3cb123a781d09a4f568bb686b23c6d8e6b82038eba8222b91c380a25281",
			ComponentSlug: "drive",
		},
		{
			Version:       "20.5.13",
			URL:           "https://dl.dell.com/FOLDER08105057M/1/Network_Firmware_NVXX9_WN64_20.5.13_A00.EXE",
			FileName:      "Network_Firmware_NVXX9_WN64_20.5.13_A00.EXE",
			Model:         "r6515",
			Checksum:      "b445417d7869bdbdffe7bad69ce32dc19fa29adc61f8e82a324545cabb53f30a",
			ComponentSlug: "nic",
		},
	}

	have.SortByInstallOrder()

	assert.Equal(t, expected, have)
}
