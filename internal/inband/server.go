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
)

type server struct {
	logger *logrus.Logger
}

// NewDeviceQueryor returns a server queryor that implements the DeviceQueryor interface
func NewDeviceQueryor(logger *logrus.Entry) device.InbandQueryor {
	return &server{logger: logger.Logger}
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
	cdevice := &common.Device{}

	if err := inventory.Collect(ctx, cdevice); err != nil {
		return nil, err
	}

	return cdevice, nil
}

func (s *server) FirmwareInstall(ctx context.Context, component, vendor, model, version, updateFile string, force bool) error {

	params := &imodel.UpdateOptions{
		AllowDowngrade: force,
		Slug:           component,
		UpdateFile:     updateFile,
		Vendor:         vendor,
		Model:          model,
	}

	dm, err := ironlib.New(s.logger)
	if err != nil {
		return err
	}

	dm.InstallUpdates(ctx, params)

	return nil
}
