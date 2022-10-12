package outofband

import (
	"fmt"

	sw "github.com/filanov/stateswitch"
	"github.com/metal-toolbox/flasher/internal/model"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
)

// taskHandler implements the taskTransitionHandler methods
type actionHandler struct{}

func (h *actionHandler) loginBMC(a sw.StateSwitch, args sw.TransitionArgs) error {
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return sm.ErrInvalidtaskHandlerContext
	}

	action, ok := a.(*model.Action)
	if !ok {
		return sm.ErrActionTypeAssertion
	}

	// init out of band device queryor
	task, err := tctx.Cache.TaskByID(tctx.Ctx, action.TaskID)
	if err != nil {
		return err
	}

	tctx.Device = NewDeviceQueryor(tctx.Ctx, &task.Parameters.Device, tctx.Logger)

	// login
	if err := tctx.Device.Open(tctx.Ctx); err != nil {
		return err
	}

	fmt.Println("login")
	return nil
}

func (h *actionHandler) conditionInstallFirmware(a sw.StateSwitch, args sw.TransitionArgs) (bool, error) {
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return false, sm.ErrInvalidtaskHandlerContext
	}

	action, ok := a.(*model.Action)
	if !ok {
		return false, sm.ErrActionTypeAssertion
	}

	_ = action
	_, err := tctx.Device.Inventory(tctx.Ctx)
	if err != nil {
		return false, err
	} //

	// compare installed with current

	return true, nil
}

func (h *actionHandler) uploadFirmware(action sw.StateSwitch, args sw.TransitionArgs) error {
	return nil
}

func (h *actionHandler) installFirmware(action sw.StateSwitch, args sw.TransitionArgs) error {
	return nil
}

func (h *actionHandler) conditionalResetBMC(action sw.StateSwitch, args sw.TransitionArgs) (bool, error) {
	// check if BMC reset is required
	return true, nil
}

func (h *actionHandler) resetBMC(action sw.StateSwitch, args sw.TransitionArgs) error {
	return nil
}

func (h *actionHandler) resetHost(action sw.StateSwitch, args sw.TransitionArgs) error {
	return nil
}

func (h *actionHandler) conditioResetHost(action sw.StateSwitch, args sw.TransitionArgs) (bool, error) {
	// check if host reset is required
	return true, nil
}

func (h *actionHandler) SaveState(action sw.StateSwitch, args sw.TransitionArgs) error {
	_, ok := args.(*sm.HandlerContext)
	if !ok {
		return sm.ErrInvalidTransitionHandler
	}

	//	action, ok := sw.(*model.Action)
	//	if !ok {
	//		return errors.Wrap(ErrSaveTask, ErrTaskTypeAssertion.Error())
	//	}
	//
	//	if err := a.cache.UpdateTaskAction(a.ctx, *task); err != nil {
	//		return errors.Wrap(ErrSaveTask, err.Error())
	//	}
	//
	return nil
}
