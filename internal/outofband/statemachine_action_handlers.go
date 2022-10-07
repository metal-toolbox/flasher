package outofband

import (
	sw "github.com/filanov/stateswitch"
)

type actionTransitioner interface {
	loginBMC(sw sw.StateSwitch, args sw.TransitionArgs) (bool, error)
	uploadFirmware(sw sw.StateSwitch, args sw.TransitionArgs) error
	installFirmware(sw sw.StateSwitch, args sw.TransitionArgs) error
	resetBMC(sw sw.StateSwitch, args sw.TransitionArgs) error
	resetHost(sw sw.StateSwitch, args sw.TransitionArgs) error
}

// taskHandler implements the taskTransitionHandler methods
type actionHandler struct{}

func (s *actionHandler) loginBMC(sw sw.StateSwitch, args sw.TransitionArgs) error {
	return nil
}

func (s *actionHandler) uploadFirmware(sw sw.StateSwitch, args sw.TransitionArgs) error {
	return nil
}

func (s *actionHandler) installFirmware(sw sw.StateSwitch, args sw.TransitionArgs) error {
	return nil
}

func (s *actionHandler) conditionalResetBMC(sw sw.StateSwitch, args sw.TransitionArgs) (bool, error) {
	// check if BMC reset is required
	return true, nil
}

func (s *actionHandler) resetBMC(sw sw.StateSwitch, args sw.TransitionArgs) error {
	return nil
}

func (s *actionHandler) resetHost(sw sw.StateSwitch, args sw.TransitionArgs) error {
	return nil
}

func (s *actionHandler) conditionalResetHost(sw sw.StateSwitch, args sw.TransitionArgs) (bool, error) {
	// check if host reset is required
	return true, nil
}

func (h *actionHandler) saveState(sw sw.StateSwitch, args sw.TransitionArgs) error {
	_, ok := args.(*taskHandlerContext)
	if !ok {
		return errInvalidTransitionHandler
	}

	//	action, ok := sw.(*model.Action)
	//	if !ok {
	//		return errors.Wrap(ErrSaveTask, ErrTaskTypeAssertions.Error())
	//	}
	//
	//	if err := a.cache.UpdateTaskAction(a.ctx, *task); err != nil {
	//		return errors.Wrap(ErrSaveTask, err.Error())
	//	}
	//
	return nil
}
