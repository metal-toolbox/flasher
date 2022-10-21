package model

import (
	"context"
	"net"
	"sort"
	"strings"

	"github.com/bmc-toolbox/common"
	"github.com/google/uuid"
)

type AppKind string

const (
	AppKindWorker AppKind = "worker"
	AppKindClient AppKind = "client"

	InventorySourceYaml          = "yaml"
	InventorySourceServerservice = "serverservice"

	LogLevelInfo  = 0
	LogLevelDebug = 1
	LogLevelTrace = 2
)

// AppKinds returns the supported flasher app kinds
func AppKinds() []AppKind { return []AppKind{AppKindWorker, AppKindClient} }

// InventorySourceKinds returns the supported asset inventory, firmware configuration sources
func InventorySourceKinds() []string {
	return []string{InventorySourceYaml, InventorySourceServerservice}
}

var (
	// FirmwareInstallOrder defines the order in which firmware is installed.
	FirmwareInstallOrder = map[string]int{
		common.SlugBMC:               0,
		common.SlugBIOS:              1,
		common.SlugCPLD:              2,
		common.SlugDrive:             3,
		common.SlugBackplaneExpander: 4,
		common.SlugStorageController: 5,
		common.SlugNIC:               6,
		common.SlugPSU:               7,
		common.SlugTPM:               8,
		common.SlugGPU:               9,
		common.SlugCPU:               10,
	}
)

type Device struct {
	ID uuid.UUID

	// Device BMC attributes
	BmcAddress  net.IP
	BmcUsername string
	BmcPassword string

	// Inventory status attribute
	State string

	// Manufacturer attributes
	Vendor string
	Model  string
	Serial string
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

// FirmwarePlanned is the list of firmware planned for install
type FirmwarePlanned []Firmware

// Sort the firmware in the order they are expected to be installed
func (p FirmwarePlanned) SortForInstall() {
	sort.Slice(p, func(i, j int) bool {
		slugi := strings.ToUpper(p[i].ComponentSlug)
		slugj := strings.ToUpper(p[j].ComponentSlug)
		return FirmwareInstallOrder[slugi] < FirmwareInstallOrder[slugj]
	})
}

type Component struct {
	Slug              string
	Serial            string
	Vendor            string
	Model             string
	FirmwareInstalled string
}

type Components []Component

// DeviceQueryor interface defines methods to query a device.
//
// This is common interface to the ironlib and bmclib libraries.
type DeviceQueryor interface {
	// Open opens the connection to the device.
	Open(ctx context.Context) error

	// Close closes the connection to the device.
	Close() error

	// SessionActive returns true if a connection is currently active for the device.
	SessionActive(ctx context.Context) bool

	PowerOn(ctx context.Context) (wasOff bool, err error)

	PowerStatus(ctx context.Context) (status string, err error)

	// Inventory returns the device inventory
	Inventory(ctx context.Context) (*common.Device, error)
}
