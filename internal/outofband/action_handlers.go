package outofband

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hashicorp/go-multierror"
	common "github.com/metal-toolbox/bmc-common"
	bconsts "github.com/metal-toolbox/bmclib/constants"
	"github.com/metal-toolbox/flasher/internal/device"
	"github.com/metal-toolbox/flasher/internal/download"
	"github.com/metal-toolbox/flasher/internal/metrics"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"

	rctypes "github.com/metal-toolbox/rivets/v2/condition"
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

	// maxVerifyAttempts is the number of times - after a firmware install this poller will spend
	// attempting to verify the installed firmware equals the expected.
	//
	// Multiple attempts to verify is required to allow the BMC time to have its information updated,
	// the Supermicro BMCs on X12SPO-NTFs, complete the update process, but take
	// a while to update the installed firmware information returned over redfish.
	//
	// 30 (maxVerifyAttempts) * 10 (delayPollStatus) = 300s (5 minutes)
	maxVerifyAttempts = 30

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
	ErrInstalledVersionUnknown   = errors.New("installed version unknown")
	ErrComponentNotFound         = errors.New("component not identified for firmware install")
	ErrRequireHostPoweredOff     = errors.New("expected host to be powered off")
)

type handler struct {
	firmware      *rctypes.Firmware
	task          *model.Task
	action        *model.Action
	deviceQueryor device.OutofbandQueryor
	publisher     model.Publisher
	logger        *logrus.Entry
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

func (h *handler) serverPoweredOff(ctx context.Context) (bool, error) {
	// init out of band device queryor - if one isn't already initialized
	// this is done conditionally to enable tests to pass in a device queryor
	if h.deviceQueryor == nil {
		h.deviceQueryor = NewDeviceQueryor(ctx, h.task.Server, h.logger)
	}

	if err := h.deviceQueryor.Open(ctx); err != nil {
		return false, err
	}

	powerState, err := h.deviceQueryor.PowerStatus(ctx)
	if err != nil {
		return false, err
	}

	if strings.Contains(strings.ToLower(powerState), "off") { // covers states - Off, PoweringOff
		return true, nil
	}

	return false, nil
}

// initialize initializes the bmc connection and powers on server if required.
func (h *handler) powerOnServer(ctx context.Context) error {
	serverIsPoweredOff, err := h.serverPoweredOff(ctx)
	if err != nil {
		return err
	}

	// server is currently powered on and it wasn't powered on by flasher
	if !serverIsPoweredOff && h.task.Data.Scratch[devicePoweredOn] != "true" {
		if h.task.Parameters.RequireHostPoweredOff {
			return ErrRequireHostPoweredOff
		}

		return nil
	}

	h.logger.WithFields(
		logrus.Fields{
			"component": h.firmware.Component,
			"version":   h.firmware.Version,
		}).Info("device is currently powered off, powering on")

	if !h.task.Parameters.DryRun {
		if err := h.deviceQueryor.SetPowerState(ctx, "on"); err != nil {
			return err
		}

		if err := sleepWithContext(ctx, delayHostPowerStatusChange); err != nil {
			return err
		}
	}

	h.task.Data.Scratch[devicePoweredOn] = "true"

	return nil
}

func (h *handler) installedEqualsExpected(ctx context.Context, component, expectedFirmware, vendor string, models []string) error {
	inv, err := h.deviceQueryor.Inventory(ctx)
	if err != nil {
		return err
	}

	h.logger.WithFields(
		logrus.Fields{
			"component": component,
		}).Debug("Querying device inventory from BMC for current component firmware")

	components, err := model.NewComponentConverter().CommonDeviceToComponents(inv)
	if err != nil {
		return err
	}

	found := components.ByNameModel(component, models)
	if found == nil {
		h.logger.WithFields(
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

	h.logger.WithFields(
		logrus.Fields{
			"component": found.Name,
			"vendor":    found.Vendor,
			"model":     found.Model,
			"serial":    found.Serial,
			"current":   found.Firmware.Installed,
			"expected":  expectedFirmware,
		}).Debug("component version check")

	if strings.TrimSpace(found.Firmware.Installed) == "" {
		return ErrInstalledVersionUnknown
	}

	if !strings.EqualFold(expectedFirmware, found.Firmware.Installed) {
		return errors.Wrap(
			ErrInstalledFirmwareNotEqual,
			fmt.Sprintf("expected: %s, current: %s", expectedFirmware, found.Firmware.Installed),
		)
	}

	return nil
}

func (h *handler) checkCurrentFirmware(ctx context.Context) error {
	if h.task.Parameters.ForceInstall {
		h.logger.WithFields(
			logrus.Fields{
				"component": h.firmware.Component,
			}).Debug("Skipped installed version lookup - task.Parameters.ForceInstall=true")

		return nil
	}

	if err := h.installedEqualsExpected(
		ctx,
		h.firmware.Component,
		h.firmware.Version,
		h.firmware.Vendor,
		h.firmware.Models,
	); err != nil {
		if errors.Is(err, ErrInstalledVersionUnknown) {
			return errors.Wrap(err, "use task.Parameters.ForceInstall=true to disable this check")
		}

		if errors.Is(err, ErrInstalledFirmwareNotEqual) {
			return nil
		}

		return err
	}

	h.logger.WithFields(
		logrus.Fields{
			"action id":    h.action.ID,
			"condition id": h.task.ID.String(),
			"component":    h.firmware.Component,
			"vendor":       h.firmware.Vendor,
			"models":       h.firmware.Models,
			"expected":     h.firmware.Version,
		}).Info("Installed firmware version equals expected")

	return model.ErrInstalledFirmwareEqual
}

func (h *handler) downloadFirmware(ctx context.Context) error {
	if h.action.FirmwareTempFile != "" {
		h.logger.WithFields(
			logrus.Fields{
				"component": h.firmware.Component,
				"file":      h.action.FirmwareTempFile,
			}).Info("firmware file path provided, skipped download")

		return nil
	}

	// create a temp download directory
	dir, err := os.MkdirTemp(downloadDir, "")
	if err != nil {
		return errors.Wrap(err, "error creating tmp directory to download firmware")
	}

	file := filepath.Join(dir, h.firmware.FileName)

	// download firmware file
	err = download.FromURLToFile(ctx, h.firmware.URL, file)
	if err != nil {
		return err
	}

	// collect download metrics
	fileInfo, err := os.Stat(file)
	if err == nil {
		metrics.DownloadBytes.With(
			prometheus.Labels{
				"component": h.firmware.Component,
				"vendor":    h.firmware.Vendor,
			},
		).Add(float64(fileInfo.Size()))
	}

	// validate checksum
	if err := download.ChecksumValidate(file, h.firmware.Checksum); err != nil {
		os.RemoveAll(filepath.Dir(file))
		return err
	}

	// store the firmware temp file location
	h.action.FirmwareTempFile = file

	h.logger.WithFields(
		logrus.Fields{
			"component": h.firmware.Component,
			"version":   h.firmware.Version,
			"url":       h.firmware.URL,
			"file":      file,
			"checksum":  h.firmware.Checksum,
		}).Info("downloaded and verified firmware file checksum")

	return nil
}

func (h *handler) uploadFirmware(ctx context.Context) error {
	// open firmware file handle
	fileHandle, err := os.Open(h.action.FirmwareTempFile)
	if err != nil {
		return errors.Wrap(err, h.action.FirmwareTempFile)
	}

	defer fileHandle.Close()
	defer os.RemoveAll(filepath.Dir(h.action.FirmwareTempFile))

	if !h.task.Parameters.DryRun {
		// initiate firmware upload
		firmwareUploadTaskID, err := h.deviceQueryor.FirmwareUpload(
			ctx,
			h.firmware.Component,
			fileHandle,
		)
		if err != nil {
			return err
		}

		if firmwareUploadTaskID == "" {
			firmwareUploadTaskID = h.action.ID
		}

		h.action.FirmwareInstallStep = string(bconsts.FirmwareInstallStepUpload)
		h.action.BMCTaskID = firmwareUploadTaskID

		// collect upload metrics
		fileInfo, err := os.Stat(h.action.FirmwareTempFile)
		if err == nil {
			metrics.UploadBytes.With(
				prometheus.Labels{
					"component": h.firmware.Component,
					"vendor":    h.firmware.Vendor,
				},
			).Add(float64(fileInfo.Size()))
		}
	}

	h.logger.WithFields(
		logrus.Fields{
			"component": h.firmware.Component,
			"update":    h.firmware.FileName,
			"version":   h.firmware.Version,
			"BMCTaskID": h.action.BMCTaskID,
		}).Info("firmware upload complete")

	return nil
}

func (h *handler) uploadFirmwareInitiateInstall(ctx context.Context) error {
	if h.action.FirmwareTempFile == "" {
		return errors.Wrap(ErrFirmwareTempFile, "expected FirmwareTempFile to be declared")
	}

	// open firmware file handle
	fileHandle, err := os.Open(h.action.FirmwareTempFile)
	if err != nil {
		return errors.Wrap(err, h.action.FirmwareTempFile)
	}

	defer fileHandle.Close()
	defer os.RemoveAll(filepath.Dir(h.action.FirmwareTempFile))

	if !h.task.Parameters.DryRun {
		// initiate firmware install
		bmcFirmwareInstallTaskID, err := h.deviceQueryor.FirmwareInstallUploadAndInitiate(
			ctx,
			h.firmware.Component,
			fileHandle,
		)
		if err != nil {
			return err
		}

		// collect upload metrics
		fileInfo, err := os.Stat(h.action.FirmwareTempFile)
		if err == nil {
			metrics.UploadBytes.With(
				prometheus.Labels{
					"component": h.firmware.Component,
					"vendor":    h.firmware.Vendor,
				},
			).Add(float64(fileInfo.Size()))
		}

		// returned bmcTaskID corresponds to a redfish task ID on BMCs that support redfish
		// for the rest we track the bmcTaskID as the h.action.ID
		if bmcFirmwareInstallTaskID == "" {
			bmcFirmwareInstallTaskID = h.action.ID
		}

		h.action.FirmwareInstallStep = string(bconsts.FirmwareInstallStepUploadInitiateInstall)
		h.action.BMCTaskID = bmcFirmwareInstallTaskID
	}

	h.logger.WithFields(
		logrus.Fields{
			"component": h.firmware.Component,
			"update":    h.firmware.FileName,
			"version":   h.firmware.Version,
			"bmcTaskID": h.action.BMCTaskID,
		}).Info("uploaded firmware and initiated install")

	return nil
}

func (h *handler) installUploadedFirmware(ctx context.Context) error {
	if !h.task.Parameters.DryRun {
		// initiate firmware install
		bmcFirmwareInstallTaskID, err := h.deviceQueryor.FirmwareInstallUploaded(
			ctx,
			h.firmware.Component,
			h.action.BMCTaskID,
		)
		if err != nil {
			return err
		}

		// returned bmcTaskID corresponds to a redfish task ID on BMCs that support redfish
		// for the rest we track the bmcTaskID as the h.action.ID
		if bmcFirmwareInstallTaskID == "" {
			bmcFirmwareInstallTaskID = h.action.ID
		}

		h.action.FirmwareInstallStep = string(bconsts.FirmwareInstallStepInstallUploaded)
		h.action.BMCTaskID = bmcFirmwareInstallTaskID
	}

	h.logger.WithFields(
		logrus.Fields{
			"component": h.firmware.Component,
			"update":    h.firmware.FileName,
			"version":   h.firmware.Version,
			"bmcTaskID": h.action.BMCTaskID,
		}).Info("initiated install for uploaded firmware")

	return nil
}

// polls firmware install status from the BMC
//
// nolint:gocyclo // for now this is best kept in the same method
func (h *handler) pollFirmwareTaskStatus(ctx context.Context) error {
	if h.task.Parameters.DryRun {
		return nil
	}

	var installTask bool
	installTaskTypes := []string{
		string(bconsts.FirmwareInstallStepUploadInitiateInstall),
		string(bconsts.FirmwareInstallStepInstallUploaded),
	}

	if slices.Contains(installTaskTypes, h.action.FirmwareInstallStep) {
		installTask = true
	}

	startTS := time.Now()

	// number of status queries attempted
	var attempts, verifyAttempts int

	var attemptErrors *multierror.Error

	// inventory is set when the loop below determines that
	// a new collection should be attempted.
	var inventory bool

	// helper func
	componentIsBMC := func(c string) bool {
		return strings.EqualFold(strings.ToUpper(c), common.SlugBMC)
	}

	h.logger.WithFields(
		logrus.Fields{
			"component":   h.action.Firmware.Component,
			"version":     h.action.Firmware.Version,
			"bmc":         h.task.Server.BMCAddress,
			"step":        h.action.FirmwareInstallStep,
			"installTask": installTask,
		}).Info("polling BMC for firmware task status")

	// the prefix we'll be using for all the poll status updates
	statusPrefix := h.task.Status.Last()

	for {
		// increment attempts
		attempts++

		// delay if we're in the second or subsequent attempts
		if attempts > 0 {
			if err := sleepWithContext(ctx, delayPollStatus); err != nil {
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
			verifyAttempts++

			err := h.installedEqualsExpected(
				ctx,
				h.firmware.Component,
				h.firmware.Version,
				h.firmware.Vendor,
				h.firmware.Models,
			)

			// nolint:errorlint // default case catches misc errors
			switch err {
			case nil:
				h.logger.WithFields(
					logrus.Fields{
						"bmc":       h.task.Server.BMCAddress,
						"component": h.firmware.Component,
					}).Debug("Installed firmware matches expected.")

				return nil

			case ErrInstalledFirmwareNotEqual:
				// if the BMC came online and is still running the previous version
				// the install failed
				if componentIsBMC(h.action.Firmware.Component) && verifyAttempts >= maxVerifyAttempts {
					errInstall := errors.New("BMC failed to install expected firmware")
					return errInstall
				}

			default:
				// includes errors - ErrInstalledVersionUnknown, ErrComponentNotFound
				attemptErrors = multierror.Append(attemptErrors, err)
				h.logger.WithFields(
					logrus.Fields{
						"bmc":       h.task.Server.BMCAddress,
						"component": h.firmware.Component,
						"elapsed":   time.Since(startTS).String(),
						"attempts":  fmt.Sprintf("attempt %d/%d", attempts, maxPollStatusAttempts),
						"err":       err.Error(),
					}).Debug("Inventory collection for component returned error")
			}

			continue
		}

		// query firmware install status
		state, status, err := h.deviceQueryor.FirmwareTaskStatus(
			ctx,
			bconsts.FirmwareInstallStep(h.action.FirmwareInstallStep),
			h.firmware.Component,
			h.action.BMCTaskID,
			h.firmware.Version,
		)

		h.logger.WithFields(
			logrus.Fields{
				"component": h.firmware.Component,
				"update":    h.firmware.FileName,
				"version":   h.firmware.Version,
				"bmc":       h.task.Server.BMCAddress,
				"elapsed":   time.Since(startTS).String(),
				"attempts":  fmt.Sprintf("attempt %d/%d", attempts, maxPollStatusAttempts),
				"taskState": state,
				"bmcTaskID": h.action.BMCTaskID,
				"status":    status,
			}).Debug("firmware task status query attempt")

		if h.publisher != nil && status != "" {
			h.task.Status.Update(h.task.Status.Last(), statusPrefix+" -- "+status)
			//nolint:errcheck // method called logs errors if any
			_ = h.publisher.Publish(ctx, h.task)
		}

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
			if componentIsBMC(h.action.Firmware.Component) && installTask {
				h.logger.WithFields(
					logrus.Fields{
						"bmc":       h.task.Server.BMCAddress,
						"delay":     delayBMCReset.String(),
						"taskState": state,
						"bmcTaskID": h.action.BMCTaskID,
						"status":    status,
						"err":       err.Error(),
					}).Debug("BMC task status lookup returned error")

				inventory = true
			}

			continue
		}

		switch state {
		// continue polling when install is running
		case bconsts.Initializing, bconsts.Queued, bconsts.Running:
			continue

		// record the unknown status as an error
		case bconsts.Unknown:
			err = errors.New("BMC firmware task status unknown")
			attemptErrors = multierror.Append(attemptErrors, err)

			continue

		// return when host power cycle is required
		case bconsts.PowerCycleHost:
			// host was power cycled for this action - wait around until the status is updated
			if h.action.HostPowerCycled {
				continue
			}

			// power cycle server and continue
			if err := h.powerCycleServer(ctx); err != nil {
				return err
			}

			h.action.HostPowerCycled = true

			// reset attempts
			attempts = 0

			continue

		// return error when install fails
		case bconsts.Failed:
			var errMsg string
			if status == "" {
				errMsg = fmt.Sprintf(
					"install failed with errors, task ID: %s",
					h.action.BMCTaskID,
				)
			} else {
				errMsg = fmt.Sprintf(
					"install failed with errors, task ID: %s, status: %s",
					h.action.BMCTaskID,
					status,
				)
			}

			// A BMC reset is required if the BMC install fails - to get it out of flash mode
			if componentIsBMC(h.action.Firmware.Component) && installTask && h.action.BMCResetOnInstallFailure {
				if err := h.powerCycleBMC(ctx); err != nil {
					h.logger.WithFields(
						logrus.Fields{
							"bmc":       h.task.Server.BMCAddress,
							"component": h.firmware.Component,
							"err":       err.Error(),
						}).Debug("install failure required a BMC reset, reset returned error")
				}

				h.logger.WithFields(
					logrus.Fields{
						"bmc":       h.task.Server.BMCAddress,
						"component": h.firmware.Component,
					}).Debug("BMC reset for failed BMC firmware install")
			}

			return errors.Wrap(ErrFirmwareInstallFailed, errMsg)

		// return nil when install is complete
		case bconsts.Complete:
			// The BMC would reset itself and returning now would mean the next install fails,
			// wait until the BMC is available again and verify its on the expected version.
			if componentIsBMC(h.action.Firmware.Component) && installTask {
				inventory = true
				// re-initialize the client to make sure we're not re-using old sessions.
				h.deviceQueryor.ReinitializeClient(ctx)

				if h.action.BMCResetPostInstall {
					if errBmcReset := h.powerCycleBMC(ctx); errBmcReset != nil {
						h.logger.WithFields(
							logrus.Fields{
								"bmc":       h.task.Server.BMCAddress,
								"component": h.firmware.Component,
								"err":       errBmcReset.Error(),
							}).Debug("install success required a BMC reset, reset returned error")
					}

					h.logger.WithFields(
						logrus.Fields{
							"bmc":       h.task.Server.BMCAddress,
							"component": h.firmware.Component,
						}).Debug("BMC reset for successful BMC firmware install")
				}

				continue
			}

			return nil

		default:
			return errors.Wrap(ErrFirmwareTaskStateUnexpected, "state: "+string(state))
		}
	}
}

func (h *handler) resetBMC(ctx context.Context) error {
	h.logger.WithFields(
		logrus.Fields{
			"component": h.firmware.Component,
			"bmc":       h.task.Server.BMCAddress,
		}).Info("resetting BMC, delay introduced: " + delayBMCReset.String())

	err := h.powerCycleBMC(ctx)
	if err != nil {
		return err
	}

	if h.task.Parameters.DryRun {
		return nil
	}

	return sleepWithContext(ctx, delayBMCReset)
}

func (h *handler) powerCycleBMC(ctx context.Context) error {
	if h.task.Parameters.DryRun {
		return nil
	}

	return h.deviceQueryor.ResetBMC(ctx)
}

func (h *handler) powerCycleServer(ctx context.Context) error {
	if h.task.Parameters.DryRun {
		return nil
	}

	h.logger.WithFields(
		logrus.Fields{
			"component": h.firmware.Component,
			"bmc":       h.task.Server.BMCAddress,
		}).Info("resetting host for firmware install")

	return h.deviceQueryor.SetPowerState(ctx, "cycle")
}

func (h *handler) conditionalPowerOffDevice(_ context.Context) (bool, error) {
	// The install provider indicated the host must be powered off
	if h.action.HostPowerOffPreInstall {
		return true, nil
	}

	// proceed to power off the device if this is the final action
	if !h.action.Last {
		return false, nil
	}

	wasPoweredOn, keyExists := h.task.Data.Scratch[devicePoweredOn]
	if !keyExists {
		return false, nil
	}

	if wasPoweredOn == "true" {
		return true, nil
	}

	return false, nil
}

// initialize initializes the bmc connection and powers on the host if required.
func (h *handler) powerOffServer(ctx context.Context) error {
	powerOffDeviceRequired, err := h.conditionalPowerOffDevice(ctx)
	if err != nil {
		return err
	}

	if !powerOffDeviceRequired {
		return nil
	}

	if !h.task.Parameters.DryRun {
		h.logger.WithFields(
			logrus.Fields{
				"component": h.firmware.Component,
				"bmc":       h.task.Server.BMCAddress,
			}).Debug("powering off device")

		if err := h.deviceQueryor.SetPowerState(ctx, "off"); err != nil {
			return err
		}
	}

	return nil
}
