package model

import (
	"time"

	"github.com/filanov/stateswitch"
	"github.com/google/uuid"
)

const (
	// ResolveMethodPredefinedVersions is a task firmware resolve method where
	// the firmware versions to be installed were pre defined at task initialization
	ResolveMethodPredefinedVersions = "ResolveMethodPredefinedVersions"

	// ResolveMethodPredefinedFirmwareSet is a task firmware resolve method where
	// the firmware versions to be installed are identified from the given firmware set.
	ResolveMethodPredefinedFirmwareSet = "ResolveMethodPredefinedFirmwareSet"

	// ResolveMethodResolveFirmwareVersions is a task firmware resolve method where
	// flasher identifies the firmware set and firmware versions for install from that set.
	ResolveMethodResolveFirmwareVersions = "ResolveMethodResolveFirmwareVersions"
)

// Task is a top level unit of work handled by flasher.
//
// A task performs one or more actions, each of the action corresponds to a Firmware
type Task struct {
	// Task unique identifier
	ID uuid.UUID

	// Status is the install status
	Status string

	// Actions to be executed for task are generated from the Firmware configuration and install parameters
	// these are generated in the `queued` stage of the task.
	Actions []Action

	// FirmwareResolved is the list of firmware to be installed based on the TaskParameters.
	FirmwareResolved []Firmware

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

// TaskParameters are the parameters set for each task flasher works
// these are parameters recieved from an operator which determines the task execution actions.
type TaskParameters struct {
	// Task priority is the task priority between 0 and 3
	// where 0 is the default and 3 is the max.
	//
	// Tasks are picked from the `queued` state based on the priority.
	//
	// When there are multiple tasks with the same priority,
	// the task CreatedAt attribute is considered.
	Priority int `json:"priority"`

	// Method is one of in-band/out-of-band
	Method string `json:"method"`

	// The firmware set ID is conditionally set at task initialization based on the FirmwareResolveMethod.
	FirmwareSetID string `json:"firmwareSetID"`

	// InstallVersions when defined sets the FirmwareResolveMethod to `ResolveMethodPredefinedVersions`,
	// which means that firmware sets will not be looked up and firmware
	InstallVersions []Firmware `json:"installVersions"`

	// Flasher determines the firmware to be installed for each component based on one of these three methods,
	// These modes are set at task initialization
	FirmwareResolveMethod string `json:"firmwareResolveMethod"`

	// Reset device BMC before firmware install
	ResetBMCBeforeInstall bool `json:"ResetBMCBeforeInstall"`

	// Force install given firmware regardless of current firmware version.
	ForceInstall bool `json:"ForceInstall"`
}

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

//type Actions []Action
//
//func (a Actions) ByTaskID(taskID string) *Action {
//	for _, action := range a {
//		if action.TaskID == taskID {
//			return &action
//		}
//	}
//
//	return nil
//}
