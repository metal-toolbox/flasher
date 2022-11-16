package worker

import (
	"context"
	"time"

	"github.com/metal-toolbox/flasher/internal/inventory"
	"github.com/metal-toolbox/flasher/internal/model"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"

	"github.com/sirupsen/logrus"
)

func (o *Worker) newtaskHandlerContext(ctx context.Context, taskID string, device *model.Device, skipCompareInstalled bool) *sm.HandlerContext {
	l := logrus.New()
	l.Formatter = o.logger.Formatter
	l.Level = o.logger.Level

	return &sm.HandlerContext{
		WorkerID:          o.id,
		Dryrun:            o.dryrun,
		TaskID:            taskID,
		Ctx:               ctx,
		Store:             o.store,
		Inv:               o.inv,
		TaskEventCh:       o.taskEventCh,
		FirmwareURLPrefix: o.firmwareURLPrefix,
		Data:              make(map[string]string),
		Device:            device,
		Logger: l.WithFields(
			logrus.Fields{
				"workerID": o.id,
				"taskID":   taskID,
				"deviceID": device.ID.String(),
				"bmc":      device.BmcAddress.String(),
			},
		),
	}
}

func (o *Worker) persistTaskStatus(taskID, info string) {
	// a new context is created here to make sure the task status is persisted
	// even if the worker parent context is closed
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	o.logger.WithFields(
		logrus.Fields{
			"taskID": taskID,
			"info":   info,
		}).Trace("received task status to persist")

	task, err := o.store.TaskByID(ctx, taskID)
	if err != nil {
		o.logger.WithFields(
			logrus.Fields{
				"taskID": taskID,
				"err":    err.Error(),
			}).Warn("error retrieving task attributes")

		return
	}

	attr := &inventory.FwInstallAttributes{
		TaskParameters: task.Parameters,
		FlasherTaskID:  task.ID.String(),
		WorkerID:       o.id,
		Status:         task.Status,
		Info:           info,
	}

	if err := o.inv.SetFlasherAttributes(ctx, task.Parameters.Device.ID.String(), attr); err != nil {
		o.logger.WithFields(
			logrus.Fields{
				"deviceID": task.Parameters.Device.ID.String(),
				"err":      err.Error(),
			}).Warn("error writing task attributes to inventory")
	}
}
