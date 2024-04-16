package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"

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
	InstallMethodInband    InstallMethod = "inband"

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
	StatePending   = rctypes.Pending
	StateActive    = rctypes.Active
	StateSucceeded = rctypes.Succeeded
	StateFailed    = rctypes.Failed

	TaskVersion = "0.1"
)

var (
	errTaskFirmwareParam = errors.New("firmware task parameters error")
)

// Task is a top level unit of work handled by flasher.
//
// A task performs one or more actions, each of the action corresponds to a Firmware planned for install.
//
// nolint:govet // fieldalignment - struct is better readable in its current form.
type Task struct {
	// StructVersion indicates the Task object version and is used to determine Task  compatibility.
	StructVersion string `json:"task_version"`

	// Task unique identifier, this is set to the Condition identifier.
	ID uuid.UUID `json:"id"`

	// state is the state of the install
	State rctypes.State `json:"state"`

	// status holds informational data on the state
	Status StatusRecord `json:"status"`

	// Flasher determines the firmware to be installed for each component based on the firmware plan method.
	FirmwarePlanMethod FirmwarePlanMethod `json:"firmware_plan_method,omitempty"`

	// ActionsPlanned to be executed for each firmware to be installed.
	ActionsPlanned Actions `json:"actions_planned,omitempty"`

	// Parameters for this task
	Parameters rctypes.FirmwareInstallTaskParameters `json:"parameters,omitempty"`

	// Fault is a field to inject failures into a flasher task execution,
	// this is set from the Condition only when the worker is run with fault-injection enabled.
	Fault *rctypes.Fault `json:"fault,omitempty"`

	// FacilityCode identifies the facility this task is to be executed in.
	FacilityCode string `json:"facility_code"`

	// Data is an arbitrary key values map available to all task, action handler methods.
	Data map[string]string `json:"data,omitempty"`

	// Asset holds attributes about the device under firmware install.
	Asset *Asset `json:"asset,omitempty"`

	// WorkerID is the identifier for the worker executing this task.
	WorkerID string `json:"worker_id,omitempty"`

	CreatedAt   time.Time `json:"created_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

// SetState implements the Task runner interface
func (t *Task) SetState(state rctypes.State) error {
	t.State = state
	return nil
}

func NewTask(conditionID uuid.UUID, params *rctypes.FirmwareInstallTaskParameters) (Task, error) {
	t := Task{
		StructVersion: TaskVersion,
		ID:            conditionID,
		Status:        NewTaskStatusRecord("initialized task"),
		State:         StatePending,
		Parameters:    *params,
		Data:          make(map[string]string),
	}

	if len(params.Firmwares) > 0 {
		t.Parameters.Firmwares = params.Firmwares
		t.FirmwarePlanMethod = FromRequestedFirmware

		return t, nil
	}

	if params.FirmwareSetID != uuid.Nil {
		t.Parameters.FirmwareSetID = params.FirmwareSetID
		t.FirmwarePlanMethod = FromFirmwareSet

		return t, nil
	}

	return t, errors.Wrap(errTaskFirmwareParam, "no firmware list or firmwareSetID specified")
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

	if len(sr.StatusMsgs) > 4 {
		sr.StatusMsgs = sr.StatusMsgs[1:]
	}

	n := StatusMsg{Timestamp: time.Now(), Msg: s}

	sr.StatusMsgs = append(sr.StatusMsgs, n)
}

func (sr *StatusRecord) Last() string {
	if len(sr.StatusMsgs) == 0 {
		return ""
	}

	return sr.StatusMsgs[len(sr.StatusMsgs)-1].Msg
}

func (sr *StatusRecord) Update(currentMsg, newMsg string) {
	for idx, r := range sr.StatusMsgs {
		if r.Msg == currentMsg {
			sr.StatusMsgs[idx].Msg = newMsg
		}
	}
}

func (sr *StatusRecord) MustMarshal() json.RawMessage {
	b, err := json.Marshal(sr)
	if err != nil {
		panic(err)
	}

	return b
}
