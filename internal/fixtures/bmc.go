package fixtures

import (
	"context"
	"errors"
	"os"

	"github.com/bmc-toolbox/common"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/sirupsen/logrus"
)

type mockBMC struct {
	hostPowerState string
	taskID         string
	deviceID       string
	logger         *logrus.Entry
}

// NewDeviceQueryor returns a mockBMC queryor that implements the DeviceQueryor interface
func NewDeviceQueryor(ctx context.Context, device *model.Device, taskID string, logger *logrus.Entry) model.DeviceQueryor {
	return &mockBMC{
		hostPowerState: "on",
		taskID:         taskID,
		deviceID:       device.ID.String(),
		logger:         logger,
	}
}

// Open creates a BMC session
func (b *mockBMC) Open(ctx context.Context) error {
	return nil
}

// SessionActive determines if the connection has an active session.
func (b *mockBMC) SessionActive(ctx context.Context) bool {
	return true
}

// Close logs out of the BMC
func (b *mockBMC) Close() error {
	return nil
}

// PowerStatus returns the device power status
func (b *mockBMC) PowerStatus(ctx context.Context) (string, error) {
	return b.hostPowerState, nil
}

// SetPowerState sets the given power state on the device
func (b *mockBMC) SetPowerState(ctx context.Context, state string) error {
	b.hostPowerState = state
	return nil
}

// ResetBMC cold resets the BMC
func (b *mockBMC) ResetBMC(ctx context.Context) error {
	return nil
}

// Inventory queries the BMC for the device inventory and returns an object with the device inventory.
func (b *mockBMC) Inventory(ctx context.Context) (*common.Device, error) {
	return CopyInventory(R6515A), nil
}

func (b *mockBMC) FirmwareInstall(ctx context.Context, componentSlug string, force bool, file *os.File) (bmcTaskID string, err error) {
	return "", nil
}

// FirmwareInstallStatus looks up the firmware install status based on the given installVersion, componentSlug, bmcTaskID parameteres
func (b *mockBMC) FirmwareInstallStatus(ctx context.Context, installVersion, componentSlug, bmcTaskID string) (model.ComponentFirmwareInstallStatus, error) {
	status := os.Getenv("MOCK_FIRMWARE_INSTALL_STATUS")

	var err error
	if status == "" {
		err = errors.New("env var MOCK_FIRMWARE_INSTALL_STATUS not defined")
	}

	return model.StatusInstallUnknown, err
}
