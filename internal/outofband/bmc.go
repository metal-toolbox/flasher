package outofband

import (
	"context"
	"strings"
	"time"

	"github.com/pkg/errors"

	bmclibv2 "github.com/bmc-toolbox/bmclib/v2"
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
)

// bmc wraps the bmclib client and implements the bmcQueryor interface
type bmc struct {
	taskID   string
	deviceID string
	client   *bmclibv2.Client
	logger   *logrus.Entry
}

// NewDeviceQueryor returns a bmc queryor that implements the DeviceQueryor interface
func NewDeviceQueryor(ctx context.Context, device *model.Device, taskID string, logger *logrus.Entry) model.DeviceQueryor {
	return &bmc{
		taskID:   taskID,
		deviceID: device.ID.String(),
		client:   newBmclibv2Client(ctx, device, logger),
		logger:   logger,
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
		b.logger.WithFields(logrus.Fields{
			"taskID":   b.taskID,
			"deviceID": b.deviceID,
		}).Trace("bmc session active, skipped login attempt")

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

// PowerOn powers on the device when its powered off
//
// returns true if the host was powered off and had to be powered on.
func (b *bmc) PowerOn(ctx context.Context) (bool, error) {
	state, err := b.client.GetPowerState(ctx)
	if err != nil {
		return false, err
	}

	// power on device when its powered off
	if strings.Contains(strings.ToLower(state), "off") { // covers states - Off, PoweringOff
		_, err = b.client.SetPowerState(ctx, "on")
		if err != nil {
			return true, err
		}

		return true, nil
	}

	return false, nil
}

// PowerStatus returns the device power status
func (b *bmc) PowerStatus(ctx context.Context) (string, error) {
	status, err := b.client.GetPowerState(ctx)
	if err != nil {
		return "", err
	}

	return status, nil
}

// Inventory queries the BMC for the device inventory and returns an object with the device inventory.
func (b *bmc) Inventory(ctx context.Context) (*common.Device, error) {
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
