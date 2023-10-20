package outofband

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/filanov/stateswitch"
	"github.com/metal-toolbox/flasher/internal/fixtures"
	"github.com/metal-toolbox/flasher/internal/model"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
	"github.com/metal-toolbox/flasher/internal/store"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
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

func newtaskHandlerContextFixture(task *model.Task, asset *model.Asset) *sm.HandlerContext {
	repository, _ := store.NewMockInventory()

	logger := logrus.New().WithField("test", "true")

	return &sm.HandlerContext{
		Task:          task,
		Publisher:     &eventEmitter{},
		Asset:         asset,
		Store:         repository,
		DeviceQueryor: fixtures.NewDeviceQueryor(context.Background(), asset, logger),
		Ctx:           context.Background(),
		Logger:        logger,
		Data:          map[string]string{},
	}
}

func Test_NewActionStateMachine(t *testing.T) {
	// init new state machine
	m, err := NewActionStateMachine("testing")
	if err != nil {
		t.Fatal(err)
	}

	// TODO(joel): at some point we'd want to test if the nodes and edges
	// in the transition rules match whats expected
	assert.Equal(t, transitionOrder(), m.TransitionOrder())
	assert.Len(t, transitionRules(), 11)
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
//
// This is basically an end to end test of the state machine run with a mock BMC device
// to check it transitions through all the states as expected.
func Test_ActionStateMachine_Run_Succeeds(t *testing.T) {
	ctx := context.Background()

	// task fixture
	task := newTaskFixture(string(model.StateActive))

	// task handler context fixture
	tctx := newtaskHandlerContextFixture(task, &model.Asset{})

	// firmware fixture
	firmware := fixtures.NewFirmware()

	// firmware blob served
	blob := []byte(`blob`)
	blobMD5Checksum := "ee26908bf9629eeb4b37dac350f4754a"

	server := httptest.NewServer(serverMux(t, blob))

	// rig firmware endpoints to point to the test service
	firmware[0].URL = server.URL + "/dummy.bin"
	firmware[0].Checksum = blobMD5Checksum
	firmware[0].FileName = "dummy.bin"

	action := model.Action{
		ID:       "foobar",
		TaskID:   task.ID.String(),
		Firmware: *firmware[0],
	}

	_ = action.SetState(model.StateActive)

	// set action planned
	task.ActionsPlanned = model.Actions{&action}

	// set test env variables
	os.Setenv(envTesting, "1")
	// this causes the mock bmc to indicate the firmware install was successful
	os.Setenv(fixtures.EnvMockBMCFirmwareInstallStatus, string(model.StatusInstallComplete))

	// init new state machine to run actions
	m, err := NewActionStateMachine("testing")
	if err != nil {
		t.Fatal(err)
	}

	// run action state machine
	err = m.Run(ctx, task.ActionsPlanned[0], tctx)
	if err != nil {
		t.Fatal(err)
	}

	server.Close()

	// assert transitions executed
	assert.Equal(t, transitionOrder(), m.TransitionsCompleted())
}

// Test runs an action state machine on a task
//
// This is basically an end to end test of the state machine run with a mock BMC device
// to check it transitions through all the states as expected.
func Test_ActionStateMachine_Run_Fails(t *testing.T) {
	ctx := context.Background()

	// task fixture
	task := newTaskFixture(string(model.StateActive))

	// task handler context fixture
	tctx := newtaskHandlerContextFixture(task, &model.Asset{})

	// firmware fixture
	firmware := fixtures.NewFirmware()

	// firmware blob served
	blob := []byte(`blob`)
	blobChecksum := "ee26908bf9629eeb4b37dac350f4754a"

	server := httptest.NewServer(serverMux(t, blob))

	// rig firmware endpoints to point to the test service
	firmware[0].URL = server.URL + "/dummy.bin"
	firmware[0].Checksum = blobChecksum
	firmware[0].FileName = "dummy.bin"

	action := model.Action{
		ID:       "foobar",
		TaskID:   task.ID.String(),
		Firmware: *firmware[0],
	}

	_ = action.SetState(model.StateActive)

	// set action planned
	task.ActionsPlanned = model.Actions{&action}

	// set test env variables
	os.Setenv(envTesting, "1")
	// this causes the firmware install poll method to fail on multiple unknown statuses returned by the mock bmc
	os.Setenv(fixtures.EnvMockBMCFirmwareInstallStatus, string(model.StatusInstallUnknown))

	// init new state machine to run actions
	m, err := NewActionStateMachine("testing")
	if err != nil {
		t.Fatal(err)
	}

	// run action state machine
	err = m.Run(ctx, task.ActionsPlanned[0], tctx)
	assert.NotNil(t, err)

	server.Close()

	expectedComplete := []stateswitch.TransitionType{
		transitionTypePowerOnDevice,
		transitionTypeCheckInstalledFirmware,
		transitionTypeDownloadFirmware,
		transitionTypePreInstallResetBMC,
		transitionTypeInitiatingInstallFirmware,
	}

	// assert transitions executed
	assert.Equal(t, expectedComplete, m.TransitionsCompleted())
}
