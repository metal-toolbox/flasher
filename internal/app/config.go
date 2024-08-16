package app

import (
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jeremywohl/flatten"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

const (
	WorkerConcurrency         = 1
	defaultNatsConnectTimeout = 60 * time.Second
)

var (
	ErrConfig = errors.New("configuration error")
)

// Config holds application configuration read from a YAML or set by env variables.
//
// nolint:govet // prefer readability over field alignment optimization for this case.
type Configuration struct {
	// LogLevel is the app verbose logging level.
	// one of - info, debug, trace
	LogLevel string `mapstructure:"log_level"`

	// AppKind is the application kind - worker / client
	AppKind model.AppKind `mapstructure:"app_kind"`

	// Worker configuration
	Concurrency int `mapstructure:"concurrency"`

	// FacilityCode limits this flasher to events in a facility.
	FacilityCode string `mapstructure:"facility_code"`

	// The inventory source - one of serverservice OR Yaml
	InventorySource string `mapstructure:"inventory_source"`

	StoreKind model.StoreKind `mapstructure:"store_kind"`

	// FleetDBAPIOptions defines the serverservice client configuration parameters
	//
	// This parameter is required when StoreKind is set to serverservice.
	FleetDBAPIOptions *FleetDBAPIOptions `mapstructure:"serverservice"`

	// ServerID parameter required for inband run mode
	ServerID string `mapstructure:"serverid"`

	// OrchestratorAPIParams required for inband run mode
	OrchestratorAPIParams *OrchestratorAPIParams `mapstructure:"orchestrator_api"`
}

// FleetDBAPIOptions defines configuration for the FleetDBAPI client.
// https://github.com/metal-toolbox/hollow-serverservice
type FleetDBAPIOptions struct {
	EndpointURL            *url.URL
	FacilityCode           string   `mapstructure:"facility_code"`
	Endpoint               string   `mapstructure:"endpoint"`
	OidcIssuerEndpoint     string   `mapstructure:"oidc_issuer_endpoint"`
	OidcAudienceEndpoint   string   `mapstructure:"oidc_audience_endpoint"`
	OidcClientSecret       string   `mapstructure:"oidc_client_secret"`
	OidcClientID           string   `mapstructure:"oidc_client_id"`
	OutofbandFirmwareNS    string   `mapstructure:"outofband_firmware_ns"`
	AssetStateAttributeNS  string   `mapstructure:"device_state_attribute_ns"`
	AssetStateAttributeKey string   `mapstructure:"device_state_attribute_key"`
	OidcClientScopes       []string `mapstructure:"oidc_client_scopes"`
	DeviceStates           []string `mapstructure:"device_states"`
	DisableOAuth           bool     `mapstructure:"disable_oauth"`
}

type InbandRunParams struct {
}

type OrchestratorAPIParams struct {
	OidcIssuerEndpoint   string   `mapstructure:"oidc_issuer_endpoint"`
	OidcAudienceEndpoint string   `mapstructure:"oidc_audience_endpoint"`
	OidcClientSecret     string   `mapstructure:"oidc_client_secret"`
	OidcClientID         string   `mapstructure:"oidc_client_id"`
	OidcClientScopes     []string `mapstructure:"oidc_client_scopes"`
	Endpoint             string   `mapstructure:"endpoint"`
	AuthDisabled         bool     `mapstructure:"disable_oauth"`
	AuthToken            string
}

// LoadConfiguration loads application configuration
//
// Reads in the cfgFile when available and overrides from environment variables.
func (a *App) LoadConfiguration(cfgFile string, storeKind model.StoreKind) error {
	a.v.SetConfigType("yaml")
	a.v.SetEnvPrefix(model.AppName)
	a.v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	a.v.AutomaticEnv()

	// these are initialized here so viper can read in configuration from env vars
	// once https://github.com/spf13/viper/pull/1429 is merged, this can go.
	a.Config.FleetDBAPIOptions = &FleetDBAPIOptions{}

	if cfgFile != "" {
		fh, err := os.Open(cfgFile)
		if err != nil {
			return errors.Wrap(ErrConfig, err.Error())
		}

		if err = a.v.ReadConfig(fh); err != nil {
			return errors.Wrap(ErrConfig, "ReadConfig error:"+err.Error())
		}
	}

	a.v.SetDefault("log.level", "info")

	if err := a.envBindVars(); err != nil {
		return errors.Wrap(ErrConfig, "env var bind error:"+err.Error())
	}

	if err := a.v.Unmarshal(a.Config); err != nil {
		return errors.Wrap(ErrConfig, "Unmarshal error: "+err.Error())
	}

	a.envVarAppOverrides()

	if storeKind == model.InventoryStoreServerservice {
		if err := a.envVarServerserviceOverrides(); err != nil {
			return errors.Wrap(ErrConfig, "serverservice env overrides error:"+err.Error())
		}
	}

	if a.Config.Concurrency == 0 {
		a.Config.Concurrency = WorkerConcurrency
	}

	if a.Mode == model.RunInband {
		if err := a.inbandInstallParams(); err != nil {
			return errors.Wrap(ErrConfig, err.Error())
		}
	}

	return nil
}

func (a *App) envVarAppOverrides() {
	if a.v.GetString("log.level") != "" {
		a.Config.LogLevel = a.v.GetString("log.level")
	}
}

// envBindVars binds environment variables to the struct
// without a configuration file being unmarshalled,
// this is a workaround for a viper bug,
//
// This can be replaced by the solution in https://github.com/spf13/viper/pull/1429
// once that PR is merged.
func (a *App) envBindVars() error {
	envKeysMap := map[string]interface{}{}
	if err := mapstructure.Decode(a.Config, &envKeysMap); err != nil {
		return err
	}

	// Flatten nested conf map
	flat, err := flatten.Flatten(envKeysMap, "", flatten.DotStyle)
	if err != nil {
		return errors.Wrap(err, "Unable to flatten config")
	}

	for k := range flat {
		if err := a.v.BindEnv(k); err != nil {
			return errors.Wrap(ErrConfig, "env var bind error: "+err.Error())
		}
	}

	return nil
}

type NatsConfig struct {
	NatsURL        string
	CredsFile      string
	KVReplicas     int
	ConnectTimeout time.Duration
}

func (a *App) NatsParams() (NatsConfig, error) {
	cfg := NatsConfig{
		ConnectTimeout: defaultNatsConnectTimeout,
	}

	if a.v.GetString("nats.url") != "" {
		cfg.NatsURL = a.v.GetString("nats.url")
	} else {
		return NatsConfig{}, errors.New("missing parameter: nats.url")
	}

	if a.v.GetString("nats.creds.file") != "" {
		cfg.CredsFile = a.v.GetString("nats.creds.file")
	} else {
		return NatsConfig{}, errors.New("missing parameter: nats.creds.file")
	}

	if a.v.GetDuration("nats.connect.timeout") != 0 {
		cfg.ConnectTimeout = a.v.GetDuration("nats.connect.timeout")
	}

	if a.v.GetInt("nats.kv.replicas") != 0 {
		cfg.KVReplicas = a.v.GetInt("nats.kv.replicas")
	}

	return cfg, nil
}

func (a *App) inbandInstallParams() error {
	errInbandParam := errors.New("inband parameter error")

	// load serverID param from env if not defined in configuration
	if a.Config.ServerID == "" {
		id := a.v.GetString("serverid")
		serverID, err := uuid.Parse(id)
		if err != nil {
			return errors.Wrap(errInbandParam, "serverid parameter invalid: "+err.Error())
		}

		a.Config.ServerID = serverID.String()
	}

	// orchestrator client env override params
	if err := a.envVarOrchestratorAPIOverrides(); err != nil {
		return errors.Wrap(errInbandParam, err.Error())
	}

	return nil
}

func (a *App) envVarOrchestratorAPIOverrides() error {
	if a.Config.OrchestratorAPIParams == nil {
		a.Config.OrchestratorAPIParams = &OrchestratorAPIParams{}
	}

	cfg := a.Config.OrchestratorAPIParams
	if a.v.GetString("orchestrator.api.endpoint") != "" {
		cfg.Endpoint = a.v.GetString("orchestrator.api.endpoint")
	} else {
		return errors.New("missing parameter: orchestrator.api.endpoint")
	}

	cfg.AuthDisabled = a.v.GetBool("orchestrator.api.disable.oauth")
	if cfg.AuthDisabled {
		return nil
	}

	if a.v.GetString("orchestrator.api.authtoken") != "" {
		cfg.AuthToken = a.v.GetString("orchestrator.api.authtoken")
	} else {
		return errors.New("missing parameter: orchestrator.api.authtoken")
	}

	if a.v.GetString("orchestrator.api.oidc.issuer.endpoint") != "" {
		cfg.OidcIssuerEndpoint = a.v.GetString("orchestrator.api.oidc.issuer.endpoint")
	}

	if cfg.OidcIssuerEndpoint == "" {
		return errors.New("orchestrator api oidc.issuer.endpoint not defined")
	}

	if a.v.GetString("orchestrator.api.oidc.audience.endpoint") != "" {
		cfg.OidcAudienceEndpoint = a.v.GetString("orchestrator.api.oidc.audience.endpoint")
	}

	if cfg.OidcAudienceEndpoint == "" {
		return errors.New("orchestrator api oidc.audience.endpoint not defined")
	}

	if a.v.GetString("orchestrator.api.oidc.client.secret") != "" {
		cfg.OidcClientSecret = a.v.GetString("orchestrator.api.oidc.client.secret")
	}

	if cfg.OidcClientSecret == "" {
		return errors.New("orchestrator.api.oidc.client.secret not defined")
	}

	if a.v.GetString("orchestrator.api.oidc.client.id") != "" {
		cfg.OidcClientID = a.v.GetString("orchestrator.api.oidc.client.id")
	}

	if cfg.OidcClientID == "" {
		return errors.New("orchestrator.api.oidc.client.id not defined")
	}

	if a.v.GetString("orchestrator.api.oidc.client.scopes") != "" {
		cfg.OidcClientScopes = a.v.GetStringSlice("orchestrator.api.oidc.client.scopes")
	}

	if len(cfg.OidcClientScopes) == 0 {
		return errors.New("orchestrator api oidc.client.scopes not defined")
	}

	return nil
}

// Server service configuration options

// nolint:gocyclo // parameter validation is cyclomatic
func (a *App) envVarServerserviceOverrides() error {
	if a.Config.FleetDBAPIOptions == nil {
		a.Config.FleetDBAPIOptions = &FleetDBAPIOptions{}
	}

	if a.v.GetString("serverservice.endpoint") != "" {
		a.Config.FleetDBAPIOptions.Endpoint = a.v.GetString("serverservice.endpoint")
	}

	endpointURL, err := url.Parse(a.Config.FleetDBAPIOptions.Endpoint)
	if err != nil {
		return errors.New("serverservice endpoint URL error: " + err.Error())
	}

	a.Config.FleetDBAPIOptions.EndpointURL = endpointURL

	if a.v.GetString("serverservice.disable.oauth") != "" {
		a.Config.FleetDBAPIOptions.DisableOAuth = a.v.GetBool("serverservice.disable.oauth")
	}

	if a.Config.FleetDBAPIOptions.DisableOAuth {
		return nil
	}

	if a.v.GetString("serverservice.oidc.issuer.endpoint") != "" {
		a.Config.FleetDBAPIOptions.OidcIssuerEndpoint = a.v.GetString("serverservice.oidc.issuer.endpoint")
	}

	if a.Config.FleetDBAPIOptions.OidcIssuerEndpoint == "" {
		return errors.New("serverservice oidc.issuer.endpoint not defined")
	}

	if a.v.GetString("serverservice.oidc.audience.endpoint") != "" {
		a.Config.FleetDBAPIOptions.OidcAudienceEndpoint = a.v.GetString("serverservice.oidc.audience.endpoint")
	}

	if a.Config.FleetDBAPIOptions.OidcAudienceEndpoint == "" {
		return errors.New("serverservice oidc.audience.endpoint not defined")
	}

	if a.v.GetString("serverservice.oidc.client.secret") != "" {
		a.Config.FleetDBAPIOptions.OidcClientSecret = a.v.GetString("serverservice.oidc.client.secret")
	}

	if a.Config.FleetDBAPIOptions.OidcClientSecret == "" {
		return errors.New("serverservice.oidc.client.secret not defined")
	}

	if a.v.GetString("serverservice.oidc.client.id") != "" {
		a.Config.FleetDBAPIOptions.OidcClientID = a.v.GetString("serverservice.oidc.client.id")
	}

	if a.Config.FleetDBAPIOptions.OidcClientID == "" {
		return errors.New("serverservice.oidc.client.id not defined")
	}

	if a.v.GetString("serverservice.oidc.client.scopes") != "" {
		a.Config.FleetDBAPIOptions.OidcClientScopes = a.v.GetStringSlice("serverservice.oidc.client.scopes")
	}

	if len(a.Config.FleetDBAPIOptions.OidcClientScopes) == 0 {
		return errors.New("serverservice oidc.client.scopes not defined")
	}

	return nil
}
