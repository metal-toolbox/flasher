package bmc

import (
	"context"
	"strings"
	"time"

	bmclibv2 "github.com/bmc-toolbox/bmclib/v2"
	logrusrv2 "github.com/bombsimon/logrusr/v2"
	"github.com/pkg/errors"

	"github.com/bmc-toolbox/common"
	"github.com/jacobweinstock/registrar"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/sirupsen/logrus"
)

var (
	// logoutTimeout is the timeout value when logging out of a bmc
	logoutTimeout = "1m"

	// login errors
	errBMCLogin             = errors.New("bmc login error")
	errBMCLoginTimeout      = errors.New("bmc login timeout")
	errBMCLoginUnAuthorized = errors.New("bmc login unauthorized")

	errBMCInventory = errors.New("bmc inventory error")

	errBMCLogout = errors.New("bmc logout error")
)

// Queryor interface defines BMC query methods
type Queryor interface {
	// Open logs into the BMC
	Open(ctx context.Context) error
	// Close logs out of the BMC, note no context is passed to this method
	// to allow it to continue to log out when the parent context has been cancelled.
	Close() error
	Inventory(ctx context.Context) (*common.Device, error)
}

// bmc wraps the bmclib client and implements the bmcQueryor interface
type bmc struct {
	client *bmclibv2.Client
	logger *logrus.Logger
}

func NewQueryor(ctx context.Context, device *model.Device, logger *logrus.Logger) Queryor {
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
	startTS := time.Now()

	if b.client == nil {
		return errors.Wrap(errBMCLogin, "bmclibv2 client not initialized")
	}

	// initiate bmc login session
	if err := b.client.Open(ctx); err != nil {
		if strings.Contains(err.Error(), "operation timed out") {
			return errors.Wrap(errBMCLoginTimeout, "operation timed out in "+time.Since(startTS).String())
		}

		if strings.Contains(err.Error(), "401: ") || strings.Contains(err.Error(), "FailedState to login") {
			return errors.Wrap(errBMCLoginUnAuthorized, err.Error())
		}

		return errors.Wrap(errBMCLogin, err.Error())
	}

	return nil
}

// Close logs out of the BMC
func (b *bmc) Close() error {
	timeout, err := time.ParseDuration(logoutTimeout)
	if err != nil {
		return errors.Wrap(errBMCLogout, err.Error())
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := b.client.Close(ctx); err != nil {
		return errors.Wrap(errBMCLogout, err.Error())
	}

	return nil
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

// newBmclibv2Client initializes a bmclibv2 client with the given credentials
func newBmclibv2Client(ctx context.Context, device *model.Device, l *logrus.Logger) *bmclibv2.Client {
	logger := logrus.New()
	logger.Formatter = l.Formatter

	// setup a logr logger for bmclib
	// bmclib uses logr, for which the trace logs are logged with log.V(3),
	// this is a hax so the logrusr lib will enable trace logging
	// since any value that is less than (logrus.LogLevel - 4) >= log.V(3) is ignored
	// https://github.com/bombsimon/logrusr/blob/master/logrusr.go#L64
	switch l.GetLevel() {
	case logrus.TraceLevel:
		logger.Level = 7
	case logrus.DebugLevel:
		logger.Level = 5
	}

	logruslogr := logrusrv2.New(logger)

	bmcClient := bmclibv2.NewClient(
		device.BmcAddress.String(),
		"", // port unset
		device.BmcUsername,
		device.BmcPassword,
		bmclibv2.WithLogger(logruslogr),
	)

	// set bmclibv2 driver
	//
	// The bmclib drivers here are limited to the HTTPS means of connection,
	// that is, drivers like ipmi are excluded.
	switch device.Vendor {
	case common.VendorDell, common.VendorHPE:
		// Set to the bmclib ProviderProtocol value
		// https://github.com/bmc-toolbox/bmclib/blob/v2/providers/redfish/redfish.go#L26
		bmcClient.Registry.Drivers = bmcClient.Registry.Using("redfish")
	case common.VendorAsrockrack:
		// https://github.com/bmc-toolbox/bmclib/blob/v2/providers/asrockrack/asrockrack.go#L20
		bmcClient.Registry.Drivers = bmcClient.Registry.Using("vendorapi")
	default:
		// attempt both drivers when vendor is unknown
		drivers := append(registrar.Drivers{},
			bmcClient.Registry.Using("redfish")...,
		)

		drivers = append(drivers,
			bmcClient.Registry.Using("vendorapi")...,
		)

		bmcClient.Registry.Drivers = drivers
	}

	return bmcClient
}
