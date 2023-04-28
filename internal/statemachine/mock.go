package statemachine

import (
	sw "github.com/filanov/stateswitch"
)

// MockTaskHandler implements the TaskTransitioner interface
type MockTaskHandler struct{}

func (h *MockTaskHandler) Init(_ sw.StateSwitch, _ sw.TransitionArgs) error {
	return nil
}

func (h *MockTaskHandler) Query(_ sw.StateSwitch, _ sw.TransitionArgs) error {
	return nil
}

func (h *MockTaskHandler) Plan(_ sw.StateSwitch, _ sw.TransitionArgs) error {
	return nil
}

func (h *MockTaskHandler) ValidatePlan(_ sw.StateSwitch, _ sw.TransitionArgs) (bool, error) {
	return true, nil
}

func (h *MockTaskHandler) Run(_ sw.StateSwitch, _ sw.TransitionArgs) error {
	return nil
}

func (h *MockTaskHandler) TaskFailed(_ sw.StateSwitch, _ sw.TransitionArgs) error {
	return nil
}

func (h *MockTaskHandler) TaskSuccessful(_ sw.StateSwitch, _ sw.TransitionArgs) error {
	return nil
}

func (h *MockTaskHandler) PublishStatus(_ sw.StateSwitch, _ sw.TransitionArgs) error {
	return nil
}
