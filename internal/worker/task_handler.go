package worker

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/bmc-toolbox/common"
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
	*runner.TaskHandlerContext
}

func newHandler(
	task *model.Task,
	storage store.Repository,
	publisher model.Publisher,
	logger *logrus.Entry,
) runner.TaskHandler {
	return &handler{
		&runner.TaskHandlerContext{
			Task:      task,
			Publisher: publisher,
			Store:     storage,
			Logger:    logger,
		},
	}
}

func (t *handler) Initialize(ctx context.Context) error {
	if t.DeviceQueryor == nil {
		// TODO(joel): DeviceQueryor is to be instantiated based on the method(s) for the firmwares to be installed
		// if its a mix of inband, out of band firmware to be installed, then both are to be queried and
		// so this DeviceQueryor would have to be extended
		//
		// For this to work with both inband and out of band, the firmware set data should include the install method.
		t.DeviceQueryor = outofband.NewDeviceQueryor(ctx, t.Task.Asset, t.Logger)
	}

	return nil
}

func (t *handler) Query(ctx context.Context) error {
	t.Logger.Debug("run query step")

	t.Task.Status.Append("connecting to device BMC")
	t.Publish(ctx)

	if err := t.DeviceQueryor.Open(ctx); err != nil {
		return err
	}

	t.Task.Status.Append("collecting inventory from device BMC")
	t.Publish(ctx)

	deviceCommon, err := t.DeviceQueryor.Inventory(ctx)
	if err != nil {
		return errors.Wrap(errTaskQueryInventory, err.Error())
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

func (t *handler) PlanActions(ctx context.Context) error {
	switch t.Task.Data.FirmwarePlanMethod {
	case model.FromFirmwareSet:
		return t.planFromFirmwareSet(ctx)
	case model.FromRequestedFirmware:
		// inband worker code will go down this route
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

	// split inband and out-of-band firmware into two slices
	var oobFW, inbFW []*model.Firmware
	for _, f := range applicable {
		if f.InstallInband {
			inbFW = append(inbFW, f)
		} else {
			oobFW = append(oobFW, f)
		}
	}

	t.Task.Data.ActionsPlanned = []*model.Action{}

	// first plan out-of-band install actions
	oobActions, err := t.planInstallOutofband(ctx, oobFW)
	if err != nil {
		return err
	}

	t.Task.Data.ActionsPlanned = append(t.Task.Data.ActionsPlanned, oobActions...)

	// second plan inband install actions
	inbActions, err := t.planInstallInband(ctx, inbFW)
	if err != nil {
		return err
	}

	t.Task.Data.ActionsPlanned = append(t.Task.Data.ActionsPlanned, inbActions...)

	if len(inbActions) > 0 {

	}

	return nil
}

func (t *handler) planInstallInband(ctx context.Context, firmwares []*model.Firmware) (model.Actions, error) {
	// sort firmware in order of install
	t.sortFirmwareByInstallOrder(firmwares)

	var info string
	if len(firmwares) > 0 {
		info = "no in-band firmware installs required"
	} else {
		info = fmt.Sprintf("%d in-band firmware installs required", len(t.Task.Data.ActionsPlanned))
	}

	t.Task.Status.Append(info)
	t.Publish(ctx)
	t.Logger.Info(info)

	// TODO: implement delegate action handler

	return nil, nil
}

// planInstall sets up the firmware install plan
//
// This returns a list of actions to added to the task and a list of action state machines for those actions.
func (t *handler) planInstallOutofband(ctx context.Context, firmwares []*model.Firmware) (model.Actions, error) {
	actions := model.Actions{}

	t.Logger.WithFields(logrus.Fields{
		"condition.id":             t.Task.ID,
		"requested.firmware.count": fmt.Sprintf("%d", len(firmwares)),
	}).Debug("checking against current inventory")

	toInstall := firmwares

	// purge any firmware that are already installed
	if !t.Task.Parameters.ForceInstall {
		toInstall = t.removeFirmwareAlreadyAtDesiredVersion(firmwares)
	}

	if len(toInstall) == 0 {
		return nil, nil
	}

	// sort firmware in order of install
	t.sortFirmwareByInstallOrder(toInstall)

	// each firmware applicable results in an ActionPlan and an Action
	for idx, firmware := range toInstall {
		actionCtx := &runner.ActionHandlerContext{
			TaskHandlerContext: t.TaskHandlerContext,
			Firmware:           firmware,
			First:              (idx == 0),
			Last:               (idx == len(toInstall)-1),
		}

		actionHandler := &outofband.ActionHandler{}
		action, err := actionHandler.ComposeAction(ctx, actionCtx)
		if err != nil {
			return nil, errors.Wrap(errTaskPlanActions, err.Error())
		}

		action.SetID(t.Task.ID.String(), firmware.Component, idx)
		action.SetState(model.StatePending)

		actions = append(actions, action)
	}

	var info string
	if len(toInstall) > 0 {
		info = "no out-of-band firmware installs required"
	} else {
		info = fmt.Sprintf("%d out-of-band firmware installs required", len(t.Task.Data.ActionsPlanned))
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
		currentVersion, ok := invMap[strings.ToLower(fw.Component)]

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
	if t.DeviceQueryor == nil {
		return
	}

	if err := t.DeviceQueryor.Close(ctx); err != nil {
		t.Logger.WithFields(logrus.Fields{"err": err.Error()}).Warn("device logout error")
	}
}

func (t *handler) OnFailure(ctx context.Context, _ *model.Task) {
	if t.DeviceQueryor == nil {
		return
	}

	if err := t.DeviceQueryor.Close(ctx); err != nil {
		t.Logger.WithFields(logrus.Fields{"err": err.Error()}).Warn("device logout error")
	}
}

func (t *handler) Publish(ctx context.Context) {
	t.Publisher.Publish(ctx, t.Task)
}
