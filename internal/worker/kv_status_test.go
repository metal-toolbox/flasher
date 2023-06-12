package worker

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
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
