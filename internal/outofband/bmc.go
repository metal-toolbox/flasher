package outofband

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"

	bmclibv2 "github.com/bmc-toolbox/bmclib/v2"
	bmclibv2consts "github.com/bmc-toolbox/bmclib/v2/constants"

	"github.com/bmc-toolbox/common"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/sirupsen/logrus"
)

var (
	// logoutTimeout is the timeout value when logging out of a bmc
	logoutTimeout = "5m"
	loginTimeout  = "5s"
	loginAttempts = 3

	// login errors
	errBMCLogin             = errors.New("bmc login error")
	errBMCLoginTimeout      = errors.New("bmc login timeout")
	errBMCLoginUnAuthorized = errors.New("bmc login unauthorized")

	errBMCInventory = errors.New("bmc inventory error")

	errBMCLogout = errors.New("bmc logout error")

	ErrBMCQuery = errors.New("error occured in bmc query")

	ErrFirmwareInstallFail             = errors.New("firmware install failed")
	ErrFirmwareInstallStatusUnexpected = errors.New("firmware install status unexpected")
)

// bmc wraps the bmclib client and implements the bmcQueryor interface
type bmc struct {
	client *bmclibv2.Client
	logger *logrus.Entry
}

// NewDeviceQueryor returns a bmc queryor that implements the DeviceQueryor interface
func NewDeviceQueryor(ctx context.Context, device *model.Device, logger *logrus.Entry) model.DeviceQueryor {
	return &bmc{
		client: newBmclibv2Client(ctx, device, logger),
		logger: logger,
	}
}

type ErrBmcQuery struct {
	cause string
}

func (e *ErrBmcQuery) Error() string {
	return e.cause
}

func newErrBmcQuery(cause string) error {
	return &ErrBmcQuery{cause: cause}
}

// Open creates a BMC session
func (b *bmc) Open(ctx context.Context) error {
	if b.client == nil {
		return errors.Wrap(errBMCLogin, "bmclibv2 client not initialized")
	}

	// return if a session is active
	if b.SessionActive(ctx) {
		b.logger.Trace("bmc session active, skipped login attempt")

		return nil
	}

	// login to the bmc with retries
	if err := b.loginWithRetries(ctx, 3); err != nil {
		return err
	}

	return nil
}

// SessionActive determines if the connection has an active session.
func (b *bmc) SessionActive(ctx context.Context) bool {
	if b.client == nil {
		return false
	}

	_, err := b.client.GetPowerState(ctx)
	if err == nil {
		return true
	}

	return false
}

// Close logs out of the BMC
func (b *bmc) Close() error {
	if b.client == nil {
		return nil
	}

	timeout, err := time.ParseDuration(logoutTimeout)
	if err != nil {
		return errors.Wrap(errBMCLogout, err.Error())
	}

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(timeout))
	defer cancel()

	if err := b.client.Close(ctx); err != nil {
		return errors.Wrap(errBMCLogout, err.Error())
	}

	b.client = nil

	return nil
}

// PowerStatus returns the device power status
func (b *bmc) PowerStatus(ctx context.Context) (string, error) {
	if err := b.Open(ctx); err != nil {
		return "", err
	}

	status, err := b.client.GetPowerState(ctx)
	if err != nil {
		return "", err
	}

	return status, nil
}

// SetPowerState sets the given power state on the device
func (b *bmc) SetPowerState(ctx context.Context, state string) error {
	if err := b.Open(ctx); err != nil {
		return err
	}

	_, err := b.client.SetPowerState(ctx, state)
	if err != nil {
		return err
	}

	return nil
}

// ResetBMC cold resets the BMC
func (b *bmc) ResetBMC(ctx context.Context) error {
	if err := b.Open(ctx); err != nil {
		return err
	}

	_, err := b.client.ResetBMC(ctx, "cold")
	if err != nil {
		return err
	}

	return nil
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

func (b *bmc) FirmwareInstall(ctx context.Context, componentSlug string, force bool, file *os.File) (bmcTaskID string, err error) {
	if err := b.Open(ctx); err != nil {
		return "", err
	}

	return b.client.FirmwareInstall(ctx, componentSlug, bmclibv2consts.FirmwareApplyOnReset, force, file)
}

// FirmwareInstallStatus looks up the firmware install status based on the given installVersion, componentSlug, bmcTaskID parameteres
func (b *bmc) FirmwareInstallStatus(ctx context.Context, installVersion, componentSlug, bmcTaskID string) (model.ComponentFirmwareInstallStatus, error) {
	if err := b.Open(ctx); err != nil {
		return model.StatusInstallUnknown, errors.Wrap(ErrBMCQuery, err.Error())
	}

	status, err := b.client.FirmwareInstallStatus(ctx, installVersion, componentSlug, bmcTaskID)
	if err != nil {
		return model.StatusInstallUnknown, errors.Wrap(ErrBMCQuery, err.Error())
	}

	switch status {
	case bmclibv2consts.FirmwareInstallInitializing, bmclibv2consts.FirmwareInstallQueued, bmclibv2consts.FirmwareInstallRunning:
		return model.StatusInstallRunning, nil
	case bmclibv2consts.FirmwareInstallPowerCyleHost:
		// if the host is under reset (this is the final state only for queueing updates)
		//	if hostWasReset {
		//		return false, nil
		//	}

		return model.StatusInstallPowerCycleHostRequired, nil
	case bmclibv2consts.FirmwareInstallPowerCycleBMC:
		// if BMC is under reset return false (this is the final state only for queuing the update)
		//	if bmcWasReset {
		//		return false, nil
		//	}

		return model.StatusInstallPowerCycleBMCRequired, nil
	case bmclibv2consts.FirmwareInstallComplete:
		return model.StatusInstallComplete, nil
	case bmclibv2consts.FirmwareInstallFailed:
		return model.StatusInstallFailed, nil
	case bmclibv2consts.FirmwareInstallUnknown:
		return model.StatusInstallUnknown, nil
	default:
		return model.StatusInstallUnknown, errors.Wrap(ErrFirmwareInstallStatusUnexpected, status)
	}
}
