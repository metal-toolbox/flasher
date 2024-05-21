package outofband

import (
	"context"
	"testing"

	bconsts "github.com/bmc-toolbox/bmclib/v2/constants"
	"github.com/metal-toolbox/flasher/internal/device"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/runner"
	rctypes "github.com/metal-toolbox/rivets/condition"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestComposeAction(t *testing.T) {
	newTestActionCtx := func() *runner.ActionHandlerContext {
		return &runner.ActionHandlerContext{
			TaskHandlerContext: &runner.TaskHandlerContext{
				Task: &model.Task{
					Parameters: rctypes.FirmwareInstallTaskParameters{},
					Asset:      &rctypes.Asset{},
				},
				Logger: logrus.NewEntry(logrus.New()),
			},
			Firmware: &model.Firmware{
				Version:   "DL6R",
				URL:       "https://downloads.dell.com/FOLDER06303849M/1/Serial-ATA_Firmware_Y1P10_WN32_DL6R_A00.EXE",
				FileName:  "Serial-ATA_Firmware_Y1P10_WN32_DL6R_A00.EXE",
				Models:    []string{"r6515"},
				Checksum:  "4189d3cb123a781d09a4f568bb686b23c6d8e6b82038eba8222b91c380a25281",
				Component: "drive",
			},
		}
	}

	tests := []struct {
		name                           string
		mockSetup                      func(actionCtx *runner.ActionHandlerContext, m *device.MockQueryor)
		expectBMCResetPreInstall       bool
		expectForceInstall             bool
		expectBMCResetPostInstall      bool
		expectBMCResetOnInstallFailure bool
		expectHostPowerOffPreInstall   bool
		expectErrContains              string
	}{
		{
			name:                     "test bmc-reset pre-install is true on first action",
			expectBMCResetPreInstall: true,
			mockSetup: func(actionCtx *runner.ActionHandlerContext, m *device.MockQueryor) {
				actionCtx.Task.Parameters.ResetBMCBeforeInstall = true
				actionCtx.First = true

				actionCtx.DeviceQueryor = m
				m.On("FirmwareInstallSteps", mock.Anything, "drive").Once().Return(
					[]bconsts.FirmwareInstallStep{
						bconsts.FirmwareInstallStepUploadInitiateInstall,
						bconsts.FirmwareInstallStepInstallStatus,
					},
					nil,
				)
			},
		},
		{
			name:               "test bmc-reset pre-install is false on first action, force is true",
			expectForceInstall: true,
			mockSetup: func(actionCtx *runner.ActionHandlerContext, m *device.MockQueryor) {
				actionCtx.First = true
				actionCtx.Task.Parameters.ForceInstall = true
				actionCtx.Task.Parameters.ResetBMCBeforeInstall = false

				actionCtx.DeviceQueryor = m
				m.On("FirmwareInstallSteps", mock.Anything, "drive").Once().Return(
					[]bconsts.FirmwareInstallStep{
						bconsts.FirmwareInstallStepUploadInitiateInstall,
						bconsts.FirmwareInstallStepInstallStatus,
					},
					nil,
				)
			},
		},
		{
			name:                      "test bmc reset post install",
			expectBMCResetPostInstall: true,
			mockSetup: func(actionCtx *runner.ActionHandlerContext, m *device.MockQueryor) {
				actionCtx.DeviceQueryor = m
				m.On("FirmwareInstallSteps", mock.Anything, "drive").Once().Return(
					[]bconsts.FirmwareInstallStep{
						bconsts.FirmwareInstallStepResetBMCPostInstall,
						bconsts.FirmwareInstallStepUploadInitiateInstall,
						bconsts.FirmwareInstallStepInstallStatus,
					},
					nil,
				)
			},
		},
		{
			name:                         "test host power off pre-install",
			expectHostPowerOffPreInstall: true,
			mockSetup: func(actionCtx *runner.ActionHandlerContext, m *device.MockQueryor) {
				actionCtx.First = true

				actionCtx.DeviceQueryor = m
				m.On("FirmwareInstallSteps", mock.Anything, "drive").Once().Return(
					[]bconsts.FirmwareInstallStep{
						bconsts.FirmwareInstallStepPowerOffHost,
						bconsts.FirmwareInstallStepUploadInitiateInstall,
						bconsts.FirmwareInstallStepInstallStatus,
					},
					nil,
				)
			},
		},
		{
			name:                           "test bmc reset on install failure",
			expectBMCResetOnInstallFailure: true,
			mockSetup: func(actionCtx *runner.ActionHandlerContext, m *device.MockQueryor) {
				actionCtx.DeviceQueryor = m
				m.On("FirmwareInstallSteps", mock.Anything, "drive").Once().Return(
					[]bconsts.FirmwareInstallStep{
						bconsts.FirmwareInstallStepResetBMCOnInstallFailure,
						bconsts.FirmwareInstallStepUploadInitiateInstall,
						bconsts.FirmwareInstallStepInstallStatus,
					},
					nil,
				)
			},
		},
		{
			name: "test error - no install steps",
			mockSetup: func(actionCtx *runner.ActionHandlerContext, m *device.MockQueryor) {
				actionCtx.DeviceQueryor = m
				m.On("FirmwareInstallSteps", mock.Anything, "drive").Once().Return(
					[]bconsts.FirmwareInstallStep{},
					nil,
				)
			},
			expectErrContains: errNoInstallSteps.Error(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actx := newTestActionCtx()

			// setup mocks
			mockDeviceQueryor := new(device.MockQueryor)
			tc.mockSetup(actx, mockDeviceQueryor)

			// init handler
			o := ActionHandler{}
			got, err := o.ComposeAction(context.Background(), actx)
			if tc.expectErrContains != "" {
				assert.ErrorContains(t, err, tc.expectErrContains)
				return
			}

			// expect no errors
			assert.Nil(t, err)
			assert.NotNil(t, got)

			// handler was assigned
			assert.NotNil(t, o.handler)

			// firmware, task objects on the handler match what was passed in
			assert.Equal(t, actx.Firmware, o.handler.firmware)
			assert.Equal(t, actx.Task, o.handler.task)
			assert.Equal(t, actx.First, o.handler.action.First)
			assert.Equal(t, actx.Last, o.handler.action.Last)

			// bmc reset before install
			if tc.expectBMCResetPreInstall {
				assert.True(t, got.BMCResetPreInstall)
			} else {
				assert.False(t, got.BMCResetPreInstall)
			}

			// force install
			if tc.expectForceInstall {
				assert.True(t, got.ForceInstall)
			} else {
				assert.False(t, got.ForceInstall)
			}

			// bmc reset post install
			if tc.expectBMCResetPostInstall {
				assert.True(t, got.BMCResetPostInstall)
			} else {
				assert.False(t, got.BMCResetPostInstall)
			}

			// bmc reset on install failure
			if tc.expectBMCResetOnInstallFailure {
				assert.True(t, got.BMCResetOnInstallFailure)
			} else {
				assert.False(t, got.BMCResetOnInstallFailure)
			}

			// host power off required before install
			if tc.expectHostPowerOffPreInstall {
				assert.True(t, got.HostPowerOffPreInstall)
			} else {
				assert.False(t, got.HostPowerOffPreInstall)
			}

			// expect atleast 5 or more steps
			assert.GreaterOrEqual(t, len(got.Steps), 5)
		})
	}
}

func TestComposeSteps(t *testing.T) {
	// test all expected steps are composed in order
	tests := []struct {
		name              string
		required          []bconsts.FirmwareInstallStep
		powerCycleBMC     bool
		expect            []model.StepName
		expectErrContains string
	}{
		{
			name:          "with firmware install, status steps and no BMC powercycle",
			powerCycleBMC: false,
			required: []bconsts.FirmwareInstallStep{
				bconsts.FirmwareInstallStepUploadInitiateInstall,
				bconsts.FirmwareInstallStepInstallStatus,
			},
			expect: []model.StepName{
				powerOnServer,
				checkInstalledFirmware,
				downloadFirmware,
				uploadFirmwareInitiateInstall,
				pollInstallStatus,
			},
		},
		{
			name:          "with firmware install, status steps and BMC powercycle",
			powerCycleBMC: true,
			required: []bconsts.FirmwareInstallStep{
				bconsts.FirmwareInstallStepUploadInitiateInstall,
				bconsts.FirmwareInstallStepInstallStatus,
			},
			expect: []model.StepName{
				powerOnServer,
				checkInstalledFirmware,
				downloadFirmware,
				preInstallResetBMC,
				uploadFirmwareInitiateInstall,
				pollInstallStatus,
			},
		},
		{
			name:          "with firmware upload and install steps",
			powerCycleBMC: true,
			required: []bconsts.FirmwareInstallStep{
				bconsts.FirmwareInstallStepUpload,
				bconsts.FirmwareInstallStepUploadStatus,
				bconsts.FirmwareInstallStepInstallUploaded,
				bconsts.FirmwareInstallStepInstallStatus,
			},
			expect: []model.StepName{
				powerOnServer,
				checkInstalledFirmware,
				downloadFirmware,
				preInstallResetBMC,
				uploadFirmware,
				pollUploadStatus,
				installUploadedFirmware,
				pollInstallStatus,
			},
		},
		{
			name:          "with host power off required for install",
			powerCycleBMC: true,
			required: []bconsts.FirmwareInstallStep{
				bconsts.FirmwareInstallStepPowerOffHost,
				bconsts.FirmwareInstallStepUploadInitiateInstall,
				bconsts.FirmwareInstallStepInstallStatus,
			},
			expect: []model.StepName{
				checkInstalledFirmware,
				downloadFirmware,
				preInstallResetBMC,
				powerOffServer,
				uploadFirmwareInitiateInstall,
				pollInstallStatus,
			},
		},
		{
			name:          "with power off, firmware upload and install steps",
			powerCycleBMC: true,
			required: []bconsts.FirmwareInstallStep{
				bconsts.FirmwareInstallStepPowerOffHost,
				bconsts.FirmwareInstallStepUpload,
				bconsts.FirmwareInstallStepUploadStatus,
				bconsts.FirmwareInstallStepInstallUploaded,
				bconsts.FirmwareInstallStepInstallStatus,
			},
			expect: []model.StepName{
				checkInstalledFirmware,
				downloadFirmware,
				preInstallResetBMC,
				powerOffServer,
				uploadFirmware,
				pollUploadStatus,
				installUploadedFirmware,
				pollInstallStatus,
			},
		},
	}

	stepNames := func(steps model.Steps) []model.StepName {
		names := []model.StepName{}
		for _, s := range steps {
			names = append(names, s.Name)
		}

		return names
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			o := ActionHandler{}
			got, err := o.composeSteps(tc.required, tc.powerCycleBMC)
			if tc.expectErrContains != "" {
				assert.ErrorContains(t, err, tc.expectErrContains)
				return
			}

			assert.Nil(t, err)
			assert.Equal(t, tc.expect, stepNames(got))
		})
	}
}

func TestConvFirmwareInstallSteps(t *testing.T) {
	tests := []struct {
		name              string
		installSteps      []bconsts.FirmwareInstallStep
		expectedStepNames []model.StepName
		expectErrContains string
	}{
		{
			"const not supported",
			[]bconsts.FirmwareInstallStep{"foo"},
			[]model.StepName{},
			"constant not supported",
		},
		{
			"no install transitions",
			[]bconsts.FirmwareInstallStep{},
			[]model.StepName{},
			errNoInstallSteps.Error(),
		},
		{
			"firmware install steps converted to transitions",
			[]bconsts.FirmwareInstallStep{
				bconsts.FirmwareInstallStepUpload,
				bconsts.FirmwareInstallStepUploadStatus,
				bconsts.FirmwareInstallStepInstallUploaded,
				bconsts.FirmwareInstallStepInstallStatus,
			},
			[]model.StepName{
				uploadFirmware,
				pollUploadStatus,
				installUploadedFirmware,
				pollInstallStatus,
			},
			"",
		},
	}
	stepNames := func(steps model.Steps) []model.StepName {
		names := []model.StepName{}
		for _, s := range steps {
			names = append(names, s.Name)
		}

		return names
	}

	o := ActionHandler{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := o.convFirmwareInstallSteps(tc.installSteps)
			if tc.expectErrContains != "" {
				assert.ErrorContains(t, err, tc.expectErrContains)
				return
			}

			assert.Nil(t, err)
			assert.Equal(t, tc.expectedStepNames, stepNames(got))
		})
	}
}
