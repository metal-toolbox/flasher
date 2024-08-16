package model

import (
	"fmt"
	"strconv"

	rctypes "github.com/metal-toolbox/rivets/condition"
	rtypes "github.com/metal-toolbox/rivets/types"
)

const (
	// Each action may be tried upto these many times.
	ActionMaxAttempts = 3
)

// Action holds attributes for each firmware to be installed
type Action struct {
	// ID is a unique identifier for this action
	ID string `json:"id"`

	// The parent task identifier
	TaskID string `json:"task_id"`

	// BMCTaskID is the task identifier to track a BMC job
	// these are returned when the firmware is uploaded and is being verified
	// or an install was initiated on the BMC .
	BMCTaskID string `json:"bmc_task_id,omitempty"`

	// Set to the component identified as the target of the firmware install
	Component *rtypes.Component `json:"component"`

	// Method of install
	InstallMethod InstallMethod `json:"install_method"`

	// status indicates the action state
	State rctypes.State `json:"state"`

	// Firmware to be installed, this is set in the Task Plan phase.
	Firmware Firmware `json:"firmware"`

	// In the remote inband case a list of firmwares will be delegated for install
	Firmwares []Firmware `json:"firmwares"`

	FirmwareInstallStep string `json:"firmware_install_step"`

	// FirmwareTempFile is the temporary file downloaded to be installed.
	//
	// This is declared once the firmware file has been downloaded for install.
	FirmwareTempFile string `json:"firmware_temp_file"`

	// ForceInstall will cause the action to skip checking the currently installed component firmware
	ForceInstall bool `json:"verify_current_firmware"`

	// BMC reset required before install
	BMCResetPreInstall bool `json:"bmc_reset_pre_install"`

	// BMC reset required after install
	BMCResetPostInstall bool `json:"bmc_reset_post_install"`

	// BMC reset required on install failure
	BMCResetOnInstallFailure bool `json:"bmc_reset_on_install_failure"`

	// HostPowerCycled is set when the host has been power cycled for the action.
	HostPowerCycled bool `json:"host_power_cycled"`

	// HostPowerCycleInitiated indicates when a power cycle has been initated for the host.
	HostPowerCycleInitiated bool `json:"host_power_cycle_initiated"`

	// HostPowerOffInitiated indicates a power off was initated on the host.
	HostPowerOffInitiated bool `json:"host_power_off_initiated"`

	// HostPowerOffPreInstall is set when the firmware install provider indicates
	// the host must be powered off before proceeding with the install step.
	HostPowerOffPreInstall bool `json:"host_power_off_pre_install"`

	// First is set to true when its the first action being executed
	First bool `json:"first"`

	// Last is set to true when its the last action being executed
	Last bool `json:"last"`

	// Attempts indicates how many times this action has been tried
	Attempts int `json:"attempts"`

	// Steps identify the smallest unit of work executed by an action
	Steps Steps `json:"steps"`
}

func (a *Action) SetID(taskID, componentSlug string, idx int) {
	a.ID = fmt.Sprintf("%s-%s-%s", taskID, componentSlug, strconv.Itoa(idx))
}

func (a *Action) SetState(state rctypes.State) {
	a.State = state
}

// Actions is a list of actions
type Actions []*Action

// ByID returns the Action matched by the identifier
func (a Actions) ByID(id string) *Action {
	for _, action := range a {
		if action.ID == id {
			return action
		}
	}

	return nil
}

func (a Actions) Prepend(action *Action) Actions {
	a = append([]*Action{action}, a...)
	return a
}
