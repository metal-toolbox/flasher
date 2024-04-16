package model

import (
	"context"

	rctypes "github.com/metal-toolbox/rivets/condition"
	"github.com/pkg/errors"
)

// A Task comprises of Action(s) for each firmware to be installed,
// An Action includes multiple steps to have firmware installed.

// StepName identifies a single step within an action
type StepName string

// StepGroup identifies a group of units.
type StepGroup string

// StepHandler defines the signature for each action unit to be executed
type StepHandler func(ctx context.Context) error

// Step is the smallest unit of work within an Action
type Step struct {
	Name        StepName      `json:"name"`
	Handler     StepHandler   `json:"-"`
	Group       StepGroup     `json:"step_group"`
	PostStep    StepHandler   `json:"-"`
	Description string        `json:"doc"`
	State       rctypes.State `json:"state"`
	Status      string        `json:"status"`
}

func (s *Step) SetState(state rctypes.State) {
	s.State = rctypes.State(state)
}

func (s *Step) SetStatus(status string) {
	s.Status = status
}

// Steps is the list of steps to be executed
type Steps []*Step

// ByName returns the step identified by its name
func (us Steps) ByName(name StepName) (u Step, err error) {
	errNotFound := errors.New("step not found by Name")
	for _, unit := range us { // nolint:gocritic // we're fine with 128 bytes being copied
		if unit.Name == name {
			return *unit, nil
		}
	}

	return Step{}, errors.Wrap(errNotFound, string(name))
}

// ByGroup returns steps identified by the matching Group attribute
func (us Steps) ByGroup(name StepGroup) (found Steps, err error) {
	errNotFound := errors.New("step not found by Group")

	for _, elem := range us { // nolint:gocritic // we're fine with 128 bytes being copied
		if elem.Group == name {
			found = append(found, elem)
		}
	}

	if len(found) == 0 {
		return found, errors.Wrap(errNotFound, string(name))
	}

	return found, nil
}

func (us Steps) Remove(name StepName) (final Steps) {
	// nolint:gocritic // insert good reason
	for _, t := range us {
		if t.Name == name {
			continue
		}

		final = append(final, t)
	}

	return final
}
