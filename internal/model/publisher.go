package model

import (
	"context"

	"github.com/metal-toolbox/rivets/events/controller"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var (
	ErrPublishStatus = errors.New("error in publish Condition status")
	ErrPublishTask   = errors.New("error in publish Condition Task")
)

// Publisher defines methods to publish task information.
type Publisher interface {
	Publish(ctx context.Context, task *Task) error
}

// StatusPublisher implements the Publisher interface
// to wrap the condition controller publish method
type StatusPublisher struct {
	logger *logrus.Entry
	csp    controller.ConditionStatusPublisher
	ctp    controller.ConditionTaskRepository
}

func NewNatsTaskStatusPublisher(logger *logrus.Entry, csp controller.ConditionStatusPublisher, ctp controller.ConditionTaskRepository) Publisher {
	return &StatusPublisher{
		logger,
		csp,
		ctp,
	}
}

func (s *StatusPublisher) Publish(ctx context.Context, task *Task) error {
	if err := s.csp.Publish(
		ctx,
		task.Asset.ID.String(),
		task.State,
		task.Status.MustMarshal(),
	); err != nil {
		err = errors.Wrap(ErrPublishStatus, err.Error())
		s.logger.WithError(err).Error("Condition status publish error")

		return err
	}

	s.logger.Trace("Condition Status publish successful")

	if err := s.ctp.Publish(ctx, task.MustMarshal()); err != nil {
		err = errors.Wrap(ErrPublishTask, err.Error())
		s.logger.WithError(err).Warn("Task publish error")

		return err
	}

	s.logger.Trace("Condition Task publish successful")

	return nil
}
