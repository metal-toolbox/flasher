package model

import (
	"context"
	"net"

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
	cp     controller.Publisher
}

func NewTaskStatusPublisher(logger *logrus.Entry, cp controller.Publisher) Publisher {
	return &StatusPublisher{
		logger,
		cp,
	}
}

func (s *StatusPublisher) Publish(ctx context.Context, task *Task) error {
	genericTask, err := CopyAsGenericTask(task)
	if err != nil {
		err = errors.Wrap(ErrPublishTask, err.Error())
		s.logger.WithError(err).Warn("Task publish error")

		return err
	}

	// overwrite credentials before this gets written back to the repository
	genericTask.Asset.BmcAddress = net.IP{}
	genericTask.Asset.BmcPassword = ""
	genericTask.Asset.BmcUsername = ""

	if err := s.cp.Publish(ctx, genericTask, false); err != nil {
		err = errors.Wrap(ErrPublishStatus, err.Error())
		s.logger.WithError(err).Error("Condition status publish error")

		return err
	}

	s.logger.Trace("Condition Status publish successful")
	return nil
}
