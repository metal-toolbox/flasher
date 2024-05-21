package outofband

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"path"
	"runtime"
	"strings"
	"time"

	bmclib "github.com/bmc-toolbox/bmclib/v2"
	"github.com/bmc-toolbox/bmclib/v2/constants"
	bmcliberrs "github.com/bmc-toolbox/bmclib/v2/errors"
	"github.com/bmc-toolbox/bmclib/v2/providers"
	logrusrv2 "github.com/bombsimon/logrusr/v2"
	"github.com/hashicorp/go-multierror"
	"github.com/jpillora/backoff"
	rctypes "github.com/metal-toolbox/rivets/condition"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel"
	"golang.org/x/exp/slices"
	"golang.org/x/net/publicsuffix"

	"github.com/sirupsen/logrus"
)

// NOTE: the constants.FirmwareInstallStep type will be moved to the FirmwareInstallProperties struct type which will make this easier
func hostPowerOffRequired(steps []constants.FirmwareInstallStep) bool {
	return slices.Contains(steps, constants.FirmwareInstallStepPowerOffHost)
}

// NOTE: the constants.FirmwareInstallStep type will be moved to the FirmwareInstallProperties struct type which will make this easier
func bmcResetParams(steps []constants.FirmwareInstallStep) (bmcResetOnInstallFailure, bmcResetPostInstall bool) {
	for _, step := range steps {
		switch step {
		case constants.FirmwareInstallStepResetBMCOnInstallFailure:
			bmcResetOnInstallFailure = true
		case constants.FirmwareInstallStepResetBMCPostInstall:
			bmcResetPostInstall = true
		}
	}

	return bmcResetOnInstallFailure, bmcResetPostInstall
}

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

// newBmclibv2Client initializes a bmclib client with the given credentials
func newBmclibv2Client(_ context.Context, asset *rctypes.Asset, l *logrus.Entry) *bmclib.Client {
	logger := logrus.New()
	if l != nil {
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
	}

	logruslogr := logrusrv2.New(logger)

	bmcClient := bmclib.NewClient(
		asset.BmcAddress.String(),
		asset.BmcUsername,
		asset.BmcPassword,
		bmclib.WithLogger(logruslogr),
		bmclib.WithHTTPClient(newHTTPClient()),
		bmclib.WithPerProviderTimeout(loginTimeout),
		bmclib.WithRedfishEtagMatchDisabled(true),
		bmclib.WithTracerProvider(otel.GetTracerProvider()),
	)

	// include bmclib drivers that support firmware related actions
	bmcClient.Registry.Drivers = bmcClient.Registry.Supports(
		providers.FeatureFirmwareInstallSteps,
	)

	return bmcClient
}

func (b *bmc) sessionActive(ctx context.Context) error {
	if b.client == nil {
		return errors.Wrap(errBMCSession, "bmclib client not initialized")
	}

	// TODO: add a SessionActive method in bmclib since the GetPowerState request
	// will not work on all devices - while they are going through an update.

	// check if we're able to query the power state
	powerStatus, err := b.with(b.installProvider).GetPowerState(ctx)
	if err != nil {
		if errors.Is(err, bmcliberrs.ErrBMCUpdating) {
			return err
		}

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

// with pins the bmclib client to use the given provider - this requires the initial Open()
// to have successfully opened a client connection with given provider.
func (b *bmc) with(provider string) *bmclib.Client {
	pc, _, _, _ := runtime.Caller(1)
	funcName := path.Base(runtime.FuncForPC(pc).Name())

	if !slices.Contains(b.availableProviders, provider) {
		b.logger.WithFields(
			logrus.Fields{
				"vendor":    b.asset.Vendor,
				"model":     b.asset.Model,
				"required":  provider,
				"available": strings.Join(b.availableProviders, ","),
				"caller":    funcName,
			},
		).Trace("required provider not in available list")

		return b.client
	}

	b.logger.WithFields(
		logrus.Fields{
			"vendor":    b.asset.Vendor,
			"model":     b.asset.Model,
			"required":  provider,
			"available": strings.Join(b.availableProviders, ","),
			"caller":    funcName,
		},
	).Info(funcName + ": with bmclib provider")

	return b.client.For(provider)
}

// login to the BMC, re-trying tries times with exponential backoff
//
// if a session is found to be active,  a bmc query is made to validate the session
// check and the login attempt is ignored.
func (b *bmc) loginWithRetries(ctx context.Context, maxAttempts int, provider string) error {
	attempts := 1

	if maxAttempts == 0 {
		maxAttempts = loginAttempts
	}

	// loop returns when a session was established or after retries attempts
	for {
		attemptCtx, cancel := context.WithTimeout(ctx, loginTimeout)
		// nolint:gocritic // deferInLoop - loop is bounded
		defer cancel()

		// if a session is active, skip login attempt
		errSessionActive := b.sessionActive(attemptCtx)
		if errSessionActive == nil {
			return nil
		}

		// Some BMCs disallow any actions when an update is in progress (AMI)
		// in these cases, attempting to check the power status returns a 401.
		if errors.Is(errSessionActive, bmcliberrs.ErrBMCUpdating) {
			b.logger.WithFields(
				logrus.Fields{
					"provider": provider,
					"err":      errSessionActive,
				},
			).Debug("BMC update active, skipping session open attempt")

			return nil
		}

		// attempt login
		errLogin := b.with(provider).Open(attemptCtx)
		if errLogin != nil {
			var errRetry error
			// failed to open connection
			attempts, errRetry = b.retry(ctx, maxAttempts, attempts, errLogin, provider)
			if errRetry != nil {
				return errRetry
			}

			continue
		}

		// when we're in middle of a firmware install, the client will lose connection/session,
		// so now, when the client retries, we want to make sure we have a session
		// with the installProvider that was identified in FirmwareInstallSteps()
		if b.installProviderAvailable() {
			if b.client != nil {
				_ = b.client.Close(attemptCtx)
			}

			var errRetry error
			errFmt := errors.New("required bmclib install provider not available: " + provider)
			attempts, errRetry = b.retry(ctx, maxAttempts, attempts, errFmt, provider)
			if errRetry != nil {
				return errRetry
			}

			continue
		}

		b.logger.WithFields(
			logrus.Fields{
				"provider":        provider,
				"successfulOpens": b.client.GetMetadata().SuccessfulProvider,
			},
		).Debug("bmc login successful")

		return nil
	}
}

// method returns nil if a retry is required and an error when a retry cannot proceed
func (b *bmc) retry(ctx context.Context, maxAttempts, attempts int, cause error, provider string) (int, error) {
	// nolint:gomnd // time duration definitions are clear as is.
	delay := &backoff.Backoff{
		Min:    5 * time.Second,
		Max:    30 * time.Second,
		Factor: 2,
		Jitter: true,
	}

	trystr := fmt.Sprintf("%d/%d", attempts, maxAttempts)
	b.logger.WithFields(
		logrus.Fields{
			"provider":        provider,
			"attempt":         trystr,
			"successfulOpens": b.client.GetMetadata().SuccessfulOpenConns,
			"cause":           cause,
		}).Debug("retrying bmc login")

	// return if attempts match tries
	if attempts >= maxAttempts {
		if strings.Contains(cause.Error(), "operation timed out") {
			cause = multierror.Append(cause, errBMCLoginTimeout)
		}

		if strings.Contains(cause.Error(), "401: ") || strings.Contains(cause.Error(), "failed to login") {
			cause = multierror.Append(cause, errBMCLoginUnAuthorized)
		}

		return 0, errors.Wrapf(errBMCLogin, "attempts: %s, last error: %s", trystr, cause.Error())
	}

	// Reinitialize bmclib client if we've lost our connection
	// to be sure we're not reusing old cookies/sessions.
	//
	// The bmclib client is re-initialized only if it was previously
	// connected successfully with a provider - set as installProvider
	if b.installProvider != "" {
		b.ReinitializeClient(ctx)
	}

	attempts++

	if err := sleepWithContext(ctx, delay.ForAttempt(float64(attempts))); err != nil {
		return 0, err
	}

	return attempts, nil
}

func (b *bmc) installProviderAvailable() bool {
	if b.installProvider == "" {
		return false
	}

	// we're in middle of an install, make sure the install provider has been opened before returning
	return !slices.Contains(
		b.client.GetMetadata().SuccessfulOpenConns,
		b.installProvider,
	)
}

func (b *bmc) provider() (string, error) {
	// the install provider is set after firmware install steps is queried
	if b.installProvider != "" {
		return b.installProvider, nil
	}

	if b.asset.Vendor == "" {
		return "", errors.Wrap(
			ErrFirmwareInstallProvider, "device has no vendor attribute set, and an install provider was not identified")
	}

	return b.asset.Vendor, nil
}
