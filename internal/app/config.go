package app

import (
	"net/url"
	"os"
	"strings"
	"time"

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

	// ServerserviceOptions defines the serverservice client configuration parameters
	//
	// This parameter is required when StoreKind is set to serverservice.
	ServerserviceOptions *ServerserviceOptions `mapstructure:"serverservice"`
}

// ServerserviceOptions defines configuration for the Serverservice client.
// https://github.com/metal-toolbox/hollow-serverservice
type ServerserviceOptions struct {
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
	a.Config.ServerserviceOptions = &ServerserviceOptions{}

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

func (a *App) NatsParams() (nurl, credsFile string, connectTimeout time.Duration, err error) {
	if a.v.GetString("nats.url") != "" {
		nurl = a.v.GetString("nats.url")
	}

	if nurl == "" {
		return "", "", 0, errors.New("missing parameter: nats.url")
	}

	if a.v.GetString("nats.creds.file") != "" {
		credsFile = a.v.GetString("nats.creds.file")
	}

	if credsFile == "" {
		return "", "", 0, errors.New("missing parameter: nats.creds.file")
	}

	connectTimeout = defaultNatsConnectTimeout
	if a.v.GetDuration("nats.connect.timeout") != 0 {
		connectTimeout = a.v.GetDuration("nats.connect.timeout")
	}

	return nurl, credsFile, connectTimeout, nil
}

// Server service configuration options

// nolint:gocyclo // parameter validation is cyclomatic
func (a *App) envVarServerserviceOverrides() error {
	if a.Config.ServerserviceOptions == nil {
		a.Config.ServerserviceOptions = &ServerserviceOptions{}
	}

	if a.v.GetString("serverservice.endpoint") != "" {
		a.Config.ServerserviceOptions.Endpoint = a.v.GetString("serverservice.endpoint")
	}

	endpointURL, err := url.Parse(a.Config.ServerserviceOptions.Endpoint)
	if err != nil {
		return errors.New("serverservice endpoint URL error: " + err.Error())
	}

	a.Config.ServerserviceOptions.EndpointURL = endpointURL

	if a.v.GetString("serverservice.disable.oauth") != "" {
		a.Config.ServerserviceOptions.DisableOAuth = a.v.GetBool("serverservice.disable.oauth")
	}

	if a.Config.ServerserviceOptions.DisableOAuth {
		return nil
	}

	if a.v.GetString("serverservice.oidc.issuer.endpoint") != "" {
		a.Config.ServerserviceOptions.OidcIssuerEndpoint = a.v.GetString("serverservice.oidc.issuer.endpoint")
	}

	if a.Config.ServerserviceOptions.OidcIssuerEndpoint == "" {
		return errors.New("serverservice oidc.issuer.endpoint not defined")
	}

	if a.v.GetString("serverservice.oidc.audience.endpoint") != "" {
		a.Config.ServerserviceOptions.OidcAudienceEndpoint = a.v.GetString("serverservice.oidc.audience.endpoint")
	}

	if a.Config.ServerserviceOptions.OidcAudienceEndpoint == "" {
		return errors.New("serverservice oidc.audience.endpoint not defined")
	}

	if a.v.GetString("serverservice.oidc.client.secret") != "" {
		a.Config.ServerserviceOptions.OidcClientSecret = a.v.GetString("serverservice.oidc.client.secret")
	}

	if a.Config.ServerserviceOptions.OidcClientSecret == "" {
		return errors.New("serverservice.oidc.client.secret not defined")
	}

	if a.v.GetString("serverservice.oidc.client.id") != "" {
		a.Config.ServerserviceOptions.OidcClientID = a.v.GetString("serverservice.oidc.client.id")
	}

	if a.Config.ServerserviceOptions.OidcClientID == "" {
		return errors.New("serverservice.oidc.client.id not defined")
	}

	if a.v.GetString("serverservice.oidc.client.scopes") != "" {
		a.Config.ServerserviceOptions.OidcClientScopes = a.v.GetStringSlice("serverservice.oidc.client.scopes")
	}

	if len(a.Config.ServerserviceOptions.OidcClientScopes) == 0 {
		return errors.New("serverservice oidc.client.scopes not defined")
	}

	return nil
}
