package model

import (
	"encoding/json"
	"time"

	sw "github.com/filanov/stateswitch"
	"github.com/google/uuid"

	rctypes "github.com/metal-toolbox/rivets/condition"
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
	StatePending   sw.State = sw.State(rctypes.Pending)
	StateActive    sw.State = sw.State(rctypes.Active)
	StateSucceeded sw.State = sw.State(rctypes.Succeeded)
	StateFailed    sw.State = sw.State(rctypes.Failed)
)

// Action holds attributes of a Task sub-statemachine
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
	state rctypes.State

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
	a.state = rctypes.State(state)

	return nil
}

func (a *Action) State() sw.State {
	return sw.State(a.state)
}

// Actions is a list of actions
type Actions []*Action

// ByID returns the Action matched by the identifier
func (a Actions) ByID(id string) *Action {
	for _, action := range a {
		if action.ID == id {
			return action
		}
	}

	return nil
}

// Task is a top level unit of work handled by flasher.
//
// A task performs one or more actions, each of the action corresponds to a Firmware planned for install.
//
// nolint:govet // fieldalignment - struct is better readable in its current form.
type Task struct {
	// Task unique identifier, this is set to the Condition identifier.
	ID uuid.UUID

	// state is the state of the install
	state rctypes.State

	// status holds informational data on the state
	Status StatusRecord

	// Flasher determines the firmware to be installed for each component based on the firmware plan method.
	FirmwarePlanMethod FirmwarePlanMethod

	// ActionsPlanned to be executed for task are generated from the InstallFirmwares and install parameters
	// these are generated in the `pending` stage of the task.
	ActionsPlanned Actions

	// Parameters for this task
	Parameters rctypes.FirmwareInstallTaskParameters

	// Fault is a field to inject failures into a flasher task execution,
	// this is set from the Condition only when the worker is run with fault-injection enabled.
	Fault *rctypes.Fault `json:"fault,omitempty"`

	CreatedAt   time.Time
	UpdatedAt   time.Time
	CompletedAt time.Time
}

// SetState implements the stateswitch statemachine interface
func (t *Task) SetState(state sw.State) error {
	t.state = rctypes.State(state)
	return nil
}

// State implements the stateswitch statemachine interface
func (t *Task) State() sw.State {
	return sw.State(t.state)
}

func NewTaskStatusRecord(s string) StatusRecord {
	sr := StatusRecord{}
	if s == "" {
		return sr
	}

	sr.Append(s)

	return sr
}

type StatusRecord struct {
	StatusMsgs []StatusMsg `json:"records"`
}

type StatusMsg struct {
	Timestamp time.Time `json:"ts,omitempty"`
	Msg       string    `json:"msg,omitempty"`
}

func (sr *StatusRecord) Append(s string) {
	if s == "" {
		return
	}

	for _, r := range sr.StatusMsgs {
		if r.Msg == s {
			return
		}
	}

	n := StatusMsg{Timestamp: time.Now(), Msg: s}

	sr.StatusMsgs = append(sr.StatusMsgs, n)
}

func (sr *StatusRecord) String() string {
	b, err := json.Marshal(sr)
	if err != nil {
		panic(err)
	}

	return string(b)
}
