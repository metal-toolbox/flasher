package outofband

import (
	"context"
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/pkg/errors"

	bmclib "github.com/bmc-toolbox/bmclib/v2"
	bconsts "github.com/bmc-toolbox/bmclib/v2/constants"

	"github.com/bmc-toolbox/common"
	"github.com/metal-toolbox/flasher/internal/device"
	"github.com/sirupsen/logrus"

	rtypes "github.com/metal-toolbox/rivets/types"
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
	ErrQueryorMethod               = errors.New("BMC queryor method error")

	ErrFirmwareInstallProvider = errors.New("firmware install provider not identified")
)

// bmc wraps the bmclib client and implements the device.Queryor interface
type bmc struct {
	client             *bmclib.Client
	logger             *logrus.Entry
	asset              *rtypes.Server
	installProvider    string
	availableProviders []string
}

// NewDeviceQueryor returns a bmc queryor that implements the DeviceQueryor interface
func NewDeviceQueryor(ctx context.Context, asset *rtypes.Server, logger *logrus.Entry) device.Queryor {
	return &bmc{
		client: newBmclibv2Client(ctx, asset, logger),
		logger: logger,
		asset:  asset,
	}
}

type ErrBmcQuery struct {
	cause string
}

func (e *ErrBmcQuery) Error() string {
	return e.cause
}

func (b *bmc) tracelog() {
	pc, _, _, _ := runtime.Caller(1)
	funcName := path.Base(runtime.FuncForPC(pc).Name())

	mapstr := func(m map[string]string) string {
		if m == nil {
			return ""
		}

		var s []string
		for k, v := range m {
			s = append(s, k+": "+v)
		}

		return strings.Join(s, ", ")
	}

	b.logger.WithFields(
		logrus.Fields{
			"asset.Vendor":         b.asset.Vendor,
			"asset.Model":          b.asset.Model,
			"attemptedProviders":   strings.Join(b.client.GetMetadata().ProvidersAttempted, ","),
			"successfulProvider":   b.client.GetMetadata().SuccessfulProvider,
			"successfulOpens":      strings.Join(b.client.GetMetadata().SuccessfulOpenConns, ","),
			"successfulCloses":     strings.Join(b.client.GetMetadata().SuccessfulCloseConns, ","),
			"failedProviderDetail": mapstr(b.client.GetMetadata().FailedProviderDetail),
		}).Trace(funcName + ": connection metadata")
}

func (b *bmc) ReinitializeClient(ctx context.Context) {
	newclient := newBmclibv2Client(ctx, b.asset, b.logger)
	b.client = newclient

	b.logger.WithFields(
		logrus.Fields{
			"provider": b.installProvider,
		},
	).Debug("bmclib client re-initialized")
}

// Open creates a BMC session
func (b *bmc) Open(ctx context.Context) error {
	if b.client == nil {
		return errors.Wrap(errBMCLogin, "bmclib client not initialized")
	}

	defer b.tracelog()

	provider, err := b.provider()
	if err != nil {
		return errors.Wrap(ErrQueryorMethod, err.Error())
	}

	// login to the bmc with retries
	if err := b.loginWithRetries(ctx, loginAttempts, provider); err != nil {
		return err
	}

	// save successful opens, since we want to make sure these are available for further actions.
	if len(b.availableProviders) == 0 {
		b.availableProviders = b.client.GetMetadata().SuccessfulOpenConns
	}

	return nil
}

// Close logs out of the BMC
func (b *bmc) Close(traceCtx context.Context) error {
	if b.client == nil {
		return nil
	}

	ctxClose, cancel := context.WithTimeout(traceCtx, logoutTimeout)
	defer cancel()

	defer b.tracelog()

	if err := b.client.Close(ctxClose); err != nil {
		return errors.Wrap(errBMCLogout, err.Error())
	}

	b.logger.Debug("bmc logout successful")

	return nil
}

// PowerStatus returns the device power status
func (b *bmc) PowerStatus(ctx context.Context) (string, error) {
	if err := b.Open(ctx); err != nil {
		return "", err
	}

	provider, err := b.provider()
	if err != nil {
		return "", errors.Wrap(ErrQueryorMethod, "PowerStatus: "+err.Error())
	}

	defer b.tracelog()
	return b.with(provider).GetPowerState(ctx)
}

// SetPowerState sets the given power state on the device
func (b *bmc) SetPowerState(ctx context.Context, state string) error {
	if err := b.Open(ctx); err != nil {
		return err
	}

	provider, err := b.provider()
	if err != nil {
		return errors.Wrap(ErrQueryorMethod, "SetPowerState: "+err.Error())
	}

	defer b.tracelog()
	_, err = b.with(provider).SetPowerState(ctx, state)

	return err
}

// ResetBMC cold resets the BMC
func (b *bmc) ResetBMC(ctx context.Context) error {
	if err := b.Open(ctx); err != nil {
		return err
	}

	provider, err := b.provider()
	if err != nil {
		return errors.Wrap(ErrQueryorMethod, "ResetBMC: "+err.Error())
	}

	defer b.tracelog()

	// BMCs may or may not return an error when resetting
	// either way we re-initialize the client to make sure
	// we're not re-using old session/cookies.
	defer b.ReinitializeClient(ctx)

	_, err = b.with(provider).ResetBMC(ctx, "GracefulRestart")
	return err
}

// Inventory queries the BMC for the device inventory and returns an object with the device inventory.
func (b *bmc) Inventory(ctx context.Context) (*common.Device, error) {
	if err := b.Open(ctx); err != nil {
		return nil, err
	}

	provider, err := b.provider()
	if err != nil {
		return nil, errors.Wrap(ErrQueryorMethod, "Inventory: "+err.Error())
	}

	inventory, err := b.with(provider).Inventory(ctx)
	if err != nil {
		// increment inventory query error count metric
		if strings.Contains(err.Error(), "no compatible System Odata IDs identified") {
			return nil, errors.Wrap(errBMCInventory, "redfish_incompatible: no compatible System Odata IDs identified")
		}

		return nil, errors.Wrap(errBMCInventory, err.Error())
	}

	// format the device inventory vendor attribute so its consistent
	inventory.Vendor = common.FormatVendorName(inventory.Vendor)

	defer b.tracelog()
	return inventory, nil
}

func (b *bmc) FirmwareInstallSteps(ctx context.Context, component string) (steps []bconsts.FirmwareInstallStep, err error) {
	err = b.Open(ctx)
	if err != nil {
		return nil, err
	}

	defer b.tracelog()
	steps, err = b.client.FirmwareInstallSteps(ctx, component)
	if err != nil {
		return nil, err
	}

	// pin the install provider once its identified
	// this makes sure the subsequent firmware requests are performed using this provider.
	provider := b.client.GetMetadata().SuccessfulProvider

	// Validate we have a provider
	//
	// generally if the FirmwareInstallSteps method returned successfully this should not occur
	if provider == "" || provider == "unknown" {
		return nil, ErrFirmwareInstallProvider
	}

	b.installProvider = provider
	b.logger.WithField("install-provider", b.installProvider).Trace("install provider")

	return steps, nil
}

func (b *bmc) FirmwareInstallUploadAndInitiate(ctx context.Context, component string, file *os.File) (taskID string, err error) {
	err = b.Open(ctx)
	if err != nil {
		return "", err
	}

	provider, err := b.provider()
	if err != nil {
		return "", errors.Wrap(ErrQueryorMethod, "Inventory: "+err.Error())
	}

	installCtx, cancel := context.WithTimeout(ctx, firmwareInstallTimeout)
	defer cancel()

	defer b.tracelog()
	return b.with(provider).FirmwareInstallUploadAndInitiate(installCtx, component, file)
}

// FirmwareTaskStatus looks up the firmware upload/install state and status values
func (b *bmc) FirmwareTaskStatus(ctx context.Context, kind bconsts.FirmwareInstallStep, component, taskID, installVersion string) (state bconsts.TaskState, status string, err error) {
	if err = b.Open(ctx); err != nil {
		return "", "", errors.Wrap(ErrBMCQuery, err.Error())
	}

	provider, err := b.provider()
	if err != nil {
		return "", "", errors.Wrap(ErrQueryorMethod, "FirmwareTaskStatus: "+err.Error())
	}

	defer b.tracelog()
	state, status, err = b.with(provider).FirmwareTaskStatus(ctx, kind, component, taskID, installVersion)

	return state, status, err
}

func (b *bmc) FirmwareUpload(ctx context.Context, component string, file *os.File) (uploadTaskID string, err error) {
	err = b.Open(ctx)
	if err != nil {
		return "", err
	}

	provider, err := b.provider()
	if err != nil {
		return "", errors.Wrap(ErrQueryorMethod, "FirmwareUpload: "+err.Error())
	}

	installCtx, cancel := context.WithTimeout(ctx, firmwareInstallTimeout)
	defer cancel()

	defer b.tracelog()
	return b.with(provider).FirmwareUpload(installCtx, component, file)
}

func (b *bmc) FirmwareInstallUploaded(ctx context.Context, component, uploadVerifyTaskID string) (installTaskID string, err error) {
	err = b.Open(ctx)
	if err != nil {
		return "", err
	}

	provider, err := b.provider()
	if err != nil {
		return "", errors.Wrap(ErrQueryorMethod, "FirmwareInstallUploaded: "+err.Error())
	}

	installCtx, cancel := context.WithTimeout(ctx, firmwareInstallTimeout)
	defer cancel()

	defer b.tracelog()
	return b.with(provider).FirmwareInstallUploaded(installCtx, component, uploadVerifyTaskID)
}
