package worker

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bmc-toolbox/common"
	sw "github.com/filanov/stateswitch"
	cptypes "github.com/metal-toolbox/conditionorc/pkg/types"
	"github.com/metal-toolbox/flasher/internal/metrics"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/outofband"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

var (
	ErrSaveTask           = errors.New("error in saveTask transition handler")
	ErrTaskTypeAssertion  = errors.New("error asserting Task type")
	errTaskQueryInventory = errors.New("error in task query inventory for installed firmware")
	errTaskPlanActions    = errors.New("error in task action planning")
)

// taskHandler implements the taskTransitionHandler methods
type taskHandler struct{}

func (h *taskHandler) Init(_ sw.StateSwitch, _ sw.TransitionArgs) error {
	return nil
}

// Query looks up the device component inventory and sets it in the task handler context.
func (h *taskHandler) Query(t sw.StateSwitch, args sw.TransitionArgs) error {
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return sm.ErrInvalidtaskHandlerContext
	}

	_, ok = t.(*model.Task)
	if !ok {
		return errors.Wrap(errTaskQueryInventory, ErrTaskTypeAssertion.Error())
	}

	tctx.Logger.WithFields(logrus.Fields{
		"condition.id": tctx.Task.ID.String(),
		"worker.id":    tctx.WorkerID.String(),
	}).Debug("run query step")

	// attempt to fetch component inventory from the device
	components, err := h.queryFromDevice(tctx)
	if err != nil {
		return errors.Wrap(errTaskQueryInventory, err.Error())
	}

	// component inventory was identified
	if len(components) > 0 {
		tctx.Asset.Components = components

		return nil
	}

	return errors.Wrap(errTaskQueryInventory, "failed to query device component inventory")
}

func (h *taskHandler) Plan(t sw.StateSwitch, args sw.TransitionArgs) error {
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return sm.ErrInvalidtaskHandlerContext
	}

	task, ok := t.(*model.Task)
	if !ok {
		return errors.Wrap(ErrSaveTask, ErrTaskTypeAssertion.Error())
	}

	tctx.Logger.WithFields(logrus.Fields{
		"condition.id": tctx.Task.ID.String(),
		"worker.id":    tctx.WorkerID.String(),
	}).Debug("create the plan")
	switch task.FirmwarePlanMethod {
	case model.FromFirmwareSet:
		return h.planFromFirmwareSet(tctx, task)
	case model.FromRequestedFirmware:
		return errors.Wrap(errTaskPlanActions, "firmware plan method not implemented"+string(model.FromRequestedFirmware))
	default:
		return errors.Wrap(errTaskPlanActions, "firmware plan method invalid: "+string(task.FirmwarePlanMethod))
	}
}

func (h *taskHandler) ValidatePlan(_ sw.StateSwitch, args sw.TransitionArgs) (bool, error) {
	tctx := args.(*sm.HandlerContext)

	tctx.Logger.WithFields(logrus.Fields{
		"condition.id": tctx.Task.ID.String(),
		"worker.id":    tctx.WorkerID.String(),
	}).Debug("validate the plan")
	return true, nil
}

func (h *taskHandler) registerActionMetrics(startTS time.Time, action *model.Action, state string) {
	metrics.ActionRuntimeSummary.With(
		prometheus.Labels{
			"vendor":    action.Firmware.Vendor,
			"component": action.Firmware.Component,
			"state":     state,
		},
	).Observe(time.Since(startTS).Seconds())
}

func (h *taskHandler) Run(t sw.StateSwitch, args sw.TransitionArgs) error {
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return sm.ErrInvalidTransitionHandler
	}

	task, ok := t.(*model.Task)
	if !ok {
		return errors.Wrap(ErrSaveTask, ErrTaskTypeAssertion.Error())
	}

	tctx.Logger.WithFields(logrus.Fields{
		"condition.id": tctx.Task.ID.String(),
		"worker.id":    tctx.WorkerID.String(),
	}).Debug("running the plan")
	// each actionSM (state machine) corresponds to a firmware to be installed
	for _, actionSM := range tctx.ActionStateMachines {
		tctx.Logger.WithFields(logrus.Fields{
			"condition.id":     tctx.Task.ID.String(),
			"worker.id":        tctx.WorkerID.String(),
			"state.machine.id": actionSM.ActionID(),
		}).Debug("state machine start")
		startTS := time.Now()

		// fetch action attributes from task
		action := task.ActionsPlanned.ByID(actionSM.ActionID())
		if err := action.SetState(model.StateActive); err != nil {
			return err
		}

		// return on context cancellation
		if tctx.Ctx.Err() != nil {
			h.registerActionMetrics(startTS, action, string(cptypes.Failed))

			return tctx.Ctx.Err()
		}

		// run the action state machine
		err := actionSM.Run(tctx.Ctx, action, tctx)
		if err != nil {
			h.registerActionMetrics(startTS, action, string(cptypes.Failed))

			return errors.Wrap(
				err,
				"while running action to install firmware on component "+action.Firmware.Component,
			)
		}

		h.registerActionMetrics(startTS, action, string(cptypes.Succeeded))
		tctx.Logger.WithFields(logrus.Fields{
			"condition.id":     tctx.Task.ID.String(),
			"worker.id":        tctx.WorkerID.String(),
			"state.machine.id": actionSM.ActionID(),
		}).Debug("state machine end")
	}

	tctx.Logger.WithFields(logrus.Fields{
		"condition.id": tctx.Task.ID.String(),
		"worker.id":    tctx.WorkerID.String(),
	}).Debug("plan finished")
	return nil
}

func (h *taskHandler) TaskFailed(_ sw.StateSwitch, args sw.TransitionArgs) error {
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return sm.ErrInvalidTransitionHandler
	}

	if tctx.DeviceQueryor != nil {
		if err := tctx.DeviceQueryor.Close(tctx.Ctx); err != nil {
			tctx.Logger.WithFields(logrus.Fields{"err": err.Error()}).Warn("device logout error")
		}
	}

	return nil
}

func (h *taskHandler) TaskSuccessful(_ sw.StateSwitch, args sw.TransitionArgs) error {
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return sm.ErrInvalidTransitionHandler
	}

	if tctx.DeviceQueryor != nil {
		if err := tctx.DeviceQueryor.Close(tctx.Ctx); err != nil {
			tctx.Logger.WithFields(logrus.Fields{"err": err.Error()}).Warn("device logout error")
		}
	}

	return nil
}

func (h *taskHandler) PublishStatus(_ sw.StateSwitch, args sw.TransitionArgs) error {
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return sm.ErrInvalidTransitionHandler
	}

	tctx.Publisher.Publish(tctx)

	return nil
}

// planFromFirmwareSet
func (h *taskHandler) planFromFirmwareSet(tctx *sm.HandlerContext, task *model.Task) error {
	applicable, err := tctx.Store.FirmwareSetByID(tctx.Ctx, task.Parameters.FirmwareSetID)
	if err != nil {
		return errors.Wrap(errTaskPlanActions, err.Error())
	}

	if len(applicable) == 0 {
		// XXX: why not just short-circuit success here on the GIGO theory?
		return errors.Wrap(errTaskPlanActions, "planFromFirmwareSet(): firmware set lacks any members")
	}

	// plan actions based and update task action list
	tctx.ActionStateMachines, task.ActionsPlanned, err = h.planInstall(tctx, task, applicable)
	if err != nil {
		return err
	}

	return nil
}

// query device components inventory from the device itself.
func (h *taskHandler) queryFromDevice(tctx *sm.HandlerContext) (model.Components, error) {
	if tctx.DeviceQueryor == nil {
		// TODO(joel): DeviceQueryor is to be instantiated based on the method(s) for the firmwares to be installed
		// if its a mix of inband, out of band firmware to be installed, then both are to be queried and
		// so this DeviceQueryor would have to be extended
		//
		// For this to work with both inband and out of band, the firmware set data should include the install method.
		tctx.DeviceQueryor = outofband.NewDeviceQueryor(tctx.Ctx, tctx.Asset, tctx.Logger)
	}

	tctx.Task.Status = "connecting to device BMC"
	tctx.Publisher.Publish(tctx)

	if err := tctx.DeviceQueryor.Open(tctx.Ctx); err != nil {
		return nil, err
	}

	tctx.Task.Status = "collecting inventory from device BMC"
	tctx.Publisher.Publish(tctx)

	deviceCommon, err := tctx.DeviceQueryor.Inventory(tctx.Ctx)
	if err != nil {
		return nil, err
	}

	if tctx.Asset.Vendor == "" {
		tctx.Asset.Vendor = deviceCommon.Vendor
	}

	if tctx.Asset.Model == "" {
		tctx.Asset.Model = common.FormatProductName(deviceCommon.Model)
	}

	return model.NewComponentConverter().CommonDeviceToComponents(deviceCommon)
}

// planInstall sets up the firmware install plan
//
// This returns a list of actions to added to the task and a list of action state machines for those actions.
func (h *taskHandler) planInstall(hCtx *sm.HandlerContext, task *model.Task, firmwares []*model.Firmware) (sm.ActionStateMachines, model.Actions, error) {
	actionMachines := make(sm.ActionStateMachines, 0)
	actions := make(model.Actions, 0)

	// final is set to true in the final action
	var final bool

	hCtx.Logger.WithFields(logrus.Fields{
		"condition.id":             task.ID,
		"requested.firmware.count": fmt.Sprintf("%d", len(firmwares)),
	}).Info("checking against current inventory")
	toInstall := firmwares
	if !task.Parameters.ForceInstall {
		toInstall = removeFirmwareAlreadyAtDesiredVersion(hCtx, firmwares)
	}

	if len(toInstall) == 0 {
		hCtx.Logger.WithFields(logrus.Fields{
			"condition.id": task.ID,
			"worker.id":    hCtx.WorkerID.String(),
			"server.id":    hCtx.Asset.ID.String(),
		}).Info("no action required for this task")
		return actionMachines, actions, nil
	}

	sortFirmwareByInstallOrder(toInstall)
	// each firmware applicable results in an ActionPlan and an Action
	for idx, firmware := range toInstall {
		// set final bool when its the last firmware in the slice
		final = (idx == len(toInstall)-1)

		// generate an action ID
		actionID := sm.ActionID(task.ID.String(), firmware.Component, idx)

		// TODO: The firmware is to define the preferred install method
		// based on that the action plan is setup.
		//
		// For now this is hardcoded to outofband.
		m, err := outofband.NewActionStateMachine(actionID)
		if err != nil {
			return nil, nil, err
		}

		// include action state machines that will be executed.
		actionMachines = append(actionMachines, m)

		newAction := model.Action{
			ID:     actionID,
			TaskID: task.ID.String(),

			// TODO: The firmware is to define the preferred install method
			// based on that the action plan is setup.
			//
			// For now this is hardcoded to outofband.
			InstallMethod: model.InstallMethodOutofband,

			// Firmware is the firmware to be installed
			Firmware: *firmwares[idx],

			// VerifyCurrentFirmware is disabled when ForceInstall is true.
			VerifyCurrentFirmware: !task.Parameters.ForceInstall,

			// Final is set to true when its the last action in the list.
			Final: final,
		}

		if err := newAction.SetState(model.StatePending); err != nil {
			return nil, nil, err
		}

		// create action thats added to the task
		actions = append(actions, &newAction)
	}

	return actionMachines, actions, nil
}

func sortFirmwareByInstallOrder(firmwares []*model.Firmware) {
	sort.Slice(firmwares, func(i, j int) bool {
		slugi := strings.ToLower(firmwares[i].Component)
		slugj := strings.ToLower(firmwares[j].Component)
		return model.FirmwareInstallOrder[slugi] < model.FirmwareInstallOrder[slugj]
	})
}

func removeFirmwareAlreadyAtDesiredVersion(hCtx *sm.HandlerContext, fws []*model.Firmware) []*model.Firmware {
	var toInstall []*model.Firmware

	// only iterate the inventory once
	invMap := make(map[string]string)
	for _, cmp := range hCtx.Asset.Components {
		invMap[strings.ToLower(cmp.Slug)] = cmp.FirmwareInstalled
	}

	// XXX: this will drop firmware for components that are specified in
	// the firmware set but not in the inventory. This is consistent with the
	// desire of users to not require a force or a re-run to accomplish an
	// attainable goal.
	for _, fw := range fws {
		currentVersion, ok := invMap[strings.ToLower(fw.Component)]
		switch {
		case !ok:
			hCtx.Logger.WithFields(logrus.Fields{
				"condition.id": hCtx.Task.ID,
				"worker.id":    hCtx.WorkerID.String(),
				"component":    fw.Component,
				"server.id":    hCtx.Asset.ID.String(),
			}).Warn("inventory missing component")
		case currentVersion == fw.Version:
			hCtx.Logger.WithFields(logrus.Fields{
				"condition.id": hCtx.Task.ID,
				"worker.id":    hCtx.WorkerID.String(),
				"component":    fw.Component,
				"server.id":    hCtx.Asset.ID.String(),
				"version":      fw.Version,
			}).Debug("inventory firmware version matches set")
		default:
			hCtx.Logger.WithFields(logrus.Fields{
				"condition.id":      hCtx.Task.ID,
				"worker.id":         hCtx.WorkerID.String(),
				"component":         fw.Component,
				"server.id":         hCtx.Asset.ID.String(),
				"installed.version": currentVersion,
				"mandated.version":  fw.Version,
			}).Debug("firmware queued for install")
			toInstall = append(toInstall, fw)
		}
	}
	return toInstall
}
