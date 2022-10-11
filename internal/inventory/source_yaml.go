package inventory

import (
	"context"
	"errors"

	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/store"
)

const (
	InventorySourceYAML = "inventorySourceYAML"
)

var (
	ErrYamlSource = errors.New("error in Yaml inventory")
)

// Yaml type implements the inventory interface
type Yaml struct {
	YamlFile     string
	fwConfigFile string
}

// NewYamlInventory returns a Yaml type that implements the inventory interface.
func NewYamlInventory(YamlFile string) (*Yaml, error) {

	return &Yaml{YamlFile: YamlFile}, nil
}

func (c *Yaml) DeviceByID(ctx context.Context, ID string) (*model.Device, error) {

	return nil, nil
}

func (c *Yaml) Firmware(ctx context.Context, device *model.Device) ([]*model.Firmware, error) {

	return nil, nil
}

func (c *Yaml) DevicesForUpdate(_ context.Context, _ store.Storage) error {
	return nil
}
