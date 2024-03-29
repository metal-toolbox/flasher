package model

import (
	"context"
	"net"
	"os"
	"strings"

	bconsts "github.com/bmc-toolbox/bmclib/v2/constants"
	"github.com/bmc-toolbox/common"
	"github.com/google/uuid"
)

type (
	AppKind   string
	StoreKind string
	// LogLevel is the logging level string.
	LogLevel string
)

const (
	AppName               = "flasher"
	AppKindWorker AppKind = "worker"
	AppKindCLI    AppKind = "cli"

	InventoryStoreYAML          StoreKind = "yaml"
	InventoryStoreServerservice StoreKind = "serverservice"

	LogLevelInfo  LogLevel = "info"
	LogLevelDebug LogLevel = "debug"
	LogLevelTrace LogLevel = "trace"
)

// AppKinds returns the supported flasher app kinds
func AppKinds() []AppKind { return []AppKind{AppKindWorker} }

// StoreKinds returns the supported asset inventory, firmware configuration sources
func StoreKinds() []StoreKind {
	return []StoreKind{InventoryStoreYAML, InventoryStoreServerservice}
}

// Asset holds attributes of a server retrieved from the inventory store.
//
// nolint:govet // fieldalignment struct is easier to read in the current format
type Asset struct {
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

	// Facility this Asset is hosted in.
	FacilityCode string

	// Device components
	Components Components
}

// Firmware includes a firmware version attributes and is part of FirmwareConfig
//
// nolint:govet // fieldalignment struct is easier to read in the current format
type Firmware struct {
	ID        string   `yaml:"id"`
	Vendor    string   `yaml:"vendor"`
	Models    []string `yaml:"models"`
	FileName  string   `yaml:"filename"`
	Version   string   `yaml:"version"`
	URL       string   `yaml:"URL"`
	Component string   `yaml:"component"`
	Checksum  string   `yaml:"checksum"`
}

var (
	// FirmwareInstallOrder defines the order in which firmware is installed.
	//
	// TODO(joel): fix up bmc-toolbox/common slugs to be of lower case instead of upper
	// nolint:gomnd // component install order number is clear as is.
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

//go:generate mockgen -source model.go -destination=../fixtures/mock.go -package=fixtures

// DeviceQueryor interface defines methods to query a device.
//
// This is common interface to the ironlib and bmclib libraries.
type DeviceQueryor interface {
	// Open opens the connection to the device.
	Open(ctx context.Context) error

	// Close closes the connection to the device.
	Close(ctx context.Context) error

	PowerStatus(ctx context.Context) (status string, err error)

	SetPowerState(ctx context.Context, state string) error

	ResetBMC(ctx context.Context) error

	// Reinitializes the underlying device queryor client to purge old session information.
	ReinitializeClient(ctx context.Context)

	// Inventory returns the device inventory
	Inventory(ctx context.Context) (*common.Device, error)

	FirmwareInstallSteps(ctx context.Context, component string) ([]bconsts.FirmwareInstallStep, error)

	FirmwareUpload(ctx context.Context, component string, reader *os.File) (uploadVerifyTaskID string, err error)

	FirmwareTaskStatus(ctx context.Context, kind bconsts.FirmwareInstallStep, component, taskID, installVersion string) (state bconsts.TaskState, status string, err error)

	FirmwareInstallUploaded(ctx context.Context, component, uploadVerifyTaskID string) (installTaskID string, err error)

	FirmwareInstallUploadAndInitiate(ctx context.Context, component string, file *os.File) (taskID string, err error)
}
