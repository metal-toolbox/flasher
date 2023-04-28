package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/metal-toolbox/flasher/internal/model"
)

const (
	InventorySourceYAML = "inventoryStoreYAML"
)

var (
	ErrYamlSource = errors.New("error in Yaml inventory")
)

// Yaml type implements the inventory interface
type Yaml struct {
	YamlFile string
}

// NewYamlInventory returns a Yaml type that implements the inventory interface.
func NewYamlInventory(yamlFile string) (Repository, error) {
	return &Yaml{YamlFile: yamlFile}, nil
}

// AssetByID returns device attributes by its identifier
func (c *Yaml) AssetByID(_ context.Context, _ string) (*model.Asset, error) {
	return nil, nil
}

// FirmwareByDeviceVendorModel returns the firmware for the device vendor, model.
func (c *Yaml) FirmwareByDeviceVendorModel(_ context.Context, _, _ string) ([]*model.Firmware, error) {
	return nil, nil
}

// FirmwareSetByID returns a list of firmwares part of a firmware set identified by the given id.
func (c *Yaml) FirmwareSetByID(_ context.Context, _ uuid.UUID) ([]*model.Firmware, error) {
	return nil, nil
}
