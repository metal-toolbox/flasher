package statemachine

import (
	"context"
	"fmt"
	"time"

	"github.com/emicklei/dot"
	sw "github.com/filanov/stateswitch"
	"github.com/metal-toolbox/flasher/internal/inventory"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/store"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	TransitionTypeQuery sw.TransitionType = "query"
	TransitionTypePlan  sw.TransitionType = "plan"
	TransitionTypeRun   sw.TransitionType = "run"

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

// TaskEvents are events sent by tasks and actions, they include information
// on the task, action being executed
type TaskEvent struct {
	TaskID string
	Info   string
}

func SendEvent(ctx context.Context, ch chan<- TaskEvent, e TaskEvent) {
	if ch == nil {
		return
	}

	select {
	// case <-ctx.Done():
	//	return
	case ch <- e:
	case <-time.After(1 * time.Second):
		fmt.Println("dropped event with info: " + e.Info)
		return
	}
}

// sw library drawbacks
// - when running the sw statemachine - each transition has be run and so the Run method below
// - Conditions do not fallthrough

// HandlerContext holds working attributes of a task
//
// This struct is passed to transition handlers which
// depend on the values provided in this struct.
type HandlerContext struct {
	// ctx is the parent cancellation context
	Ctx context.Context

	// Dryrun skips any disruptive actions on the device - power on/off, bmc resets, firmware installs,
	// the task and its actions run as expected, and the device state in the inventory is updated as well,
	// although the firmware is not installed.
	//
	// It is upto the Action handler implementations to ensure the dry run works as described.
	Dryrun bool

	// WorkerID is the identifier of the worker running this task.
	WorkerID string

	// taskID is available when an action is invoked under a task.
	TaskID string

	// This value is prefixed to the path generated to download
	// the firmware to install.
	FirmwareURLPrefix string

	// plan is an ordered list of actions planned to complete this task.
	ActionStateMachines ActionStateMachines

	// err is set when a transition fails in run()
	Err error

	// DeviceQueryor is an interface run queries on a device.
	DeviceQueryor model.DeviceQueryor

	// Data is key values the handler may decide to store
	// so as to record, read handler specific values.
	Data map[string]string

	// Device is the device this task is executing on.
	Device *model.Device

	// TaskEventCh is where a Task or an Action will emit an event
	// which includes task information.
	TaskEventCh chan TaskEvent

	// TODO(joel): move Inv, Store into the Task, Action handler context
	// so this package does not depend on those package.
	Store  store.Storage
	Inv    inventory.Inventory
	Logger *logrus.Entry
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

func NewTaskStateMachine(ctx context.Context, task *model.Task, handler TaskTransitioner) (*TaskStateMachine, error) {
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
			Description: "Query device inventory for component firmware verisons - from the configured inventory source, fall back to querying inventory from the device.",
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
		DestinationState: model.StateSuccess,
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
		SourceStates:     sw.States{model.StateQueued, model.StateActive},
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
		DestinationState: model.StateSuccess,
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

// Describe returns a JSON output describing the action statemachine.
func (m *TaskStateMachine) Describe() *dot.Graph {
	return m.sm.AsDotGraph()
}

func (m *TaskStateMachine) addDocumentation() {
	m.sm.DescribeState(model.StateRequested, sw.StateDoc{
		Name:        "Requested",
		Description: "In this state the task has been requested (this is done outside of the state machine).",
	})

	m.sm.DescribeState(model.StateQueued, sw.StateDoc{
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

	m.sm.DescribeState(model.StateSuccess, sw.StateDoc{
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
