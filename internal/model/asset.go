package model

import (
	"net"

	"github.com/google/uuid"
)

// Asset holds attributes of a server retrieved from the inventory store.
//
// nolint:govet // fieldalignment struct is easier to read in the current format
type Asset struct {
	ID uuid.UUID

	// Device BMC attributes
	BmcAddress  net.IP
	BmcUsername string
	BmcPassword string

	// Inventory status attribute
	State string

	// Manufacturer attributes
	Vendor string
	Model  string
	Serial string

	// Facility this Asset is hosted in.
	FacilityCode string

	// Device components
	Components Components
}
