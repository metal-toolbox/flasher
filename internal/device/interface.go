package device

import (
	"context"
	"os"

	"github.com/bmc-toolbox/common"

	bconsts "github.com/bmc-toolbox/bmclib/v2/constants"
	imodel "github.com/metal-toolbox/ironlib/model"
)

//go:generate mockgen -source model.go -destination=../fixtures/mock.go -package=fixtures

// QueryorOutofband interface defines the out-of-band methods to query a device.
//
// This is common interface to the ironlib and bmclib libraries.
type OutofbandQueryor interface {
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

type InbandQueryor interface {
	// Inventory returns the device inventory
	Inventory(ctx context.Context) (*common.Device, error)
	FirmwareInstall(ctx context.Context, component, vendor, model, version, updateFile string, force bool) error
	FirmwareInstallRequirements(ctx context.Context, component, vendor, model string) (*imodel.UpdateRequirements, error)
}
