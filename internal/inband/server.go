package inband

import (
	"context"

	"github.com/bmc-toolbox/common"
	"github.com/metal-toolbox/flasher/internal/device"
	"github.com/metal-toolbox/ironlib"
	"github.com/metal-toolbox/ironlib/actions"
	ironlibm "github.com/metal-toolbox/ironlib/model"
	ironlibu "github.com/metal-toolbox/ironlib/utils"
	"github.com/sirupsen/logrus"

	rctypes "github.com/metal-toolbox/rivets/condition"
)

type server struct {
	dm     actions.DeviceManager
	logger *logrus.Logger
	asset  *rctypes.Asset
}

// NewDeviceQueryor returns a server queryor that implements the DeviceQueryor interface
func NewDeviceQueryor(asset *rctypes.Asset, logger *logrus.Entry) (device.InbandQueryor, error) {
	l := logger.Logger
	return &server{asset: asset, logger: l}, nil

}

func (s *server) Open(ctx context.Context) error {
	dm, err := ironlib.New(s.logger)
	if err != nil {
		return err
	}

	s.dm = dm

	return nil
}

func (s *server) Inventory(ctx context.Context) (*common.Device, error) {
	dm, err := ironlib.New(s.logger)
	if err != nil {
		return nil, err
	}

	disable := []ironlibm.CollectorUtility{ironlibu.UtilityUefiFirmwareParser}
	return dm.GetInventory(ctx, actions.WithDisabledCollectorUtilities(disable))
}

func (s *server) FirmwareInstall(ctx context.Context, component, version string, force bool) error {
	_ = &ironlibm.UpdateOptions{
		AllowDowngrade: force,
	}

	_, err := ironlib.New(s.logger)
	if err != nil {
		return err
	}

	return nil
}
