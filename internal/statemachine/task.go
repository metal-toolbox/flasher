package statemachine

import (
	"context"
	"fmt"
	"time"

	sw "github.com/filanov/stateswitch"
	"github.com/metal-toolbox/flasher/internal/inventory"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/store"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	TransitionTypeQuery sw.TransitionType = "query"
	TransitionTypePlan  sw.TransitionType = "plan"
	TransitionTypeRun   sw.TransitionType = "run"

	// transition for successful tasks
	TransitionTypeTaskSuccess sw.TransitionType = "success"
	// transition for failed tasks
	TransitionTypeTaskFail sw.TransitionType = "failed"
)

var (
	// errors
	ErrInvalidTransitionHandler  = errors.New("expected a valid transitionHandler{} type")
	ErrInvalidtaskHandlerContext = errors.New("expected a HandlerContext{} type")
	ErrTaskTransition            = errors.New("error in task transition")
)

// TaskEvents are events sent by tasks and actions, they include information
// on the task, action being executed
type TaskEvent struct {
	TaskID string
	Info   string
}

func SendEvent(ctx context.Context, ch chan<- TaskEvent, e TaskEvent) {
	if ch == nil {
		return
	}

	select {
	// case <-ctx.Done():
	//	return
	case ch <- e:
	case <-time.After(1 * time.Second):
		fmt.Println("dropped event with info: " + e.Info)
		return
	}
}

// sw library drawbacks
// - when running the sw statemachine - each transition has be run and so the Run method below
// - Conditions do not fallthrough

// HandlerContext holds working attributes of a task
//
// This struct is passed to transition handlers which
// depend on the values provided in this struct.
type HandlerContext struct {
	// ctx is the parent cancellation context
	Ctx context.Context

	// Dryrun skips any disruptive actions on the device - power on/off, bmc resets, firmware installs,
	// the task and its actions run as expected, and the device state in the inventory is updated as well,
	// although the firmware is not installed.
	//
	// It is upto the Action handler implementations to ensure the dry run works as described.
	Dryrun bool

	// WorkerID is the identifier of the worker running this task.
	WorkerID string

	// taskID is available when an action is invoked under a task.
	TaskID string

	// This value is prefixed to the path generated to download
	// the firmware to install.
	FirmwareURLPrefix string

	// plan is an ordered list of actions planned to complete this task.
	ActionStateMachines ActionStateMachines

	// err is set when a transition fails in run()
	Err error

	// DeviceQueryor is an interface run queries on a device.
	DeviceQueryor model.DeviceQueryor

	// Data is key values the handler may decide to store
	// so as to record, read handler specific values.
	Data map[string]string

	// Device is the device this task is executing on.
	Device *model.Device

	TaskEventCh chan TaskEvent
	Store       store.Storage
	Inv         inventory.Inventory
	Logger      *logrus.Entry
}

// TaskTransitioner defines stateswitch methods that handle state transitions.
type TaskTransitioner interface {
	// Query queries information for planning task actions.
	Query(task sw.StateSwitch, args sw.TransitionArgs) error

	// Plan creates a set of task actions to be executed.
	Plan(task sw.StateSwitch, args sw.TransitionArgs) error

	// ValidatePlan is called before invoking Run.
	ValidatePlan(task sw.StateSwitch, args sw.TransitionArgs) (bool, error)

	// Run executes the task actions.
	Run(task sw.StateSwitch, args sw.TransitionArgs) error

	// PersistState persists the task status
	PersistState(task sw.StateSwitch, args sw.TransitionArgs) error

	// TaskFailed is called when the task fails.
	TaskFailed(task sw.StateSwitch, args sw.TransitionArgs) error

	// TaskSuccessful is called when th task succeeds.
	TaskSuccessful(task sw.StateSwitch, args sw.TransitionArgs) error
}

// TaskStateMachine drives the task
type TaskStateMachine struct {
	sm sw.StateMachine

	transitions []sw.TransitionType
}

func NewTaskStateMachine(ctx context.Context, task *model.Task, handler TaskTransitioner) (*TaskStateMachine, error) {
	// transitions are executed in this order
	transitionOrder := []sw.TransitionType{
		TransitionTypeQuery,
		TransitionTypePlan,
		TransitionTypeRun,
	}

	m := &TaskStateMachine{sm: sw.NewStateMachine(), transitions: transitionOrder}

	// The SM has transition rules define the transitionHandler methods
	// each transitionHandler method is passed as values to the transition rule.

	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   TransitionTypeQuery,
		SourceStates:     sw.States{model.StateActive},
		DestinationState: model.StateActive,

		// Condition for the transition, transition will be executed only if this function return true
		// Can be nil, in this case it's considered as return true, nil
		Condition: nil,

		// Transition is users business logic, should not set the state or return next state
		// If condition returns true this function will be executed
		Transition: handler.Query,

		// PostTransition will be called if condition and transition are successful.
		PostTransition: handler.PersistState,
	})

	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   TransitionTypePlan,
		SourceStates:     sw.States{model.StateActive},
		DestinationState: model.StateActive,
		Condition:        nil,
		Transition:       handler.Plan,
		PostTransition:   handler.PersistState,
	})

	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   TransitionTypeRun,
		SourceStates:     sw.States{model.StateActive},
		DestinationState: model.StateSuccess,
		Condition:        handler.ValidatePlan,
		Transition:       handler.Run,
		PostTransition:   handler.PersistState,
	})

	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   TransitionTypeTaskFail,
		SourceStates:     sw.States{model.StateQueued, model.StateActive},
		DestinationState: model.StateFailed,
		Condition:        nil,
		Transition:       handler.TaskFailed,
		PostTransition:   handler.PersistState,
	})

	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   TransitionTypeTaskSuccess,
		SourceStates:     sw.States{model.StateActive},
		DestinationState: model.StateSuccess,
		Condition:        nil,
		Transition:       handler.TaskSuccessful,
		PostTransition:   handler.PersistState,
	})

	return m, nil
}

func (m *TaskStateMachine) SetTransitionOrder(transitions []sw.TransitionType) {
	m.transitions = transitions
}

func (m *TaskStateMachine) TransitionFailed(ctx context.Context, task *model.Task, tctx *HandlerContext) error {
	return m.sm.Run(TransitionTypeTaskFail, task, tctx)
}

func (m *TaskStateMachine) TransitionSuccess(ctx context.Context, task *model.Task, tctx *HandlerContext) error {
	return m.sm.Run(TransitionTypeTaskSuccess, task, tctx)
}

func (m *TaskStateMachine) Run(ctx context.Context, task *model.Task, handler TaskTransitioner, tctx *HandlerContext) error {
	var err error
	var finalTransition sw.TransitionType

	for _, transitionType := range m.transitions {
		err = m.sm.Run(transitionType, task, tctx)
		if err != nil {
			err = errors.Wrap(err, string(transitionType))
			// update error to include some useful context
			if errors.Is(err, sw.NoConditionPassedToRunTransaction) {
				err = errors.Wrap(
					ErrTaskTransition,
					fmt.Sprintf("no transition rule found for task transition type '%s' and state '%s'", transitionType, task.Status),
				)
			}

			// include error in task
			task.Info = err.Error()

			// run transition failed handler
			if txErr := m.TransitionFailed(ctx, task, tctx); txErr != nil {
				err = errors.Wrap(err, string(TransitionTypeActionFailed)+": "+txErr.Error())
			}

			return err
		}
	}

	// run transition success handler when the final successful transition is as expected
	if finalTransition == TransitionTypeRun {
		if err := m.TransitionSuccess(ctx, task, tctx); err != nil {
			return errors.Wrap(err, string(TransitionTypeActionSuccess)+": "+err.Error())
		}
	}

	return nil
}
