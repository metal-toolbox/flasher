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
	TransitionTypeActive sw.TransitionType = "active"
	TransitionTypeQuery  sw.TransitionType = "query"
	TransitionTypePlan   sw.TransitionType = "plan"
	TransitionTypeRun    sw.TransitionType = "run"

	// TransitionTypeTaskSuccess is transition type for successful tasks
	TransitionTypeTaskSuccess sw.TransitionType = "succeeded"
	// TransitionTypeTaskFailed is transition type for failed tasks
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

// SendEvent sends the TaskEvent on the channel with a timeout.
func SendEvent(ctx context.Context, ch chan<- TaskEvent, e TaskEvent) {
	if ch == nil {
		return
	}

	select {
	case ch <- e:
	case <-time.After(1 * time.Second):
		logrus.New().Debug("dropped event with info: " + e.Info)
		return
	}
}

// HandlerContext holds references to objects it requires to complete task and action transitions.
//
// The HandlerContext is passed to every transition handler.
type HandlerContext struct {
	// ctx is the parent cancellation context
	Ctx context.Context

	// err is set when a transition fails to complete its transittions in run()
	// the err value is then passed into the task information
	// as the state machine transitions into a failed state.
	Err error

	// DeviceQueryor is the interface to query target device under firmware install.
	DeviceQueryor model.DeviceQueryor

	// Store is the task storage
	//
	// TODO(joel): move Inv, Store into the Task, Action handler context
	// so this package does not depend on those package.
	Store store.Storage

	// Inv is the device inventory backend.
	Inv inventory.Inventory

	// Data is an arbitrary key values map available to all task, action handler methods.
	Data map[string]string

	// Device holds attributes about the device under firmware install.
	Device *model.Device

	// TaskEventCh is where a Task or an Action may emit an event
	// which includes information on the status information of a task.
	TaskEventCh chan TaskEvent

	// Logger is the task, action handler logger.
	Logger *logrus.Entry

	// WorkerID is the identifier for the worker executing this task.
	WorkerID string

	// TaskID is the identifier for this task.
	TaskID string

	// This value is prefixed to the path generated to download the firmware to install.
	//
	// TODO(joel): remove this once the firmware data contains the full URL to the firmware
	FirmwareURLPrefix string

	// ActionStateMachines are sub-statemachines of this Task
	// each firmware applicable has a Action statmachine that is
	// executed as part of this task.
	ActionStateMachines ActionStateMachines

	// Dryrun skips any disruptive actions on the device - power on/off, bmc resets, firmware installs,
	// the task and its actions run as expected, and the device state in the inventory is updated as well,
	// although the firmware is not installed.
	//
	// It is upto the Action handler implementations to ensure the dry run works as described.
	Dryrun bool
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

// NewTaskStateMachine declares, initializes and returns a TaskStateMachine object to execute flasher tasks.
func NewTaskStateMachine(handler TaskTransitioner) (*TaskStateMachine, error) {
	// transitions are executed in this order
	transitionOrder := []sw.TransitionType{
		TransitionTypeQuery,
		TransitionTypePlan,
		TransitionTypeRun,
	}

	m := &TaskStateMachine{sm: sw.NewStateMachine(), transitions: transitionOrder}
	m.addDocumentation()

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
		Documentation: sw.TransitionRuleDoc{
			Name:        "Query device inventory",
			Description: "Query device inventory for component firmware versions - from the configured inventory source, fall back to querying inventory from the device.",
		},
	})

	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   TransitionTypePlan,
		SourceStates:     sw.States{model.StateActive},
		DestinationState: model.StateActive,
		Condition:        nil,
		Transition:       handler.Plan,
		PostTransition:   handler.PersistState,
		Documentation: sw.TransitionRuleDoc{
			Name:        "Plan install actions",
			Description: "Prepare a plan - Action (sub) state machines for each firmware to be installed. Firmwares applicable is decided based on task parameters and by comparing the versions currently installed.",
		},
	})

	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   TransitionTypeRun,
		SourceStates:     sw.States{model.StateActive},
		DestinationState: model.StateSucceeded,
		Condition:        handler.ValidatePlan,
		Transition:       handler.Run,
		PostTransition:   handler.PersistState,
		Documentation: sw.TransitionRuleDoc{
			Name:        "Run install actions",
			Description: "Run executes the planned Action (sub) state machines prepared in the Plan stage.",
		},
	})

	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   TransitionTypeTaskFail,
		SourceStates:     sw.States{model.StatePending, model.StateActive},
		DestinationState: model.StateFailed,
		Condition:        nil,
		Transition:       handler.TaskFailed,
		PostTransition:   handler.PersistState,
		Documentation: sw.TransitionRuleDoc{
			Name:        "Task failed",
			Description: "Task execution has failed because of a failed task action or task handler.",
		},
	})

	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   TransitionTypeTaskSuccess,
		SourceStates:     sw.States{model.StateActive},
		DestinationState: model.StateSucceeded,
		Condition:        nil,
		Transition:       handler.TaskSuccessful,
		PostTransition:   handler.PersistState,
		Documentation: sw.TransitionRuleDoc{
			Name:        "Task successful",
			Description: "Task execution completed successfully.",
		},
	})

	return m, nil
}

func (m *TaskStateMachine) addDocumentation() {
	m.sm.DescribeState(model.StatePending, sw.StateDoc{
		Name:        "Requested",
		Description: "In this state the task has been requested (this is done outside of the state machine).",
	})

	m.sm.DescribeState(model.StatePending, sw.StateDoc{
		Name:        "Queued",
		Description: "In this state the task is being initialized (this is done outside of the state machine).",
	})

	m.sm.DescribeState(model.StateActive, sw.StateDoc{
		Name:        "Active",
		Description: "In this state the task has been initialized and begun execution in the statemachine.",
	})

	m.sm.DescribeState(model.StateFailed, sw.StateDoc{
		Name:        "Failed",
		Description: "In this state the task execution has failed.",
	})

	m.sm.DescribeState(model.StateSucceeded, sw.StateDoc{
		Name:        "Success",
		Description: "In this state the task execution has completed successfully.",
	})

	m.sm.DescribeTransitionType(TransitionTypeQuery, sw.TransitionTypeDoc{
		Name:        "Query",
		Description: "In this transition the device component firmware information is being queried.",
	})

	m.sm.DescribeTransitionType(TransitionTypePlan, sw.TransitionTypeDoc{
		Name:        "Plan",
		Description: "In this transition the actions (sub state machines) for the firmware install is being planned for execution.",
	})

	m.sm.DescribeTransitionType(TransitionTypeRun, sw.TransitionTypeDoc{
		Name:        "Run",
		Description: "In this transition the actions (sub state machines) for the firmware install are being executed.",
	})

	m.sm.DescribeTransitionType(TransitionTypeTaskFail, sw.TransitionTypeDoc{
		Name:        string(TransitionTypeTaskFail),
		Description: "In this transition the task has failed and any post failure steps are being executed.",
	})

	m.sm.DescribeTransitionType(TransitionTypeTaskSuccess, sw.TransitionTypeDoc{
		Name:        string(TransitionTypeTaskSuccess),
		Description: "In this transition the task has completed successfully and any post failure steps are being executed.",
	})
}

// DescribeAsJSON returns a JSON output describing the task statemachine.
func (m *TaskStateMachine) DescribeAsJSON() ([]byte, error) {
	return m.sm.AsJSON()
}

// SetTransitionOrder sets the expected task state transition order.
func (m *TaskStateMachine) SetTransitionOrder(transitions []sw.TransitionType) {
	m.transitions = transitions
}

// TransitionFailed is the task failed transition handler.
func (m *TaskStateMachine) TransitionFailed(task *model.Task, tctx *HandlerContext) error {
	return m.sm.Run(TransitionTypeTaskFail, task, tctx)
}

// TransitionSuccess is the task success transition handler.
func (m *TaskStateMachine) TransitionSuccess(task *model.Task, tctx *HandlerContext) error {
	return m.sm.Run(TransitionTypeTaskSuccess, task, tctx)
}

// Run executes the transitions in the expected order while handling any failures.
func (m *TaskStateMachine) Run(task *model.Task, tctx *HandlerContext) error {
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
			if txErr := m.TransitionFailed(task, tctx); txErr != nil {
				err = errors.Wrap(err, string(TransitionTypeActionFailed)+": "+txErr.Error())
			}

			return err
		}
	}

	// run transition success handler when the final successful transition is as expected
	if finalTransition == TransitionTypeRun {
		if err := m.TransitionSuccess(task, tctx); err != nil {
			return errors.Wrap(err, string(TransitionTypeActionSuccess)+": "+err.Error())
		}
	}

	return nil
}
