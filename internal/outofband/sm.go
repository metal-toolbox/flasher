package outofband

import (
	"context"
	"strconv"

	sw "github.com/filanov/stateswitch"
	"github.com/metal-toolbox/flasher/internal/inventory"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/store"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const ()

var (
	// errors
	errInvalidTransitionHandler = errors.New("expected a valid transitionHandler{} type")

	errInvalidTaskStateMachineContext = errors.New("expected a taskStateMachineContext{} type")

	errTaskInstallParametersUndefined = errors.New("task install parameters undefined")
)

// ActionStateMachineList is an ordered map of taskIDs -> Action IDs -> Action state machine
type ActionStateMachineList []map[string]map[string]*ActionStateMachine

// TaskStateMachineContext holds working attributes of a task
//
// This struct is passed to transition handlers which
// depend on the values provided in this struct.
type taskStateMachineContext struct {
	// taskID is only available when an action is invoked under a task.
	taskID string

	// ctx is the parent context
	ctx context.Context

	// action state machines is an ordered list of action state machines
	// this is populated in the `init` state
	actionStateMachineList ActionStateMachineList

	cache  store.Storage
	inv    inventory.Inventory
	logger *logrus.Logger
}

// TaskStateMachine drives the task
type TaskStateMachine struct {
	sm sw.StateMachine
}

// ActionStateMachine drives the firmware install actions

func (m *TaskStateMachine) TransitionFailed(ctx context.Context, task *model.Task, smCtx *taskStateMachineContext) error {
	return m.sm.Run(taskFailed, task, smCtx)
}

func NewTaskStateMachine(ctx context.Context, task *model.Task, transitionHandler taskTransitionHandler) (*TaskStateMachine, error) {
	m := &TaskStateMachine{sm: sw.NewStateMachine()}

	// The SM has transition rules define the transitionHandler methods
	// each transitionHandler method is passed as values to the transition rule.
	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   resolveActionPrerequisites,
		SourceStates:     sw.States{stateQueued},
		DestinationState: stateActive,

		// Condition for the transition, transition will be executed only if this function return true
		// Can be nil, in this case it's considered as return true, nil
		Condition: nil,

		// Transition is users business logic, should not set the state or return next state
		// If condition returns true this function will be executed
		Transition: transitionHandler.resolveActionPrerequisites,

		// PostTransition will be called if condition and transition are successful.
		PostTransition: transitionHandler.saveState,
	})

	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   runActions,
		SourceStates:     sw.States{stateActive},
		DestinationState: stateSuccess,
		Condition:        transitionHandler.validateActionPrequisites,
		Transition:       transitionHandler.runActions,
		PostTransition:   transitionHandler.saveState,
	})

	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   taskFailed,
		SourceStates:     sw.States{stateActive, stateQueued},
		DestinationState: stateFailed,
		Condition:        nil,
		Transition:       transitionHandler.saveState,
		PostTransition:   nil,
	})

	return m, nil
}

func (m *TaskStateMachine) run(ctx context.Context, task *model.Task, taskCtx *taskStateMachineContext) error {
	order := []sw.TransitionType{
		resolveActionPrerequisites,
		runActions,
	}

	for _, transitionType := range order {
		err := m.sm.Run(transitionType, task, taskCtx)
		if err != nil {
			if err := m.TransitionFailed(ctx, task, taskCtx); err != nil {
				return err
			}

			return err
		}
	}

	return nil
}

// actionStateMachinesForTask returns a slice of state machines for each of the firmware versions to be installed in a task.
func actionStateMachinesForTask(ctx context.Context, task *model.Task) (ActionStateMachineList, error) {
	// actionStateMachines is an ordered map of taskIDs -> Action IDs -> Action state machine
	actionStateMachineList := make(ActionStateMachineList, 0)

	// each firmware install parameter results in an action
	for idx, firmware := range task.FirmwareResolved {
		actionSM, err := NewActionStateMachine(ctx)
		if err != nil {
			return nil, err
		}

		action := model.Action{
			ID:     firmware.ComponentSlug + "-" + strconv.Itoa(idx),
			Status: string(stateQueued),
			// Firmware is populated by the task firmware resolve transition
			Firmware: task.FirmwareResolved[idx],
		}

		m := map[string]map[string]*ActionStateMachine{
			task.ID.String(): map[string]*ActionStateMachine{
				action.ID: actionSM,
			},
		}

		actionStateMachineList = append(actionStateMachineList, m)
	}

	return actionStateMachineList, nil
}

type ActionStateMachine struct {
	sm sw.StateMachine
}

func NewActionStateMachine(ctx context.Context) (*ActionStateMachine, error) {
	m := &ActionStateMachine{sm: sw.NewStateMachine()}

	handler := &actionHandler{}

	// The SM has transition rules define the transitionHandler methods
	// each transitionHandler method is passed as values to the transition rule.
	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   transitionTypeLoginBMC,
		SourceStates:     sw.States{stateQueued},
		DestinationState: stateActive,

		// Condition for the transition, transition will be executed only if this function return true
		// Can be nil, in this case it's considered as return true, nil
		Condition: nil,

		// Transition is users business logic, should not set the state or return next state
		// If condition returns true this function will be executed
		Transition: handler.loginBMC,

		// PostTransition will be called if condition and transition are successful.
		PostTransition: handler.saveState,
	})

	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   transitionTypeUploadFirmware,
		SourceStates:     sw.States{stateLoginBMC},
		DestinationState: stateUploadFirmware,
		Condition:        nil,
		Transition:       handler.uploadFirmware,
		PostTransition:   handler.saveState,
	})

	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   transitionTypeInstallFirmware,
		SourceStates:     sw.States{stateUploadFirmware},
		DestinationState: stateUploadFirmware,
		Condition:        nil,
		Transition:       handler.uploadFirmware,
		PostTransition:   handler.saveState,
	})

	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   transitionTypeResetBMC,
		SourceStates:     sw.States{stateInstallFirmware},
		DestinationState: stateResetBMC,
		Condition:        handler.conditionalResetBMC,
		Transition:       handler.resetBMC,
		PostTransition:   handler.saveState,
	})

	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   transitionTypeResetHost,
		SourceStates:     sw.States{stateInstallFirmware},
		DestinationState: stateResetHost,
		Condition:        handler.conditionalResetHost,
		Transition:       handler.resetHost,
		PostTransition:   handler.saveState,
	})

	m.sm.AddTransition(sw.TransitionRule{
		TransitionType: transitionTypeActionFailed,
		SourceStates: sw.States{
			stateLoginBMC,
			stateUploadFirmware,
			stateInstallFirmware,
			stateResetBMC,
			stateResetHost,
		},
		DestinationState: stateInstallFailed,
		Condition:        nil,
		Transition:       handler.saveState,
		PostTransition:   nil,
	})

	return m, nil
}

func (a *ActionStateMachine) TransitionFailed(ctx context.Context, action *model.Action, smCtx *taskStateMachineContext) error {
	return a.sm.Run(transitionTypeActionFailed, action, smCtx)
}

func (a *ActionStateMachine) run(ctx context.Context, action *model.Action, smCtx *taskStateMachineContext) error {
	order := []sw.TransitionType{
		transitionTypeLoginBMC,
		transitionTypeInstallFirmware,
		transitionTypeUploadFirmware,
		transitionTypeResetBMC,
		transitionTypeResetHost,
	}

	for _, transitionType := range order {
		err := a.sm.Run(transitionType, action, smCtx)
		if err != nil {
			if err := a.TransitionFailed(ctx, action, smCtx); err != nil {
				return err
			}

			return err
		}

	}

	return nil
}
