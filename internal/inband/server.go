package inband

import (
	"context"

	common "github.com/metal-toolbox/bmc-common"
	"github.com/metal-toolbox/flasher/internal/device"
	"github.com/metal-toolbox/ironlib"
	iactions "github.com/metal-toolbox/ironlib/actions"
	ironlibm "github.com/metal-toolbox/ironlib/model"
	iutils "github.com/metal-toolbox/ironlib/utils"
	"github.com/sirupsen/logrus"
)

type server struct {
	logger *logrus.Logger
	dm     iactions.DeviceManager
}

// NewDeviceQueryor returns a server queryor that implements the DeviceQueryor interface
func NewDeviceQueryor(logger *logrus.Entry) device.InbandQueryor {
	return &server{logger: logger.Logger}
}

func (s *server) Inventory(ctx context.Context) (*common.Device, error) {
	dm, err := ironlib.New(s.logger)
	if err != nil {
		return nil, err
	}

	s.dm = dm

	disabledCollectors := []ironlibm.CollectorUtility{
		iutils.UefiFirmwareParserUtility,
		iutils.UefiVariableCollectorUtility,
		iutils.LsblkUtility,
	}

	return dm.GetInventory(ctx, iactions.WithDisabledCollectorUtilities(disabledCollectors))
}

func (s *server) FirmwareInstall(ctx context.Context, component, vendor, model, _, updateFile string, force bool) error {
	params := &ironlibm.UpdateOptions{
		ForceInstall: force,
		Slug:         component,
		UpdateFile:   updateFile,
		Vendor:       vendor,
		Model:        model,
	}

	if s.dm == nil {
		dm, err := ironlib.New(s.logger)
		if err != nil {
			return err
		}

		s.dm = dm
	}

	return s.dm.InstallUpdates(ctx, params)
}

func (s *server) FirmwareInstallRequirements(ctx context.Context, component, vendor, model string) (*ironlibm.UpdateRequirements, error) {
	if s.dm == nil {
		dm, err := ironlib.New(s.logger)
		if err != nil {
			return nil, err
		}

		s.dm = dm
	}

	return s.dm.UpdateRequirements(ctx, component, vendor, model)
}
