// nolint
package worker

import (
	"testing"

	"github.com/google/uuid"
	cptypes "github.com/metal-toolbox/conditionorc/pkg/types"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/stretchr/testify/assert"
)


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
				Kind:       cptypes.FirmwareInstall,
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
			got, err := newTaskFromCondition(tt.condition, false)
			if (err != nil) != tt.wantErr {
				t.Errorf("newTaskFromCondition() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			assert.Equal(t, tt.want, got)
		})
	}
}
