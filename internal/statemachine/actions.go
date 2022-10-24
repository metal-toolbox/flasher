package statemachine

import (
	"context"
	"fmt"
	"strconv"

	sw "github.com/filanov/stateswitch"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/pkg/errors"
)

const (
	TransitionTypeActionFailed    sw.TransitionType = "actionFailed"
	transitionTypeActionsComplete sw.TransitionType = "actionsComplete"
)

var (
	ErrActionTransition    = errors.New("error in action transition")
	ErrActionTypeAssertion = errors.New("error asserting the Action type")
)

type ErrAction struct {
	handler string
	cause   string
}

func (e *ErrAction) Error() string {
	return fmt.Sprintf("error occured in action handler '%s': %s", e.handler, e.cause)
}

func newErrAction(handler, cause string) error {
	return &ErrAction{handler, cause}
}

type ActionStateMachine struct {
	actionID    string
	transitions []sw.TransitionType
	sm          sw.StateMachine
}

func (a *ActionStateMachine) SetTransitionOrder(transitions []sw.TransitionType) {
	a.transitions = transitions
}

func (a *ActionStateMachine) ActionID() string {
	return a.actionID
}

// ActionStateMachines is an ordered list of actions planned
type ActionStateMachines []*ActionStateMachine

func (a ActionStateMachines) ByActionID(id string) *ActionStateMachine {
	for _, m := range a {
		if m.actionID == id {
			return m
		}
	}

	return nil
}
func ActionID(taskID, componentSlug string, idx int) string {
	return fmt.Sprintf("%s-%s-%s", taskID, componentSlug, strconv.Itoa(idx))
}

func NewActionStateMachine(ctx context.Context, actionID string, transitions []sw.TransitionType, transitionRules []sw.TransitionRule) (*ActionStateMachine, error) {
	m := &ActionStateMachine{actionID: actionID, sm: sw.NewStateMachine(), transitions: transitions}

	for _, transitionRule := range transitionRules {
		m.sm.AddTransition(transitionRule)
	}

	return m, nil
}

func (a *ActionStateMachine) TransitionFailed(ctx context.Context, action *model.Action, hctx *HandlerContext) error {
	return a.sm.Run(TransitionTypeActionFailed, action, hctx)
}

func (a *ActionStateMachine) Run(ctx context.Context, action *model.Action, hctx *HandlerContext) error {
	for _, transitionType := range a.transitions {
		err := a.sm.Run(transitionType, action, hctx)
		if err != nil {
			if errors.Is(err, sw.NoConditionPassedToRunTransaction) {
				return errors.Wrap(
					ErrActionTransition,
					fmt.Sprintf("no transition rule found for transition type '%s' and state '%s'", transitionType, action.Status),
				)
			}

			return newErrAction(string(transitionType), err.Error())
		}
	}

	return nil
}
