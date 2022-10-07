package outofband

import (
	"context"

	"github.com/bmc-toolbox/common"
	"github.com/metal-toolbox/flasher/internal/fixtures"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/sirupsen/logrus"
)

// bmc wraps the bmclib client and implements the bmcQueryor interface
type mockBmc struct {
	device *model.Device
}

func NewBmcMockQueryor(ctx context.Context, device *model.Device, logger *logrus.Logger) bmcQueryor {
	return &mockBmc{device: device}
}

// Open creates a BMC session
func (b *mockBmc) Open(ctx context.Context) error {
	return nil
}

// Close logs out of the BMC
func (b *mockBmc) Close() error {
	return nil
}

// Inventory queries the BMC for the device inventory and returns an object with the device inventory.
func (b *mockBmc) Inventory(ctx context.Context) (*common.Device, error) {
	return fixtures.CopyInventory(fixtures.R6515A), nil
}
