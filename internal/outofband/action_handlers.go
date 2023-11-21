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

	bconsts "github.com/bmc-toolbox/bmclib/v2/constants"
)

const (
	// delayHostPowerStatusChange is the delay after the host has been power cycled or powered on
	// this delay ensures that any existing pending updates are applied and that the
	// the host components are initialized properly before inventory and other actions are attempted.
	delayHostPowerStatusChange = 5 * time.Minute

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

	ErrFirmwareTempFile          = errors.New("error firmware temp file")
	ErrSaveAction                = errors.New("error occurred in action state save")
	ErrActionTypeAssertion       = errors.New("error occurred in action object type assertion")
	ErrContextCancelled          = errors.New("context canceled")
	ErrUnexpected                = errors.New("unexpected error occurred")
	ErrInstalledFirmwareNotEqual = errors.New("installed and expected firmware not equal")
	ErrInstalledFirmwareEqual    = errors.New("installed and expected firmware are equal, no action necessary")
	ErrInstalledVersionUnknown   = errors.New("installed version unknown")
	ErrComponentNotFound         = errors.New("component not found for firmware install")
	ErrRequireHostPoweredOff     = errors.New("expected host to be powered off")
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

func (h *actionHandler) hostPoweredOff(_ *model.Action, tctx *sm.HandlerContext) (bool, error) {
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

	hostIsPoweredOff, err := h.hostPoweredOff(action, tctx)
	if err != nil {
		return err
	}

	// host is currently powered on and it wasn't powered on by flasher
	if !hostIsPoweredOff && tctx.Data[devicePoweredOn] != "true" {
		if tctx.Task.Parameters.RequireHostPoweredOff {
			return ErrRequireHostPoweredOff
		}

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

func (h *actionHandler) installedEqualsExpected(tctx *sm.HandlerContext, component, expectedFirmware, vendor string, models []string) error {
	inv, err := tctx.DeviceQueryor.Inventory(tctx.Ctx)
	if err != nil {
		return err
	}

	tctx.Logger.WithFields(
		logrus.Fields{
			"component": component,
		}).Debug("Querying device inventory from BMC for current component firmware")

	components, err := model.NewComponentConverter().CommonDeviceToComponents(inv)
	if err != nil {
		return err
	}

	found := components.BySlugModel(component, models)
	if found == nil {
		tctx.Logger.WithFields(
			logrus.Fields{
				"component": component,
				"vendor":    vendor,
				"models":    models,
				"err":       ErrComponentNotFound,
			}).Error("no component found for given component/vendor/model")

		return errors.Wrap(ErrComponentNotFound,
			fmt.Sprintf("component: %s, vendor: %s, model: %s", component,
				vendor,
				models,
			),
		)
	}

	tctx.Logger.WithFields(
		logrus.Fields{
			"component": found.Slug,
			"vendor":    found.Vendor,
			"model":     found.Model,
			"serial":    found.Serial,
			"installed": found.FirmwareInstalled,
			"expected":  expectedFirmware,
		}).Debug("found component")

	if strings.TrimSpace(found.FirmwareInstalled) == "" {
		return ErrInstalledVersionUnknown
	}

	if !strings.EqualFold(expectedFirmware, found.FirmwareInstalled) {
		return ErrInstalledFirmwareNotEqual
	}

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

	if err = h.installedEqualsExpected(
		tctx,
		action.Firmware.Component,
		action.Firmware.Version,
		action.Firmware.Vendor,
		action.Firmware.Models,
	); err != nil {
		if errors.Is(err, ErrInstalledVersionUnknown) {
			return errors.Wrap(err, "use TaskParameters.Force=true to disable this check")
		}

		return err
	}

	tctx.Logger.WithFields(
		logrus.Fields{
			"action id":       action.ID,
			"condition id":    action.TaskID,
			"component":       action.Firmware.Component,
			"vendor":          action.Firmware.Vendor,
			"models":          action.Firmware.Models,
			"expectedVersion": action.Firmware.Version,
		}).Info("Installed firmware version equals expected")

	return ErrInstalledFirmwareEqual
}

func (h *actionHandler) downloadFirmware(a sw.StateSwitch, c sw.TransitionArgs) error {
	action, tctx, err := actionTaskCtxFromInterfaces(a, c)
	if err != nil {
		return err
	}

	if action.FirmwareTempFile != "" {
		tctx.Logger.WithFields(
			logrus.Fields{
				"component": action.Firmware.Component,
				"file":      action.FirmwareTempFile,
			}).Info("firmware to be installed")

		return nil
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

func (h *actionHandler) uploadFirmware(a sw.StateSwitch, c sw.TransitionArgs) error {
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
		// initiate firmware upload
		firmwareUploadTaskID, err := tctx.DeviceQueryor.FirmwareUpload(
			tctx.Ctx,
			action.Firmware.Component,
			fileHandle,
		)
		if err != nil {
			return err
		}

		if firmwareUploadTaskID == "" {
			firmwareUploadTaskID = action.ID
		}

		action.FirmwareInstallStep = string(bconsts.FirmwareInstallStepUpload)
		action.BMCTaskID = firmwareUploadTaskID

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
	}

	tctx.Logger.WithFields(
		logrus.Fields{
			"component": action.Firmware.Component,
			"update":    action.Firmware.FileName,
			"version":   action.Firmware.Version,
			"BMCTaskID": action.BMCTaskID,
		}).Info("firmware upload complete")

	return nil
}

func (h *actionHandler) uploadFirmwareInitiateInstall(a sw.StateSwitch, c sw.TransitionArgs) error {
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
		bmcFirmwareInstallTaskID, err := tctx.DeviceQueryor.FirmwareInstall(
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
		if bmcFirmwareInstallTaskID == "" {
			bmcFirmwareInstallTaskID = action.ID
		}

		action.FirmwareInstallStep = string(bconsts.FirmwareInstallStepUploadInitiateInstall)
		action.BMCTaskID = bmcFirmwareInstallTaskID
	}

	tctx.Logger.WithFields(
		logrus.Fields{
			"component": action.Firmware.Component,
			"update":    action.Firmware.FileName,
			"version":   action.Firmware.Version,
			"bmcTaskID": action.BMCTaskID,
		}).Info("uploaded firmware and initiated install")

	return nil
}

func (h *actionHandler) installUploadedFirmware(a sw.StateSwitch, c sw.TransitionArgs) error {
	action, tctx, err := actionTaskCtxFromInterfaces(a, c)
	if err != nil {
		return err
	}

	if !tctx.Dryrun {
		// initiate firmware install
		bmcFirmwareInstallTaskID, err := tctx.DeviceQueryor.FirmwareInstallUploaded(
			tctx.Ctx,
			action.Firmware.Component,
			action.BMCTaskID,
		)
		if err != nil {
			return err
		}

		// returned bmcTaskID corresponds to a redfish task ID on BMCs that support redfish
		// for the rest we track the bmcTaskID as the action.ID
		if bmcFirmwareInstallTaskID == "" {
			bmcFirmwareInstallTaskID = action.ID
		}

		action.FirmwareInstallStep = string(bconsts.FirmwareInstallStepInstallUploaded)
		action.BMCTaskID = bmcFirmwareInstallTaskID
	}

	tctx.Logger.WithFields(
		logrus.Fields{
			"component": action.Firmware.Component,
			"update":    action.Firmware.FileName,
			"version":   action.Firmware.Version,
			"bmcTaskID": action.BMCTaskID,
		}).Info("initiated install for uploaded firmware")

	return nil
}

// polls firmware install status from the BMC
//
// nolint:gocyclo // for now this is best kept in the same method
func (h *actionHandler) pollFirmwareTaskStatus(a sw.StateSwitch, c sw.TransitionArgs) error {
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

	// inventory is set when the loop below determines that
	// a new collection should be attempted.
	var inventory bool

	// helper func
	componentIsBMC := func(c string) bool {
		return strings.EqualFold(strings.ToUpper(c), common.SlugBMC)
	}

	tctx.Logger.WithFields(
		logrus.Fields{
			"component": action.Firmware.Component,
			"version":   action.Firmware.Version,
			"bmc":       tctx.Asset.BmcAddress,
		}).Info("polling BMC for firmware task status")

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
				"%d attempts querying FirmwareTaskStatus(), elapsed: %s",
				attempts,
				time.Since(startTS).String(),
			))

			return attemptErrors
		}

		// TODO: break into its own method
		if inventory {
			err := h.installedEqualsExpected(
				tctx,
				action.Firmware.Component,
				action.Firmware.Version,
				action.Firmware.Vendor,
				action.Firmware.Models,
			)
			switch err {
			case nil:
				tctx.Logger.WithFields(
					logrus.Fields{
						"bmc":       tctx.Asset.BmcAddress,
						"component": action.Firmware.Component,
					}).Debug("Installed firmware matches expected.")

				return nil

			case ErrInstalledFirmwareNotEqual:
				// if the BMC came online and is still running the previous version
				// the install failed
				if componentIsBMC(action.Firmware.Component) {
					errInstall := errors.New("BMC failed to install expected firmware")
					return errInstall
				}

			default:
				// includes errors - ErrInstalledVersionUnknown, ErrComponentNotFound
				attemptErrors = multierror.Append(attemptErrors, err)
				tctx.Logger.WithFields(
					logrus.Fields{
						"bmc":       tctx.Asset.BmcAddress,
						"component": action.Firmware.Component,
						"err":       err.Error(),
					}).Debug("Inventory collection for component returned error")
			}

			continue
		}

		// query firmware install status
		state, status, err := tctx.DeviceQueryor.FirmwareTaskStatus(
			tctx.Ctx,
			bconsts.FirmwareInstallStep(action.FirmwareInstallStep),
			action.Firmware.Component,
			action.BMCTaskID,
			action.Firmware.Version,
			true, // need to be false for AsRockRack E3C256D4ID-NL
		)

		tctx.Logger.WithFields(
			logrus.Fields{
				"component": action.Firmware.Component,
				"update":    action.Firmware.FileName,
				"version":   action.Firmware.Version,
				"bmc":       tctx.Asset.BmcAddress,
				"elapsed":   time.Since(startTS).String(),
				"attempts":  fmt.Sprintf("attempt %d/%d", attempts, maxPollStatusAttempts),
				"taskState": state,
				"bmcTaskID": action.BMCTaskID,
				"status":    status,
			}).Debug("firmware task status query attempt")

		// error check returns when maxPollStatusAttempts have been reached
		if err != nil {
			attemptErrors = multierror.Append(attemptErrors, err)

			// no implementations available.
			if strings.Contains(err.Error(), "no FirmwareTaskVerifier implementations found") {
				return errors.Wrap(
					ErrFirmwareInstallFailed,
					"Firmware install support for component not available:"+err.Error(),
				)
			}

			// When BMCs are updating its own firmware, they can go unreachable
			// they apply the new firmware and in most cases the BMC task information is lost.
			//
			// And so if we get an error and its a BMC component that was being updated, we wait for
			// the BMC to be available again and validate its firmware matches the one expected.
			if componentIsBMC(action.Firmware.Component) {
				tctx.Logger.WithFields(
					logrus.Fields{
						"bmc":       tctx.Asset.BmcAddress,
						"delay":     delayBMCReset.String(),
						"taskState": state,
						"bmcTaskID": action.BMCTaskID,
						"status":    status,
						"err":       err.Error(),
					}).Debug("BMC task status lookup returned error")

				inventory = true
			}

			continue
		}

		switch state {
		// continue polling when install is running
		case bconsts.FirmwareInstallInitializing, bconsts.FirmwareInstallQueued, bconsts.FirmwareInstallRunning:
			continue

		// record the unknown status as an error
		case bconsts.FirmwareInstallUnknown:
			err = errors.New("BMC firmware task status unknown")
			attemptErrors = multierror.Append(attemptErrors, err)

			continue

		// return when bmc power cycle is required
		case bconsts.FirmwareInstallPowerCycleBMC:
			action.BMCPowerCycleRequired = true
			return nil

		// return when host power cycle is required
		case bconsts.FirmwareInstallPowerCycleHost:
			action.HostPowerCycleRequired = true
			return nil

		// return error when install fails
		case bconsts.FirmwareInstallFailed:
			return errors.Wrap(
				ErrFirmwareInstallFailed,
				"check logs on the BMC for information, bmc task ID: "+action.BMCTaskID,
			)

		// return nil when install is complete
		case bconsts.FirmwareInstallComplete:
			// The BMC would reset itself and returning now would mean the next install fails,
			// wait until the BMC is available again and verify its on the expected version.
			if componentIsBMC(action.Firmware.Component) {
				inventory = true

				continue
			}

			return nil

		default:
			return errors.Wrap(ErrFirmwareTaskStateUnexpected, "state: "+(state))
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

	// TODO: implement an poll BMC availability method instead
	return h.pollFirmwareTaskStatus(a, c)
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

	// TODO: check if this is required
	return h.pollFirmwareTaskStatus(a, c)
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

func (h *actionHandler) publishStatus(_ sw.StateSwitch, args sw.TransitionArgs) error {
	tctx, ok := args.(*sm.HandlerContext)
	if !ok {
		return sm.ErrInvalidTransitionHandler
	}

	tctx.Publisher.Publish(tctx)

	return nil
}
