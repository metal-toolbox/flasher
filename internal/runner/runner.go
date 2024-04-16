package runner

import (
	"context"
	"runtime/debug"
	"time"

	"github.com/metal-toolbox/flasher/internal/device"
	"github.com/metal-toolbox/flasher/internal/metrics"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/store"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	rctypes "github.com/metal-toolbox/rivets/condition"
)

// A Runner instance runs a single task, setting up and executing the actions required to install firmware
// on one or more server components.
type Runner struct {
	logger *logrus.Entry
}

type Handler interface {
	Initialize(ctx context.Context) error
	Query(ctx context.Context) error
	PlanActions(ctx context.Context) error
	RunActions(ctx context.Context) error
	OnSuccess(ctx context.Context, task *model.Task)
	OnFailure(ctx context.Context, task *model.Task)
	Publish(ctx context.Context)
}

// The TaskHandlerContext is passed to task handlers
type TaskHandlerContext struct {
	// Publisher provides a method to publish task information
	Publisher model.Publisher

	// The task this action belongs to
	Task *model.Task

	// Logger is the task, action handler logger.
	Logger *logrus.Entry

	// Device queryor interface
	DeviceQueryor device.Queryor

	// Data store repository
	Store store.Repository
}

type ActionHandler interface {
	ComposeAction(ctx context.Context, actionCtx *ActionHandlerContext) (*model.Action, error)
}

// The ActionHandlerContext passed to action handlers
type ActionHandlerContext struct {
	*TaskHandlerContext

	// The first action to be run
	First bool

	// The final action to be run
	Last bool

	// The firmware to be installed
	Firmware *model.Firmware
}

func New(logger *logrus.Entry) *Runner {
	return &Runner{
		logger: logger,
	}
}

func (r *Runner) RunTask(ctx context.Context, task *model.Task, handler Handler) error {
	// nolint:govet // struct field optimization not required
	funcs := []struct {
		name   string
		method func(context.Context) error
	}{
		{"Initialize", handler.Initialize},
		{"Query", handler.Query},
		{"PlanActions", handler.PlanActions},
		{"RunActions", handler.RunActions},
	}

	taskFailed := func(err error) error {
		// no error returned
		_ = task.SetState(model.StateFailed)
		task.Status.Append("task failed")
		task.Status.Append(err.Error())
		handler.Publish()

		handler.OnFailure(ctx, task)

		return err
	}

	taskSuccess := func() error {
		// no error returned
		_ = task.SetState(model.StateSucceeded)
		task.Status.Append("task completed successfully")
		handler.Publish()

		handler.OnSuccess(ctx, task)

		return nil
	}

	defer func() error {
		if rec := recover(); rec != nil {
			r.logger.Printf("!!panic %s: %s", rec, debug.Stack())
			r.logger.Error("Panic occurred while running task")
			return taskFailed(errors.New("Task fatal error, check logs for details"))
		}
		return nil
	}() // nolint:errcheck // nope

	// no error returned
	_ = task.SetState(model.StateActive)
	handler.Publish()

	for _, f := range funcs {
		if cferr := r.conditionalFault(f.name, task, handler); cferr != nil {
			return taskFailed(cferr)
		}

		if err := f.method(ctx); err != nil {
			return taskFailed(err)
		}
	}

	return taskSuccess()
}

// conditionalFault is invoked before each runner method to induce a fault if specified
func (r *Runner) conditionalFault(fname string, task *model.Task, handler Handler) error {
	var errConditionFault = errors.New("condition induced fault")

	if task.Fault == nil {
		return nil
	}

	if task.Fault.Panic {
		panic("condition induced panic..")
	}

	if task.Fault.FailAt == fname {
		return errors.Wrap(errConditionFault, fname)
	}

	if task.Fault.DelayDuration != "" {
		td, err := time.ParseDuration(task.Fault.DelayDuration)
		if err != nil {
			// invalid duration string is ignored
			// nolint:nilerr // nil error returned intentionally
			return nil
		}

		task.Status.Append("condition induced delay: " + td.String())
		handler.Publish()

		r.logger.WithField("delay", td.String()).Warn("condition induced delay in execution")
		time.Sleep(td)

		// purge delay duration string, this is to execute only once
		// and this method is called at each transition.
		task.Fault.DelayDuration = ""
	}

	return nil
}
