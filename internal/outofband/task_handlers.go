package outofband

import (
	"fmt"

	sw "github.com/filanov/stateswitch"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/pkg/errors"
)

const (
	// task states
	//
	// states the task transitions through
	stateQueued  sw.State = "queued"
	stateActive  sw.State = "active"
	stateSuccess sw.State = "success"
	stateFailed  sw.State = "failed"

	resolveActionPrerequisites sw.TransitionType = "resolveActionPrerequisites"
	runActions   sw.TransitionType = "runActions"
	taskFailed sw.TransitionType = "taskFailed"
)

var (
	ErrSaveTask           = errors.New("error in saveTask transition handler")
	ErrTaskTypeAssertions = errors.New("error asserting Task type")

	errTaskActionsInit = errors.New("error initializing task actions")
)

type taskTransitionHandler interface {
	resolveActionPrerequisites(sw sw.StateSwitch, args sw.TransitionArgs) error	
	runActions(sw sw.StateSwitch, args sw.TransitionArgs) error
	saveState(sw sw.StateSwitch, args sw.TransitionArgs) error
	validateActionPrequisites(sw sw.StateSwitch, args sw.TransitionArgs) (bool, error)
}

// taskHandler implements the taskTransitionHandler methods
type taskHandler struct{}


func (h *taskHandler) resolveActionPrerequisites(sw sw.StateSwitch, args sw.TransitionArgs) error {
	taskCtx, ok := args.(*taskStateMachineContext)
	if !ok {
		return false, errInvalidTaskStateMachineContext
	}

	// 1. query inventory for inventory, firmwares
	// 2. resolve firmware to be installed
	// 3. 
	// TODO: add actions to task
	actionSMs, err := actionsFromTask(ctx, task.Parameters.Install)
	if err != nil {
		return nil, nil, errors.Wrap(errTaskActionsInit, err.Error())
	}

	if len(actionSMs) == 0 {
		return nil, nil, errors.Wrap(errTaskActionsInit, "no actions identified for firmware install")
	}
	return nil
}

func (h *taskHandler) validateActionPrequisites(sw sw.StateSwitch, args sw.TransitionArgs) (bool, error) {
	taskCtx, ok := args.(*taskStateMachineContext)
	if !ok {
		return false, errInvalidTaskStateMachineContext
	}

	if len(task.Firmwares) == 0 {
		return nil, nil, errTaskInstallParametersUndefined
	}

	if len(taskCtx.actionStateMachineList) == 0 {
		return nil, nil, err
	}

	return true, nil

}

func (h *taskHandler) runActions(sw sw.StateSwitch, args sw.TransitionArgs) error {
	mctx, ok := args.(*StateMachineContext)
	if !ok {
		return errInvalidTransitionHandler
	}

	for _, action := range mCtx.actionsSM {
		action.run(mCtx.ctx, <need model.Action here>, mctx)
	}

	fmt.Println("here")
	return nil
}

func (h *taskHandler) saveState(sw sw.StateSwitch, args sw.TransitionArgs) error {
	// check currently queued count of tasks
	a, ok := args.(*StateMachineContext)
	if !ok {
		return errInvalidTransitionHandler
	}

	task, ok := sw.(*model.Task)
	if !ok {
		return errors.Wrap(ErrSaveTask, ErrTaskTypeAssertions.Error())
	}

	if err := a.cache.UpdateTask(a.ctx, *task); err != nil {
		return errors.Wrap(ErrSaveTask, err.Error())
	}

	return nil
}
