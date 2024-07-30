package store

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	fleetdbapi "github.com/metal-toolbox/fleetdb/pkg/api/v1"
	rfleetdb "github.com/metal-toolbox/rivets/fleetdb"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"golang.org/x/oauth2/clientcredentials"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/metal-toolbox/flasher/internal/app"
	"github.com/metal-toolbox/flasher/internal/metrics"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/pkg/errors"
)

const (
	// connectionTimeout is the maximum amount of time spent on each http connection to fleetdb API.
	connectionTimeout = 30 * time.Second

	pkgName = "internal/store"
)

var (
	ErrNoAttributes          = errors.New("no flasher attribute found")
	ErrAttributeList         = errors.New("error in fleetdb API attribute list")
	ErrAttributeCreate       = errors.New("error in fleetdb API attribute create")
	ErrAttributeUpdate       = errors.New("error in fleetdb API attribute update")
	ErrVendorModelAttributes = errors.New("device vendor, model attributes not found in fleetdb API")
	ErrDeviceStatus          = errors.New("error fleetdb API device status")

	ErrDeviceID = errors.New("device UUID error")

	// ErrBMCAddress is returned when an error occurs in the BMC address lookup.
	ErrBMCAddress = errors.New("error in server BMC Address")

	// ErrDeviceState is returned when an error occurs in the device state  lookup.
	ErrDeviceState = errors.New("error in device state")

	// ErrServerserviceAttrObj is retuned when an error occurred in unpacking the attribute.
	ErrServerserviceAttrObj = errors.New("fleetdb API attribute error")

	// ErrServerserviceVersionedAttrObj is retuned when an error occurred in unpacking the versioned attribute.
	ErrServerserviceVersionedAttrObj = errors.New("fleetdb API versioned attribute error")

	// ErrServerserviceQuery is returned when a server service query fails.
	ErrServerserviceQuery = errors.New("fleetdb API query returned error")

	ErrFirmwareSetLookup = errors.New("firmware set error")
)

type FleetDBAPI struct {
	config *app.FleetDBAPIOptions
	// componentSlugs map[string]string
	client *fleetdbapi.Client
	logger *logrus.Logger
}

func NewServerserviceStore(ctx context.Context, config *app.FleetDBAPIOptions, logger *logrus.Logger) (Repository, error) {
	var client *fleetdbapi.Client
	var err error

	if !config.DisableOAuth {
		client, err = newClientWithOAuth(ctx, config, logger)
		if err != nil {
			return nil, err
		}
	} else {
		client, err = fleetdbapi.NewClientWithToken("fake", config.Endpoint, nil)
		if err != nil {
			return nil, err
		}
	}

	return &FleetDBAPI{
		client: client,
		config: config,
		logger: logger,
	}, nil
}

// returns a fleetdb API retryable http client with Otel and Oauth wrapped in
func newClientWithOAuth(ctx context.Context, cfg *app.FleetDBAPIOptions, logger *logrus.Logger) (*fleetdbapi.Client, error) {
	// init retryable http client
	retryableClient := retryablehttp.NewClient()

	// set retryable HTTP client to be the otel http client to collect telemetry
	retryableClient.HTTPClient = otelhttp.DefaultClient

	// disable default debug logging on the retryable client
	if logger.Level < logrus.DebugLevel {
		retryableClient.Logger = nil
	} else {
		retryableClient.Logger = logger
	}

	// setup oidc provider
	provider, err := oidc.NewProvider(ctx, cfg.OidcIssuerEndpoint)
	if err != nil {
		return nil, err
	}

	clientID := "flasher"

	if cfg.OidcClientID != "" {
		clientID = cfg.OidcClientID
	}

	// setup oauth configuration
	oauthConfig := clientcredentials.Config{
		ClientID:       clientID,
		ClientSecret:   cfg.OidcClientSecret,
		TokenURL:       provider.Endpoint().TokenURL,
		Scopes:         cfg.OidcClientScopes,
		EndpointParams: url.Values{"audience": []string{cfg.OidcAudienceEndpoint}},
	}

	// wrap OAuth transport, cookie jar in the retryable client
	oAuthclient := oauthConfig.Client(ctx)

	retryableClient.HTTPClient.Transport = oAuthclient.Transport
	retryableClient.HTTPClient.Jar = oAuthclient.Jar

	httpClient := retryableClient.StandardClient()
	httpClient.Timeout = connectionTimeout

	return fleetdbapi.NewClientWithToken(
		cfg.OidcClientSecret,
		cfg.Endpoint,
		httpClient,
	)
}

func (s *FleetDBAPI) registerMetric(queryKind string) {
	metrics.StoreQueryErrorCount.With(
		prometheus.Labels{
			"storeKind": "serverservice",
			"queryKind": queryKind,
		},
	).Inc()
}

// AssetByID returns an Asset object with various attributes populated.
func (s *FleetDBAPI) AssetByID(ctx context.Context, id string) (*model.Asset, error) {
	ctx, span := otel.Tracer(pkgName).Start(ctx, "FleetDBAPI.AssetByID")
	defer span.End()

	deviceUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, errors.Wrap(ErrDeviceID, err.Error()+id)
	}

	asset := &model.Asset{ID: deviceUUID}

	// query credentials
	credential, _, err := s.client.GetCredential(ctx, deviceUUID, fleetdbapi.ServerCredentialTypeBMC)
	if err != nil {
		s.registerMetric("GetCredential")

		return nil, errors.Wrap(ErrServerserviceQuery, "GetCredential: "+err.Error())
	}

	asset.BmcUsername = credential.Username
	asset.BmcPassword = credential.Password

	// query the server object
	srv, _, err := s.client.Get(ctx, deviceUUID)
	if err != nil {
		s.registerMetric("GetServer")

		return nil, errors.Wrap(ErrServerserviceQuery, "GetServer: "+err.Error())
	}

	asset.FacilityCode = srv.FacilityCode

	// set bmc address
	asset.BmcAddress, err = s.bmcAddressFromAttributes(srv.Attributes)
	if err != nil {
		return nil, err
	}

	// set device state attribute
	asset.State, err = s.assetStateAttribute(srv.Attributes)
	if err != nil {
		return nil, err
	}

	// set asset vendor attributes
	asset.Vendor, asset.Model, asset.Serial, err = s.vendorModelFromAttributes(srv.Attributes)
	if err != nil {
		s.logger.WithError(err).Warn(ErrVendorModelAttributes)
	}

	// query asset component inventory -- the default on the server object do not
	// include all required information
	components, _, err := s.client.GetComponents(ctx, deviceUUID, nil)
	if err != nil {
		s.registerMetric("GetComponents")

		s.logger.WithError(err).Warn(errors.Wrap(ErrServerserviceQuery, "component information query failed"))

		return asset, nil
	}

	// convert from fleetdb API components to Asset.Components
	asset.Components = s.fromServerserviceComponents(components)

	return asset, nil
}

// FirmwareSetByID returns a list of firmwares part of a firmware set identified by the given id.
func (s *FleetDBAPI) FirmwareSetByID(ctx context.Context, id uuid.UUID) ([]*model.Firmware, error) {
	ctx, span := otel.Tracer(pkgName).Start(ctx, "FleetDBAPI.FirmwareSetByID")
	defer span.End()

	firmwareset, _, err := s.client.GetServerComponentFirmwareSet(ctx, id)
	if err != nil {
		s.registerMetric("GetFirmwareSet")

		return nil, errors.Wrap(ErrServerserviceQuery, "GetFirmwareSet: "+err.Error())
	}

	return intoFirmwaresSlice(firmwareset.ComponentFirmware), nil
}

// FirmwareByDeviceVendorModel returns the firmware for the device vendor, model.
func (s *FleetDBAPI) FirmwareByDeviceVendorModel(ctx context.Context, deviceVendor, deviceModel string) ([]*model.Firmware, error) {
	// lookup flasher task attribute
	params := &fleetdbapi.ComponentFirmwareSetListParams{
		AttributeListParams: []fleetdbapi.AttributeListParams{
			{
				Namespace: rfleetdb.FirmwareSetAttributeNS,
				Keys:      []string{"model"},
				Operator:  "eq",
				Value:     deviceModel,
			},
			{
				Namespace: rfleetdb.FirmwareSetAttributeNS,
				Keys:      []string{"vendor"},
				Operator:  "eq",
				Value:     deviceVendor,
			},
		},
	}

	firmwaresets, _, err := s.client.ListServerComponentFirmwareSet(ctx, params)
	if err != nil {
		return nil, errors.Wrap(ErrServerserviceQuery, err.Error())
	}

	if len(firmwaresets) == 0 {
		return nil, errors.Wrap(
			ErrFirmwareSetLookup,
			fmt.Sprintf(
				"lookup by device vendor: %s, model: %s returned no firmware set",
				deviceVendor,
				deviceModel,
			),
		)
	}

	if len(firmwaresets) > 1 {
		return nil, errors.Wrap(
			ErrFirmwareSetLookup,
			fmt.Sprintf(
				"lookup by device vendor: %s, model: %s returned multiple firmware sets, expected one",
				deviceVendor,
				deviceModel,
			),
		)
	}

	if len(firmwaresets[0].ComponentFirmware) == 0 {
		return nil, errors.Wrap(
			ErrFirmwareSetLookup,
			fmt.Sprintf(
				"lookup by device vendor: %s, model: %s returned firmware set with no component firmware",
				deviceVendor,
				deviceModel,
			),
		)
	}

	found := []*model.Firmware{}

	// nolint:gocritic // rangeValCopy - the data is returned by fleetdb API in this form.
	for _, set := range firmwaresets {
		found = append(found, intoFirmwaresSlice(set.ComponentFirmware)...)
	}

	return found, nil
}

func intoFirmwaresSlice(componentFirmware []fleetdbapi.ComponentFirmwareVersion) []*model.Firmware {
	strSliceToLower := func(sl []string) []string {
		lowered := make([]string, 0, len(sl))

		for _, s := range sl {
			lowered = append(lowered, strings.ToLower(s))
		}

		return lowered
	}

	firmwares := make([]*model.Firmware, 0, len(componentFirmware))

	// nolint:gocritic // rangeValCopy - componentFirmware is returned by fleetdb API in this form.
	for _, firmware := range componentFirmware {
		firmwares = append(firmwares, &model.Firmware{
			ID:        firmware.UUID.String(),
			Vendor:    strings.ToLower(firmware.Vendor),
			Models:    strSliceToLower(firmware.Model),
			FileName:  firmware.Filename,
			Version:   firmware.Version,
			Component: strings.ToLower(firmware.Component),
			Checksum:  firmware.Checksum,
			URL:       firmware.RepositoryURL,
		})
	}

	return firmwares
}
