package worker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/metal-toolbox/flasher/internal/inventory"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/outofband"
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

// planInstall sets up the firmware install plan
//
// This returns a list of actions to added to the task and a list of action state machines for those actions.
func planInstall(ctx context.Context, task *model.Task, firmwareURLPrefix string) (sm.ActionStateMachines, model.Actions, error) {
	plans := make(sm.ActionStateMachines, 0)
	actions := make(model.Actions, 0)

	// sort the firmware for install
	task.FirmwaresPlanned.SortForInstall()

	var final bool
	// each firmware planned results in an ActionPlan and an Action
	for idx, firmware := range task.FirmwaresPlanned {
		actionID := sm.ActionID(task.ID.String(), firmware.ComponentSlug, idx)

		// TODO: The firmware is to define the preferred install method
		// based on that the action plan is setup.
		//
		// For now this is hardcoded to outofband.
		m, err := outofband.NewOutofbandActionStateMachine(ctx, actionID)
		if err != nil {
			return nil, nil, err
		}

		plans = append(plans, m)

		if len(task.FirmwaresPlanned) > 1 {
			final = (idx == len(task.FirmwaresPlanned)-1)
		} else {
			final = true
		}

		// set download url based on device vendor, model attributes
		// example : https://firmware.hosted/firmware/dell/r640/bmc/iDRAC-with-Lifecycle-Controller_Firmware_P8HC9_WN64_5.10.00.00_A00.EXE
		task.FirmwaresPlanned[idx].URL = fmt.Sprintf(
			"%s/%s/%s/%s/%s",
			firmwareURLPrefix,
			task.Parameters.Device.Vendor,
			task.Parameters.Device.Model,
			strings.ToLower(firmware.ComponentSlug),
			firmware.FileName,
		)

		actions = append(actions, model.Action{
			ID:     actionID,
			TaskID: task.ID.String(),
			// TODO: The firmware is to define the preferred install method
			// based on that the action plan is setup.
			//
			// For now this is hardcoded to outofband.
			InstallMethod: model.InstallMethodOutofband,
			Status:        string(model.StateQueued),
			Firmware:      task.FirmwaresPlanned[idx],
			// Final is set to true when its the last action in the list.
			Final: final,
		})

	}

	return plans, actions, nil
}

func (o *Worker) persistTaskStatus(taskID, info string) {
	// a new context is created here to make sure the task status is persisted
	// even if the worker parent context is closed
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	task, err := o.store.TaskByID(ctx, taskID)
	if err != nil {
		o.logger.WithFields(
			logrus.Fields{
				"deviceID": task.Parameters.Device.ID.String(),
				"err":      err.Error(),
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
