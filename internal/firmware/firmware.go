package firmware

import (
	"context"

	"github.com/metal-toolbox/flasher/internal/fixtures"
	"github.com/metal-toolbox/flasher/internal/inventory"
	"github.com/metal-toolbox/flasher/internal/model"
)

type Planner interface {
	// FromInstalled identifies candidate firmware from the inventory
	// and returns the firmware applicable for the device based on the device components.
	FromInstalled(ctx context.Context, deviceID string) ([]model.Firmware, error)
}

type Plan struct {
	// skipVersionCompare tells the planner to skip comparing installed firmware versions.
	skipVersionCompare bool

	// device vendor string
	deviceVendor string

	// device model string
	deviceModel string

	// inventory to lookup candidate firmware
	inv inventory.Inventory
}

func NewPlanner(skipVersionCompare bool, deviceVendor, deviceModel string) Planner {
	return &Plan{
		skipVersionCompare: skipVersionCompare,
		deviceVendor:       deviceVendor,
		deviceModel:        deviceModel,
	}
}

func (p *Plan) FromInstalled(ctx context.Context, deviceID string) ([]model.Firmware, error) {
	return nil, nil
}

type MockPlan struct{}

func NewMockPlanner() Planner {
	return &MockPlan{}
}

func (m *MockPlan) FromInstalled(ctx context.Context, deviceID string) ([]model.Firmware, error) {
	return fixtures.Firmware, nil
}
