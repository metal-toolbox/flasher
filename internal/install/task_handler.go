package install

import (
	"context"

	"github.com/bmc-toolbox/common"
	"github.com/metal-toolbox/flasher/internal/device"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/outofband"
	"github.com/metal-toolbox/flasher/internal/runner"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	rtypes "github.com/metal-toolbox/rivets/types"
)

var (
	ErrSaveTask           = errors.New("error in saveTask transition handler")
	ErrTaskTypeAssertion  = errors.New("error asserting Task type")
	errTaskQueryInventory = errors.New("error in task query inventory for installed firmware")
)

// handler implements the Runner.Handler interface
//
// The handler is instantiated to run a single task
type handler struct {
	taskCtx  *runner.TaskHandlerContext
	fwFile   string
	onlyPlan bool
}

func (t *handler) Initialize(ctx context.Context) error {
	if t.taskCtx.DeviceQueryor == nil {
		// TODO(joel): DeviceQueryor is to be instantiated based on the method(s) for the firmwares to be installed
		// if its a mix of inband, out of band firmware to be installed, then both are to be queried and
		// so this DeviceQueryor would have to be extended
		//
		// For this to work with both inband and out of band, the firmware set data should include the install method.
		t.taskCtx.DeviceQueryor = outofband.NewDeviceQueryor(ctx, t.taskCtx.Task.Server, t.taskCtx.Logger)
	}

	return nil
}

// Query looks up the device component inventory and sets it in the task handler context.
func (t *handler) Query(ctx context.Context) error {
	t.taskCtx.Logger.Debug("run query step")

	// attempt to fetch component inventory from the device
	components, err := t.queryFromDevice(ctx)
	if err != nil {
		return errors.Wrap(errTaskQueryInventory, err.Error())
	}

	// component inventory was identified
	if len(components) > 0 {
		t.taskCtx.Task.Server.Components = components

		return nil
	}

	return errors.Wrap(errTaskQueryInventory, "failed to query device component inventory")
}

func (t *handler) PlanActions(ctx context.Context) error {
	t.taskCtx.Logger.Debug("create the plan")

	param := t.taskCtx.Task.Parameters.Firmwares[0]

	firmware := &model.Firmware{
		Component: param.Component,
		Vendor:    param.Vendor,
		Models:    []string{param.Vendor},
		Version:   param.Version,
	}

	actionCtx := &runner.ActionHandlerContext{
		TaskHandlerContext: t.taskCtx,
		Firmware:           firmware,
		First:              true,
		Last:               true,
	}

	aHandler := &outofband.ActionHandler{}
	action, err := aHandler.ComposeAction(ctx, actionCtx)
	if err != nil {
		return err
	}

	action.FirmwareTempFile = t.fwFile

	//nolint:errcheck  // SetState never returns an error
	action.SetState(model.StatePending)

	t.taskCtx.Task.Data.ActionsPlanned = []*model.Action{action}

	return nil
}

func (t *handler) Publish(context.Context) {}

// query device components inventory from the device itself.
func (t *handler) queryFromDevice(ctx context.Context) ([]*rtypes.Component, error) {
	if t.taskCtx.DeviceQueryor == nil {
		// TODO(joel): DeviceQueryor is to be instantiated based on the method(s) for the firmwares to be installed
		// if its a mix of inband, out of band firmware to be installed, then both are to be queried and
		// so this DeviceQueryor would have to be extended
		//
		// For this to work with both inband and out of band, the firmware set data should include the install method.
		t.taskCtx.DeviceQueryor = outofband.NewDeviceQueryor(ctx, t.taskCtx.Task.Server, t.taskCtx.Logger)
	}

	t.taskCtx.Task.Status.Append("connecting to device BMC")

	if err := t.taskCtx.DeviceQueryor.(device.OutofbandQueryor).Open(ctx); err != nil {
		return nil, err
	}

	t.taskCtx.Task.Status.Append("collecting inventory from device BMC")

	deviceCommon, err := t.taskCtx.DeviceQueryor.(device.OutofbandQueryor).Inventory(ctx)
	if err != nil {
		return nil, err
	}

	if t.taskCtx.Task.Server.Vendor == "" {
		t.taskCtx.Task.Server.Vendor = deviceCommon.Vendor
	}

	if t.taskCtx.Task.Server.Model == "" {
		t.taskCtx.Task.Server.Model = common.FormatProductName(deviceCommon.Model)
	}

	return model.NewComponentConverter().CommonDeviceToComponents(deviceCommon)
}

func (t *handler) OnSuccess(ctx context.Context, _ *model.Task) {
	if t.taskCtx.DeviceQueryor == nil {
		return
	}

	if err := t.taskCtx.DeviceQueryor.(device.OutofbandQueryor).Close(ctx); err != nil {
		t.taskCtx.Logger.WithFields(logrus.Fields{"err": err.Error()}).Warn("device logout error")
	}
}

func (t *handler) OnFailure(ctx context.Context, _ *model.Task) {
	if t.taskCtx.DeviceQueryor == nil {
		return
	}

	if err := t.taskCtx.DeviceQueryor.(device.OutofbandQueryor).Close(ctx); err != nil {
		t.taskCtx.Logger.WithFields(logrus.Fields{"err": err.Error()}).Warn("device logout error")
	}
}
