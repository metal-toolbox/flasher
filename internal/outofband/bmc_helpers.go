package outofband

import (
	"context"
	"fmt"
	"strings"
	"time"

	bmclibv2 "github.com/bmc-toolbox/bmclib/v2"
	logrusrv2 "github.com/bombsimon/logrusr/v2"
	"github.com/hashicorp/go-multierror"
	"github.com/jpillora/backoff"
	"github.com/pkg/errors"

	"github.com/bmc-toolbox/common"
	"github.com/jacobweinstock/registrar"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/sirupsen/logrus"
)

// newBmclibv2Client initializes a bmclibv2 client with the given credentials
func newBmclibv2Client(ctx context.Context, device *model.Device, l *logrus.Entry) *bmclibv2.Client {
	logger := logrus.New()
	logger.Formatter = l.Logger.Formatter

	// setup a logr logger for bmclib
	// bmclib uses logr, for which the trace logs are logged with log.V(3),
	// this is a hax so the logrusr lib will enable trace logging
	// since any value that is less than (logrus.LogLevel - 4) >= log.V(3) is ignored
	// https://github.com/bombsimon/logrusr/blob/master/logrusr.go#L64
	switch l.Logger.GetLevel() {
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

// login to the BMC, re-trying tries times with exponential backoff
//
// if a session is found to be active,  a bmc query is made to validate the session
// check and the login attempt is ignored.
func (b *bmc) loginWithRetries(ctx context.Context, tries int) error {
	attempts := 1
	delay := &backoff.Backoff{
		Min:    5 * time.Second,
		Max:    30 * time.Second,
		Factor: 2,
		Jitter: true,
	}

	if tries == 0 {
		tries = loginAttempts
	}

	// loop returns when a session was established or after retries attempts
	for {
		attemptstr := fmt.Sprintf("%d/%d", attempts, tries)
		b.logger.WithField("attempt", attemptstr).Trace("bmc login attempt")

		// set login timeout
		timeout, err := time.ParseDuration(loginTimeout)
		if err != nil {
			return errors.Wrap(errBMCLogin, err.Error())
		}

		ctx, cancel := context.WithDeadline(ctx, time.Now().Add(timeout))
		defer cancel()

		// attempt login
		err = b.client.Open(ctx)
		if err != nil {
			b.logger.WithFields(
				logrus.Fields{
					"attempt": attemptstr,
					"err":     err,
				}).Trace("bmc login error")

			// return if attempts match tries
			if attempts >= tries {
				if strings.Contains(err.Error(), "operation timed out") {
					err = multierror.Append(errBMCLoginTimeout, err)
				}

				if strings.Contains(err.Error(), "401: ") || strings.Contains(err.Error(), "failed to login") {
					err = multierror.Append(errBMCLoginUnAuthorized, err)
				}

				return errors.Wrapf(errBMCLogin, "attempts: %s", attemptstr)
			}

			attempts++

			time.Sleep(delay.Duration())

			continue
		}

		b.logger.WithField("attempt", attemptstr).Trace("new bmc session active")
		return nil
	}
}
