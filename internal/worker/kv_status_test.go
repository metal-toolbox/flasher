package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	cotyp "github.com/metal-toolbox/conditionorc/pkg/types"
	"github.com/metal-toolbox/flasher/internal/model"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
	"github.com/metal-toolbox/flasher/types"
	"github.com/nats-io/nats-server/v2/server"
	srvtest "github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"go.hollow.sh/toolbox/events"
	"go.hollow.sh/toolbox/events/pkg/kv"
	"go.hollow.sh/toolbox/events/registry"
)

func startJetStreamServer(t *testing.T) *server.Server {
	t.Helper()
	opts := srvtest.DefaultTestOptions
	opts.Port = -1
	opts.JetStream = true
	return srvtest.RunServer(&opts)
}

func jetStreamContext(t *testing.T, s *server.Server) (*nats.Conn, nats.JetStreamContext) {
	t.Helper()
	nc, err := nats.Connect(s.ClientURL())
	if err != nil {
		t.Fatalf("connect => %v", err)
	}
	js, err := nc.JetStream(nats.MaxWait(10 * time.Second))
	if err != nil {
		t.Fatalf("JetStream => %v", err)
	}
	return nc, js
}

func shutdownJetStream(t *testing.T, s *server.Server) {
	t.Helper()
	var sd string
	if config := s.JetStreamConfig(); config != nil {
		sd = config.StoreDir
	}
	s.Shutdown()
	if sd != "" {
		if err := os.RemoveAll(sd); err != nil {
			t.Fatalf("Unable to remove storage %q: %v", sd, err)
		}
	}
	s.WaitForShutdown()
}

func TestPublisher(t *testing.T) {
	srv := startJetStreamServer(t)
	defer shutdownJetStream(t, srv)
	nc, js := jetStreamContext(t, srv) // nc is closed on evJS.Close(), js needs no cleanup
	evJS := events.NewJetstreamFromConn(nc)
	defer evJS.Close()

	pub := NewStatusKVPublisher(evJS, logrus.New(), kv.WithReplicas(1))
	require.NotNil(t, pub, "publisher constructor")

	readHandle, err := js.KeyValue("firmwareInstall")
	require.NoError(t, err, "read handle")

	taskID := uuid.New()
	assetID := uuid.New()

	testContext := &sm.HandlerContext{
		Ctx: context.TODO(),
		Task: &model.Task{
			ID:     taskID,
			Status: "some-status",
		},
		WorkerID: registry.GetID("kvtest"),
		Asset: &model.Asset{
			ID:           assetID,
			FacilityCode: "fac13",
		},
	}
	testContext.Task.SetState(model.StatePending)
	require.NotPanics(t, func() { pub.Publish(testContext) }, "publish initial")
	require.NotEqual(t, 0, testContext.LastRev, "last rev - 1")

	entry, err := readHandle.Get("fac13." + taskID.String())
	require.Equal(t, entry.Revision(), testContext.LastRev, "last rev - 2")

	sv := &types.StatusValue{}
	err = json.Unmarshal(entry.Value(), sv)
	require.NoError(t, err, "unmarshal")

	require.Equal(t, types.Version, sv.MsgVersion, "version check")
	require.Equal(t, assetID.String(), sv.Target, "sv Target")
	require.Equal(t, json.RawMessage(`{"msg":"some-status"}`), sv.Status, "sv Status")

	testContext.Task.SetState(model.StateActive)
	require.NotPanics(t, func() { pub.Publish(testContext) }, "publish revision")

	entry, err = readHandle.Get("fac13." + taskID.String())
	require.Equal(t, entry.Revision(), testContext.LastRev, "last rev - 3")
}

func TestTaskInProgress(t *testing.T) {
	srv := startJetStreamServer(t)
	defer shutdownJetStream(t, srv)
	nc, js := jetStreamContext(t, srv)
	evJS := events.NewJetstreamFromConn(nc)
	defer evJS.Close()

	// set up the fake status KV
	cfg := &nats.KeyValueConfig{
		Bucket: string(cotyp.FirmwareInstall),
	}
	writeHandle, err := js.CreateKeyValue(cfg)
	require.NoError(t, err, "creating KV")

	worker := Worker{
		stream: evJS,
		logger: &logrus.Logger{
			Formatter: &logrus.JSONFormatter{},
		},
		facilityCode: "test1",
	}

	conditionID := uuid.New()
	key := fmt.Sprintf("test1.%s", conditionID.String())

	// first scenario: nothing in the KV
	val := worker.taskInProgress(conditionID.String())
	require.Equal(t, notStarted, val, "empty KV test")

	// write a non StatusValue to the KV
	_, err = writeHandle.Put(key, []byte("non-status-value"))
	val = worker.taskInProgress(conditionID.String())
	require.Equal(t, indeterminate, val, "bad status value")

	// write a failed StatusValue
	sv := &types.StatusValue{
		State: "failed",
	}
	_, err = writeHandle.Put(key, sv.MustBytes())
	require.NoError(t, err, "finished status value")
	val = worker.taskInProgress(conditionID.String())
	require.Equal(t, complete, val, "failed status")

	sv.WorkerID = "some junk id"
	sv.State = "pending"

	_, err = writeHandle.Put(key, sv.MustBytes())
	require.NoError(t, err, "update status value to pending")
	val = worker.taskInProgress(conditionID.String())
	require.Equal(t, indeterminate, val, "bogus worker id")

	// initialize the registry before we do anything else
	err = registry.InitializeRegistryWithOptions(evJS, kv.WithReplicas(1))
	require.NoError(t, err, "initialize registry")

	flasherID := registry.GetID("other-flasher")
	err = registry.RegisterController(flasherID)
	require.NoError(t, err, "register test flasher")

	sv.WorkerID = flasherID.String()

	_, err = writeHandle.Put(key, sv.MustBytes())
	require.NoError(t, err, "update status value to pending")
	val = worker.taskInProgress(conditionID.String())
	require.Equal(t, inProgress, val, "pending status")

	err = registry.DeregisterController(flasherID)
	require.NoError(t, err, "deregister test flasher")

	val = worker.taskInProgress(conditionID.String())
	require.Equal(t, orphaned, val, "no live workers")
}
