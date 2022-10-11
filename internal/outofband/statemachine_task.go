package outofband

import (
	"context"
	"fmt"
	"strconv"

	"github.com/filanov/stateswitch"
	sw "github.com/filanov/stateswitch"
	"github.com/metal-toolbox/flasher/internal/firmware"
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
	errInvalidTransitionHandler       = errors.New("expected a valid transitionHandler{} type")
	errInvalidtaskHandlerContext      = errors.New("expected a taskHandlerContext{} type")
	errTaskInstallParametersUndefined = errors.New("task install parameters undefined")
	errTaskTransition                 = errors.New("error in task transition")
)

// taskHandlerContext holds working attributes of a task
//
// This struct is passed to transition handlers which
// depend on the values provided in this struct.
type taskHandlerContext struct {
	// taskID is only available when an action is invoked under a task.
	taskID string

	// ctx is the parent context
	ctx context.Context

	// plan is an ordered list of actions identified to
	// complete this task
	actionPlan ActionPlan

	// err is set when a transition fails in run()
	err error

	// fwPlanner provides methods to plan the firmware to be installed.
	fwPlanner firmware.Planner

	// bmc is the BMC client to query the BMC.
	bmc    bmcQueryor
	cache  store.Storage
	inv    inventory.Inventory
	logger *logrus.Logger
}

// TaskStateMachine drives the task
type TaskStateMachine struct {
	sm          sw.StateMachine
	transitions []sw.TransitionType
}

// ActionPlanMachine drives the firmware install actions

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
		//	Condition:        handler.validatePlanAction,
		Transition:     handler.runActions,
		PostTransition: handler.saveState,
	})

	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   taskFailed,
		SourceStates:     sw.States{stateActive, stateQueued},
		DestinationState: stateFailed,
		Condition:        nil,
		Transition:       handler.failed,
		PostTransition:   handler.saveState,
	})

	return m, nil
}

func (m *TaskStateMachine) setTransitionOrder(transitions []sw.TransitionType) {
	m.transitions = transitions
}

func (m *TaskStateMachine) run(ctx context.Context, task *model.Task, handler *taskHandler, tctx *taskHandlerContext) error {
	var err error

	// To ensure that the task state is saved when sm.Run fails
	// because of a invalid state transition attempt,
	//
	// the error for these cases is in the form,
	// 'no transition rule found for transition type 'runActions' to state 'success': no condition found to run transition'
	//
	// The error is returned to the caller and the task is marked as failed
	defer func() {
		if err != nil {
			task.Info = err.Error()
			// errors from these methods are ignored
			// so as to not overwrite the original error
			_ = task.SetState(sw.State(stateFailed))
			_ = handler.saveState(task, tctx)
		}
	}()

	for _, transitionType := range m.transitions {
		err = m.sm.Run(transitionType, task, tctx)
		if err != nil {
			// update error to include some useful context
			if errors.Is(err, stateswitch.NoConditionPassedToRunTransaction) {
				err = errors.Wrap(
					errTaskTransition,
					fmt.Sprintf("no transition rule found for transition type '%s' and state '%s'", transitionType, task.Status),
				)
			}

			// set task handler err
			tctx.err = err

			return err
		}
	}

	return nil
}

// planInstallActions plans the firmware install actions
//
// The given task is updated with Actions based on the FirmwaresPlanned attribute
// and an actionPlan is returned which is to be executed.
func planInstallActions(ctx context.Context, task *model.Task) (ActionPlan, error) {
	plans := make(ActionPlan, 0)

	// each firmware install parameter results in an action
	for idx, firmware := range task.FirmwaresPlanned {
		actionID := actionID(task.ID.String(), firmware.ComponentSlug, idx)

		m, err := NewActionPlanMachine(ctx)
		if err != nil {
			return nil, err
		}

		m.actionID = actionID
		plans = append(plans, m)

		action := model.Action{
			ID:       actionID,
			Status:   string(stateQueued),
			Firmware: task.FirmwaresPlanned[idx],
		}

		task.ActionsPlanned = append(task.ActionsPlanned, action)
	}

	return plans, nil
}

func actionID(taskID, componentSlug string, idx int) string {
	return fmt.Sprintf("%s-%s-%s", taskID, componentSlug, strconv.Itoa(idx))
}
