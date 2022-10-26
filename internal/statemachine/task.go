package statemachine

import (
	"context"
	"fmt"

	sw "github.com/filanov/stateswitch"
	"github.com/metal-toolbox/flasher/internal/inventory"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/store"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	TransitionTypePlan sw.TransitionType = "plan"
	TransitionTypeRun  sw.TransitionType = "run"

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

// sw library drawbacks
// - when running the sw statemachine - each transition has be run and so the Run method below
// - Conditions do not fallthrough

// HandlerContext holds working attributes of a task
//
// This struct is passed to transition handlers which
// depend on the values provided in this struct.
type HandlerContext struct {
	// WorkerName is the name of the worker running this task.
	WorkerName string

	// taskID is available when an action is invoked under a task.
	TaskID string

	// ctx is the parent cancellation context
	Ctx context.Context

	// plan is an ordered list of actions planned to complete this task.
	ActionStateMachines ActionStateMachines

	// err is set when a transition fails in run()
	Err error

	// DeviceQueryor is an interface run queries on a device.
	DeviceQueryor model.DeviceQueryor

	// Data is key values the handler may decide to store
	// so as to record, read handler specific values.
	Data map[string]string

	Store  store.Storage
	Inv    inventory.Inventory
	Logger *logrus.Entry
}

// TaskTransitioner defines stateswitch methods that handle state transitions.
type TaskTransitioner interface {
	Plan(task sw.StateSwitch, args sw.TransitionArgs) error
	Run(task sw.StateSwitch, args sw.TransitionArgs) error
	PersistState(task sw.StateSwitch, args sw.TransitionArgs) error
	TaskFailed(task sw.StateSwitch, args sw.TransitionArgs) error
	TaskSuccessful(task sw.StateSwitch, args sw.TransitionArgs) error
	Validate(task sw.StateSwitch, args sw.TransitionArgs) (bool, error)
}

// TaskStateMachine drives the task
type TaskStateMachine struct {
	sm          sw.StateMachine
	transitions []sw.TransitionType
}

// ActionStateMachine drives the firmware install actions

func NewTaskStateMachine(ctx context.Context, task *model.Task, handler TaskTransitioner) (*TaskStateMachine, error) {
	// transitions are executed in this order
	transitionOrder := []sw.TransitionType{
		TransitionTypePlan,
		TransitionTypeRun,
	}

	m := &TaskStateMachine{sm: sw.NewStateMachine(), transitions: transitionOrder}

	// The SM has transition rules define the transitionHandler methods
	// each transitionHandler method is passed as values to the transition rule.

	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   TransitionTypePlan,
		SourceStates:     sw.States{model.StateQueued},
		DestinationState: model.StateActive,

		// Condition for the transition, transition will be executed only if this function return true
		// Can be nil, in this case it's considered as return true, nil
		Condition: nil,

		// Transition is users business logic, should not set the state or return next state
		// If condition returns true this function will be executed
		Transition: handler.Plan,

		// PostTransition will be called if condition and transition are successful.
		PostTransition: handler.PersistState,
	})

	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   TransitionTypeRun,
		SourceStates:     sw.States{model.StateActive},
		DestinationState: model.StateSuccess,
		Condition:        handler.Validate,
		Transition:       handler.Run,
		PostTransition:   handler.PersistState,
	})

	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   TransitionTypeTaskFail,
		SourceStates:     sw.States{model.StateActive, model.StateQueued},
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

	// To ensure that the task state is saved when sm.Run fails
	// because of a invalid state transition attempt,
	//
	// the error for these cases is in the form,
	// 'no transition rule found for transition type 'Run' to state 'success': no condition found to run transition'
	//
	// The error is returned to the caller and the task is marked as FailedState
	//
	// TODO(joel): extend the stateswitch library to have an OnFailure handler and remove this defer
	//	defer func() {
	//		if err != nil {
	//			// save the handler error in the task information
	//			task.Info = err.Error()
	//
	//			// save task state, wrap error if any
	//			if errSetState := task.SetState(sw.State(model.StateFailed)); errSetState != nil {
	//				err = errors.Wrap(errSetState, err.Error())
	//			}
	//
	//			if errSaveState := handler.PersistState(task, tctx); errSaveState != nil {
	//				err = errors.Wrap(errSaveState, err.Error())
	//			}
	//		}
	//	}()

	var finalTransition sw.TransitionType

	for _, transitionType := range m.transitions {
		err = m.sm.Run(transitionType, task, tctx)
		if err != nil {
			err = errors.Wrap(err, string(transitionType))
			// update error to include some useful context
			if errors.Is(err, sw.NoConditionPassedToRunTransaction) {
				err = errors.Wrap(
					ErrTaskTransition,
					fmt.Sprintf("no transition rule found for transition type '%s' and state '%s'", transitionType, task.Status),
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
