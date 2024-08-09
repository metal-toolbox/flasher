package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/metal-toolbox/flasher/internal/model"
	rtypes "github.com/metal-toolbox/rivets/types"
)

type Repository interface {
	// AssetByID returns asset.
	AssetByID(ctx context.Context, id string) (*rtypes.Server, error)

	FirmwareSetByID(ctx context.Context, id uuid.UUID) ([]*model.Firmware, error)

	// FirmwareByDeviceVendorModel returns the firmware for the device vendor, model.
	FirmwareByDeviceVendorModel(ctx context.Context, deviceVendor, deviceModel string) ([]*model.Firmware, error)
}
