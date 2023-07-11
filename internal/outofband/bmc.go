package outofband

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	bmclibv2 "github.com/bmc-toolbox/bmclib/v2"
	bmclibv2consts "github.com/bmc-toolbox/bmclib/v2/constants"

	"github.com/bmc-toolbox/common"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/sirupsen/logrus"
)

const (
	pkgName = "internal/outofband"
)

var (
	// logoutTimeout is the timeout value when logging out of a bmc
	logoutTimeout = 1 * time.Minute
	loginTimeout  = 3 * time.Minute
	loginAttempts = 3

	// firmwareInstallTimeout is set on the context when invoking the firmware install method
	firmwareInstallTimeout = 15 * time.Minute

	// login errors
	errBMCLogin             = errors.New("bmc login error")
	errBMCLoginTimeout      = errors.New("bmc login timeout")
	errBMCLoginUnAuthorized = errors.New("bmc login unauthorized")
	errBMCSession           = errors.New("bmc session error")

	errBMCInventory = errors.New("bmc inventory error")

	errBMCLogout = errors.New("bmc logout error")

	ErrBMCQuery                        = errors.New("error occurred in bmc query")
	ErrMaxBMCQueryAttempts             = errors.New("reached maximum BMC query attempts")
	ErrFirmwareInstallFailed           = errors.New("firmware install failed")
	ErrFirmwareInstallStatusUnexpected = errors.New("firmware install status unexpected")
)

// bmc wraps the bmclib client and implements the bmcQueryor interface
type bmc struct {
	client *bmclibv2.Client
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
	ctx, span := otel.Tracer(pkgName).Start(ctx, "bmclib.Open")
	defer span.End()

	span.SetAttributes(attribute.String("bmc-ip", b.client.Auth.Host))

	if b.client == nil {
		return errors.Wrap(errBMCLogin, "bmclibv2 client not initialized")
	}

	// login to the bmc with retries
	return b.loginWithRetries(ctx, loginAttempts)
}

// Close logs out of the BMC
func (b *bmc) Close(traceCtx context.Context) error {
	// this context is not used for the close method further below
	// since we want to make sure the BMC session is always closed and is not left open
	// because of a context cancellation.
	_, span := otel.Tracer(pkgName).Start(traceCtx, "bmclib.Close")
	defer span.End()

	if b.client == nil {
		return nil
	}

	ctxClose, cancel := context.WithTimeout(context.Background(), logoutTimeout)
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
	ctx, span := otel.Tracer(pkgName).Start(ctx, "bmclib.PowerStatus")
	defer span.End()

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
	ctx, span := otel.Tracer(pkgName).Start(ctx, "bmclib.SetPowerState")
	defer span.End()

	if err := b.Open(ctx); err != nil {
		return err
	}

	_, err := b.client.SetPowerState(ctx, state)

	return err
}

// ResetBMC cold resets the BMC
func (b *bmc) ResetBMC(ctx context.Context) error {
	ctx, span := otel.Tracer(pkgName).Start(ctx, "bmclib.ResetBMC")
	defer span.End()

	span.SetAttributes(attribute.String("bmc-ip", b.client.Auth.Host))

	if err := b.Open(ctx); err != nil {
		return err
	}

	_, err := b.client.ResetBMC(ctx, "GracefulRestart")

	return err
}

// Inventory queries the BMC for the device inventory and returns an object with the device inventory.
func (b *bmc) Inventory(ctx context.Context) (*common.Device, error) {
	ctx, span := otel.Tracer(pkgName).Start(ctx, "bmclib.Inventory")
	defer span.End()

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
	ctx, span := otel.Tracer(pkgName).Start(ctx, "bmclib.FirmwareInstall")
	defer span.End()

	span.SetAttributes(attribute.String("bmc-ip", b.client.Auth.Host))

	if err := b.Open(ctx); err != nil {
		return "", err
	}

	installCtx, cancel := context.WithTimeout(ctx, firmwareInstallTimeout)
	defer cancel()

	return b.client.FirmwareInstall(installCtx, componentSlug, bmclibv2consts.FirmwareApplyOnReset, force, file)
}

// FirmwareInstallStatus looks up the firmware install status based on the given installVersion, componentSlug, bmcTaskID parameters
func (b *bmc) FirmwareInstallStatus(ctx context.Context, installVersion, componentSlug, bmcTaskID string) (model.ComponentFirmwareInstallStatus, error) {
	ctx, span := otel.Tracer(pkgName).Start(ctx, "bmclib.FirmwareInstallStatus")
	defer span.End()

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
