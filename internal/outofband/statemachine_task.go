package outofband

import (
	"context"
	"fmt"

	"github.com/filanov/stateswitch"
	sw "github.com/filanov/stateswitch"
	"github.com/metal-toolbox/flasher/internal/inventory"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/store"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	// task states
	//
	// states the task transitions through
	stateQueued  sw.State = "queued"
	stateActive  sw.State = "active"
	stateSuccess sw.State = "success"
	stateFailed  sw.State = "failed"

	planActions sw.TransitionType = "planActions"
	runActions  sw.TransitionType = "runActions"
	taskFailed  sw.TransitionType = "taskFailed"
)

var (
	// errors
	errInvalidTransitionHandler = errors.New("expected a valid transitionHandler{} type")

	errInvalidtaskHandlerContext = errors.New("expected a taskHandlerContext{} type")

	errTaskInstallParametersUndefined = errors.New("task install parameters undefined")
)

// ActionStateMachineList is an ordered map of taskIDs -> Action IDs -> Action state machine
type ActionStateMachineList []map[string]map[string]*ActionStateMachine

// taskHandlerContext holds working attributes of a task
//
// This struct is passed to transition handlers which
// depend on the values provided in this struct.
type taskHandlerContext struct {
	// taskID is only available when an action is invoked under a task.
	taskID string

	// ctx is the parent context
	ctx context.Context

	// action plan is an ordered list of action state machines
	// this is populated in the `init` state
	actionPlan ActionStateMachineList

	// err is set when a transition fails in run()
	err error

	cache  store.Storage
	inv    inventory.Inventory
	logger *logrus.Logger
}

// TaskStateMachine drives the task
type TaskStateMachine struct {
	sm          sw.StateMachine
	transitions []sw.TransitionType
}

// ActionStateMachine drives the firmware install actions

func (m *TaskStateMachine) TransitionFailed(ctx context.Context, task *model.Task, tctx *taskHandlerContext) error {
	return m.sm.Run(taskFailed, task, tctx)
}

func NewTaskStateMachine(ctx context.Context, task *model.Task, handler taskTransitioner) (*TaskStateMachine, error) {
	// transitions are executed in this order
	transitionOrder := []sw.TransitionType{
		planActions,
		runActions,
	}

	m := &TaskStateMachine{sm: sw.NewStateMachine(), transitions: transitionOrder}

	// The SM has transition rules define the transitionHandler methods
	// each transitionHandler method is passed as values to the transition rule.
	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   planActions,
		SourceStates:     sw.States{stateQueued},
		DestinationState: stateActive,

		// Condition for the transition, transition will be executed only if this function return true
		// Can be nil, in this case it's considered as return true, nil
		Condition: nil,

		// Transition is users business logic, should not set the state or return next state
		// If condition returns true this function will be executed
		Transition: handler.planActions,

		// PostTransition will be called if condition and transition are successful.
		PostTransition: handler.saveState,
	})

	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   runActions,
		SourceStates:     sw.States{stateActive},
		DestinationState: stateSuccess,
		Condition:        handler.validatePlanAction,
		Transition:       handler.runActions,
		PostTransition:   handler.saveState,
	})

	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   taskFailed,
		SourceStates:     sw.States{stateActive, stateQueued},
		DestinationState: stateFailed,
		Condition:        nil,
		Transition:       handler.saveState,
		PostTransition:   nil,
	})

	return m, nil
}

func (m *TaskStateMachine) setTransitionOrder(transitions []sw.TransitionType) {
	m.transitions = transitions
}

func (m *TaskStateMachine) run(ctx context.Context, task *model.Task, tctx *taskHandlerContext) error {
	for _, transitionType := range m.transitions {
		err := m.sm.Run(transitionType, task, tctx)
		if err != nil {
			// update error to include some useful context
			if errors.Is(err, stateswitch.NoConditionPassedToRunTransaction) {
				errors.Wrap(
					err,
					fmt.Sprintf("transition type: %s has no rule for task state: %s", transitionType, task.Status),
				)
			}

			// set task handler err
			tctx.err = err

			// run transition failed handler
			if err := m.TransitionFailed(ctx, task, tctx); err != nil {
				return err
			}

			return err
		}
	}

	return nil
}
