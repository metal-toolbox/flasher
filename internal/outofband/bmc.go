package outofband

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"

	bmclib "github.com/bmc-toolbox/bmclib/v2"
	bconsts "github.com/bmc-toolbox/bmclib/v2/constants"

	"github.com/bmc-toolbox/common"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/sirupsen/logrus"
)

var (
	// logoutTimeout is the timeout value when logging out of a bmc
	logoutTimeout = 1 * time.Minute
	loginTimeout  = 3 * time.Minute
	loginAttempts = 3

	// firmwareInstallTimeout is set on the context when invoking the firmware install method
	firmwareInstallTimeout = 20 * time.Minute

	// login errors
	errBMCLogin             = errors.New("bmc login error")
	errBMCLoginTimeout      = errors.New("bmc login timeout")
	errBMCLoginUnAuthorized = errors.New("bmc login unauthorized")
	errBMCSession           = errors.New("bmc session error")

	errBMCInventory = errors.New("bmc inventory error")

	errBMCLogout = errors.New("bmc logout error")

	ErrBMCQuery                    = errors.New("error occurred in bmc query")
	ErrMaxBMCQueryAttempts         = errors.New("reached maximum BMC query attempts")
	ErrTaskNotFound                = errors.New("task not found")
	ErrFirmwareInstallFailed       = errors.New("firmware install failed")
	ErrFirmwareTaskStateUnexpected = errors.New("BMC returned unexpected firmware task state")
)

// bmc wraps the bmclib client and implements the bmcQueryor interface
type bmc struct {
	client *bmclib.Client
	logger *logrus.Entry
}

// NewDeviceQueryor returns a bmc queryor that implements the DeviceQueryor interface
func NewDeviceQueryor(ctx context.Context, asset *model.Asset, logger *logrus.Entry) model.DeviceQueryor {
	return &bmc{
		client: newBmclibv2Client(ctx, asset, logger),
		logger: logger,
	}
}

type ErrBmcQuery struct {
	cause string
}

func (e *ErrBmcQuery) Error() string {
	return e.cause
}

// Open creates a BMC session
func (b *bmc) Open(ctx context.Context) error {
	if b.client == nil {
		return errors.Wrap(errBMCLogin, "bmclib client not initialized")
	}

	// login to the bmc with retries
	return b.loginWithRetries(ctx, loginAttempts)
}

// Close logs out of the BMC
func (b *bmc) Close(traceCtx context.Context) error {
	if b.client == nil {
		return nil
	}

	ctxClose, cancel := context.WithTimeout(traceCtx, logoutTimeout)
	defer cancel()

	if err := b.client.Close(ctxClose); err != nil {
		return errors.Wrap(errBMCLogout, err.Error())
	}

	b.logger.Debug("bmc logout successful")

	b.client = nil

	return nil
}

// PowerStatus returns the device power status
func (b *bmc) PowerStatus(ctx context.Context) (string, error) {
	if err := b.Open(ctx); err != nil {
		return "", err
	}

	return b.client.GetPowerState(ctx)
}

// SetPowerState sets the given power state on the device
func (b *bmc) SetPowerState(ctx context.Context, state string) error {
	if err := b.Open(ctx); err != nil {
		return err
	}

	_, err := b.client.SetPowerState(ctx, state)

	return err
}

// ResetBMC cold resets the BMC
func (b *bmc) ResetBMC(ctx context.Context) error {
	if err := b.Open(ctx); err != nil {
		return err
	}

	_, err := b.client.ResetBMC(ctx, "GracefulRestart")

	return err
}

// Inventory queries the BMC for the device inventory and returns an object with the device inventory.
func (b *bmc) Inventory(ctx context.Context) (*common.Device, error) {
	if err := b.Open(ctx); err != nil {
		return nil, err
	}

	inventory, err := b.client.Inventory(ctx)
	if err != nil {
		// increment inventory query error count metric
		if strings.Contains(err.Error(), "no compatible System Odata IDs identified") {
			return nil, errors.Wrap(errBMCInventory, "redfish_incompatible: no compatible System Odata IDs identified")
		}

		return nil, errors.Wrap(errBMCInventory, err.Error())
	}

	// format the device inventory vendor attribute so its consistent
	inventory.Vendor = common.FormatVendorName(inventory.Vendor)

	return inventory, nil
}

func (b *bmc) FirmwareInstallSteps(ctx context.Context, component string) ([]bconsts.FirmwareInstallStep, error) {
	if err := b.Open(ctx); err != nil {
		return nil, err
	}

	return b.client.FirmwareInstallSteps(ctx, component)
}

func (b *bmc) FirmwareInstall(ctx context.Context, componentSlug string, force bool, file *os.File) (bmcTaskID string, err error) {
	if err := b.Open(ctx); err != nil {
		return "", err
	}

	installCtx, cancel := context.WithTimeout(ctx, firmwareInstallTimeout)
	defer cancel()

	return b.client.FirmwareInstall(installCtx, componentSlug, string(bconsts.OnReset), force, file)
}

// FirmwareTaskStatus looks up the firmware upload/install state and status values
func (b *bmc) FirmwareTaskStatus(ctx context.Context, kind bconsts.FirmwareInstallStep, component, taskID, installVersion string, tryOpen bool) (state, status string, err error) {
	if tryOpen {
		if err := b.Open(ctx); err != nil {
			return "", "", errors.Wrap(ErrBMCQuery, err.Error())
		}
	}

	return b.client.FirmwareTaskStatus(ctx, kind, component, taskID, installVersion)
}

func (b *bmc) FirmwareUpload(ctx context.Context, component string, file *os.File) (uploadTaskID string, err error) {
	if err := b.Open(ctx); err != nil {
		return "", err
	}

	installCtx, cancel := context.WithTimeout(ctx, firmwareInstallTimeout)
	defer cancel()

	return b.client.FirmwareUpload(installCtx, component, file)
}

func (b *bmc) FirmwareInstallUploaded(ctx context.Context, component, uploadVerifyTaskID string) (installTaskID string, err error) {
	if err := b.Open(ctx); err != nil {
		return "", err
	}

	installCtx, cancel := context.WithTimeout(ctx, firmwareInstallTimeout)
	defer cancel()

	return b.client.FirmwareInstallUploaded(installCtx, component, uploadVerifyTaskID)
}
