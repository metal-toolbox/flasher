package outofband

import (
	sw "github.com/filanov/stateswitch"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/pkg/errors"
)

const (
	// action states
	//
	// the SM transitions through these states for each component being updated.
	stateLoginBMC        sw.State = "loginBMC"
	stateUploadFirmware  sw.State = "uploadFirmware"
	stateInstallFirmware sw.State = "installFirmware"
	stateResetBMC        sw.State = "resetBMC"
	stateResetHost       sw.State = "resetHost"

	transitionTypeLoginBMC        sw.TransitionType = "loginBMC"
	transitionTypeInstallFirmware sw.TransitionType = "installFirmware"
	transitionTypeUploadFirmware  sw.TransitionType = "uploadFirmware"
	transitionTypeResetBMC        sw.TransitionType = "resetBMC"
	transitionTypeResetHost       sw.TransitionType = "resetHost"

	// state, transition for failed actions
	stateInstallFailed         sw.State          = "installFailed"
	transitionTypeActionFailed sw.TransitionType = "actionFailed"
)

type actionTransitionHandler interface {
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
	a, ok := args.(*StateMachineContext)
	if !ok {
		return errInvalidTransitionHandler
	}

	action, ok := sw.(*model.Action)
	if !ok {
		return errors.Wrap(ErrSaveTask, ErrTaskTypeAssertions.Error())
	}

	if err := a.cache.UpdateTaskAction(a.ctx, *task); err != nil {
		return errors.Wrap(ErrSaveTask, err.Error())
	}

	return nil
}
