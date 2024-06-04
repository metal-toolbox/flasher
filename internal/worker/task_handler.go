package worker

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/bmc-toolbox/common"
	"github.com/metal-toolbox/flasher/internal/device"
	"github.com/metal-toolbox/flasher/internal/inband"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/outofband"
	"github.com/metal-toolbox/flasher/internal/runner"
	"github.com/metal-toolbox/flasher/internal/store"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var (
	ErrSaveTask           = errors.New("error in saveTask transition handler")
	ErrTaskTypeAssertion  = errors.New("error asserting Task type")
	errTaskQueryInventory = errors.New("error in task query inventory for installed firmware")
	errTaskPlanActions    = errors.New("error in task action planning")
)

// handler implements the task.Handler interface
//
// The handler is instantiated to run a single task
type handler struct {
	mode model.RunMode
	*runner.TaskHandlerContext
}

func newHandler(
	mode model.RunMode,
	task *model.Task,
	storage store.Repository,
	publisher model.Publisher,
	logger *logrus.Entry,
) runner.TaskHandler {
	return &handler{
		mode: mode,
		TaskHandlerContext: &runner.TaskHandlerContext{
			Task:      task,
			Publisher: publisher,
			Store:     storage,
			Logger:    logger,
		},
	}
}

func (t *handler) Initialize(ctx context.Context) error {
	if t.DeviceQueryor == nil {
		switch t.mode {
		case model.RunInband:
			t.DeviceQueryor = inband.NewDeviceQueryor(t.Logger)
		case model.RunOutofband:
			t.DeviceQueryor = outofband.NewDeviceQueryor(ctx, t.Task.Asset, t.Logger)
		}
	}

	return nil
}

func (t *handler) Query(ctx context.Context) error {
	t.Logger.Debug("run query step")

	var err error
	var deviceCommon *common.Device
	switch t.mode {
	case model.RunInband:
		deviceCommon, err = t.inventoryInband(ctx)
		if err != nil {
			return err
		}
	case model.RunOutofband:
		deviceCommon, err = t.inventoryOutofband(ctx)
		if err != nil {
			return err
		}
	}

	if t.Task.Asset.Vendor == "" {
		t.Task.Asset.Vendor = deviceCommon.Vendor
	}

	if t.Task.Asset.Model == "" {
		t.Task.Asset.Model = common.FormatProductName(deviceCommon.Model)
	}

	components, err := model.NewComponentConverter().CommonDeviceToComponents(deviceCommon)
	if err != nil {
		return errors.Wrap(errTaskQueryInventory, err.Error())
	}

	// component inventory was identified
	if len(components) > 0 {
		t.Task.Asset.Components = components

		return nil
	}

	return errors.Wrap(errTaskQueryInventory, "failed to query device component inventory")
}

func (t handler) inventoryOutofband(ctx context.Context) (*common.Device, error) {
	if err := t.DeviceQueryor.(device.OutofbandQueryor).Open(ctx); err != nil {
		return nil, err
	}

	t.Task.Status.Append("connecting to device BMC")
	t.Publish(ctx)
	if err := t.DeviceQueryor.(device.OutofbandQueryor).Open(ctx); err != nil {
		return nil, err
	}

	t.Task.Status.Append("collecting inventory from device BMC")
	t.Publish(ctx)

	deviceCommon, err := t.DeviceQueryor.(device.OutofbandQueryor).Inventory(ctx)
	if err != nil {
		return nil, errors.Wrap(errTaskQueryInventory, err.Error())
	}

	return deviceCommon, nil
}

func (t handler) inventoryInband(ctx context.Context) (*common.Device, error) {
	t.Task.Status.Append("collecting inventory from server")
	t.Publish(ctx)

	deviceCommon, err := t.DeviceQueryor.(device.InbandQueryor).Inventory(ctx)
	if err != nil {
		return nil, errors.Wrap(errTaskQueryInventory, err.Error())
	}

	return deviceCommon, nil
}

func (t *handler) PlanActions(ctx context.Context) error {
	if t.Task.State == model.StateActive && len(t.Task.Data.ActionsPlanned) > 0 {
		t.Logger.WithFields(logrus.Fields{
			"condition.id":             t.Task.ID,
		}).Info("")
	}

	switch t.Task.Data.FirmwarePlanMethod {
	case model.FromFirmwareSet:
		return t.planFromFirmwareSet(ctx)
	case model.FromRequestedFirmware:
		return errors.Wrap(errTaskPlanActions, "firmware plan method not implemented"+string(model.FromRequestedFirmware))
	default:
		return errors.Wrap(errTaskPlanActions, "firmware plan method invalid: "+string(t.Task.Data.FirmwarePlanMethod))
	}
}

// planFromFirmwareSet
func (t *handler) planFromFirmwareSet(ctx context.Context) error {
	applicable, err := t.Store.FirmwareSetByID(ctx, t.Task.Parameters.FirmwareSetID)
	if err != nil {
		return errors.Wrap(errTaskPlanActions, err.Error())
	}

	if len(applicable) == 0 {
		// XXX: why not just short-circuit success here on the GIGO theory?
		return errors.Wrap(errTaskPlanActions, "planFromFirmwareSet(): firmware set lacks any members")
	}

	actions, err := t.planInstallActions(ctx, applicable)
	if err != nil {
		return err
	}

	t.Task.Data.ActionsPlanned = append(t.Task.Data.ActionsPlanned, actions...)

	return nil
}

// planInstall sets up the firmware install plan
//
// This returns a list of actions to added to the task and a list of action state machines for those actions.
func (t *handler) planInstallActions(ctx context.Context, firmwares []*model.Firmware) (model.Actions, error) {
	toInstall := []*model.Firmware{}

	for _, fw := range firmwares {
		if t.mode == model.RunOutofband && !fw.InstallInband {
			toInstall = append(toInstall, fw)
		}

		if t.mode == model.RunInband && fw.InstallInband {
			toInstall = append(toInstall, fw)
		}
	}

	t.Logger.WithFields(logrus.Fields{
		"condition.id":             t.Task.ID,
		"requested.firmware.count": fmt.Sprintf("%d", len(toInstall)),
	}).Debug("checking against current inventory")

	// purge any firmware that are already installed
	if !t.Task.Parameters.ForceInstall {
		toInstall = t.removeFirmwareAlreadyAtDesiredVersion(toInstall)
	}

	if len(toInstall) == 0 {
		info := fmt.Sprintf("no %s firmware installs required", t.mode)
		t.Task.Status.Append(info)
		t.Publish(ctx)

		return nil, nil
	}

	// sort firmware in order of install
	t.sortFirmwareByInstallOrder(toInstall)

	actions := model.Actions{}
	// each firmware applicable results in an ActionPlan and an Action
	for idx, firmware := range toInstall {
		var actionHander runner.ActionHandler

		if t.mode == model.RunOutofband {
			if firmware.InstallInband {
				continue
			}

			actionHander = &outofband.ActionHandler{}
		}

		if t.mode == model.RunInband {
			if !firmware.InstallInband {
				continue
			}

			actionHander = &inband.ActionHandler{}
		}

		actionCtx := &runner.ActionHandlerContext{
			TaskHandlerContext: t.TaskHandlerContext,
			Firmware:           firmware,
			First:              (idx == 0),
			Last:               (idx == len(toInstall)-1),
		}

		action, err := actionHander.ComposeAction(ctx, actionCtx)
		if err != nil {
			return nil, errors.Wrap(errTaskPlanActions, err.Error())
		}

		action.SetID(t.Task.ID.String(), firmware.Component, idx)
		action.SetState(model.StatePending)
		actions = append(actions, action)
	}

	var info string
	if len(actions) > 0 {
		info = fmt.Sprintf("firmware installs planned, method: %s, count: %d", len(actions), t.mode)
	} else {
		info = fmt.Sprintf("no %s firmware installs required", t.mode)
	}

	t.Task.Status.Append(info)
	t.Publish(ctx)
	t.Logger.Info(info)

	return actions, nil
}

func (t *handler) sortFirmwareByInstallOrder(firmwares []*model.Firmware) {
	sort.Slice(firmwares, func(i, j int) bool {
		slugi := strings.ToLower(firmwares[i].Component)
		slugj := strings.ToLower(firmwares[j].Component)
		return model.FirmwareInstallOrder[slugi] < model.FirmwareInstallOrder[slugj]
	})
}

// returns a list of firmware applicable and a list of causes for firmwares that were removed from the install list.
func (t *handler) removeFirmwareAlreadyAtDesiredVersion(fws []*model.Firmware) []*model.Firmware {
	var toInstall []*model.Firmware

	//	key := func(cmpName, cmpSerial string) string {
	//		return fmt.Sprintf("%s.%s", cmpName, cmpSerial)
	//	}

	// NOTE: if theres drives of two different models then we want to update those
	// this map will not enable that.
	invMap := make(map[string]string)
	for _, cmp := range t.Task.Asset.Components {
		invMap[strings.ToLower(cmp.Name)] = cmp.Firmware.Installed
	}

	fmtCause := func(component, cause, currentV, requestedV string) string {
		if currentV != "" && requestedV != "" {
			return fmt.Sprintf("[%s] %s, current=%s, requested=%s", component, cause, currentV, requestedV)
		}

		return fmt.Sprintf("[%s] %s", component, cause)
	}

	// XXX: this will drop firmware for components that are specified in
	// the firmware set but not in the inventory. This is consistent with the
	// desire of users to not require a force or a re-run to accomplish an
	// attainable goal.
	for _, fw := range fws {

		currentVersion, ok := invMap[fw.Component]

		// skip install if current firmware version was not identified
		if currentVersion == "" && !t.Task.Parameters.ForceInstall {
			info := "Current firmware version returned empty, skipped install, use force to override"
			t.Task.Status.Append(
				fmtCause(
					fw.Component,
					info,
					currentVersion,
					fw.Version,
				),
			)

			t.Logger.WithFields(logrus.Fields{
				"component": fw.Component,
			}).Warn()

			continue
		}

		switch {
		case !ok:
			cause := "component not found in inventory"
			t.Logger.WithFields(logrus.Fields{
				"component": fw.Component,
			}).Warn(cause)

			t.Task.Status.Append(fmtCause(fw.Component, cause, "", ""))

		case strings.EqualFold(currentVersion, fw.Version):
			cause := "component firmware version equal"
			t.Logger.WithFields(logrus.Fields{
				"component": fw.Component,
				"version":   fw.Version,
			}).Debug(cause)

			t.Task.Status.Append(fmtCause(fw.Component, cause, currentVersion, fw.Version))

		default:
			t.Logger.WithFields(logrus.Fields{
				"component":         fw.Component,
				"installed.version": currentVersion,
				"mandated.version":  fw.Version,
			}).Debug("firmware queued for install")

			toInstall = append(toInstall, fw)

			t.Task.Status.Append(
				fmtCause(fw.Component, "firmware queued for install", currentVersion, fw.Version),
			)
		}
	}

	return toInstall
}

func (t *handler) OnSuccess(ctx context.Context, _ *model.Task) {
	if t.mode == model.RunInband || t.DeviceQueryor == nil {
		return
	}

	if err := t.DeviceQueryor.(device.OutofbandQueryor).Close(ctx); err != nil {
		t.Logger.WithFields(logrus.Fields{"err": err.Error()}).Warn("device logout error")
	}
}

func (t *handler) OnFailure(ctx context.Context, _ *model.Task) {
	if t.mode == model.RunInband || t.DeviceQueryor == nil {
		return
	}

	if err := t.DeviceQueryor.(device.OutofbandQueryor).Close(ctx); err != nil {
		t.Logger.WithFields(logrus.Fields{"err": err.Error()}).Warn("device logout error")
	}
}

func (t *handler) Publish(ctx context.Context) {
	t.Publisher.Publish(ctx, t.Task)
}
