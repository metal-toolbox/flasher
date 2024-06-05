package runner

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/metal-toolbox/flasher/internal/metrics"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/store"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	rctypes "github.com/metal-toolbox/rivets/condition"
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

	// The task this action belongs to
	Task *model.Task

	// Logger is the task, action handler logger.
	Logger *logrus.Entry

	// Device queryor interface
	//
	// type asserted to InbandQueryor or OutofbandQueryor at invocation
	DeviceQueryor any

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

	finalize := func(state rctypes.State, startTS time.Time, action *model.Action, err error) error {
		action.SetState(state)
		handler.Publish(ctx)
		registerMetric(startTS, action, state)

		return err
	}

	// each action corresponds to a firmware to be installed
	for _, action := range task.Data.ActionsPlanned {
		startTS := time.Now()

		// return on context cancellation
		if ctx.Err() != nil {
			return finalize(rctypes.Failed, startTS, action, ctx.Err())
		}

		actionLogger := r.logger.WithFields(logrus.Fields{
			"action":    action.ID,
			"component": action.Firmware.Component,
			"fwversion": action.Firmware.Version,
		})

		resumeAction, err := r.resumeAction(ctx, action, handler)
		if err != nil {
			return finalize(rctypes.Failed, startTS, action, err)
		}

		if !resumeAction {
			continue
		}

		// fetch action attributes from task
		action.SetState(model.StateActive)
		handler.Publish(ctx)

		// return
		runNext, err := r.runActionSteps(ctx, task, action, handler, actionLogger)
		if err != nil {
			return finalize(rctypes.Failed, startTS, action, err)
		}

		if !runNext {
			info := "no further actions required"
			actionLogger.Info(info)
			task.Status.Append(info)

			return finalize(rctypes.Succeeded, startTS, action, nil)
		}

		// log and publish status
		action.SetState(rctypes.Succeeded)
		handler.Publish(ctx)
		registerMetric(startTS, action, rctypes.Succeeded)
		actionLogger.Info("action steps for component completed successfully")
	}

	return nil
}

// resumeAction returns true when the action can be resumed, when a false is returned with no error, the action is to be skipped.
func (r *Runner) resumeAction(ctx context.Context, action *model.Action, handler TaskHandler) (resume bool, err error) {
	errResumeAction := errors.New("error in resuming action")

	actionLogger := r.logger.WithFields(logrus.Fields{
		"action":    action.ID,
		"component": action.Firmware.Component,
		"fwversion": action.Firmware.Version,
	})

	switch action.State {
	case model.StatePending:
		actionLogger.Info("running action")
		return true, nil

	case model.StateSucceeded:
		actionLogger.WithField("state", action.State).Info("skipping previously successful action")
		return false, nil

	case model.StateActive:
		if action.Attempts > model.ActionMaxAttempts {

			info := "reached maximum attempts on action"
			actionLogger.WithFields(
				logrus.Fields{"state": action.State, "attempts": action.Attempts},
			).Warn(info)

			action.SetState(model.StateFailed)
			handler.Publish(ctx)

			return false, errors.Wrap(errResumeAction, fmt.Sprintf("%s: %d", info, action.Attempts))
		}

		actionLogger.WithFields(
			logrus.Fields{"state": action.State, "attempts": action.Attempts},
		).Info("resuming active action..")

		action.Attempts++
		return true, nil

	case model.StateFailed:
		handler.Publish(ctx)

		return false, errors.Wrap(errResumeAction, "action previously failed, will not be re-attempted")

	default:
		return false, errors.Wrap(errResumeAction, "unmanaged state: "+string(action.State))
	}
}

func (r *Runner) runActionSteps(ctx context.Context, task *model.Task, action *model.Action, handler TaskHandler, logger *logrus.Entry) (proceed bool, err error) {
	// helper func to log and publish step status
	publish := func(state rctypes.State, action *model.Action, step *model.Step, logger *logrus.Entry) {
		logger.WithField("step", step.Name).Debug("running step")
		step.SetState(state)
		method := string(model.RunOutofband)
		if action.Firmware.InstallInband {
			method = string(model.RunInband)
		}

		task.Status.Append(fmt.Sprintf(
			"[%s] install %s version: %s, state: %s, step %s",
			action.Firmware.Component,
			method,
			action.Firmware.Version,
			state,
			step.Name,
		))

		handler.Publish(ctx)
	}

	for _, step := range action.Steps {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}

		resume, err := r.resumeStep(step, logger)
		if err != nil {
			publish(model.StateFailed, action, step, logger)
			return false, err
		}

		if !resume {
			continue
		}

		publish(model.StateActive, action, step, logger)

		if step.Handler == nil {
			publish(model.StateFailed, action, step, logger)
			return false, errors.Wrap(
				err,
				fmt.Sprintf(
					"error while running step=%s to install firmware on component=%s, handler was nil",
					step.Name,
					action.Firmware.Component,
				),
			)
		}

		// run step
		if err := step.Handler(ctx); err != nil {
			// installed firmware equals expected
			if errors.Is(err, model.ErrInstalledFirmwareEqual) {
				task.Status.Append(
					fmt.Sprintf(
						"[%s] %s",
						action.Firmware.Component,
						"Installed and expected firmware are equal",
					),
				)

				publish(model.StateSucceeded, action, step, logger)

				// no further actions
				return false, nil
			}

			publish(model.StateFailed, action, step, logger)
			return false, errors.Wrap(
				err,
				fmt.Sprintf(
					"error while running step=%s to install firmware on component=%s",
					step.Name,
					action.Firmware.Component,
				),
			)
		}

		// publish step status
		publish(model.StateSucceeded, action, step, logger)
	}

	return true, nil
}

// resumeStep returns true when the step can be resumed, when a false is returned with no error, the step is to be skipped.
func (r *Runner) resumeStep(step *model.Step, logger *logrus.Entry) (resume bool, err error) {
	errResumeStep := errors.New("error in resuming step")

	le := logger.WithFields(
		logrus.Fields{
			"stepName": step.Name,
			"state":    step.State,
			"attempts": step.Attempts,
		},
	)

	switch step.State {
	case model.StatePending:
		return true, nil

	case model.StateSucceeded:
		le.Info("skipping previously successful step")
		return false, nil

	case model.StateActive:
		if step.Attempts > model.StepMaxAttempts {
			info := "reached maximum attempts on step"
			le.Warn(info)
			step.SetState(model.StateFailed)
			return false, errors.Wrap(errResumeStep, fmt.Sprintf("%s: %d", info, step.Attempts))
		}

		le.Info("resuming active step..")

		step.Attempts++
		return true, nil

	case model.StateFailed:
		return false, errors.Wrap(errResumeStep, "step previously failed, will not be re-attempted")

	default:
		return false, errors.Wrap(errResumeStep, "unmanaged state: "+string(step.State))
	}
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
