package outofband

import (
	"fmt"

	sw "github.com/filanov/stateswitch"
	"github.com/metal-toolbox/flasher/internal/model"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
	"github.com/pkg/errors"
)

var (
	ErrSaveAction          = errors.New("error in action state save")
	ErrActionTypeAssertion = errors.New("error in action object type assertion")
)

// taskHandler implements the taskTransitionHandler methods
type actionHandler struct{}

func (h *actionHandler) loginBMC(a sw.StateSwitch, args sw.TransitionArgs) error {
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return sm.ErrInvalidtaskHandlerContext
	}

	// init out of band device queryor - if one isn't already initialized
	// this is done conditionally to enable tests to pass in a device queryor
	if tctx.DeviceQueryor == nil {
		action, ok := a.(*model.Action)
		if !ok {
			return sm.ErrActionTypeAssertion
		}

		task, err := tctx.Store.TaskByID(tctx.Ctx, action.TaskID)
		if err != nil {
			return err
		}

		tctx.DeviceQueryor = NewDeviceQueryor(tctx.Ctx, &task.Parameters.Device, tctx.Logger)
	}

	fmt.Println("login")
	// login
	if err := tctx.DeviceQueryor.Open(tctx.Ctx); err != nil {
		return err
	}

	return nil
}

func (h *actionHandler) conditionInstallFirmware(a sw.StateSwitch, args sw.TransitionArgs) (bool, error) {
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return false, sm.ErrInvalidtaskHandlerContext
	}

	fmt.Println("install condition")
	action, ok := a.(*model.Action)
	if !ok {
		return false, sm.ErrActionTypeAssertion
	}

	_ = action
	_, err := tctx.DeviceQueryor.Inventory(tctx.Ctx)
	if err != nil {
		return false, err
	} //

	// compare installed with current

	return true, nil
}

func (h *actionHandler) uploadFirmware(action sw.StateSwitch, args sw.TransitionArgs) error {

	fmt.Println("upload")
	return nil
}

func (h *actionHandler) installFirmware(action sw.StateSwitch, args sw.TransitionArgs) error {

	fmt.Println("install")
	return nil
}

func (h *actionHandler) conditionalResetBMC(action sw.StateSwitch, args sw.TransitionArgs) (bool, error) {
	// check if BMC reset is required

	fmt.Println("conditional reset")
	return true, nil
}

func (h *actionHandler) resetBMC(action sw.StateSwitch, args sw.TransitionArgs) error {
	return nil
}

func (h *actionHandler) resetHost(action sw.StateSwitch, args sw.TransitionArgs) error {
	return nil
}

func (h *actionHandler) conditionalResetHost(action sw.StateSwitch, args sw.TransitionArgs) (bool, error) {
	// check if host reset is required
	return true, nil
}

func (h *actionHandler) SaveState(a sw.StateSwitch, args sw.TransitionArgs) error {
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return sm.ErrInvalidTransitionHandler
	}

	action, ok := a.(*model.Action)
	if !ok {
		return errors.Wrap(ErrSaveAction, ErrActionTypeAssertion.Error())
	}

	if err := tctx.Store.UpdateTaskAction(tctx.Ctx, tctx.TaskID, *action); err != nil {
		return errors.Wrap(ErrSaveAction, err.Error())
	}

	return nil
}
