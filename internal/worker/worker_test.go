// nolint
package worker

import (
	"testing"

	"github.com/google/uuid"
	cptypes "github.com/metal-toolbox/conditionorc/pkg/types"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/stretchr/testify/assert"
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

func Test_newTaskFromCondition(t *testing.T) {
	tests := []struct {
		name      string
		condition *cptypes.Condition
		want      *model.Task
		wantErr   bool
	}{
		{
			"condition parameters parsed into task parameters",
			&cptypes.Condition{
				ID:         uuid.MustParse("abc81024-f62a-4288-8730-3fab8ccea777"),
				Kind:       cptypes.FirmwareInstallOutofband,
				Version:    "1",
				Parameters: []byte(`{"assetID":"ede81024-f62a-4288-8730-3fab8cceab78","firmwareSetID":"9d70c28c-5f65-4088-b014-205c54ad4ac7", "forceInstall": true, "resetBMCBeforeInstall": true}`),
			},
			func() *model.Task {
				t, _ := newTask(
					uuid.MustParse("abc81024-f62a-4288-8730-3fab8ccea777"),
					&model.TaskParameters{
						AssetID:               uuid.MustParse("ede81024-f62a-4288-8730-3fab8cceab78"),
						FirmwareSetID:         uuid.MustParse("9d70c28c-5f65-4088-b014-205c54ad4ac7"),
						ForceInstall:          true,
						ResetBMCBeforeInstall: true,
					},
				)
				return &t
			}(),
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newTaskFromCondition(tt.condition)
			if (err != nil) != tt.wantErr {
				t.Errorf("newTaskFromCondition() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			assert.Equal(t, tt.want, got)
		})
	}
}
