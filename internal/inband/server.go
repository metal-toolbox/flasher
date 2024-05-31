package inband

import (
	"context"

	"github.com/bmc-toolbox/common"
	"github.com/metal-toolbox/flasher/internal/device"
	"github.com/metal-toolbox/ironlib"
	"github.com/metal-toolbox/ironlib/actions"
	iactions "github.com/metal-toolbox/ironlib/actions"
	imodel "github.com/metal-toolbox/ironlib/model"
	iutils "github.com/metal-toolbox/ironlib/utils"
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
	trace := s.logger.Level == logrus.TraceLevel
	collectors := &iactions.Collectors{
		InventoryCollector: iutils.NewLshwCmd(trace),
		DriveCollectors: []iactions.DriveCollector{
			iutils.NewSmartctlCmd(trace),
		},
		NICCollector: iutils.NewMlxupCmd(trace),
	}

	inventory := iactions.NewInventoryCollectorAction(s.logger, actions.WithCollectors(collectors))
	device := &common.Device{}

	if err := inventory.Collect(ctx, device); err != nil {
		return nil, err
	}

	return device, nil
}

func (s *server) FirmwareInstall(ctx context.Context, component, vendor, model, version, updateFile string, force bool) error {
	dm, err := ironlib.New(s.logger)
	if err != nil {
		return err
	}

	params := &imodel.UpdateOptions{
		AllowDowngrade: force,
		Slug:           component,
		UpdateFile:     updateFile,
		Vendor:         vendor,
		Model:          model,
	}

	dm.InstallUpdates(ctx, params)

	return nil
}
