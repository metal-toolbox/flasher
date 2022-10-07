package model

import (
	"net"

	"github.com/google/uuid"
)

const (
	AppKindOutofband = "outofband"
	AppKindInband    = "inband"

	InventorySourceYaml          = "Yaml"
	InventorySourceServerservice = "serverservice"

	LogLevelInfo  = 0
	LogLevelDebug = 1
	LogLevelTrace = 2
)

// AppKinds returns the supported flasher app kinds
func AppKinds() []string { return []string{AppKindInband, AppKindOutofband} }

// InventorySourceKinds returns the supported asset inventory, firmware configuration sources
func InventorySourceKinds() []string {
	return []string{InventorySourceYaml, InventorySourceServerservice}
}

type Device struct {
	ID uuid.UUID

	// Device BMC attributes
	BmcAddress  net.IP
	BmcUsername string
	BmcPassword string

	// Manufacturer attributes
	Vendor string
	Model  string
}

type DeviceFwInstallAttribute struct {
	Method   string `json:"method"`
	Status   string `json:"status"`
	Worker   string `json:"worker"`
	User     string `json:"user"`
	Priority int    `json:"priority"`
}

// Firmware includes a firmware version attributes and is part of FirmwareConfig
type Firmware struct {
	Version       string `yaml:"version"`
	URL           string `yaml:"URL"`
	FileName      string `yaml:"filename"`
	Utility       string `yaml:"utility"`
	Model         string `yaml:"model"`
	Vendor        string `yaml:"vendor"`
	ComponentSlug string `yaml:"componentslug"`
	Checksum      string `yaml:"checksum"`
}
