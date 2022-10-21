package model

import (
	"time"

	sw "github.com/filanov/stateswitch"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

// InstallMethod is one of 'outofband' OR 'inband'
// it is the method by which the firmware is installed on the device.
type InstallMethod string

// FirmwarePlanMethod type defines the firmware resolution method by which
// the firmware to applied is planned.
type FirmwarePlanMethod string

const (
	// InstallMethodOutofband indicates the out of band firmware install method.
	InstallMethodOutofband InstallMethod = "outofband"

	// PlanPredefinedFirmaware is a TaskParameter attribute that indicates the
	// firmware to be installed was provided at task initialization (through a CLI parameter or inventory device attribute)
	// and so no further firmware planning is required.
	PlanRequestedFirmware FirmwarePlanMethod = "predefined"

	// PlanFromFirmwareSet is a TaskParameter attribute that indicates a
	// firmware set ID was provided at task initialization (through a CLI parameter or inventory device attribute)
	// the firmware versions to be installed are to be planned from the given firmware set ID.
	PlanFromFirmwareSet FirmwarePlanMethod = "fromFirmwareSet"

	// PlanFromInstalledFirmware is a TaskParameter attribute that indicates
	// the firmware versions to be installed have to be planned
	// based on the firmware currently installed on the device.
	//PlanFromInstalledFirmware FirmwarePlanMethod = "fromInstalledFirmware"

	// task states
	//
	// states the task transitions through
	StateRequested sw.State = "requested"
	StateQueued    sw.State = "queued"
	StateActive    sw.State = "active"
	StateSuccess   sw.State = "success"
	StateFailed    sw.State = "failed"
)

var (
	ErrTaskFirmwareParam = errors.New("error in task firmware parameters")
)

// Action is part of a task, it is resolved from the Firmware configuration
//
// Actions transition through states as the action progresses
// those states are `queued`, `active`, `success`, `FailedState`.
type Action struct {
	// ID is a unique identifier for this action
	ID string

	// The parent task identifier
	TaskID string

	// Method of install
	InstallMethod InstallMethod
	// BMCTaskID ...

	// Status indicates the action status
	Status string

	// Firmware to be installed
	Firmware Firmware
}

func (a *Action) SetState(state sw.State) error {

	a.Status = string(state)

	return nil
}

func (a *Action) State() sw.State {
	return sw.State(a.Status)
}

// Actions is a list of actions
type Actions []Action

// ByID returns the Action matched by the identifier
func (a Actions) ByID(id string) *Action {
	for _, action := range a {
		if action.ID == id {
			return &action
		}
	}

	return nil
}

// Task is a top level unit of work handled by flasher.
//
// A task performs one or more actions, each of the action corresponds to a Firmware
type Task struct {
	// Task unique identifier
	ID uuid.UUID

	// Status is the install status
	Status string

	// Info is informational data and includes errors in task execution if any.
	Info string

	// ActionsPlanned to be executed for task are generated from the FirmwaresPlanned and install parameters
	// these are generated in the `queued` stage of the task.
	ActionsPlanned Actions

	// FirmwaresPlanned is the list of firmware planned for install.
	FirmwaresPlanned FirmwarePlanned

	// Parameters for this task
	Parameters TaskParameters

	CreatedAt   time.Time
	UpdatedAt   time.Time
	CompletedAt time.Time
}

// SetState implements the stateswitch statemachine interface
func (t *Task) SetState(state sw.State) error {
	t.Status = string(state)
	return nil
}

// State implements the stateswitch statemachine interface
func (t *Task) State() sw.State {
	return sw.State(t.Status)
}

// NewTask returns a new Task
//
// method, device parameters are required.
// firmwareSetID, firmware are optional and mutually exclusive.
func NewTask(firmwareSetID string, firmware []Firmware) (Task, error) {
	task := Task{ID: uuid.New()}

	if firmwareSetID != "" && len(firmware) > 0 {
		return task, errors.Wrap(ErrTaskFirmwareParam, "expected a firmware setID OR firmware version(s), got both")
	}

	if len(firmware) > 0 {
		task.FirmwaresPlanned = firmware
		task.Parameters.FirmwarePlanMethod = PlanRequestedFirmware

		return task, nil
	}

	// firmwareSetID when defined sets the plan to PlanFromFirmwareSet,
	// in the case where the firmwareSetID AND no firmware versions were defined
	// the firmwareSetID is looked up in the task handlers.
	if firmwareSetID != "" || (firmwareSetID == "" && len(firmware) == 0) {
		task.Parameters.FirmwareSetID = firmwareSetID
		task.Parameters.FirmwarePlanMethod = PlanFromFirmwareSet
	}

	return task, nil
}

// TaskParameters are the parameters set for each task flasher works
// these are parameters received from an operator which determines the task execution actions.
type TaskParameters struct {
	// Reset device BMC before firmware install
	ResetBMCBeforeInstall bool `json:"resetBMCBeforeInstall,omitempty"`

	// Force install given firmware regardless of current firmware version.
	ForceInstall bool `json:"forceInstall,omitempty"`

	// Task priority is the task priority between 0 and 3
	// where 0 is the default and 3 is the max.
	//
	// Tasks are picked from the `queued` state based on the priority.
	//
	// When there are multiple tasks with the same priority,
	// the task CreatedAt attribute is considered.
	Priority int `json:"priority,omitempty"`

	// The firmware set ID is conditionally set at task initialization based on the FirmwareResolveMethod.
	FirmwareSetID string `json:"firmwareSetID,omitempty"`

	// Flasher determines the firmware to be installed for each component based on the firmware plan method,
	// The method is set at task initialization
	FirmwarePlanMethod FirmwarePlanMethod `json:"firmwarePlanMethod,omitempty"`

	// Device attributes
	Device Device `json:"-"`
}
