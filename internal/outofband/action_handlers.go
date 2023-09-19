package outofband

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bmc-toolbox/common"
	sw "github.com/filanov/stateswitch"
	"github.com/hashicorp/go-multierror"
	"github.com/metal-toolbox/flasher/internal/metrics"
	"github.com/metal-toolbox/flasher/internal/model"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

const (
	// delayHostPowerStatusChange is the delay after the host has been power cycled or powered on
	// this delay ensures that any existing pending updates are applied and that the
	// the host components are initialized properly before inventory and other actions are attempted.
	delayHostPowerStatusChange = 10 * time.Minute

	// delay after when the BMC was reset
	delayBMCReset = 5 * time.Minute

	// delay between polling the firmware install status
	delayPollStatus = 10 * time.Second

	// maxPollStatusAttempts is set based on how long the loop below should keep polling
	// for a finalized state before giving up
	//
	// 600 (maxAttempts) * 10s (delayPollInstallStatus) = 100 minutes (1.6hours)
	maxPollStatusAttempts = 600

	// this value indicates the device was powered on by flasher
	devicePoweredOn = "devicePoweredOn"

	// firmware files are downloaded into this directory
	downloadDir = "/tmp"
)

var (

	// exponential backoff parameters
	//
	// nolint:revive // variable names are clear as is
	//	backoffMin    = 20 * time.Second
	//	backoffMax    = 10 * time.Minute
	//	backoffFactor = 2

	// envTesting is set by tests to '1' to skip sleeps and backoffs in the handlers.
	//
	// nolint:gosec // no gosec, this isn't a credential
	envTesting = "ENV_TESTING"

	ErrFirmwareTempFile        = errors.New("error firmware temp file")
	ErrSaveAction              = errors.New("error occurred in action state save")
	ErrActionTypeAssertion     = errors.New("error occurred in action object type assertion")
	ErrContextCancelled        = errors.New("context canceled")
	ErrUnexpected              = errors.New("unexpected error occurred")
	ErrInstalledFirmwareEqual  = errors.New("installed and expected firmware equal")
	ErrInstalledVersionUnknown = errors.New("installed version unknown")
	ErrComponentNotFound       = errors.New("component not found for firmware install")
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

func sleepWithContext(ctx context.Context, t time.Duration) error {
	// skip sleep in tests
	if os.Getenv(envTesting) == "1" {
		return nil
	}

	select {
	case <-time.After(t):
		return nil
	case <-ctx.Done():
		return ErrContextCancelled
	}
}

func (h *actionHandler) conditionPowerOnDevice(_ *model.Action, tctx *sm.HandlerContext) (bool, error) {
	// init out of band device queryor - if one isn't already initialized
	// this is done conditionally to enable tests to pass in a device queryor
	if tctx.DeviceQueryor == nil {
		tctx.DeviceQueryor = NewDeviceQueryor(tctx.Ctx, tctx.Asset, tctx.Logger)
	}

	if err := tctx.DeviceQueryor.Open(tctx.Ctx); err != nil {
		return false, err
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
	action, tctx, err := actionTaskCtxFromInterfaces(a, c)
	if err != nil {
		return err
	}

	powerOnRequired, err := h.conditionPowerOnDevice(action, tctx)
	if err != nil {
		return err
	}

	if !powerOnRequired {
		return nil
	}

	tctx.Logger.WithFields(
		logrus.Fields{
			"component": action.Firmware.Component,
			"version":   action.Firmware.Version,
		}).Info("device is currently powered off, powering on")

	if !tctx.Dryrun {
		if err := tctx.DeviceQueryor.SetPowerState(tctx.Ctx, "on"); err != nil {
			return err
		}

		if err := sleepWithContext(tctx.Ctx, delayHostPowerStatusChange); err != nil {
			return err
		}
	}

	tctx.Data[devicePoweredOn] = "true"

	return nil
}

func (h *actionHandler) checkCurrentFirmware(a sw.StateSwitch, c sw.TransitionArgs) error {
	action, tctx, err := actionTaskCtxFromInterfaces(a, c)
	if err != nil {
		return err
	}

	if !action.VerifyCurrentFirmware {
		tctx.Logger.WithFields(
			logrus.Fields{
				"component": action.Firmware.Component,
			}).Debug("Skipped installed version lookup - action.VerifyCurrentFirmware was disabled")

		return nil
	}

	tctx.Logger.WithFields(
		logrus.Fields{
			"component": action.Firmware.Component,
		}).Debug("Querying device inventory from BMC for current component firmware")

	inv, err := tctx.DeviceQueryor.Inventory(tctx.Ctx)
	if err != nil {
		return err
	}

	components, err := model.NewComponentConverter().CommonDeviceToComponents(inv)
	if err != nil {
		return err
	}

	component := components.BySlugModel(action.Firmware.Component, action.Firmware.Models)
	if component == nil {
		tctx.Logger.WithFields(
			logrus.Fields{
				"component": action.Firmware.Component,
				"vendor":    action.Firmware.Vendor,
				"models":    action.Firmware.Models,
				"err":       ErrComponentNotFound,
			}).Error("no component found for given component/vendor/model")

		return errors.Wrap(ErrComponentNotFound,
			fmt.Sprintf("component: %s, vendor: %s, model: %s", action.Firmware.Component,
				action.Firmware.Vendor,
				action.Firmware.Models,
			),
		)
	}

	tctx.Logger.WithFields(
		logrus.Fields{
			"component":        component.Slug,
			"vendor":           component.Vendor,
			"model":            component.Model,
			"serial":           component.Serial,
			"firmware.version": component.FirmwareInstalled,
		}).Debug("found component")

	if component.FirmwareInstalled == "" {
		return errors.Wrap(ErrInstalledVersionUnknown, "set TaskParameters.Force=true to skip this check")
	}

	equal := strings.EqualFold(component.FirmwareInstalled, action.Firmware.Version)
	if equal {
		tctx.Logger.WithFields(
			logrus.Fields{
				"action id":       action.ID,
				"condition id":    action.TaskID,
				"component":       action.Firmware.Component,
				"vendor":          action.Firmware.Vendor,
				"models":          action.Firmware.Models,
				"expectedVersion": action.Firmware.Version,
			}).Warn("Installed firmware version equals expected")
		return ErrInstalledFirmwareEqual
	}

	return nil
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
	err = download(tctx.Ctx, action.Firmware.URL, file)
	if err != nil {
		return err
	}

	// collect download metrics
	fileInfo, err := os.Stat(file)
	if err == nil {
		metrics.DownloadBytes.With(
			prometheus.Labels{
				"component": action.Firmware.Component,
				"vendor":    action.Firmware.Vendor,
			},
		).Add(float64(fileInfo.Size()))
	}

	// validate checksum
	if err := checksumValidate(file, action.Firmware.Checksum); err != nil {
		os.RemoveAll(filepath.Dir(file))
		return err
	}

	// store the firmware temp file location
	action.FirmwareTempFile = file

	tctx.Logger.WithFields(
		logrus.Fields{
			"component": action.Firmware.Component,
			"version":   action.Firmware.Version,
			"url":       action.Firmware.URL,
			"file":      file,
			"checksum":  action.Firmware.Checksum,
		}).Info("downloaded and verified firmware file checksum")

	return nil
}

func (h *actionHandler) initiateInstallFirmware(a sw.StateSwitch, c sw.TransitionArgs) error {
	action, tctx, err := actionTaskCtxFromInterfaces(a, c)
	if err != nil {
		return err
	}

	if action.FirmwareTempFile == "" {
		return errors.Wrap(ErrFirmwareTempFile, "expected FirmwareTempFile to be declared")
	}

	// open firmware file handle
	fileHandle, err := os.Open(action.FirmwareTempFile)
	if err != nil {
		return errors.Wrap(err, action.FirmwareTempFile)
	}

	defer fileHandle.Close()
	defer os.RemoveAll(filepath.Dir(action.FirmwareTempFile))

	if !tctx.Dryrun {
		// initiate firmware install
		bmcTaskID, err := tctx.DeviceQueryor.FirmwareInstall(
			tctx.Ctx,
			action.Firmware.Component,
			tctx.Task.Parameters.ForceInstall,
			fileHandle,
		)
		if err != nil {
			return err
		}

		// collect upload metrics
		fileInfo, err := os.Stat(action.FirmwareTempFile)
		if err == nil {
			metrics.UploadBytes.With(
				prometheus.Labels{
					"component": action.Firmware.Component,
					"vendor":    action.Firmware.Vendor,
				},
			).Add(float64(fileInfo.Size()))
		}

		// returned bmcTaskID corresponds to a redfish task ID on BMCs that support redfish
		// for the rest we track the bmcTaskID as the action.ID
		if bmcTaskID == "" {
			bmcTaskID = action.ID
		}

		action.BMCTaskID = bmcTaskID
	}

	tctx.Logger.WithFields(
		logrus.Fields{
			"component": action.Firmware.Component,
			"update":    action.Firmware.FileName,
			"version":   action.Firmware.Version,
			"bmcTaskID": action.BMCTaskID,
		}).Info("initiated firmware install")

	return nil
}

// polls firmware install status from the BMC
//
// nolint:gocyclo // for now this is best kept in the same method
func (h *actionHandler) pollFirmwareInstallStatus(a sw.StateSwitch, c sw.TransitionArgs) error {
	action, tctx, err := actionTaskCtxFromInterfaces(a, c)
	if err != nil {
		return err
	}

	if tctx.Dryrun {
		return nil
	}

	startTS := time.Now()

	// number of status queries attempted
	var attempts int

	var attemptErrors *multierror.Error

	tctx.Logger.WithFields(
		logrus.Fields{
			"component": action.Firmware.Component,
			"version":   action.Firmware.Version,
			"bmc":       tctx.Asset.BmcAddress,
		}).Info("polling BMC for firmware install status")

	for {
		// increment attempts
		attempts++

		// delay if we're in the second or subsequent attempts
		if attempts > 0 {
			if err := sleepWithContext(tctx.Ctx, delayPollStatus); err != nil {
				return err
			}
		}

		// return when attempts exceed maxPollStatusAttempts
		if attempts >= maxPollStatusAttempts {
			attemptErrors = multierror.Append(attemptErrors, errors.Wrapf(
				ErrMaxBMCQueryAttempts,
				"%d attempts querying FirmwareInstallStatus(), elapsed: %s",
				attempts,
				time.Since(startTS).String(),
			))

			return attemptErrors
		}

		// query firmware install status
		status, err := tctx.DeviceQueryor.FirmwareInstallStatus(
			tctx.Ctx,
			action.Firmware.Version,
			action.Firmware.Component,
			action.BMCTaskID,
		)

		tctx.Logger.WithFields(
			logrus.Fields{
				"component": action.Firmware.Component,
				"update":    action.Firmware.FileName,
				"version":   action.Firmware.Version,
				"bmc":       tctx.Asset.BmcAddress,
				"elapsed":   time.Since(startTS).String(),
				"attempts":  fmt.Sprintf("attempt %d/%d", attempts, maxPollStatusAttempts),
				"taskState": status,
			}).Debug("firmware install status query attempt")

		// error check returns when maxPollStatusAttempts have been reached
		if err != nil {
			attemptErrors = multierror.Append(attemptErrors, err)

			continue
		}

		switch status {
		// continue polling when install is running
		case model.StatusInstallRunning:
			continue

		// record the unknown status as an error
		case model.StatusInstallUnknown:
			err = errors.New("firmware install status unknown")
			attemptErrors = multierror.Append(attemptErrors, err)

			continue

		// return when bmc power cycle is required
		case model.StatusInstallPowerCycleBMCRequired:
			action.BMCPowerCycleRequired = true
			return nil

		// return when host power cycle is required
		case model.StatusInstallPowerCycleHostRequired:
			action.HostPowerCycleRequired = true
			return nil

		// return error when install fails
		case model.StatusInstallFailed:
			return errors.Wrap(
				ErrFirmwareInstallFailed,
				"check logs on the BMC for information, bmc task ID: "+action.BMCTaskID,
			)

		// return nil when install is complete
		case model.StatusInstallComplete:
			if strings.EqualFold(action.Firmware.Component, common.SlugBMC) {
				tctx.Logger.WithFields(
					logrus.Fields{
						"bmc":   tctx.Asset.BmcAddress,
						"delay": delayBMCReset.String(),
					}).Debug("BMC firmware install completed, added delay to allow the BMC to complete its update process..")

				if err := sleepWithContext(tctx.Ctx, delayBMCReset); err != nil {
					return errors.Wrap(
						ErrFirmwareInstallFailed,
						err.Error(),
					)
				}
			}

			return nil

		default:
			return errors.Wrap(ErrFirmwareInstallStatusUnexpected, string(status))
		}
	}
}

func (h *actionHandler) resetBMC(a sw.StateSwitch, c sw.TransitionArgs) error {
	action, tctx, err := actionTaskCtxFromInterfaces(a, c)
	if err != nil {
		return err
	}

	// proceed with reset only if these flags are set
	if !action.BMCPowerCycleRequired && !tctx.Task.Parameters.ResetBMCBeforeInstall {
		return nil
	}

	tctx.Logger.WithFields(
		logrus.Fields{
			"component":                             action.Firmware.Component,
			"bmc":                                   tctx.Asset.BmcAddress,
			"task.Parameters.ResetBMCBeforeInstall": tctx.Task.Parameters.ResetBMCBeforeInstall,
			"action.BMCPowerCycleRequired":          action.BMCPowerCycleRequired,
		}).Info("resetting BMC")

	if !tctx.Dryrun {
		if err := tctx.DeviceQueryor.ResetBMC(tctx.Ctx); err != nil {
			return err
		}

		if err := sleepWithContext(tctx.Ctx, delayBMCReset); err != nil {
			return err
		}
	}

	// skip install status poll if this was a preinstall BMC reset
	if tctx.Task.Parameters.ResetBMCBeforeInstall {
		// set this to false to prevent the rest of the actions from attempting a preInstall BMC reset.
		tctx.Task.Parameters.ResetBMCBeforeInstall = false

		return nil
	}

	return h.pollFirmwareInstallStatus(a, c)
}

func (h *actionHandler) resetDevice(a sw.StateSwitch, c sw.TransitionArgs) error {
	action, tctx, err := actionTaskCtxFromInterfaces(a, c)
	if err != nil {
		return err
	}

	if !action.HostPowerCycleRequired {
		return nil
	}

	tctx.Logger.WithFields(
		logrus.Fields{
			"component": action.Firmware.Component,
			"bmc":       tctx.Asset.BmcAddress,
		}).Info("resetting host for firmware install")

	if !tctx.Dryrun {
		if err := tctx.DeviceQueryor.SetPowerState(tctx.Ctx, "cycle"); err != nil {
			return err
		}

		if err := sleepWithContext(tctx.Ctx, delayHostPowerStatusChange); err != nil {
			return err
		}
	}

	return h.pollFirmwareInstallStatus(a, c)
}

func (h *actionHandler) conditionPowerOffDevice(action *model.Action, tctx *sm.HandlerContext) (bool, error) {
	// proceed to power off the device if this is the final action
	if !action.Final {
		return false, nil
	}

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
	action, tctx, err := actionTaskCtxFromInterfaces(a, c)
	if err != nil {
		return err
	}

	powerOffDeviceRequired, err := h.conditionPowerOffDevice(action, tctx)
	if err != nil {
		return err
	}

	if !powerOffDeviceRequired {
		return nil
	}

	if !tctx.Dryrun {
		tctx.Logger.WithFields(
			logrus.Fields{
				"component": action.Firmware.Component,
				"bmc":       tctx.Asset.BmcAddress,
			}).Debug("powering off device")

		if err := tctx.DeviceQueryor.SetPowerState(tctx.Ctx, "off"); err != nil {
			return err
		}
	}

	return nil
}

func (h *actionHandler) PublishStatus(_ sw.StateSwitch, args sw.TransitionArgs) error {
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return sm.ErrInvalidTransitionHandler
	}

	tctx.Publisher.Publish(tctx)

	return nil
}

func (h *actionHandler) actionFailed(_ sw.StateSwitch, _ sw.TransitionArgs) error {
	return nil
}

func (h *actionHandler) actionSuccessful(_ sw.StateSwitch, _ sw.TransitionArgs) error {
	return nil
}
