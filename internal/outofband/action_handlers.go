package outofband

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	sw "github.com/filanov/stateswitch"
	"github.com/jpillora/backoff"
	"github.com/metal-toolbox/flasher/internal/model"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
)

const (
	// this value indicates the device was powered on by flasher
	devicePoweredOn = "devicePoweredOn"

	// firmware files are downloaded into this directory
	downloadDir = "/tmp"
)

var (
	// delayHostPowerStatusChange is the delay after the host has been power cycled or powered on
	// this delay ensures that any existing pending updates are applied and that the
	// the host components are initialized properly before inventory and other actions are attempted.
	delayHostPowerStatusChange = 10 * time.Minute

	delayBMCReset = 5 * time.Minute

	// exponential backoff parameters
	//
	// nolint:revive // variable names are clear as is
	backoffMin    = 20 * time.Second
	backoffMax    = 10 * time.Minute
	backoffFactor = 2

	ErrFirmwareTempFile    = errors.New("error firmware temp file")
	ErrSaveAction          = errors.New("error occurred in action state save")
	ErrActionTypeAssertion = errors.New("error occurred in action object type assertion")
	ErrContextCancelled    = errors.New("context canceled")
)

// actionHandler implements the actionTransitionHandler methods
type actionHandler struct{}

func actionTaskCtxFromInterfaces(a sw.StateSwitch, c sw.TransitionArgs) (*model.Action, *sm.HandlerContext, error) {
	action, ok := a.(*model.Action)
	if !ok {
		return nil, nil, sm.ErrActionTypeAssertion
	}

	taskContext, ok := c.(*sm.HandlerContext)
	if !ok {
		return nil, nil, sm.ErrInvalidtaskHandlerContext
	}

	return action, taskContext, nil
}

func (h *actionHandler) conditionPowerOnDevice(a sw.StateSwitch, c sw.TransitionArgs) (bool, error) {
	action, tctx, err := actionTaskCtxFromInterfaces(a, c)
	if err != nil {
		return false, err
	}

	task, err := tctx.Store.TaskByID(tctx.Ctx, action.TaskID)
	if err != nil {
		return false, err
	}

	// init out of band device queryor - if one isn't already initialized
	// this is done conditionally to enable tests to pass in a device queryor
	if tctx.DeviceQueryor == nil {
		tctx.DeviceQueryor = NewDeviceQueryor(tctx.Ctx, &task.Parameters.Device, tctx.Logger)
	}

	powerState, err := tctx.DeviceQueryor.PowerStatus(tctx.Ctx)
	if err != nil {
		return false, err
	}

	if strings.Contains(strings.ToLower(powerState), "off") { // covers states - Off, PoweringOff
		return true, nil

	}

	return false, nil
}

// initialize initializes the bmc connection and powers on the host if required.
func (h *actionHandler) powerOnDevice(a sw.StateSwitch, c sw.TransitionArgs) error {
	_, tctx, err := actionTaskCtxFromInterfaces(a, c)
	if err != nil {
		return err
	}

	if err := tctx.DeviceQueryor.SetPowerState(tctx.Ctx, "on"); err != nil {
		return err
	}

	tctx.Data[devicePoweredOn] = strconv.FormatBool(true)

	if err := sleepWithContext(tctx.Ctx, delayHostPowerStatusChange); err != nil {
		return err
	}

	return nil
}

func (h *actionHandler) conditionInstallFirmware(a sw.StateSwitch, c sw.TransitionArgs) (bool, error) {
	action, tctx, err := actionTaskCtxFromInterfaces(a, c)
	if err != nil {
		return false, err
	}

	inv, err := tctx.DeviceQueryor.Inventory(tctx.Ctx)
	if err != nil {
		return false, err
	}

	task, err := tctx.Store.TaskByID(tctx.Ctx, tctx.TaskID)
	if err != nil {
		return false, err
	}

	// compare installed firmware versions with the planned versions
	//
	// returns an error if a component is unsupported
	equals, err := h.installedFirmwareVersionEqualsNew(inv, action.Firmware)
	if err != nil {
		return false, err
	}

	// force install ignores version comparison
	if task.Parameters.ForceInstall {
		return true, nil
	}

	return equals, nil
}

func (h *actionHandler) downloadFirmware(a sw.StateSwitch, c sw.TransitionArgs) error {
	action, tctx, err := actionTaskCtxFromInterfaces(a, c)
	if err != nil {
		return err
	}

	// create a temp download directory
	dir, err := os.MkdirTemp(downloadDir, "")
	if err != nil {
		return errors.Wrap(err, "error creating tmp directory to download firmware")
	}

	file := filepath.Join(dir, action.Firmware.FileName)

	// download firmware file
	if err := download(tctx.Ctx, action.Firmware.URL, file); err != nil {
		return err
	}

	// validate checksum
	//
	// This assumes the checksum is of type SHA256
	// it would be ideal if the firmware object indicated the type of checksum.
	if err := checksumValidateSHA256(file, action.Firmware.Checksum); err != nil {
		os.RemoveAll(filepath.Dir(tctx.FirmwareTempFile))
		return err
	}

	// store the firmware temp file location
	action.FirmwareTempFile = file
	// tctx.Store.UpdateTaskAction(tctx.Ctx, tctx.TaskID, *action)

	tctx.Logger.WithFields(
		logrus.Fields{
			"component": action.Firmware.ComponentSlug,
			"update":    action.Firmware.FileName,
		}).Info("downloaded and verified firmware file checksum")

	return nil
}

func (h *actionHandler) installFirmware(a sw.StateSwitch, c sw.TransitionArgs) error {
	action, tctx, err := actionTaskCtxFromInterfaces(a, c)
	if err != nil {
		return err
	}

	if action.FirmwareTempFile == "" {
		return errors.Wrap(ErrFirmwareTempFile, "expected FirmwareTempFile to be declared")
	}

	task, err := tctx.Store.TaskByID(tctx.Ctx, tctx.TaskID)
	if err != nil {
		return err
	}

	// open firmware file handle
	fileHandle, err := os.Open(tctx.FirmwareTempFile)
	if err != nil {
		return err
	}

	defer fileHandle.Close()
	defer os.RemoveAll(filepath.Dir(tctx.FirmwareTempFile))

	// initiate firmware install
	bmcTaskID, err := tctx.DeviceQueryor.FirmwareInstall(
		tctx.Ctx,
		action.Firmware.ComponentSlug,
		task.Parameters.ForceInstall,
		fileHandle,
	)
	if err != nil {
		return err
	}

	// returned bmcTaskID corresponds to a redfish task ID on BMCs that support redfish
	// for the rest we track the bmcTaskID as the action.ID
	if bmcTaskID == "" {
		bmcTaskID = action.ID
	}

	action.BMCTaskID = bmcTaskID

	tctx.Logger.WithFields(
		logrus.Fields{
			"component": action.Firmware.ComponentSlug,
			"update":    action.Firmware.FileName,
			"version":   action.Firmware.Version,
		}).Info("initiated firmware install")

	return nil
}

func (h *actionHandler) pollInstallStatus(a sw.StateSwitch, c sw.TransitionArgs) error {
	action, tctx, err := actionTaskCtxFromInterfaces(a, c)
	if err != nil {
		return err
	}

	task, err := tctx.Store.TaskByID(tctx.Ctx, tctx.TaskID)
	if err != nil {
		return err
	}

	delay := &backoff.Backoff{
		Min:    backoffMin,
		Max:    backoffMax,
		Factor: float64(backoffFactor),
		Jitter: true,
	}

	// the component firmware install status is considered final when its in one of these states
	finalizedStatus := []model.ComponentFirmwareInstallStatus{
		model.StatusInstallFailed,
		model.StatusInstallComplete,
		model.StatusInstallPowerCycleBMCRequired,
		model.StatusInstallPowerCycleHostRequired,
	}

	// maxFailures here is set based on how long the loop below should keep polling for a finalized state before giving up
	// 10 (maxFailures) * 600s (delay.Max) = 100 minutes (1.6hours)
	maxFailures := 10

	startTS := time.Now()

	// number of status queries attempted
	var attempts int

	for {
		// increment attempts
		attempts++

		// delay with backoff if we're in the second or subsequent attempts
		if attempts > 0 {
			time.Sleep(delay.Duration())
		}

		// initiate firmware install
		status, err := tctx.DeviceQueryor.FirmwareInstallStatus(
			tctx.Ctx,
			action.Firmware.Version,
			action.Firmware.ComponentSlug,
			action.BMCTaskID,
		)

		// error check returns when maxFailures have been reached
		if err != nil {
			tctx.Logger.WithFields(
				logrus.Fields{
					"component": action.Firmware.ComponentSlug,
					"update":    action.Firmware.FileName,
					"version":   action.Firmware.Version,
					"bmc":       task.Parameters.Device.BmcAddress,
					"elapsed":   time.Since(startTS).String(),
					"attempts":  fmt.Sprintf("attempt %d/%d", attempts, maxFailures),
					"taskState": status,
					"err":       err,
				}).Debug("firmware install status query attempt")

			if attempts >= maxFailures {
				return errors.Wrap(ErrBMCQuery, "too many failures querying FirmwareInstallStatus: "+strconv.Itoa(maxFailures))
			}

			continue
		}

		// at this point the install most likely requires a BMC/host reset
		// declare bmc/host power cycle based on finalized status
		if slices.Contains(finalizedStatus, status) {
			switch status {
			case model.StatusInstallPowerCycleBMCRequired:
				action.BMCPowerCycleRequired = true
			case model.StatusInstallPowerCycleHostRequired:
				action.HostPowerCycleRequired = true
			}

			return nil
		}
	}
}

func (h *actionHandler) conditionResetBMC(a sw.StateSwitch, c sw.TransitionArgs) (bool, error) {
	action, _, err := actionTaskCtxFromInterfaces(a, c)
	if err != nil {
		return false, err
	}

	return action.BMCPowerCycleRequired, nil
}

func (h *actionHandler) resetBMC(a sw.StateSwitch, c sw.TransitionArgs) error {
	_, tctx, err := actionTaskCtxFromInterfaces(a, c)
	if err != nil {
		return err
	}

	if err := tctx.DeviceQueryor.ResetBMC(tctx.Ctx); err != nil {
		return err
	}

	if err := sleepWithContext(tctx.Ctx, delayBMCReset); err != nil {
		return err
	}

	return h.pollInstallStatus(a, c)
}

func (h *actionHandler) conditionResetHost(a sw.StateSwitch, c sw.TransitionArgs) (bool, error) {
	action, _, err := actionTaskCtxFromInterfaces(a, c)
	if err != nil {
		return false, err
	}

	return action.HostPowerCycleRequired, nil
}

func (h *actionHandler) resetHost(a sw.StateSwitch, c sw.TransitionArgs) error {
	_, tctx, err := actionTaskCtxFromInterfaces(a, c)
	if err != nil {
		return err
	}

	if err := tctx.DeviceQueryor.SetPowerState(tctx.Ctx, "cycle"); err != nil {
		return err
	}

	if err := sleepWithContext(tctx.Ctx, delayHostPowerStatusChange); err != nil {
		return err
	}

	return h.pollInstallStatus(a, c)
}

func (h *actionHandler) conditionPowerOffDevice(a sw.StateSwitch, c sw.TransitionArgs) (bool, error) {
	action, tctx, err := actionTaskCtxFromInterfaces(a, c)
	if err != nil {
		return false, err
	}

	// proceed if this is the final action
	if !action.Final {
		return false, nil
	}

	// proceed if this task powered on the device
	wasPoweredOn, keyExists := tctx.Data[devicePoweredOn]
	if !keyExists {
		return false, nil
	}

	if wasPoweredOn == "true" {
		return true, nil
	}

	return false, nil
}

// initialize initializes the bmc connection and powers on the host if required.
func (h *actionHandler) powerOffDevice(a sw.StateSwitch, c sw.TransitionArgs) error {
	_, tctx, err := actionTaskCtxFromInterfaces(a, c)
	if err != nil {
		return err
	}

	if err := tctx.DeviceQueryor.SetPowerState(tctx.Ctx, "off"); err != nil {
		return err
	}

	return nil
}

func (h *actionHandler) SaveState(a sw.StateSwitch, args sw.TransitionArgs) error {
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return sm.ErrInvalidTransitionHandler
	}

	action, ok := a.(*model.Action)
	if !ok {
		return errors.Wrap(ErrSaveAction, ErrActionTypeAssertion.Error())
	}

	if err := tctx.Store.UpdateTaskAction(tctx.Ctx, tctx.TaskID, *action); err != nil {
		return errors.Wrap(ErrSaveAction, err.Error())
	}

	return nil
}
