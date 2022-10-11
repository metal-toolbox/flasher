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
	ErrActionTransition = errors.New("error in action transition")
)

type ActionPlanMachine struct {
	actionID    string
	transitions []sw.TransitionType
	sm          sw.StateMachine
}

func (a *ActionPlanMachine) SetTransitionOrder(transitions []sw.TransitionType) {
	a.transitions = transitions
}

func (a *ActionPlanMachine) ActionID() string {
	return a.actionID
}

// ActionPlan is an ordered list of actions planned
type ActionPlan []*ActionPlanMachine

func (a ActionPlan) ByActionID(id string) *ActionPlanMachine {
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

func NewActionPlanMachine(ctx context.Context, actionID string, transitions []sw.TransitionType, transitionRules []sw.TransitionRule) (*ActionPlanMachine, error) {
	m := &ActionPlanMachine{actionID: actionID, sm: sw.NewStateMachine(), transitions: transitions}

	for _, transitionRule := range transitionRules {
		m.sm.AddTransition(transitionRule)
	}

	return m, nil
}

func (a *ActionPlanMachine) TransitionFailed(ctx context.Context, action *model.Action, hctx *HandlerContext) error {
	return a.sm.Run(TransitionTypeActionFailed, action, hctx)
}

func (a *ActionPlanMachine) Run(ctx context.Context, action *model.Action, hctx *HandlerContext) error {
	for _, transitionType := range a.transitions {
		err := a.sm.Run(transitionType, action, hctx)
		if err != nil {
			if errors.Is(err, sw.NoConditionPassedToRunTransaction) {
				return errors.Wrap(
					ErrActionTransition,
					fmt.Sprintf("no transition rule found for transition type '%s' and state '%s'", transitionType, action.Status),
				)
			}

			return errors.Wrap(ErrActionTransition, err.Error())
		}
	}

	return nil
}
