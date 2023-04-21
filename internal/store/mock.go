package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/metal-toolbox/flasher/internal/fixtures"
	"github.com/metal-toolbox/flasher/internal/model"
)

type Mock struct{}

func NewMockInventory() (Repository, error) {
	return &Mock{}, nil
}

// AssetByID returns device attributes by its identifier
func (s *Mock) AssetByID(ctx context.Context, id string) (*model.Asset, error) {
	return nil, nil
}

// FirmwareSetByID returns a list of firmwares part of a firmware set identified by the given id.
func (s *Mock) FirmwareSetByID(ctx context.Context, id uuid.UUID) ([]*model.Firmware, error) {
	return nil, nil
}

func (s *Mock) FirmwareByDeviceVendorModel(ctx context.Context, deviceVendor, deviceModel string) ([]*model.Firmware, error) {
	return fixtures.Firmware, nil
}
