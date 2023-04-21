package store

import (
	"context"
	"fmt"
	"strings"

	sservice "go.hollow.sh/serverservice/pkg/api/v1"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/metal-toolbox/flasher/internal/app"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/pkg/errors"
)

const (
	// serverservice attribute namespace for device vendor, model, serial attributes
	serverAttributeNSVendor = "sh.hollow.alloy.server_vendor_attributes"

	// serverservice attribute namespace for the BMC address
	serverAttributeNSBmcAddress = "sh.hollow.bmc_info"

	// serverservice attribute namespace for firmware set labels
	firmwareAttributeNSFirmwareSetLabels = "sh.hollow.firmware_set.labels"

	component = "inventory.serverservice"
)

var (
	ErrNoAttributes          = errors.New("no flasher attribute found")
	ErrAttributeList         = errors.New("error in serverservice flasher attribute list")
	ErrAttributeCreate       = errors.New("error in serverservice flasher attribute create")
	ErrAttributeUpdate       = errors.New("error in serverservice flasher attribute update")
	ErrVendorModelAttributes = errors.New("device vendor, model attributes not found in serverservice")
	ErrDeviceStatus          = errors.New("error serverservice device status")

	ErrDeviceID = errors.New("device UUID error")

	// ErrBMCAddress is returned when an error occurs in the BMC address lookup.
	ErrBMCAddress = errors.New("error in server BMC Address")

	// ErrDeviceState is returned when an error occurs in the device state  lookup.
	ErrDeviceState = errors.New("error in device state")

	// ErrServerserviceAttrObj is retuned when an error occurred in unpacking the attribute.
	ErrServerserviceAttrObj = errors.New("serverservice attribute error")

	// ErrServerserviceVersionedAttrObj is retuned when an error occurred in unpacking the versioned attribute.
	ErrServerserviceVersionedAttrObj = errors.New("serverservice versioned attribute error")

	// ErrServerserviceQuery is returned when a server service query fails.
	ErrServerserviceQuery = errors.New("serverservice query returned error")

	ErrFirmwareSetLookup = errors.New("firmware set error")
)

type Serverservice struct {
	config *app.ServerserviceOptions
	// componentSlugs map[string]string
	client *sservice.Client
	logger *logrus.Logger
}

func NewServerserviceStore(ctx context.Context, config *app.ServerserviceOptions, logger *logrus.Logger) (Repository, error) {
	// TODO: add helper method for OIDC auth
	client, err := sservice.NewClientWithToken("fake", config.Endpoint, nil)
	if err != nil {
		return nil, err
	}

	serverservice := &Serverservice{
		client: client,
		config: config,
		logger: logger,
	}

	return serverservice, nil
}

// AssetByID returns an Asset object with various attributes populated.
func (s *Serverservice) AssetByID(ctx context.Context, id string) (*model.Asset, error) {
	deviceUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, errors.Wrap(ErrDeviceID, err.Error()+id)
	}

	asset := &model.Asset{ID: deviceUUID}

	// query credentials
	credential, _, err := s.client.GetCredential(ctx, deviceUUID, sservice.ServerCredentialTypeBMC)
	if err != nil {
		return nil, errors.Wrap(ErrServerserviceQuery, err.Error())
	}

	asset.BmcUsername = credential.Username
	asset.BmcPassword = credential.Password

	// query attributes
	attributes, _, err := s.client.ListAttributes(ctx, deviceUUID, nil)
	if err != nil {
		return nil, errors.Wrap(ErrServerserviceQuery, err.Error())
	}

	// set bmc address
	asset.BmcAddress, err = s.bmcAddressFromAttributes(attributes)
	if err != nil {
		return nil, err
	}

	// set device state attribute
	asset.State, err = s.assetStateAttribute(attributes)
	if err != nil {
		return nil, err
	}

	// set asset vendor attributes
	asset.Vendor, asset.Model, asset.Serial, err = s.vendorModelFromAttributes(attributes)
	if err != nil {
		return nil, errors.Wrap(ErrVendorModelAttributes, err.Error())
	}

	// query asset component inventory
	components, _, err := s.client.GetComponents(ctx, deviceUUID, nil)
	if err != nil {
		return nil, errors.Wrap(ErrServerserviceQuery, "device component query error: "+err.Error())
	}

	// convert from serverservice components to Asset.Components
	asset.Components = s.fromServerserviceComponents(components)

	// set device state attribute
	asset.State, err = s.assetStateAttribute(attributes)
	if err != nil {
		return nil, err
	}

	return asset, nil
}

// FirmwareSetByID returns a list of firmwares part of a firmware set identified by the given id.
func (s *Serverservice) FirmwareSetByID(ctx context.Context, id uuid.UUID) ([]*model.Firmware, error) {
	firmwareset, _, err := s.client.GetServerComponentFirmwareSet(ctx, id)
	if err != nil {
		return nil, errors.Wrap(ErrServerserviceQuery, err.Error())
	}

	return intoFirmwaresSlice(firmwareset.ComponentFirmware), nil
}

// FirmwareByDeviceVendorModel returns the firmware for the device vendor, model.
func (s *Serverservice) FirmwareByDeviceVendorModel(ctx context.Context, deviceVendor, deviceModel string) ([]*model.Firmware, error) {
	// lookup flasher task attribute
	params := &sservice.ComponentFirmwareSetListParams{
		AttributeListParams: []sservice.AttributeListParams{
			{
				Namespace: firmwareAttributeNSFirmwareSetLabels,
				Keys:      []string{"model"},
				Operator:  "eq",
				Value:     deviceModel,
			},
			{
				Namespace: firmwareAttributeNSFirmwareSetLabels,
				Keys:      []string{"vendor"},
				Operator:  "eq",
				Value:     deviceVendor,
			},
		},
	}

	firmwaresets, _, err := s.client.ListServerComponentFirmwareSet(ctx, params)
	if err != nil {
		return nil, errors.Wrap(ErrServerserviceQuery, err.Error())
	}

	if len(firmwaresets) == 0 {
		return nil, errors.Wrap(
			ErrFirmwareSetLookup,
			fmt.Sprintf(
				"lookup by device vendor: %s, model: %s returned no firmware set",
				deviceVendor,
				deviceModel,
			),
		)
	}

	if len(firmwaresets) > 1 {
		return nil, errors.Wrap(
			ErrFirmwareSetLookup,
			fmt.Sprintf(
				"lookup by device vendor: %s, model: %s returned multiple firmware sets, expected one",
				deviceVendor,
				deviceModel,
			),
		)
	}

	if len(firmwaresets[0].ComponentFirmware) == 0 {
		return nil, errors.Wrap(
			ErrFirmwareSetLookup,
			fmt.Sprintf(
				"lookup by device vendor: %s, model: %s returned firmware set with no component firmware",
				deviceVendor,
				deviceModel,
			),
		)
	}

	found := []*model.Firmware{}
	for _, set := range firmwaresets {
		found = append(found, intoFirmwaresSlice(set.ComponentFirmware)...)
	}

	return found, nil
}

func intoFirmwaresSlice(componentFirmware []sservice.ComponentFirmwareVersion) []*model.Firmware {
	strSliceToLower := func(sl []string) []string {
		lowered := make([]string, 0, len(sl))

		for _, s := range sl {
			lowered = append(lowered, strings.ToLower(s))
		}

		return lowered
	}

	firmwares := make([]*model.Firmware, 0, len(componentFirmware))

	for _, firmware := range componentFirmware {
		firmwares = append(firmwares, &model.Firmware{
			ID:        firmware.UUID.String(),
			Vendor:    strings.ToLower(firmware.Vendor),
			Models:    strSliceToLower(firmware.Model),
			FileName:  firmware.Filename,
			Version:   firmware.Version,
			Component: strings.ToLower(firmware.Component),
			Checksum:  firmware.Checksum,
			URL:       firmware.RepositoryURL,
		})
	}

	return firmwares
}
