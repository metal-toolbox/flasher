package statemachine

import (
	"context"
	"fmt"
	"strconv"
	"time"

	sw "github.com/filanov/stateswitch"
	"github.com/metal-toolbox/flasher/internal/metrics"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

const (

	// state for successful actions
	StateActionSuccessful sw.State = sw.State(model.StateSucceeded)
	// state for failed actions
	StateActionFailed sw.State = sw.State(model.StateFailed)
)

var (
	// ErrActionTransition is returned when an action transition fails.
	ErrActionTransition = errors.New("error in action transition")

	// ErrActionTypeAssertion is returned when an action handler receives an unexpected type.
	ErrActionTypeAssertion = errors.New("error asserting the Action type")
)

// ErrAction is an error type containing information on the Action failure.
type ErrAction struct {
	handler   string
	component string
	status    string
	cause     string
}

// Error implements the Error() interface
func (e *ErrAction) Error() string {
	return fmt.Sprintf(
		"action '%s' on component '%s' with status '%s', returned error: %s",
		e.handler,
		e.component,
		e.status,
		e.cause,
	)
}

func newErrAction(handler, component, status, cause string) error {
	return &ErrAction{handler, component, status, cause}
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
func NewActionStateMachine(actionID string, transitionRules []sw.TransitionRule) (*ActionStateMachine, error) {
	m := &ActionStateMachine{
		actionID:    actionID,
		sm:          sw.NewStateMachine(),
		transitions: []sw.TransitionType{},
	}

	for _, transitionRule := range transitionRules {
		m.transitions = append(m.transitions, transitionRule.TransitionType)
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

func (a *ActionStateMachine) registerTransitionMetrics(startTS time.Time, action *model.Action, transitionType, state string) {
	metrics.ActionHandlerRunTimeSummary.With(
		prometheus.Labels{
			"vendor":     action.Firmware.Vendor,
			"component":  action.Firmware.Component,
			"transition": transitionType,
			"state":      state,
		},
	).Observe(time.Since(startTS).Seconds())
}

// Run executes the transitions in the action statemachine while handling errors returned from any failed actions.
func (a *ActionStateMachine) Run(ctx context.Context, action *model.Action, tctx *HandlerContext) error {
	for _, transitionType := range a.transitions {
		startTS := time.Now()

		// publish task action running
		tctx.Logger.WithFields(logrus.Fields{
			"action":    action.ID,
			"condition": action.TaskID,
			"component": action.Firmware.Component,
			"version":   action.Firmware.Version,
			"step":      transitionType,
		}).Info("running action step")

		tctx.Task.Status.Append(fmt.Sprintf(
			"component: %s, install version: %s, running step %s",
			action.Firmware.Component,
			action.Firmware.Version,
			string(transitionType),
		))

		tctx.Publisher.Publish(tctx)

		// purposefully introduced fault
		if err := a.ConditionalFault(tctx.Task, transitionType); err != nil {
			return err
		}

		// return on context cancellation
		if ctx.Err() != nil {
			a.registerTransitionMetrics(startTS, action, string(transitionType), "failed")

			return ctx.Err()
		}

		// run step
		err := a.sm.Run(transitionType, action, tctx)
		if err == nil {
			a.transitionsCompleted = append(a.transitionsCompleted, transitionType)
			a.registerTransitionMetrics(startTS, action, string(transitionType), "succeeded")

			tctx.Task.Status.Append(fmt.Sprintf(
				"component: %s, install version: %s, completed step %s",
				action.Firmware.Component,
				action.Firmware.Version,
				string(transitionType),
			))

			tctx.Publisher.Publish(tctx)

			continue
		}

		_ = action.SetState(StateActionFailed)

		// error occurred
		tctx.Logger.WithError(err).WithFields(logrus.Fields{
			"action":    action.ID,
			"condition": action.TaskID,
			"component": action.Firmware.Component,
			"version":   action.Firmware.Version,
			"step":      transitionType,
		}).Info("action step error")
		a.registerTransitionMetrics(startTS, action, string(transitionType), "failed")

		// When the condition returns false, run the next transition
		if errors.Is(err, sw.NoConditionPassedToRunTransaction) {
			return newErrAction(
				string(transitionType),
				action.Firmware.Component,
				string(action.State()),
				err.Error(),
			)
		}

		return newErrAction(
			string(transitionType),
			action.Firmware.Component,
			string(action.State()),
			err.Error(),
		)
	}

	_ = action.SetState(StateActionSuccessful)

	tctx.Task.Status.Append(fmt.Sprintf(
		"[%s] component, completed firmware install, version: %s",
		action.Firmware.Component,
		action.Firmware.Version,
	))

	tctx.Publisher.Publish(tctx)

	return nil
}

// ConditionalFault is invoked before each transition to induce a fault if specified.
func (a *ActionStateMachine) ConditionalFault(task *model.Task, transitionType sw.TransitionType) error {
	var errConditionFault = errors.New("condition induced fault")

	if task.Fault == nil {
		return nil
	}

	if task.Fault.FailAt == string(transitionType) {
		return errors.Wrap(errConditionFault, string(transitionType))
	}

	return nil
}
