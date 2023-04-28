package fixtures

import (
	"github.com/jinzhu/copier"
	"github.com/metal-toolbox/flasher/internal/model"
)

var (
	Firmware = []*model.Firmware{
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
	}
)

func copyFirmware(src []*model.Firmware) []*model.Firmware {
	dst := []*model.Firmware{}

	copyOptions := copier.Option{IgnoreEmpty: true, DeepCopy: true}

	err := copier.CopyWithOption(&dst, &src, copyOptions)
	if err != nil {
		panic(err)
	}

	return dst
}

func NewFirmware() []*model.Firmware {
	return copyFirmware(Firmware)
}
