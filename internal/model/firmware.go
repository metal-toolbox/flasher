package model

import (
	"strings"

	common "github.com/metal-toolbox/bmc-common"
)

var (
	// FirmwareInstallOrder defines the order in which firmware is installed.
	//
	// TODO(joel): fix up bmc-toolbox/common slugs to be of lower case instead of upper
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
