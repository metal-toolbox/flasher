package model

import (
	"context"

	"github.com/metal-toolbox/rivets/events/controller"
)

// Publisher defines methods to publish task information.
type Publisher interface {
	Publish(ctx context.Context, task *Task)
}

// StatusPublisher implements the Publisher interface
// to wrap the condition controller publish method
type StatusPublisher struct {
	cp controller.ConditionStatusPublisher
}

// TODO: this needs to publish the whole task to the KV
func NewNatsTaskStatusPublisher(cp controller.ConditionStatusPublisher) Publisher {
	return &StatusPublisher{cp}
}

func (s *StatusPublisher) Publish(ctx context.Context, task *Task) {
	s.cp.Publish(
		ctx,
		task.Asset.ID.String(),
		task.State,
		task.Status.MustMarshal(),
	)
}
