package model

import (
	"context"
	"net"

	"github.com/bmc-toolbox/common"
	"github.com/google/uuid"
)

const (
	AppKindWorker = "worker"
	AppKindClient = "client"

	InventorySourceYaml          = "Yaml"
	InventorySourceServerservice = "serverservice"

	LogLevelInfo  = 0
	LogLevelDebug = 1
	LogLevelTrace = 2
)

// AppKinds returns the supported flasher app kinds
func AppKinds() []string { return []string{AppKindWorker, AppKindClient} }

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

// DeviceQueryor interface defines methods to query a device.
//
// This is common interface to the ironlib and bmclib libraries.
type DeviceQueryor interface {
	// Open logs into the BMC
	Open(ctx context.Context) error
	// Close logs out of the BMC, note no context is passed to this method
	// to allow it to continue to log out when the parent context has been canceled.
	Close() error
	Inventory(ctx context.Context) (*common.Device, error)
}
