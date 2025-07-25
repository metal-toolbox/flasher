package inband

import (
	"context"
	"fmt"
	"strings"

	"github.com/metal-toolbox/flasher/internal/device"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/runner"
	rctypes "github.com/metal-toolbox/rivets/v2/condition"
	rtypes "github.com/metal-toolbox/rivets/v2/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	ironlibactions "github.com/metal-toolbox/ironlib/actions"
	imodel "github.com/metal-toolbox/ironlib/model"
)

var (
	errCompose = errors.New("error in composing steps for firmware install")
)

const (
	// transition types implemented and defined further below
	powerOffServer         model.StepName = "powerOffServer"
	powerCycleServer       model.StepName = "powerCycleServer"
	checkInstalledFirmware model.StepName = "checkInstalledFirmware"
	downloadFirmware       model.StepName = "downloadFirmware"
	installFirmware        model.StepName = "installFirmware"
	pollInstallStatus      model.StepName = "pollInstallStatus"
)

const (
	PreInstall  model.StepGroup = "PreInstall"
	PostInstall model.StepGroup = "PostInstall"
	Install     model.StepGroup = "Install"
	PowerState  model.StepGroup = "PowerState"
)

type ActionHandler struct {
	handler *handler
}

func (i *ActionHandler) identifyComponent(ctx context.Context, component string, models []string) (*rtypes.Component, error) {
	var components rtypes.Components

	if len(i.handler.actionCtx.Task.Server.Components) > 0 {
		components = rtypes.Components(i.handler.actionCtx.Task.Server.Components)
	} else {
		deviceCommon, err := i.handler.deviceQueryor.Inventory(ctx)
		if err != nil {
			return nil, err
		}

		components, err = model.NewComponentConverter().CommonDeviceToComponents(deviceCommon)
		if err != nil {
			return nil, err
		}
	}

	found := components.ByNameModel(component, models)
	if found == nil {
		// nolint:err113 // its clearer to define this error here
		errComponentMatch := fmt.Errorf(
			"unable to identify component '%s' from inventory for given models: %s",
			component,
			strings.Join(models, ","),
		)

		return nil, errComponentMatch
	}

	return found, nil
}

func (i *ActionHandler) ComposeAction(ctx context.Context, actionCtx *runner.ActionHandlerContext) (*model.Action, error) {
	i.handler = initHandler(actionCtx)

	component, err := i.identifyComponent(ctx, actionCtx.Firmware.Component, actionCtx.Firmware.Models)
	if err != nil {
		return nil, errors.Wrap(ErrComponentNotFound, err.Error())
	}

	i.handler.logger.WithFields(logrus.Fields{
		"component": actionCtx.Firmware.Component,
		"model":     component.Model,
		"current":   component.Firmware.Installed,
	}).Info("target component identified for firmware install")

	required, err := i.handler.deviceQueryor.FirmwareInstallRequirements(
		ctx,
		actionCtx.Firmware.Component,
		actionCtx.Firmware.Vendor,
		component.Model,
	)
	if err != nil {
		// fatal error only if the updater utility is not identified
		if errors.Is(err, ironlibactions.ErrUpdaterUtilNotIdentified) {
			return nil, err
		}

		i.handler.logger.WithFields(logrus.Fields{
			"component": actionCtx.Firmware.Component,
			"model":     actionCtx.Firmware.Models,
		}).WithError(err).
			Info("No firmware install requirements were identified for component")
	}

	i.handler.action = &model.Action{
		InstallMethod: model.InstallMethodInband,
		Firmware:      *actionCtx.Firmware,
		ForceInstall:  actionCtx.Task.Parameters.ForceInstall,
		First:         actionCtx.First,
		Last:          actionCtx.Last,
		Component:     component,
	}

	steps, err := i.composeSteps(required)
	if err != nil {
		return nil, errors.Wrap(errCompose, err.Error())
	}

	i.handler.action.Steps = steps

	return i.handler.action, nil
}

func initHandler(actionCtx *runner.ActionHandlerContext) *handler {
	var deviceQueryor device.InbandQueryor

	if actionCtx.DeviceQueryor == nil {
		deviceQueryor = NewDeviceQueryor(actionCtx.Logger)
	} else {
		deviceQueryor = actionCtx.DeviceQueryor.(device.InbandQueryor)
	}

	return &handler{
		actionCtx:     actionCtx,
		deviceQueryor: deviceQueryor,
		logger:        actionCtx.Logger,
	}
}

func (i *ActionHandler) composeSteps(required *imodel.UpdateRequirements) (model.Steps, error) {
	var final model.Steps

	// pre-install steps
	preinstall, err := i.definitions().ByGroup(PreInstall)
	if err != nil {
		return nil, err
	}

	final = append(final, preinstall...)

	// install steps
	install, err := i.definitions().ByGroup(Install)
	if err != nil {
		return nil, err
	}

	final = append(final, install...)

	if required != nil && required.PostInstallHostPowercycle {
		i.handler.actionCtx.Task.Data.HostPowercycleRequired = true
	}

	if i.handler.action.Last && i.handler.actionCtx.Task.Data.HostPowercycleRequired {
		powerCycle, errDef := i.definitions().ByName(powerCycleServer)
		if errDef != nil {
			return nil, err
		}

		final = append(final, &powerCycle)
	}

	postinstall, err := i.definitions().ByGroup(PostInstall)
	if err != nil {
		return nil, err
	}

	final = append(final, postinstall...)

	return final, nil
}

// Assign action step handlers to a previously initialized action
//
// This is mainly for resumed actions which were loaded from active Task object the store (KV)
// since the actions were previously composed, now they just have to be assigned the step handler methods.
func AssignStepHandlers(action *model.Action, actionCtx *runner.ActionHandlerContext) error {
	if actionCtx.DeviceQueryor == nil {
		actionCtx.DeviceQueryor = NewDeviceQueryor(actionCtx.Logger)
	}

	handler := initHandler(actionCtx)
	ah := &ActionHandler{handler}

	for _, step := range action.Steps {
		if rctypes.StateIsComplete(step.State) {
			continue
		}

		h, err := ah.definitions().ByName(step.Name)
		if err != nil {
			return err
		}

		step.Handler = h.Handler
	}

	ah.handler.action = action

	return nil
}

func (i *ActionHandler) definitions() model.Steps {
	return model.Steps{
		{
			Name:        checkInstalledFirmware,
			Group:       PreInstall,
			Handler:     i.handler.checkCurrentFirmware,
			Description: "Check firmware currently installed on component",
			State:       model.StatePending,
		},
		{
			Name:        downloadFirmware,
			Group:       PreInstall,
			Handler:     i.handler.downloadFirmware,
			Description: "Download and verify firmware file checksum.",
			State:       model.StatePending,
		},
		{
			Name:        installFirmware,
			Group:       Install,
			Handler:     i.handler.installFirmware,
			Description: "Install firmware.",
			State:       model.StatePending,
		},
		{
			Name:        powerCycleServer,
			Group:       PowerState,
			Handler:     i.handler.powerCycleServer,
			Description: "Turn the computer off and on again.",
			State:       model.StatePending,
		},
		{
			Name:        checkInstalledFirmware,
			Group:       PostInstall,
			Handler:     i.handler.checkCurrentFirmware,
			Description: "Check firmware currently installed on components",
			State:       model.StatePending,
		},
	}
}
