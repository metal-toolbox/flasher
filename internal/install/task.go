package install

import (
	"github.com/google/uuid"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/pkg/errors"

	rctypes "github.com/metal-toolbox/rivets/condition"
)

var (
	errTaskFirmwareParam = errors.New("error in task firmware parameters")
)

func newTask(params *rctypes.FirmwareInstallTaskParameters) (model.Task, error) {
	task := model.Task{
		ID:         uuid.New(),
		Parameters: *params,
		Status:     model.NewTaskStatusRecord("initialized task"),
	}

	//nolint:errcheck // this method returns nil unconditionally
	task.SetState(model.StatePending)

	if len(params.Firmwares) > 0 {
		task.Parameters.Firmwares = params.Firmwares
		task.FirmwarePlanMethod = model.FromRequestedFirmware

		return task, nil
	}

	if params.FirmwareSetID != uuid.Nil {
		task.Parameters.FirmwareSetID = params.FirmwareSetID
		task.FirmwarePlanMethod = model.FromFirmwareSet

		return task, nil
	}

	return task, errors.Wrap(errTaskFirmwareParam, "no firmware list or firmwareSetID specified")
}
