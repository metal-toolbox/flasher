package statemachine

import (
	"context"
	"fmt"
	"strconv"

	sw "github.com/filanov/stateswitch"
	"github.com/hashicorp/go-multierror"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/pkg/errors"
)

const (

	// state for successful actions
	StateActionSuccessful sw.State = model.StateSucceeded
	// state for failed actions
	StateActionFailed sw.State = model.StateFailed

	// transition for completed actions
	TransitionTypeActionSuccess sw.TransitionType = "succeeded"
	// transition for failed actions
	TransitionTypeActionFailed sw.TransitionType = "failed"
)

var (
	// ErrActionTransition is returned when an action transition fails.
	ErrActionTransition = errors.New("error in action transition")

	// ErrActionTypeAssertion is returned when an action handler receives an unexpected type.
	ErrActionTypeAssertion = errors.New("error asserting the Action type")
)

// ErrAction is an error type containing information on the Action failure.
type ErrAction struct {
	handler string
	status  string
	cause   string
}

// Error implements the Error() interface
func (e *ErrAction) Error() string {
	return fmt.Sprintf("action '%s' with status '%s', returned error: %s", e.handler, e.status, e.cause)
}

func newErrAction(handler, status, cause string) error {
	return &ErrAction{handler, status, cause}
}

// ActionStateMachine is an object holding the action statemachine.
// action statemachines are sub-statemachines of a Task statemachine.
//
// A Action statemachine corresponds to a task action
// which is to install a planned firmware on a device component.
type ActionStateMachine struct {
	sm                   sw.StateMachine
	actionID             string
	transitions          []sw.TransitionType
	transitionsCompleted []sw.TransitionType
}

// SetTransitionOrder sets the expected order of transition execution.
func (a *ActionStateMachine) SetTransitionOrder(transitions []sw.TransitionType) {
	a.transitions = transitions
}

// TransitionOrder returns the current order of transition execution.
func (a *ActionStateMachine) TransitionOrder() []sw.TransitionType {
	return a.transitions
}

// TransitionsCompleted returns the transitions that completed successfully.
func (a *ActionStateMachine) TransitionsCompleted() []sw.TransitionType {
	return a.transitionsCompleted
}

// ActionID returns the action this statemachine was planned for.
func (a *ActionStateMachine) ActionID() string {
	return a.actionID
}

// ActionStateMachines is an ordered list of actions planned
type ActionStateMachines []*ActionStateMachine

// ByActionID returns the Action statemachine identified by id.
func (a ActionStateMachines) ByActionID(id string) *ActionStateMachine {
	for _, m := range a {
		if m.actionID == id {
			return m
		}
	}

	return nil
}

// ActionID returns the action identifier based on the related task, component attributes.
func ActionID(taskID, componentSlug string, idx int) string {
	return fmt.Sprintf("%s-%s-%s", taskID, componentSlug, strconv.Itoa(idx))
}

// NewActionStateMachine initializes an action state machine to install firmware on a component.
func NewActionStateMachine(ctx context.Context, actionID string, transitions []sw.TransitionType, transitionRules []sw.TransitionRule) (*ActionStateMachine, error) {
	m := &ActionStateMachine{
		actionID:    actionID,
		sm:          sw.NewStateMachine(),
		transitions: transitions,
	}

	for _, transitionRule := range transitionRules {
		m.sm.AddTransition(transitionRule)
	}

	return m, nil
}

// AddStateTransitionDocumentation adds the given state, transition documentation to the action state machine
func (a *ActionStateMachine) AddStateTransitionDocumentation(stateDocumentation []sw.StateDoc, transitionDocumentation []sw.TransitionTypeDoc) {
	for _, stateDoc := range stateDocumentation {
		a.sm.DescribeState(sw.State(stateDoc.Name), stateDoc)
	}

	for _, transitionDoc := range transitionDocumentation {
		a.sm.DescribeTransitionType(sw.TransitionType(transitionDoc.Name), transitionDoc)
	}
}

// DescribeAsJSON returns a JSON output describing the action statemachine.
func (a *ActionStateMachine) DescribeAsJSON() ([]byte, error) {
	return a.sm.AsJSON()
}

// TransitionFailed is the action statemachine handler that runs when an action fails.
func (a *ActionStateMachine) TransitionFailed(ctx context.Context, action *model.Action, hctx *HandlerContext) error {
	return a.sm.Run(TransitionTypeActionFailed, action, hctx)
}

// TransitionSuccess is the action statemachine handler that runs when an action succeeds.
func (a *ActionStateMachine) TransitionSuccess(ctx context.Context, action *model.Action, hctx *HandlerContext) error {
	return a.sm.Run(TransitionTypeActionSuccess, action, hctx)
}

// Run executes the transitions in the action statemachine while handling errors returned from any failed actions.
func (a *ActionStateMachine) Run(ctx context.Context, action *model.Action, tctx *HandlerContext) error {
	for _, transitionType := range a.transitions {
		// send event task action is running
		SendEvent(
			ctx,
			tctx.TaskEventCh,
			TaskEvent{
				tctx.TaskID,
				fmt.Sprintf(
					"component: %s, running action: %s ",
					action.Firmware.ComponentSlug,
					string(transitionType),
				),
			},
		)

		err := a.sm.Run(transitionType, action, tctx)
		if err != nil {
			// When the condition returns false, run the next transition
			// note: do we want to log this for debugging?
			if errors.Is(err, sw.NoConditionPassedToRunTransaction) {
				continue
			}

			// run transition failed handler
			if txErr := a.TransitionFailed(ctx, action, tctx); txErr != nil {
				err = multierror.Append(err, errors.Wrap(txErr, "actionSM TransitionFailed() error"))
			}

			err = newErrAction(action.Status, string(transitionType), err.Error())

			return err
		}

		a.transitionsCompleted = append(a.transitionsCompleted, transitionType)

		// send event task action is complete
		SendEvent(
			ctx,
			tctx.TaskEventCh,
			TaskEvent{
				tctx.TaskID,
				fmt.Sprintf(
					"component: %s, completed action: %s ",
					action.Firmware.ComponentSlug,
					string(transitionType),
				),
			},
		)
	}

	// run transition success handler
	if err := a.TransitionSuccess(ctx, action, tctx); err != nil {
		return errors.Wrap(err, err.Error())
	}

	return nil
}
