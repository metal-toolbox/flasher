//nolint:gomnd //useless opinions
package worker

import (
	"fmt"
	"time"

	cotyp "github.com/metal-toolbox/conditionorc/pkg/types"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
	"github.com/metal-toolbox/flasher/types"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"

	"go.hollow.sh/toolbox/events"
	"go.hollow.sh/toolbox/events/pkg/kv"
)

var (
	statusKVName  = string(cotyp.FirmwareInstall)
	defaultKVOpts = []kv.Option{
		kv.WithDescription("flasher condition status tracking"),
		kv.WithTTL(10 * 24 * time.Hour),
	}
)

type statusKVPublisher struct {
	kv  nats.KeyValue
	log *logrus.Logger
}

// Publish implements the statemachine Publisher interface.
func (s *statusKVPublisher) Publish(hCtx *sm.HandlerContext) {
	facility := "facility"
	if hCtx.Asset.FacilityCode != "" {
		facility = hCtx.Asset.FacilityCode
	}
	key := fmt.Sprintf("%s.%s", facility, hCtx.Task.ID.String())
	payload := statusFromContext(hCtx)

	var err error
	var rev uint64
	if hCtx.LastRev == 0 {
		rev, err = s.kv.Create(key, payload)
	} else {
		rev, err = s.kv.Update(key, payload, hCtx.LastRev)
	}

	if err != nil {
		s.log.WithError(err).WithFields(logrus.Fields{
			"asset_id":       hCtx.Asset.ID.String(),
			"asset_facility": hCtx.Asset.FacilityCode,
			"task_id":        hCtx.Task.ID.String(),
			"last_rev":       hCtx.LastRev,
		}).Warn("unable to write task status")
		return
	}
	hCtx.LastRev = rev
}

func statusFromContext(hCtx *sm.HandlerContext) []byte {
	sv := &types.StatusValue{
		WorkerID: hCtx.WorkerID.String(),
		Target:   hCtx.Asset.ID.String(),
		State:    string(hCtx.Task.State()),
		Status:   statusInfoJSON(hCtx.Task.Status),
		// ResourceVersion:  XXX: the handler context has no concept of this! does this make
		// sense at the controller-level?
		UpdatedAt: time.Now(),
	}
	return sv.MustBytes()
}

func NewStatusKVPublisher(s events.Stream, log *logrus.Logger, opts ...kv.Option) sm.Publisher {
	js, ok := s.(*events.NatsJetstream)
	if !ok {
		log.Fatal("status-kv publisher is only supported on NATS")
	}

	kvOpts := defaultKVOpts
	kvOpts = append(kvOpts, opts...)

	statusKV, err := kv.CreateOrBindKVBucket(js, statusKVName, kvOpts...)
	if err != nil {
		log.WithError(err).Fatal("unable to bind status KV bucket")
	}

	return &statusKVPublisher{
		kv:  statusKV,
		log: log,
	}
}
