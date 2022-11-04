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
	StateActionSuccessful sw.State = "success"
	// state for failed actions
	StateActionFailed sw.State = "failed"

	// transition for completed actions
	TransitionTypeActionSuccess sw.TransitionType = "success"
	// transition for failed actions
	TransitionTypeActionFailed sw.TransitionType = "failed"
)

var (
	ErrActionTransition    = errors.New("error in action transition")
	ErrActionTypeAssertion = errors.New("error asserting the Action type")

	ErrConditionFailed = errors.New("transition condition failed")

	// ErrActionSkipped is returned, when an action handler determines that no further steps are to be carried out in an action.
	ErrActionSkipped = errors.New("action skipped")
)

type ErrAction struct {
	handler string
	status  string
	cause   string
}

func (e *ErrAction) Error() string {
	return fmt.Sprintf("action '%s' with status '%s', returned error: %s", e.handler, e.status, e.cause)
}

func newErrAction(handler, status, cause string) error {
	return &ErrAction{handler, status, cause}
}

type ActionStateMachine struct {
	sm                   sw.StateMachine
	actionID             string
	transitions          []sw.TransitionType
	transitionsCompleted []sw.TransitionType
}

func (a *ActionStateMachine) SetTransitionOrder(transitions []sw.TransitionType) {
	a.transitions = transitions
}

func (a *ActionStateMachine) TransitionOrder() []sw.TransitionType {
	return a.transitions
}

func (a *ActionStateMachine) TransitionsCompleted() []sw.TransitionType {
	return a.transitionsCompleted
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

func (a *ActionStateMachine) TransitionFailed(ctx context.Context, action *model.Action, hctx *HandlerContext) error {
	return a.sm.Run(TransitionTypeActionFailed, action, hctx)
}

func (a *ActionStateMachine) TransitionSuccess(ctx context.Context, action *model.Action, hctx *HandlerContext) error {
	return a.sm.Run(TransitionTypeActionSuccess, action, hctx)
}

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
