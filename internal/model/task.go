package model

import (
	"time"

	"github.com/filanov/stateswitch"
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
	// and so no futher firmware planning is required.
	PlanUseDefinedFirmware FirmwarePlanMethod = "predefined"

	// PlanFromFirmwareSet is a TaskParameter attribute that indicates a
	// firmware set ID was provided at task initialization (through a CLI parameter or inventory device attribute)
	// the firmware versions to be installed are to be planned from the given firmware set ID.
	PlanFromFirmwareSet FirmwarePlanMethod = "fromFirmwareSet"

	// PlanFromInstalledFirmware is a TaskParameter attribute that indicates
	// the firmware versions to be installed have to be planned
	// based on the firmware currently installed on the device.
	PlanFromInstalledFirmware FirmwarePlanMethod = "fromInstalledFirmware"
)

// Action is part of a task, it is resolved from the Firmware configuration
//
// Actions transition through states as the action progresses
// those states are `queued`, `active`, `success`, `failed`.
type Action struct {
	// ID is a unique identifier for this action
	// e.g: bmc-<id>
	ID string

	// BMCTaskID ...

	// Status indicates the action status
	Status string

	// Firmware to be installed
	Firmware Firmware
}

func (a *Action) SetState(state stateswitch.State) error {
	a.Status = string(state)
	return nil
}

func (a *Action) State() stateswitch.State {
	return stateswitch.State(a.Status)
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

	// ActionsPlanned to be executed for task are generated from the Firmware configuration and install parameters
	// these are generated in the `queued` stage of the task.
	ActionsPlanned Actions

	// FirmwaresPlanned is the list of firmware planned for install.
	FirmwaresPlanned []Firmware

	// Parameters for this task
	Parameters TaskParameters

	// Device attributes
	Device Device

	CreatedAt   time.Time
	UpdatedAt   time.Time
	CompletedAt time.Time
}

func (t *Task) SetState(state stateswitch.State) error {
	t.Status = string(state)
	return nil
}

func (s *Task) State() stateswitch.State {
	return stateswitch.State(s.Status)
}

// NewTask returns a new Task
//
// method, device parameters are required.
// firmwareSetID, firmware are optional and mutually exclusive.
func NewTask(method InstallMethod, firmwareSetID string, firmware []Firmware) (Task, error) {
	task := Task{
		ID: uuid.New(),
		Parameters: TaskParameters{
			InstallMethod: method,
		},
	}

	if firmwareSetID != "" && len(firmware) > 0 {
		return task, errors.New("fasdsad")
	}

	if firmwareSetID != "" {
		task.Parameters.FirmwareSetID = firmwareSetID
		task.Parameters.FirmwarePlanMethod = PlanFromFirmwareSet
	}

	if len(firmware) > 0 {
		task.FirmwaresPlanned = firmware
		task.Parameters.FirmwarePlanMethod = PlanUseDefinedFirmware
	}

	if firmwareSetID == "" && len(firmware) == 0 {
		task.Parameters.FirmwarePlanMethod = PlanFromInstalledFirmware
	}

	return task, nil
}

// TaskParameters are the parameters set for each task flasher works
// these are parameters received from an operator which determines the task execution actions.
type TaskParameters struct {
	// Reset device BMC before firmware install
	ResetBMCBeforeInstall bool `json:"resetBMCBeforeInstall"`

	// Force install given firmware regardless of current firmware version.
	ForceInstall bool `json:"forceInstall"`

	// Task priority is the task priority between 0 and 3
	// where 0 is the default and 3 is the max.
	//
	// Tasks are picked from the `queued` state based on the priority.
	//
	// When there are multiple tasks with the same priority,
	// the task CreatedAt attribute is considered.
	Priority int `json:"priority"`

	// InstallMethod is one of inband/outofband
	InstallMethod InstallMethod `json:"installMethod"`

	// The firmware set ID is conditionally set at task initialization based on the FirmwareResolveMethod.
	FirmwareSetID string `json:"firmwareSetID"`

	// Flasher determines the firmware to be installed for each component based on the firmware plan method,
	// The method is set at task initialization
	FirmwarePlanMethod FirmwarePlanMethod `json:"firmwarePlanMethod"`
}
