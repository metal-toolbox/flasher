package model

import (
	"encoding/json"
	"reflect"

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

	TaskDataStructVersion = "1.0"
)

var (
	errTaskFirmwareParam = errors.New("firmware task parameters error")
)

// Alias parameterized model.Task
type Task rctypes.Task[*rctypes.FirmwareInstallTaskParameters, *TaskData]

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
	StructVersion string `json:"struct_version"`
	// Flasher determines the firmware to be installed for each component based on the firmware plan method.
	FirmwarePlanMethod FirmwarePlanMethod `json:"firmware_plan_method,omitempty"`

	// ActionsPlanned to be executed for each firmware to be installed.
	ActionsPlanned Actions `json:"actions_planned,omitempty"`

	// Scratch is an arbitrary key values map available to all task, action handler methods.
	Scratch map[string]string `json:"data,omitempty"`
}

func (td *TaskData) MapStringInterfaceToStruct(m map[string]interface{}) error {
	jsonData, err := json.Marshal(m)
	if err != nil {
		return err
	}

	return json.Unmarshal(jsonData, td)
}

func (td *TaskData) JSON() (json.RawMessage, error) {
	return json.Marshal(td)
}

func (td *TaskData) Unmarshal(r json.RawMessage) error {
	return json.Unmarshal(r, td)
}

func NewTask(conditionID uuid.UUID, kind rctypes.Kind, params *rctypes.FirmwareInstallTaskParameters) (Task, error) {
	t := Task{
		StructVersion: rctypes.TaskVersion1,
		ID:            conditionID,
		Kind:          kind,
		Data:          &TaskData{},
		Status:        rctypes.NewTaskStatusRecord("initialized task"),
		State:         StatePending,
		Parameters:    params,
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
	errTaskConv := errors.New("error in generic Task conversion")

	// convert task.Parameters which is of type json.RawMessage
	taskParamsMap, ok := task.Parameters.(map[string]interface{})
	if !ok {
		msg := "Task.Parameters expected to be of type map[string]interface{}, current type: " + reflect.TypeOf(task.Parameters).String()
		return nil, errors.Wrap(errTaskConv, msg)
	}

	fwInstallParams := &rctypes.FirmwareInstallTaskParameters{}
	if err := fwInstallParams.MapStringInterfaceToStruct(taskParamsMap); err != nil {
		return nil, errors.Wrap(errTaskConv, err.Error()+": Task.Parameters")
	}

	taskDataMap, ok := task.Data.(map[string]interface{})
	if !ok {
		msg := "Task.Data expected to be of type map[string]interface{}, current type: " + reflect.TypeOf(task.Data).String()
		return nil, errors.Wrap(errTaskConv, msg)
	}

	taskData := &TaskData{}
	if err := taskData.MapStringInterfaceToStruct(taskDataMap); err != nil {
		return nil, errors.Wrap(errTaskConv, err.Error()+": Task.Data")
	}

	return &Task{
		StructVersion: task.StructVersion,
		ID:            task.ID,
		Kind:          task.Kind,
		State:         task.State,
		Status:        task.Status,
		Data:          taskData,
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

func ConvToGenericTask(task *Task) (*rctypes.Task[any, any], error) {
	errTaskConv := errors.New("error in firmware install Task conversion")

	paramsJSON, err := task.Parameters.JSON()
	if err != nil {
		return nil, errors.Wrap(errTaskConv, err.Error()+": Task.Parameters")
	}

	dataJSON, err := task.Data.JSON()
	if err != nil {
		return nil, errors.Wrap(errTaskConv, err.Error()+": Task.Data")
	}

	return &rctypes.Task[any, any]{
		StructVersion: task.StructVersion,
		ID:            task.ID,
		Kind:          task.Kind,
		State:         task.State,
		Status:        task.Status,
		Data:          dataJSON,
		Parameters:    paramsJSON,
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
