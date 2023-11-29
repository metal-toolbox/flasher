package outofband

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"runtime"
	"testing"

	bconsts "github.com/bmc-toolbox/bmclib/v2/constants"
	sw "github.com/filanov/stateswitch"
	"github.com/metal-toolbox/flasher/internal/fixtures"
	"github.com/metal-toolbox/flasher/internal/model"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
	"github.com/metal-toolbox/flasher/internal/store"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func newTaskFixture(status string) *model.Task {
	task := &model.Task{}
	task.Status.Append(status)

	// task.Parameters.Device =
	return task
}

// eventEmitter implements the statemachine.Publisher interface
type eventEmitter struct{}

func (e *eventEmitter) Publish(_ *sm.HandlerContext) {}

func newtaskHandlerContextFixture(t *testing.T, task *model.Task, asset *model.Asset) *sm.HandlerContext {
	repository, _ := store.NewMockInventory()

	logger := logrus.New().WithField("test", "true")

	return &sm.HandlerContext{
		Task:      task,
		Publisher: &eventEmitter{},
		Asset:     asset,
		Store:     repository,
		Ctx:       context.Background(),
		Logger:    logger,
		Data:      map[string]string{},
	}
}

func TestComposeTransitions(t *testing.T) {
	defined := definitions()
	tests := []struct {
		name                    string
		installSteps            []bconsts.FirmwareInstallStep
		expectedTransitionNames []sw.TransitionType
		expectErrContains       string
	}{
		{
			"with firmware install, status steps",
			[]bconsts.FirmwareInstallStep{
				bconsts.FirmwareInstallStepUploadInitiateInstall,
				bconsts.FirmwareInstallStepInstallStatus,
			},
			[]sw.TransitionType{
				powerOnDevice,
				checkInstalledFirmware,
				downloadFirmware,
				preInstallResetBMC,
				uploadFirmwareInitiateInstall,
				pollInstallStatus,
				postInstallResetBMC,
			},
			"",
		},
		{
			"with firmware upload and install steps",
			[]bconsts.FirmwareInstallStep{
				bconsts.FirmwareInstallStepUpload,
				bconsts.FirmwareInstallStepUploadStatus,
				bconsts.FirmwareInstallStepInstallUploaded,
				bconsts.FirmwareInstallStepInstallStatus,
			},
			[]sw.TransitionType{
				powerOnDevice,
				checkInstalledFirmware,
				downloadFirmware,
				preInstallResetBMC,
				uploadFirmware,
				pollUploadStatus,
				installUploadedFirmware,
				pollInstallStatus,
				postInstallResetBMC,
			},
			"",
		},
		{
			"with host power off for install",
			[]bconsts.FirmwareInstallStep{
				bconsts.FirmwareInstallStepPowerOffHost,
				bconsts.FirmwareInstallStepUploadInitiateInstall,
				bconsts.FirmwareInstallStepInstallStatus,
			},
			[]sw.TransitionType{
				checkInstalledFirmware,
				downloadFirmware,
				preInstallResetBMC,
				powerOffDevice,
				uploadFirmwareInitiateInstall,
				pollInstallStatus,
				postInstallResetBMC,
			},
			"",
		},
		{
			"with power off, firmware upload and install steps",
			[]bconsts.FirmwareInstallStep{
				bconsts.FirmwareInstallStepPowerOffHost,
				bconsts.FirmwareInstallStepUpload,
				bconsts.FirmwareInstallStepUploadStatus,
				bconsts.FirmwareInstallStepInstallUploaded,
				bconsts.FirmwareInstallStepInstallStatus,
			},
			[]sw.TransitionType{
				checkInstalledFirmware,
				downloadFirmware,
				preInstallResetBMC,
				powerOffDevice,
				uploadFirmware,
				pollUploadStatus,
				installUploadedFirmware,
				pollInstallStatus,
				postInstallResetBMC,
			},
			"",
		},
	}

	transitionNames := func(transitions Transitions) []sw.TransitionType {
		names := []sw.TransitionType{}
		for _, tr := range transitions {
			names = append(names, sw.TransitionType(tr.Name))
		}

		return names
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := composeTransitions(defined, tc.installSteps)
			if tc.expectErrContains != "" {
				assert.ErrorContains(t, err, tc.expectErrContains)
				return
			}

			assert.Nil(t, err)
			assert.Equal(t, tc.expectedTransitionNames, transitionNames(got))
		})
	}

}

func TestConvFirmwareInstallSteps(t *testing.T) {
	tests := []struct {
		name                    string
		installSteps            []bconsts.FirmwareInstallStep
		expectedTransitionNames []string
		expectErrContains       string
	}{
		{
			"const not supported",
			[]bconsts.FirmwareInstallStep{"foo"},
			[]string{},
			"constant not supported",
		},
		{
			"no install transitions",
			[]bconsts.FirmwareInstallStep{},
			[]string{},
			"no required install transitions",
		},
		{
			"firmware install steps converted to transitions",
			[]bconsts.FirmwareInstallStep{
				bconsts.FirmwareInstallStepUpload,
				bconsts.FirmwareInstallStepUploadStatus,
				bconsts.FirmwareInstallStepInstallUploaded,
				bconsts.FirmwareInstallStepInstallStatus,
			},
			[]string{
				"uploadFirmware",
				"pollUploadStatus",
				"installUploadedFirmware",
				"pollInstallStatus",
			},
			"",
		},
	}

	transitionNames := func(transitions Transitions) []string {
		names := []string{}
		for _, tr := range transitions {
			names = append(names, string(tr.Name))
		}

		return names
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := convFirmwareInstallSteps(tc.installSteps, definitions())
			if tc.expectErrContains != "" {
				assert.ErrorContains(t, err, tc.expectErrContains)
				return
			}

			assert.Nil(t, err)
			assert.Equal(t, tc.expectedTransitionNames, transitionNames(got))
		})
	}

}

type mockHandler struct{}

func (h *mockHandler) powerOnDevice(a sw.StateSwitch, c sw.TransitionArgs) error {
	return nil
}
func (h *mockHandler) publishStatus(a sw.StateSwitch, c sw.TransitionArgs) error {
	return nil
}
func (h *mockHandler) install(a sw.StateSwitch, c sw.TransitionArgs) error {
	return nil
}

func TestPrepareTransitions(t *testing.T) {
	handler := &mockHandler{}

	tests := []struct {
		name        string
		transitions Transitions
		expected    []sw.TransitionRule
	}{
		{
			name: "Test transitions are prepared",
			transitions: Transitions{
				{
					Name:           "powerOnDevice",
					Kind:           PowerStateOn,
					DestState:      "devicePoweredOn",
					Handler:        handler.powerOnDevice,
					PostTransition: handler.publishStatus,
					TransitionDoc: sw.TransitionRuleDoc{
						Name:        "Power on device",
						Description: "Power on device - if it's currently powered off.",
					},
					DestStateDoc: sw.StateDoc{
						Name:        "devicePoweredOn",
						Description: "This action state indicates the device has been (conditionally) powered on for a component firmware install.",
					},
				},
				{
					Name:           "checkInstalledFirmware",
					Kind:           PreInstall,
					DestState:      "installedFirmwareChecked",
					Handler:        handler.install,
					PostTransition: handler.publishStatus,
					TransitionDoc: sw.TransitionRuleDoc{
						Name:        "Check installed firmware",
						Description: "Check firmware installed on component",
					},
					DestStateDoc: sw.StateDoc{
						Name:        "installedFirmwareChecked",
						Description: "This action state indicates the installed firmware on the component has been checked.",
					},
				},
			},
			expected: []sw.TransitionRule{
				{
					TransitionType:   "powerOnDevice",
					SourceStates:     sw.States{model.StateActive},
					DestinationState: "devicePoweredOn",
					Condition:        nil,
					Transition:       handler.powerOnDevice,
					PostTransition:   handler.publishStatus,
					Documentation: sw.TransitionRuleDoc{
						Name:        "Power on device",
						Description: "Power on device - if it's currently powered off.",
					},
				},
				{
					TransitionType:   "checkInstalledFirmware",
					SourceStates:     sw.States{"devicePoweredOn"},
					DestinationState: "installedFirmwareChecked",
					Condition:        nil,
					Transition:       handler.install,
					PostTransition:   handler.publishStatus,
					Documentation: sw.TransitionRuleDoc{
						Name:        "Check installed firmware",
						Description: "Check firmware installed on component",
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.transitions.prepare()
			assert.Equal(t, len(tc.transitions), len(got))

			for idx, tr := range tc.transitions {
				assert.Equal(t, tr.Name, got[idx].TransitionType)
				assert.Equal(t, tr.DestState, got[idx].DestinationState)
				assert.Equal(t, tr.TransitionDoc, got[idx].Documentation)
				assert.Equal(t, tc.expected[idx].SourceStates, got[idx].SourceStates)

				// compare func names
				// credits to https://github.com/stretchr/testify/issues/182#issuecomment-495359313
				expectFunc := runtime.FuncForPC(reflect.ValueOf(tr.Handler).Pointer()).Name()
				gotFunc := runtime.FuncForPC(reflect.ValueOf(got[idx].Transition).Pointer()).Name()
				assert.Equal(t, expectFunc, gotFunc)
			}
		})
	}
}

func serverMux(t *testing.T, serveblob []byte) *http.ServeMux {
	t.Helper()

	handler := http.NewServeMux()
	handler.HandleFunc(
		"/dummy.bin",
		func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				// the response here is
				resp := serveblob

				_, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(resp)
			default:
				t.Fatal("expected GET request, got: " + r.Method)
			}
		},
	)

	return handler
}

// Test runs an action state machine on a task
func TestActionStateMachine(t *testing.T) {
	ctx := context.Background()

	// task fixture
	task := newTaskFixture(string(model.StateActive))

	// task handler context fixture
	tctx := newtaskHandlerContextFixture(t, task, &model.Asset{})

	// firmware blob served
	blob := []byte(`blob`)
	blobMD5Checksum := "ee26908bf9629eeb4b37dac350f4754a"

	server := httptest.NewServer(serverMux(t, blob))
	defer server.Close()

	firmware := model.Firmware{
		Component: "bios",
		Vendor:    "snowflake",
		Models:    []string{"never", "works"},
		URL:       server.URL + "/dummy.bin",
		FileName:  "dummy.bin",
		Checksum:  blobMD5Checksum,
	}

	tests := []struct {
		name                      string
		installSteps              []bconsts.FirmwareInstallStep
		action                    func() model.Action
		mock                      func() (*gomock.Controller, *fixtures.MockDeviceQueryor)
		expectTransitionsComplete []sw.TransitionType
		expectActionState         sw.State
	}{
		{
			"successful run",
			[]bconsts.FirmwareInstallStep{
				bconsts.FirmwareInstallStepUploadInitiateInstall,
				bconsts.FirmwareInstallStepInstallStatus,
			},
			func() model.Action {
				a := model.Action{
					ID:       "foobar",
					TaskID:   task.ID.String(),
					Firmware: firmware,
				}

				a.SetState(model.StateActive)

				return a
			},
			func() (*gomock.Controller, *fixtures.MockDeviceQueryor) {
				ctrl := gomock.NewController(t)
				q := fixtures.NewMockDeviceQueryor(ctrl)

				q.EXPECT().Open(gomock.Any()).Return(nil).Times(1)
				q.EXPECT().PowerStatus(gomock.Any()).Return("on", nil).Times(1)
				q.EXPECT().FirmwareInstallUploadAndInitiate(gomock.Any(), gomock.Any(), gomock.Any()).Return("123", nil).Times(1)
				q.EXPECT().FirmwareTaskStatus(
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
				).AnyTimes().Return(bconsts.Complete, "some status", nil)

				return ctrl, q
			},
			[]sw.TransitionType{
				powerOnDevice,
				checkInstalledFirmware,
				downloadFirmware,
				preInstallResetBMC,
				uploadFirmwareInitiateInstall,
				pollInstallStatus,
				postInstallResetBMC,
			},
			model.StateSucceeded,
		},
		{
			"failed run",
			[]bconsts.FirmwareInstallStep{
				bconsts.FirmwareInstallStepUploadInitiateInstall,
				bconsts.FirmwareInstallStepInstallStatus,
			},
			func() model.Action {
				a := model.Action{
					ID:       "foobar",
					TaskID:   task.ID.String(),
					Firmware: firmware,
				}

				a.SetState(model.StateActive)

				return a
			},
			func() (*gomock.Controller, *fixtures.MockDeviceQueryor) {
				ctrl := gomock.NewController(t)
				q := fixtures.NewMockDeviceQueryor(ctrl)

				q.EXPECT().Open(gomock.Any()).Return(nil).Times(1)
				q.EXPECT().PowerStatus(gomock.Any()).Return("on", nil).Times(1)
				q.EXPECT().FirmwareInstallUploadAndInitiate(gomock.Any(), gomock.Any(), gomock.Any()).Return("123", nil).Times(1)
				q.EXPECT().FirmwareTaskStatus(
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
					gomock.Any(),
				).AnyTimes().Return(bconsts.Failed, "some status", nil)

				return ctrl, q
			},
			[]sw.TransitionType{
				powerOnDevice,
				checkInstalledFirmware,
				downloadFirmware,
				preInstallResetBMC,
				uploadFirmwareInitiateInstall,
				pollInstallStatus,
				postInstallResetBMC,
				resetDevice,
			},
			model.StateFailed,
		},
	}

	// env var set to cause the polling loop to skip long sleeps
	os.Setenv(envTesting, "1")
	defer os.Unsetenv(envTesting)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock, queryor := tc.mock()
			defer mock.Finish()

			tctx.DeviceQueryor = queryor

			action := tc.action()
			task.ActionsPlanned = model.Actions{&action}

			// init new state machine to run actions
			m, err := NewActionStateMachine("testing", tc.installSteps)
			if err != nil {
				t.Fatal(err)
			}

			// run action state machine
			err = m.Run(ctx, task.ActionsPlanned[0], tctx)
			assert.Equal(t, tc.expectActionState, task.ActionsPlanned[0].State())

			if tc.expectActionState == model.StateFailed {
				assert.NotNil(t, err)

				return
			}

			assert.Nil(t, err)
			assert.Equal(t, tc.expectTransitionsComplete, m.TransitionsCompleted())
		})
	}

}
