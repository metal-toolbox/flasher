package store

import (
	"context"

	"github.com/google/uuid"
	rctypes "github.com/metal-toolbox/rivets/v2/condition"
	rtypes "github.com/metal-toolbox/rivets/v2/types"
)

type Repository interface {
	// AssetByID returns asset.
	AssetByID(ctx context.Context, id string) (*rtypes.Server, error)

	FirmwareSetByID(ctx context.Context, id uuid.UUID) ([]*rctypes.Firmware, error)

	// FirmwareByDeviceVendorModel returns the firmware for the device vendor, model.
	FirmwareByDeviceVendorModel(ctx context.Context, deviceVendor, deviceModel string) ([]*rctypes.Firmware, error)
}
