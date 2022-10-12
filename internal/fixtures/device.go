package fixtures

import (
	"context"
	"net"

	"github.com/bmc-toolbox/common"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/sirupsen/logrus"

	"github.com/google/uuid"
)

var (
	Device1 = uuid.New()
	Device2 = uuid.New()

	Devices = map[string]model.Device{
		Device1.String(): {
			ID:          Device1,
			BmcAddress:  net.ParseIP("127.0.0.1"),
			BmcUsername: "root",
			BmcPassword: "hunter2",
		},

		Device2.String(): {
			ID:          Device2,
			BmcAddress:  net.ParseIP("127.0.0.2"),
			BmcUsername: "root",
			BmcPassword: "hunter2",
		},
	}
)

// bmc wraps the bmclib client and implements the bmcQueryor interface
type mock struct {
	device *model.Device
}

func NewMockDeviceQueryor(ctx context.Context, device *model.Device, logger *logrus.Logger) model.DeviceQueryor {
	return &mock{device: device}
}

// Open creates a BMC session
func (b *mock) Open(ctx context.Context) error {
	return nil
}

// Close logs out of the BMC
func (b *mock) Close() error {
	return nil
}

// Inventory queries the BMC for the device inventory and returns an object with the device inventory.
func (b *mock) Inventory(ctx context.Context) (*common.Device, error) {
	return CopyInventory(R6515A), nil
}
