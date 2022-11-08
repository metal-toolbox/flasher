package model

import (
	"context"
	"net"
	"os"
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

	// Device components
	Components Components
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

// SortByInstallOrder sorts the firmwares planned in the order they should be installed
func (p FirmwarePlanned) SortByInstallOrder() {
	sort.Slice(p, func(i, j int) bool {
		slugi := strings.ToLower(p[i].ComponentSlug)
		slugj := strings.ToLower(p[j].ComponentSlug)
		return FirmwareInstallOrder[slugi] < FirmwareInstallOrder[slugj]
	})
}

var (
	// FirmwareInstallOrder defines the order in which firmware is installed.
	//
	// TODO(joel): fix up bmc-toolbox/common slugs to be of lower case instead of upper
	FirmwareInstallOrder = map[string]int{
		strings.ToLower(common.SlugBMC):               0,
		strings.ToLower(common.SlugBIOS):              1,
		strings.ToLower(common.SlugCPLD):              2,
		strings.ToLower(common.SlugDrive):             3,
		strings.ToLower(common.SlugBackplaneExpander): 4,
		strings.ToLower(common.SlugStorageController): 5,
		strings.ToLower(common.SlugNIC):               6,
		strings.ToLower(common.SlugPSU):               7,
		strings.ToLower(common.SlugTPM):               8,
		strings.ToLower(common.SlugGPU):               9,
		strings.ToLower(common.SlugCPU):               10,
	}
)

type Component struct {
	Slug              string
	Serial            string
	Vendor            string
	Model             string
	FirmwareInstalled string
}

type Components []Component


// BySlug returns a component that matches the slug value.
func (c Components) BySlugVendorModel(cSlug, cVendor, cModel string) *Component {
	cModels := []string{cModel}

	// split if model is a list
	if strings.Contains(cModel, ",") {
		cModels = strings.Split(cModel, ",")
	}

	for _, component := range c {
		// skip non matching component slug
		if !strings.EqualFold(cSlug, component.Slug) {
			continue
		}

		// skip non matching component vendor
		if !strings.EqualFold(component.Vendor, cVendor) {
			continue
		}

		// match component model with contains
		for _, findModel := range cModels {
			if strings.Contains(strings.ToLower(component.Model), strings.TrimSpace(findModel)) {
				return &component
			}
		}
	}

	return nil
}

// ComponentFirmwareInstallStatus is the device specific firmware install status returned by the FirmwareInstallStatus method
// in the DeviceQueryor interface.
//
// As an example, the BMCs return various firmware install statuses based on the vendor implementation
// and so these statuses defined reduce all of those differences into a few generic status values
//
// Note: this is not related to the Flasher task status.
type ComponentFirmwareInstallStatus string

var (
	StatusInstallRunning                ComponentFirmwareInstallStatus = "running"
	StatusInstallComplete               ComponentFirmwareInstallStatus = "complete"
	StatusInstallUnknown                ComponentFirmwareInstallStatus = "unknown"
	StatusInstallFailed                 ComponentFirmwareInstallStatus = "failed"
	StatusInstallPowerCycleHostRequired ComponentFirmwareInstallStatus = "powerCycleHostRequired"
	StatusInstallPowerCycleBMCRequired  ComponentFirmwareInstallStatus = "powerCycleBMCRequired"
)

// DeviceQueryor interface defines methods to query a device.
//
// This is common interface to the ironlib and bmclib libraries.
type DeviceQueryor interface {
	// Open opens the connection to the device.
	Open(ctx context.Context) error

	// Close closes the connection to the device.
	Close() error

	PowerStatus(ctx context.Context) (status string, err error)

	SetPowerState(ctx context.Context, state string) error

	ResetBMC(ctx context.Context) error

	// Inventory returns the device inventory
	Inventory(ctx context.Context) (*common.Device, error)

	// FirmwareInstall initiates the firmware install process returning a taskID for the install if any.
	FirmwareInstall(ctx context.Context, componentSlug string, force bool, file *os.File) (taskID string, err error)

	FirmwareInstallStatus(ctx context.Context, installVersion, componentSlug, bmcTaskID string) (ComponentFirmwareInstallStatus, error)
}
