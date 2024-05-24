package model

import (
	"encoding/json"

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
)

var (
	errTaskFirmwareParam = errors.New("firmware task parameters error")
)

// Task is a top level unit of work handled by flasher.
//
// A task performs one or more actions, each of the action corresponds to a Firmware planned for install.
//
// nolint:govet // fieldalignment - struct is better readable in its current form.
//type Task struct {
//	// StructVersion indicates the Task object version and is used to determine Task  compatibility.
//	StructVersion string `json:"task_version"`
//
//	// Task unique identifier, this is set to the Condition identifier.
//	ID uuid.UUID `json:"id"`
//
//	// state is the state of the install
//	State rctypes.State `json:"state"`
//
//	// status holds informational data on the state
//	Status StatusRecord `json:"status"`
//
//	// Flasher determines the firmware to be installed for each component based on the firmware plan method.
//	FirmwarePlanMethod FirmwarePlanMethod `json:"firmware_plan_method,omitempty"`
//
//	// ActionsPlanned to be executed for each firmware to be installed.
//	ActionsPlanned Actions `json:"actions_planned,omitempty"`
//
//	// Parameters for this task
//	Parameters rctypes.FirmwareInstallTaskParameters `json:"parameters,omitempty"`
//
//	// Fault is a field to inject failures into a flasher task execution,
//	// this is set from the Condition only when the worker is run with fault-injection enabled.
//	Fault *rctypes.Fault `json:"fault,omitempty"`
//
//	// FacilityCode identifies the facility this task is to be executed in.
//	FacilityCode string `json:"facility_code"`
//
//	// Data is an arbitrary key values map available to all task, action handler methods.
//	Data map[string]string `json:"data,omitempty"`
//
//	// Asset holds attributes about the device under firmware install.
//	Asset *Asset `json:"asset,omitempty"`
//
//	// WorkerID is the identifier for the worker executing this task.
//	WorkerID string `json:"worker_id,omitempty"`
//
//	// Delegations holds the statuses for each of the conditions delegated by this task
//	Delegations []*rctypes.StatusValue
//
//	CreatedAt   time.Time `json:"created_at,omitempty"`
//	UpdatedAt   time.Time `json:"updated_at,omitempty"`
//	CompletedAt time.Time `json:"completed_at,omitempty"`
//}

// Alias parameterized model.Task
type Task rctypes.Task[rctypes.FirmwareInstallTaskParameters, *TaskData]

func (t *Task) SetState(s rctypes.State) {
	t.State = s
}

func (t *Task) MustMarshal() json.RawMessage {
	b, err := json.Marshal(t)
	if err != nil {
		panic(err)
	}

	return b
}

type TaskData struct {
	// Flasher determines the firmware to be installed for each component based on the firmware plan method.
	FirmwarePlanMethod FirmwarePlanMethod `json:"firmware_plan_method,omitempty"`

	// ActionsPlanned to be executed for each firmware to be installed.
	ActionsPlanned Actions `json:"actions_planned,omitempty"`

	// Scratch is an arbitrary key values map available to all task, action handler methods.
	Scratch map[string]string `json:"data,omitempty"`
}

func NewTask(conditionID uuid.UUID, kind rctypes.Kind, params *rctypes.FirmwareInstallTaskParameters) (Task, error) {
	t := Task{
		StructVersion: rctypes.TaskVersion1,
		ID:            conditionID,
		Kind:          kind,
		Status:        rctypes.NewTaskStatusRecord("initialized task"),
		State:         StatePending,
		Parameters:    *params,
	}

	t.Data.Scratch = make(map[string]string)
	if len(params.Firmwares) > 0 {
		t.Parameters.Firmwares = params.Firmwares
		t.Data.FirmwarePlanMethod = FromRequestedFirmware

		return t, nil
	}

	if params.FirmwareSetID != uuid.Nil {
		t.Parameters.FirmwareSetID = params.FirmwareSetID
		t.Data.FirmwarePlanMethod = FromFirmwareSet

		return t, nil
	}

	return t, errors.Wrap(errTaskFirmwareParam, "no firmware list or firmwareSetID specified")
}

func ConvToFwInstallTask(task *rctypes.Task[any, any]) (*Task, error) {
	fwInstallParams, ok := task.Parameters.(rctypes.FirmwareInstallTaskParameters)
	if !ok {
		return nil, errors.New("parameters are not of type FirmwareInstallTaskParameters")
	}

	return &Task{
		StructVersion: task.StructVersion,
		ID:            task.ID,
		Kind:          task.Kind,
		State:         task.State,
		Status:        task.Status,
		Data:          &TaskData{},
		Parameters:    fwInstallParams,
		Fault:         task.Fault,
		FacilityCode:  task.FacilityCode,
		Asset:         task.Asset,
		WorkerID:      task.WorkerID,
		TraceID:       task.TraceID,
		SpanID:        task.SpanID,
		CreatedAt:     task.CreatedAt,
		UpdatedAt:     task.UpdatedAt,
		CompletedAt:   task.CompletedAt,
	}, nil
}

func ConvToGenericTask(task *Task) *rctypes.Task[any, any] {
	return &rctypes.Task[any, any]{
		StructVersion: task.StructVersion,
		ID:            task.ID,
		Kind:          task.Kind,
		State:         task.State,
		Status:        task.Status,
		Data:          task.Data,
		Parameters:    task.Parameters,
		Fault:         task.Fault,
		FacilityCode:  task.FacilityCode,
		Asset:         task.Asset,
		WorkerID:      task.WorkerID,
		TraceID:       task.TraceID,
		SpanID:        task.SpanID,
		CreatedAt:     task.CreatedAt,
		UpdatedAt:     task.UpdatedAt,
		CompletedAt:   task.CompletedAt,
	}
}
