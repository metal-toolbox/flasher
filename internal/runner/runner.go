package runner

import (
	"context"
	"time"

	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
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
	Publish()
}

func New(logger *logrus.Entry) *Runner {
	return &Runner{
		logger: logger,
	}
}

func (r *Runner) RunTask(ctx context.Context, task *model.Task, handler Handler) error {
	funcs := map[string]func(context.Context) error{
		"Initialize":  handler.Initialize,
		"Query":       handler.Query,
		"PlanActions": handler.PlanActions,
		"RunActions":  handler.RunActions,
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

	// no error returned
	_ = task.SetState(model.StateActive)
	handler.Publish()

	for fname, f := range funcs {
		if cferr := r.conditionalFault(fname, task, handler); cferr != nil {
			return taskFailed(cferr)
		}

		if err := f(ctx); err != nil {
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
