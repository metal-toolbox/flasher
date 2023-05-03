package model

import (
	"net/url"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

const (
	WorkerConcurrency = 1
)

var (
	ErrConfig = errors.New("configuration error")
)

// Config holds application configuration read from a YAML or set by env variables.
//
// nolint:govet // prefer readability over field alignment optimization for this case.
type Config struct {
	// File is the configuration file path
	File string
	// LogLevel is the app verbose logging level.
	LogLevel int

	// FirmwareURLPrefix is prefixed to the firmware download url
	FirmwareURLPrefix string `mapstructure:"firmware_url_prefix"`

	// AppKind is the application kind - worker / client
	AppKind AppKind `mapstructure:"app_kind"`

	// Worker configuration
	Concurrency int `mapstructure:"concurrency"`

	// The inventory source - one of serverservice OR Yaml
	InventorySource string `mapstructure:"inventory_source"`
	Serverservice   `mapstructure:"serverservice"`
}

// Serverservice is the Hollow server inventory store
// https://github.com/metal-toolbox/hollow-serverservice
type Serverservice struct {
	EndpointURL        *url.URL
	Endpoint           string   `mapstructure:"endpoint"`
	OidcIssuerEndpoint string   `mapstructure:"oidc_issuer_endpoint"`
	OidcAudience       string   `mapstructure:"oidc_audience"`
	OidcClientSecret   string   `mapstructure:"oidc_client_secret"`
	OidcClientID       string   `mapstructure:"oidc_client_id"`
	FacilityCode       string   `mapstructure:"facility_code"`
	OidcClientScopes   []string `mapstructure:"oidc_client_scopes"`
	Concurrency        int      `mapstructure:"concurrency"`
	DisableOAuth       bool     `mapstructure:"disable_oauth"`
	// OutofbandFirmwareNS is the (versioned attribute) namespace in which
	// the installed firmware version data present.
	OutofbandFirmwareNS string `mapstructure:"outofband_firmware_ns"`
	// DeviceStates are the node (device) states that flasher is allowed to acquire a device
	// for firmware install.
	//
	// This DeviceStates are looked up from the server attribute DeviceStateAttributeNS namespace.
	DeviceStates []string `mapstructure:"device_states"`
	// DeviceStateAttributeNS specifies the server attribute namespace to look in
	// for the node (device) state.
	//
	// In the below example, the value for the DeviceStateAttributeNS field is "com.hollow.sh.node"
	// An example of such a server attribute is,
	//
	//  {
	//    "namespace": "com.hollow.sh.node",
	//    "data": {
	//      "state": "in_use",
	//   }
	DeviceStateAttributeNS string `mapstructure:"device_state_attribute_ns"`
	// DeviceStateAttributeKey specifies the DeviceStateAttributeNS namespace data key name
	// to look under for the device state value.
	//
	// In the below example, the value for the DeviceStateAttributeKey field is "state"
	//  {
	//    "namespace": "com.hollow.sh.node",
	//    "data": {
	//      "state": "in_use",
	//   },
	//
	DeviceStateAttributeKey string `mapstructure:"device_state_attribute_key"`
}

func (c *Config) Load(cfgFile string) error {
	if cfgFile != "" {
		c.File = cfgFile
	} else {
		homedir, err := os.UserHomeDir()
		if err != nil {
			return err
		}

		c.File = homedir + "/" + ".flasher.yml"
	}

	h, err := os.Open(c.File)
	if err != nil {
		return err
	}

	viper.SetConfigFile(c.File)

	if errViper := viper.ReadConfig(h); errViper != nil {
		return errors.Wrap(errViper, c.File)
	}

	if err = viper.Unmarshal(c); err != nil {
		return errors.Wrap(err, c.File)
	}

	if c.FirmwareURLPrefix == "" {
		return errors.Wrap(err, "expected a valid FirmwareURLPrefix value")
	}

	if c.Concurrency == 0 {
		c.Concurrency = WorkerConcurrency
	}

	switch c.InventorySource {
	case InventorySourceServerservice:
		return c.validateServerServiceParams()
	case InventorySourceYaml:
		return c.validateYamlParams()
	default:
		return errors.Wrap(ErrConfig, "unknown inventory source: "+c.InventorySource)
	}
}

func (c *Config) validateYamlParams() error {
	return nil
}

// validateServerServiceParams checks required serverservice configuration parameters are present
// and returns the serverservice URL endpoint
//
//nolint:gocyclo // XXX: This is a temporary lint exception
func (c *Config) validateServerServiceParams() error {
	if c.Serverservice.Endpoint == "" {
		return errors.Wrap(ErrConfig, "Serverservice endpoint not defined")
	}

	var err error

	c.Serverservice.EndpointURL, err = url.Parse(c.Serverservice.Endpoint)
	if err != nil {
		return errors.Wrap(ErrConfig, "Serverservice endpoint URL error: "+err.Error())
	}

	if c.AppKind == AppKindWorker {
		if len(c.Serverservice.DeviceStates) == 0 {
			return errors.Wrap(ErrConfig, "worker serverservice config must define DeviceStates")
		}

		if c.Serverservice.DeviceStateAttributeNS == "" {
			return errors.Wrap(ErrConfig, "worker serverservice config must define DeviceStateAttributeNS")
		}

		if c.Serverservice.DeviceStateAttributeKey == "" {
			return errors.Wrap(ErrConfig, "worker serverservice config must define DeviceStateAttributeKey")
		}
	}

	if c.Serverservice.DisableOAuth {
		return nil
	}

	if c.Serverservice.OidcIssuerEndpoint == "" {
		return errors.Wrap(ErrConfig, "OIDC issuer endpoint not defined")
	}

	if c.Serverservice.OidcAudience == "" {
		return errors.Wrap(ErrConfig, "OIDC Audience not defined")
	}

	if c.Serverservice.OutofbandFirmwareNS == "" {
		return errors.Wrap(ErrConfig, "OutofbandFirmwareNS not defined")
	}

	return nil
}
