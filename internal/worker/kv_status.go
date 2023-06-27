//nolint:gomnd,revive //useless opinions
package worker

import (
	"encoding/json"
	"fmt"
	"time"

	cotyp "github.com/metal-toolbox/conditionorc/pkg/types"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
	"github.com/metal-toolbox/flasher/types"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"

	"go.hollow.sh/toolbox/events"
	"go.hollow.sh/toolbox/events/pkg/kv"
	"go.hollow.sh/toolbox/events/registry"
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

type taskState int

const (
	notStarted    taskState = iota
	inProgress              // another flasher has started it, is still around and updated recently
	complete                // task is done
	orphaned                // the flasher that started this task doesn't exist anymore
	indeterminate           // we got an error in the process of making the check
)

func (o *Worker) taskInProgress(cID string) taskState {
	handle, err := events.AsNatsJetStreamContext(o.stream.(*events.NatsJetstream)).KeyValue(statusKVName)
	if err != nil {
		o.logger.WithError(err).WithFields(logrus.Fields{
			"condition_id": cID,
		}).Warn("unable to connect to status KV for condition lookup")

		return indeterminate
	}

	lookupKey := fmt.Sprintf("%s.%s", o.facilityCode, cID)
	entry, err := handle.Get(lookupKey)
	switch err {
	case nats.ErrKeyNotFound:
		// This should be by far the most common path through this code.
		return notStarted
	case nil:
		break // we'll handle this outside the switch
	default:
		o.logger.WithError(err).WithFields(logrus.Fields{
			"condition_id": cID,
		}).Warn("error reading condition status")

		return indeterminate
	}

	// we have an status entry for this condition, is is complete?
	sv := types.StatusValue{}
	if errJson := json.Unmarshal(entry.Value(), &sv); errJson != nil {
		o.logger.WithError(errJson).WithFields(logrus.Fields{
			"condition_id": cID,
		}).Warn("unable to construct a sane status for this condition")

		return indeterminate
	}

	if cotyp.ConditionState(sv.State) == cotyp.Failed ||
		cotyp.ConditionState(sv.State) == cotyp.Succeeded {
		o.logger.WithFields(logrus.Fields{
			"condition_id":    cID,
			"condition_state": sv.State,
		}).Info("this condition is already complete")

		return complete
	}

	// is the worker handling this condition alive?
	worker, err := registry.ControllerIDFromString(sv.WorkerID)
	if err != nil {
		o.logger.WithError(err).WithFields(logrus.Fields{
			"condition_id": cID,
			"worker_id":    sv.WorkerID,
		}).Warn("bad worker id")

		return indeterminate
	}

	activeAt, err := registry.LastContact(worker)
	switch err {
	case nats.ErrKeyNotFound:
		// the data for this worker aged-out, it's no longer active
		// XXX: the most conservative thing to do here is to return
		// indeterminate but most times this will indicate that the
		// worker crashed/restarted and this task should be restarted.
		o.logger.WithFields(logrus.Fields{
			"condition_id": cID,
			"worker_id":    sv.WorkerID,
		}).Info("original worker not found")

		return orphaned
	case nil:
		timeStr, _ := activeAt.MarshalText()
		o.logger.WithError(err).WithFields(logrus.Fields{
			"condition_id": cID,
			"worker_id":    sv.WorkerID,
			"last_active":  timeStr,
		}).Warn("error looking up worker last contact")

		return inProgress
	default:
		o.logger.WithError(err).WithFields(logrus.Fields{
			"condition_id": cID,
			"worker_id":    sv.WorkerID,
		}).Warn("error looking up worker last contact")

		return indeterminate
	}
}
