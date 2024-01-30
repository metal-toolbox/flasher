package statemachine

import (
	"context"

	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/store"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.hollow.sh/toolbox/events/registry"
)

var (
	ErrInvalidtaskHandlerContext = errors.New("expected a HandlerContext{} type")
	ErrInvalidTransitionHandler  = errors.New("expected a valid transitionHandler{} type")
)

// Publisher defines methods to publish task information.
type Publisher interface {
	Publish(ctx *HandlerContext)
}

// HandlerContext holds references to objects required to complete firmware install task and action transitions.
//
// The HandlerContext is passed to every transition handler.
type HandlerContext struct {
	// ctx is the parent cancellation context
	Ctx context.Context

	// Task is the task being executed.
	Task *model.Task

	// Publisher provides the Publish method to publish Task status changes.
	Publisher Publisher

	// err is set when a transition fails to complete its transitions in run()
	// the err value is then passed into the task information
	// as the state machine transitions into a failed state.
	Err error

	// DeviceQueryor is the interface to query target device under firmware install.
	DeviceQueryor model.DeviceQueryor

	// Store is the asset inventory store.
	Store store.Repository

	// Data is an arbitrary key values map available to all task, action handler methods.
	Data map[string]string

	// Asset holds attributes about the device under firmware install.
	Asset *model.Asset

	// FacilityCode limits the task handler to Assets in the given facility.
	FacilityCode string

	// Logger is the task, action handler logger.
	Logger *logrus.Entry

	// WorkerID is the identifier for the worker executing this task.
	WorkerID registry.ControllerID

	// ActionStateMachines are sub-statemachines of this Task
	// each firmware applicable has a Action statemachine that is
	// executed as part of this task.
	ActionStateMachines ActionStateMachines

	// Dryrun skips any disruptive actions on the device - power on/off, bmc resets, firmware installs,
	// the task and its actions run as expected, and the device state in the inventory is updated as well,
	// although the firmware is not installed.
	//
	// It is upto the Action handler implementations to ensure the dry run works as described.
	Dryrun bool

	// LastRev is the last revision of the status data for this task stored in NATS KV
	LastRev uint64
}
