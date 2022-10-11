package outofband

import (
	"context"
	"fmt"

	"github.com/filanov/stateswitch"
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

	transitionTypeLoginBMC        sw.TransitionType = "logginBMC"
	transitionTypeInstallFirmware sw.TransitionType = "installingFirmware"
	transitionTypeUploadFirmware  sw.TransitionType = "uploadingFirmware"
	transitionTypeResetBMC        sw.TransitionType = "resettingBMC"
	transitionTypeResetHost       sw.TransitionType = "resettingHost"

	// state, transition for failed actions
	stateInstallFailed            sw.State          = "installFailed"
	transitionTypeActionFailed    sw.TransitionType = "actionFailed"
	transitionTypeActionsComplete sw.TransitionType = "actionsComplete"
)

var (
	errActionTransition = errors.New("error in action transition")
)

type ActionPlanMachine struct {
	actionID    string
	transitions []sw.TransitionType
	sm          sw.StateMachine
}

func (a *ActionPlanMachine) setTransitionOrder(transitions []sw.TransitionType) {
	a.transitions = transitions
}

// ActionPlan is an ordered list of actions planned
type ActionPlan []*ActionPlanMachine

func (a ActionPlan) ByActionID(id string) *ActionPlanMachine {
	for _, m := range a {
		if m.actionID == id {
			return m
		}
	}

	return nil
}

func NewActionPlanMachine(ctx context.Context) (*ActionPlanMachine, error) {
	order := []sw.TransitionType{
		transitionTypeLoginBMC,
		transitionTypeUploadFirmware,
		transitionTypeInstallFirmware,
		transitionTypeResetBMC,
		transitionTypeResetHost,
	}

	m := &ActionPlanMachine{sm: sw.NewStateMachine(), transitions: order}

	handler := &actionHandler{}

	// The SM has transition rules define the transitionHandler methods
	// each transitionHandler method is passed as values to the transition rule.
	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   transitionTypeLoginBMC,
		SourceStates:     sw.States{stateQueued},
		DestinationState: stateLoginBMC,

		// Condition for the transition, transition will be executed only if this function return true
		// Can be nil, in this case it's considered as return true, nil
		Condition: nil,

		// Transition is users business logic, should not set the state or return next state
		// If condition returns true this function will be executed
		Transition: handler.loginBMC,

		// PostTransition will be called if condition and transition are successful.
		PostTransition: handler.saveState,
	})

	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   transitionTypeUploadFirmware,
		SourceStates:     sw.States{stateLoginBMC},
		DestinationState: stateUploadFirmware,
		Condition:        nil,
		Transition:       handler.uploadFirmware,
		PostTransition:   handler.saveState,
	})

	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   transitionTypeInstallFirmware,
		SourceStates:     sw.States{stateUploadFirmware},
		DestinationState: stateInstallFirmware,
		Condition:        nil,
		Transition:       handler.uploadFirmware,
		PostTransition:   handler.saveState,
	})

	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   transitionTypeResetBMC,
		SourceStates:     sw.States{stateInstallFirmware},
		DestinationState: stateResetHost,
		Condition:        handler.conditionalResetBMC,
		Transition:       handler.resetBMC,
		PostTransition:   handler.saveState,
	})

	m.sm.AddTransition(sw.TransitionRule{
		TransitionType:   transitionTypeResetHost,
		SourceStates:     sw.States{stateResetHost},
		DestinationState: stateSuccess,
		Condition:        handler.conditionalResetHost,
		Transition:       handler.resetHost,
		PostTransition:   handler.saveState,
	})

	m.sm.AddTransition(sw.TransitionRule{
		TransitionType: transitionTypeActionFailed,
		SourceStates: sw.States{
			stateLoginBMC,
			stateUploadFirmware,
			stateInstallFirmware,
			stateResetBMC,
			stateResetHost,
		},
		DestinationState: stateInstallFailed,
		Condition:        nil,
		Transition:       handler.saveState,
		PostTransition:   nil,
	})

	return m, nil
}

func (a *ActionPlanMachine) TransitionFailed(ctx context.Context, action *model.Action, smCtx *taskHandlerContext) error {
	return a.sm.Run(transitionTypeActionFailed, action, smCtx)
}

func (a *ActionPlanMachine) run(ctx context.Context, action *model.Action, smCtx *taskHandlerContext) error {
	for _, transitionType := range a.transitions {
		err := a.sm.Run(transitionType, action, smCtx)
		if err != nil {
			if errors.Is(err, stateswitch.NoConditionPassedToRunTransaction) {
				return errors.Wrap(
					errActionTransition,
					fmt.Sprintf("no transition rule found for transition type '%s' and state '%s'", transitionType, action.Status),
				)
			}

			return errors.Wrap(errActionTransition, err.Error())
		}

	}

	return nil
}
