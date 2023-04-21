package model

import (
	"time"

	sw "github.com/filanov/stateswitch"
	"github.com/google/uuid"
	cptypes "github.com/metal-toolbox/conditionorc/pkg/types"
	"go.hollow.sh/toolbox/events"
	"go.infratographer.com/x/pubsubx"
	"go.infratographer.com/x/urnx"
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

	// FromFirmwareSet is a TaskParameter attribute that declares the
	// the firmware versions to be installed are to be planned from the given firmware set ID.
	FromFirmwareSet FirmwarePlanMethod = "fromFirmwareSet"

	// FromRequestedFirmware is a TaskParameter attribute that declares the
	// firmware versions to be installed have been defined as part of the request,
	// and so no further firmware planning is required.
	FromRequestedFirmware FirmwarePlanMethod = "fromRequestedFirmware"

	// task states
	//
	// states the task state machine transitions through
	StatePending   sw.State = sw.State(cptypes.Pending)
	StateActive    sw.State = sw.State(cptypes.Active)
	StateSucceeded sw.State = sw.State(cptypes.Succeeded)
	StateFailed    sw.State = sw.State(cptypes.Failed)
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

	// BMCTaskID is the task identifier to track a BMC job
	// these are returned whent he firmware install is initiated on the BMC.
	BMCTaskID string

	// Method of install
	InstallMethod InstallMethod

	// status indicates the action state
	state cptypes.ConditionState

	// Firmware to be installed, this is set in the Task Plan phase.
	Firmware Firmware

	// FirwareTempFile is the temporary file downloaded to be installed.
	//
	// This is declared once the firmware file has been downloaded for install.
	FirmwareTempFile string

	// VerifyCurrentFirmware will cause the action to verify the current firmware
	// on the component is not equal to one being installed. If its equal, the action will return an error.
	VerifyCurrentFirmware bool

	// BMCPowerCycleRequired is set when an action handler determines the BMC requires a reset.
	BMCPowerCycleRequired bool

	// HostPowerCycleRequired is set when an action handler determines the Host requires a reset.
	HostPowerCycleRequired bool

	// Final is set to true when its the last action being executed
	Final bool
}

func (a *Action) SetState(state sw.State) error {
	a.state = cptypes.ConditionState(state)

	return nil
}

func (a *Action) State() sw.State {
	return sw.State(a.state)
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
// A task performs one or more actions, each of the action corresponds to a Firmware planned for install.
type Task struct {
	// Task unique identifier, this is set to the Condition identifier.
	ID uuid.UUID

	// state is the state of the install
	state cptypes.ConditionState

	// status holds informational data on the state
	Status string

	// Flasher determines the firmware to be installed for each component based on the firmware plan method.
	FirmwarePlanMethod FirmwarePlanMethod

	// ActionsPlanned to be executed for task are generated from the InstallFirmwares and install parameters
	// these are generated in the `pending` stage of the task.
	ActionsPlanned Actions

	// InstallFirmwares is the list of firmware planned for install.
	InstallFirmwares []*Firmware

	// Parameters for this task
	Parameters TaskParameters

	CreatedAt   time.Time
	UpdatedAt   time.Time
	CompletedAt time.Time
}

// StreamEvent holds properties of a message recieved on the stream.
type StreamEvent struct {
	// Msg is the original message that created this task.
	// This is here so that the events subsystem can be acked/notified as the task makes progress.
	Msg events.Message

	// Data is the data parsed from Msg for the task runner.
	Data *pubsubx.Message

	// Condition defines the kind of work to be performed,
	// its parsed from the message.
	Condition *cptypes.Condition

	// Urn is the URN parsed from Msg for the task runner.
	Urn *urnx.URN
}

// SetState implements the stateswitch statemachine interface
func (t *Task) SetState(state sw.State) error {
	t.state = cptypes.ConditionState(state)
	return nil
}

// State implements the stateswitch statemachine interface
func (t *Task) State() sw.State {
	return sw.State(t.state)
}

// TaskParameters are the parameters set for each task flasher works
// these are parameters received from an operator which determines the task execution actions.
type TaskParameters struct {
	// Inventory identifier for the asset to install firmware on.
	AssetID uuid.UUID `json:"assetID"`

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

	// Firmwares is the list of firmwares to be installed.
	Firmwares []Firmware `json:"firmwares,omitempty"`

	// FirmwareSetID specifies the firmware set to be applied.
	FirmwareSetID uuid.UUID `json:"firmwareSetID,omitempty"`
}
