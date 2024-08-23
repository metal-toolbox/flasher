package model

import (
	"encoding/json"
	"reflect"

	"github.com/google/uuid"
	"github.com/mitchellh/copystructure"
	"github.com/pkg/errors"

	rctypes "github.com/metal-toolbox/rivets/condition"
	rtypes "github.com/metal-toolbox/rivets/types"
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

	// This flag is set when a action requires a host power cycle.
	HostPowercycleRequired bool `json:"host_powercycle_required,omitempty"`

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

func (td *TaskData) Marshal() (json.RawMessage, error) {
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

func convTaskParams(params any) (*rctypes.FirmwareInstallTaskParameters, error) {
	errParamsConv := errors.New("error in Task.Parameters conversion")

	fwInstallParams := &rctypes.FirmwareInstallTaskParameters{}
	switch v := params.(type) {
	// When unpacked from a http request by the condition orc client,
	// Parameters are of this type.
	case map[string]interface{}:
		if err := fwInstallParams.MapStringInterfaceToStruct(v); err != nil {
			return nil, errors.Wrap(errParamsConv, err.Error())
		}
	// When received over NATS its of this type.
	case json.RawMessage:
		if err := fwInstallParams.Unmarshal(v); err != nil {
			return nil, errors.Wrap(errParamsConv, err.Error())
		}
	default:
		msg := "Task.Parameters expected to be one of map[string]interface{} or json.RawMessage, current type: " + reflect.TypeOf(params).String()
		return nil, errors.Wrap(errParamsConv, msg)
	}

	return fwInstallParams, nil
}

func convTaskData(data any) (*TaskData, error) {
	errDataConv := errors.New("error in Task.Data conversion")

	taskData := &TaskData{}
	switch v := data.(type) {
	// When unpacked from a http request by the condition orc client,
	// Parameters are of this type.
	case map[string]interface{}:
		if err := taskData.MapStringInterfaceToStruct(v); err != nil {
			return nil, errors.Wrap(errDataConv, err.Error())
		}
	// When received over NATS its of this type.
	case json.RawMessage:
		if err := taskData.Unmarshal(v); err != nil {
			return nil, errors.Wrap(errDataConv, err.Error())
		}
	default:
		msg := "Task.Data expected to be one of map[string]interface{} or json.RawMessage, current type: " + reflect.TypeOf(data).String()
		return nil, errors.Wrap(errDataConv, msg)
	}

	return taskData, nil
}

func CopyAsFwInstallTask(task *rctypes.Task[any, any]) (*Task, error) {
	errTaskConv := errors.New("error in generic Task conversion")

	params, err := convTaskParams(task.Parameters)
	if err != nil {
		return nil, errors.Wrap(errTaskConv, err.Error())
	}

	data, err := convTaskData(task.Data)
	if err != nil {
		return nil, errors.Wrap(errTaskConv, err.Error())
	}

	// deep copy fields referenced by pointer
	asset, err := copystructure.Copy(task.Server)
	if err != nil {
		return nil, errors.Wrap(errTaskConv, err.Error()+": Task.Server")
	}

	fault, err := copystructure.Copy(task.Fault)
	if err != nil {
		return nil, errors.Wrap(errTaskConv, err.Error()+": Task.Fault")
	}

	if len(params.Firmwares) > 0 {
		data.FirmwarePlanMethod = FromRequestedFirmware
	}

	if params.FirmwareSetID != uuid.Nil && len(params.Firmwares) == 0 {
		data.FirmwarePlanMethod = FromFirmwareSet
	}

	return &Task{
		StructVersion: task.StructVersion,
		ID:            task.ID,
		Kind:          task.Kind,
		State:         task.State,
		Status:        task.Status,
		Data:          data,
		Parameters:    params,
		Fault:         fault.(*rctypes.Fault),
		FacilityCode:  task.FacilityCode,
		Server:        asset.(*rtypes.Server),
		WorkerID:      task.WorkerID,
		TraceID:       task.TraceID,
		SpanID:        task.SpanID,
		CreatedAt:     task.CreatedAt,
		UpdatedAt:     task.UpdatedAt,
		CompletedAt:   task.CompletedAt,
	}, nil
}

func CopyAsGenericTask(task *Task) (*rctypes.Task[any, any], error) {
	errTaskConv := errors.New("error in firmware install Task conversion")

	paramsJSON, err := task.Parameters.Marshal()
	if err != nil {
		return nil, errors.Wrap(errTaskConv, err.Error()+": Task.Parameters")
	}

	dataJSON, err := task.Data.Marshal()
	if err != nil {
		return nil, errors.Wrap(errTaskConv, err.Error()+": Task.Data")
	}

	// deep copy fields referenced by pointer
	asset, err := copystructure.Copy(task.Server)
	if err != nil {
		return nil, errors.Wrap(errTaskConv, err.Error()+": Task.Server")
	}

	fault, err := copystructure.Copy(task.Fault)
	if err != nil {
		return nil, errors.Wrap(errTaskConv, err.Error()+": Task.Fault")
	}

	return &rctypes.Task[any, any]{
		StructVersion: task.StructVersion,
		ID:            task.ID,
		Kind:          task.Kind,
		State:         task.State,
		Status:        task.Status,
		Data:          dataJSON,
		Parameters:    paramsJSON,
		Fault:         fault.(*rctypes.Fault),
		FacilityCode:  task.FacilityCode,
		Server:        asset.(*rtypes.Server),
		WorkerID:      task.WorkerID,
		TraceID:       task.TraceID,
		SpanID:        task.SpanID,
		CreatedAt:     task.CreatedAt,
		UpdatedAt:     task.UpdatedAt,
		CompletedAt:   task.CompletedAt,
	}, nil
}
