package outofband

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"

	bmclibv2 "github.com/bmc-toolbox/bmclib/v2"
	logrusrv2 "github.com/bombsimon/logrusr/v2"
	"github.com/hashicorp/go-multierror"
	"github.com/jpillora/backoff"
	"github.com/pkg/errors"
	"golang.org/x/net/publicsuffix"

	"github.com/bmc-toolbox/common"
	"github.com/jacobweinstock/registrar"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/sirupsen/logrus"
)

func newHTTPClient() *http.Client {
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		panic(err)
	}

	// nolint:gomnd // time duration declarations are clear as is.
	return &http.Client{
		Timeout: time.Second * 600,
		Jar:     jar,
		Transport: &http.Transport{
			// nolint:gosec // BMCs don't have valid certs.
			TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
			DisableKeepAlives: true,
			Dial: (&net.Dialer{
				Timeout:   180 * time.Second,
				KeepAlive: 180 * time.Second,
			}).Dial,
			TLSHandshakeTimeout:   180 * time.Second,
			ResponseHeaderTimeout: 600 * time.Second,
			IdleConnTimeout:       180 * time.Second,
		},
	}
}

// newBmclibv2Client initializes a bmclibv2 client with the given credentials
func newBmclibv2Client(_ context.Context, asset *model.Asset, l *logrus.Entry) *bmclibv2.Client {
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
		asset.BmcAddress.String(),
		asset.BmcUsername,
		asset.BmcPassword,
		bmclibv2.WithLogger(logruslogr),
		bmclibv2.WithHTTPClient(newHTTPClient()),
		bmclibv2.WithPerProviderTimeout(loginTimeout),
	)

	// set bmclibv2 driver
	//
	// The bmclib drivers here are limited to the HTTPS means of connection,
	// that is, drivers like ipmi are excluded.
	switch asset.Vendor {
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

func (b *bmc) sessionActive(ctx context.Context) error {
	if b.client == nil {
		return errors.Wrap(errBMCSession, "bmclibv2 client not initialized")
	}

	// check if we're able to query the power state
	powerStatus, err := b.client.GetPowerState(ctx)
	if err != nil {
		b.logger.WithFields(
			logrus.Fields{
				"err": err.Error(),
			},
		).Trace("session not active, checked with GetPowerState()")

		return errors.Wrap(errBMCSession, err.Error())
	}

	b.logger.WithFields(
		logrus.Fields{
			"powerStatus": powerStatus,
		},
	).Trace("session currently active, checked with GetPowerState()")

	return nil
}

// login to the BMC, re-trying tries times with exponential backoff
//
// if a session is found to be active,  a bmc query is made to validate the session
// check and the login attempt is ignored.
func (b *bmc) loginWithRetries(ctx context.Context, tries int) error {
	attempts := 1

	// nolint:gomnd // time duration definitions are clear as is.
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
		attemptCtx, cancel := context.WithTimeout(ctx, loginTimeout)
		// nolint:gocritic // deferInLoop - loop is bounded
		defer cancel()

		// if a session is active, skip login attempt
		if err := b.sessionActive(attemptCtx); err == nil {
			return nil
		}

		// attempt login
		err := b.client.Open(attemptCtx)
		if err != nil {
			b.logger.WithFields(
				logrus.Fields{
					"attempt": attemptstr,
					"err":     err,
				}).Debug("bmc login error")

			// return if attempts match tries
			if attempts >= tries {
				if strings.Contains(err.Error(), "operation timed out") {
					err = multierror.Append(errBMCLoginTimeout, err)
				}

				if strings.Contains(err.Error(), "401: ") || strings.Contains(err.Error(), "failed to login") {
					err = multierror.Append(errBMCLoginUnAuthorized, err)
				}

				return errors.Wrapf(errBMCLogin, "attempts: %s, last error: %s", attemptstr, err.Error())
			}

			attempts++

			time.Sleep(delay.Duration())

			continue
		}

		b.logger.WithField("attempt", attemptstr).Debug("bmc login successful")

		return nil
	}
}
