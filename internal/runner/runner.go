package runner

import (
	"context"
	"fmt"
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
	"github.com/metal-toolbox/rivets/events/controller"
)

// A Runner instance runs a single task, to install firmware on one or more server components.
type Runner struct {
	logger *logrus.Entry
}

type TaskHandler interface {
	Initialize(ctx context.Context) error
	Query(ctx context.Context) error
	PlanActions(ctx context.Context) error
	OnSuccess(ctx context.Context, task *model.Task)
	OnFailure(ctx context.Context, task *model.Task)
	Publish(ctx context.Context)
}

// The TaskHandlerContext is passed to task handlers
type TaskHandlerContext struct {
	// Publisher provides a method to publish task information
	Publisher model.Publisher

	// ConditionRequestor provides methods to delegate and query a condition status
	ConditionRequestor controller.ConditionRequestor

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

func (r *Runner) RunTask(ctx context.Context, task *model.Task, handler TaskHandler) error {
	// nolint:govet // struct field optimization not required
	funcs := []struct {
		name   string
		method func(context.Context) error
	}{
		{"Initialize", handler.Initialize},
		{"Query", handler.Query},
		{"PlanActions", handler.PlanActions},
	}

	taskFailed := func(err error) error {
		// no error returned
		task.SetState(model.StateFailed)
		task.Status.Append("task failed")
		task.Status.Append(err.Error())
		handler.Publish(ctx)

		handler.OnFailure(ctx, task)

		return err
	}

	taskSuccess := func() error {
		// no error returned
		task.SetState(model.StateSucceeded)
		task.Status.Append("task completed successfully")
		handler.Publish(ctx)

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
	task.SetState(model.StateActive)
	handler.Publish(ctx)

	// initialize, plan actions
	for _, f := range funcs {
		if cferr := r.conditionalFault(ctx, f.name, task, handler); cferr != nil {
			return taskFailed(cferr)
		}

		if err := f.method(ctx); err != nil {
			return taskFailed(err)
		}
	}

	r.logger.WithField("planned.actions", len(task.Data.ActionsPlanned)).Debug("start running planned actions")

	if err := r.runActions(ctx, task, handler); err != nil {
		return taskFailed(err)
	}

	r.logger.WithField("planned.actions", len(task.Data.ActionsPlanned)).Debug("finished running planned actions")

	return taskSuccess()
}

func (r *Runner) runActions(ctx context.Context, task *model.Task, handler TaskHandler) error {
	registerMetric := func(startTS time.Time, action *model.Action, state rctypes.State) {
		registerActionMetric(startTS, action, string(state))
	}

	// helper func to log and publish step status
	publish := func(state rctypes.State, action *model.Action, stepName model.StepName, logger *logrus.Entry) {
		logger.WithField("step", stepName).Debug("running step")
		task.Status.Append(fmt.Sprintf(
			"[%s] install version: %s, inband: %s, state: %s, step %s",
			action.Firmware.Component,
			action.Firmware.Version,
			action.Firmware.InstallInband,
			state,
			stepName,
		))

		handler.Publish(ctx)
	}

	// each action corresponds to a firmware to be installed
	for _, action := range task.Data.ActionsPlanned {
		startTS := time.Now()

		actionLogger := r.logger.WithFields(logrus.Fields{
			"action":    action.ID,
			"component": action.Firmware.Component,
			"fwversion": action.Firmware.Version,
		})

		// fetch action attributes from task
		action.SetState(model.StateActive)

		// return on context cancellation
		if ctx.Err() != nil {
			registerMetric(startTS, action, rctypes.Failed)
			return ctx.Err()
		}

		actionLogger.Info("running action steps for firmware install")
		for _, step := range action.Steps {
			publish(model.StateActive, action, step.Name, actionLogger)

			// run step
			if err := step.Handler(ctx); err != nil {
				action.SetState(model.StateFailed)
				publish(model.StateFailed, action, step.Name, actionLogger)

				registerMetric(startTS, action, rctypes.Failed)
				return errors.Wrap(
					err,
					fmt.Sprintf(
						"error while running step=%s to install firmware on component=%s",
						step.Name,
						action.Firmware.Component,
					),
				)
			}

			// log and publish status
			action.SetState(model.StateSucceeded)
			publish(model.StateSucceeded, action, step.Name, actionLogger)
		}

		registerMetric(startTS, action, rctypes.Succeeded)
		actionLogger.Info("action steps for component completed successfully")
	}

	return nil
}

// conditionalFault is invoked before each runner method to induce a fault if specified
func (r *Runner) conditionalFault(ctx context.Context, fname string, task *model.Task, handler TaskHandler) error {
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
		handler.Publish(ctx)

		r.logger.WithField("delay", td.String()).Warn("condition induced delay in execution")
		time.Sleep(td)

		// purge delay duration string, this is to execute only once
		// and this method is called at each transition.
		task.Fault.DelayDuration = ""
	}

	return nil
}

func registerActionMetric(startTS time.Time, action *model.Action, state string) {
	metrics.ActionRuntimeSummary.With(
		prometheus.Labels{
			"vendor":    action.Firmware.Vendor,
			"component": action.Firmware.Component,
			"state":     state,
		},
	).Observe(time.Since(startTS).Seconds())
}
