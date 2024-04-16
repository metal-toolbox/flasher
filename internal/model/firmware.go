package model

import (
	"strings"

	"github.com/bmc-toolbox/common"
)

// Firmware includes a firmware version attributes and is part of FirmwareConfig
//
// nolint:govet // fieldalignment struct is easier to read in the current format
type Firmware struct {
	ID            string        `yaml:"id"`
	Vendor        string        `yaml:"vendor"`
	Models        []string      `yaml:"models"`
	FileName      string        `yaml:"filename"`
	Version       string        `yaml:"version"`
	URL           string        `yaml:"URL"`
	Component     string        `yaml:"component"`
	Checksum      string        `yaml:"checksum"`
	InstallMethod InstallMethod `yaml:"install_method"`
}

var (
	// FirmwareInstallOrder defines the order in which firmware is installed.
	//
	// TODO(joel): fix up bmc-toolbox/common slugs to be of lower case instead of upper
	// nolint:gomnd // component install order number is clear as is.
	FirmwareInstallOrder = map[string]int{
		strings.ToLower(common.SlugBMC):               0,
		strings.ToLower(common.SlugBIOS):              1,
		strings.ToLower(common.SlugCPLD):              2,
		strings.ToLower(common.SlugDrive):             3,
		strings.ToLower(common.SlugBackplaneExpander): 4,
		strings.ToLower(common.SlugStorageController): 5,
		strings.ToLower(common.SlugNIC):               6,
		strings.ToLower(common.SlugPSU):               7,
		strings.ToLower(common.SlugTPM):               8,
		strings.ToLower(common.SlugGPU):               9,
		strings.ToLower(common.SlugCPU):               10,
	}
)
