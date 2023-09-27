// nolint
package worker

import (
	"testing"

	"github.com/google/uuid"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/stretchr/testify/assert"

	rctypes "github.com/metal-toolbox/rivets/condition"
)

func Test_newTaskFromCondition(t *testing.T) {
	tests := []struct {
		name      string
		condition *rctypes.Condition
		want      *model.Task
		wantErr   bool
	}{
		{
			"condition parameters parsed into task parameters",
			&rctypes.Condition{
				ID:         uuid.MustParse("abc81024-f62a-4288-8730-3fab8ccea777"),
				Kind:       rctypes.FirmwareInstall,
				Version:    "1",
				Parameters: []byte(`{"asset_id":"ede81024-f62a-4288-8730-3fab8cceab78","firmware_set_id":"9d70c28c-5f65-4088-b014-205c54ad4ac7", "force_install": true, "reset_bmc_before_install": true}`),
			},
			func() *model.Task {
				t, _ := newTask(
					uuid.MustParse("abc81024-f62a-4288-8730-3fab8ccea777"),
					&rctypes.FirmwareInstallTaskParameters{
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
			got, err := newTaskFromCondition(tt.condition, false)
			if (err != nil) != tt.wantErr {
				t.Errorf("newTaskFromCondition() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			assert.Equal(t, tt.want, got)
		})
	}
}
