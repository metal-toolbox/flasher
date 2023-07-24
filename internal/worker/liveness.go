package worker

import (
	"context"
	"sync"
	"time"

	"go.hollow.sh/toolbox/events"
	"go.hollow.sh/toolbox/events/pkg/kv"
	"go.hollow.sh/toolbox/events/registry"

	"github.com/metal-toolbox/flasher/internal/metrics"
	"github.com/nats-io/nats.go"
	"github.com/pkg/errors"
)

var (
	once           sync.Once
	checkinCadence = 30 * time.Second
	livenessTTL    = 3 * time.Minute
)

// This starts a go-routine to peridocally check in with the NATS kv
func (w *Worker) startWorkerLivenessCheckin(ctx context.Context) {
	once.Do(func() {
		w.id = registry.GetID(w.name)
		natsJS, ok := w.stream.(*events.NatsJetstream)
		if !ok {
			w.logger.Error("Non-NATS stores are not supported for worker-liveness")
			return
		}

		opts := []kv.Option{
			kv.WithTTL(livenessTTL),
		}

		// any setting of replicas (even 1) chokes NATS in non-clustered mode
		if w.replicaCount != 1 {
			opts = append(opts, kv.WithReplicas(w.replicaCount))
		}

		if err := registry.InitializeRegistryWithOptions(natsJS, opts...); err != nil {
			metrics.NATSError("initialize liveness registry")
			w.logger.WithError(err).Error("unable to initialize active worker registry")
			return
		}

		go w.checkinRoutine(ctx)
	})
}

func (w *Worker) checkinRoutine(ctx context.Context) {
	if err := registry.RegisterController(w.id); err != nil {
		w.logger.WithError(err).Warn("unable to do initial worker liveness registration")
	}

	tick := time.NewTicker(checkinCadence)
	defer tick.Stop()

	var stop bool
	for !stop {
		select {
		case <-tick.C:
			err := registry.ControllerCheckin(w.id)
			if err != nil {
				w.logger.WithError(err).
					WithField("id", w.id.String()).
					Warn("worker checkin failed")
				metrics.NATSError("liveness checkin")
				if err = refreshWorkerToken(w.id); err != nil {
					w.logger.WithError(err).
						WithField("id", w.id.String()).
						Fatal("unable to refresh worker liveness token")
				}
			}
		case <-ctx.Done():
			w.logger.Info("liveness check-in stopping on done context")
			stop = true
		}
	}
}

// try to de-register/re-register this id.
func refreshWorkerToken(id registry.ControllerID) error {
	err := registry.DeregisterController(id)
	if err != nil && !errors.Is(err, nats.ErrKeyNotFound) {
		metrics.NATSError("liveness refresh: de-register")
		return err
	}
	err = registry.RegisterController(id)
	if err != nil {
		metrics.NATSError("liveness referesh: register")
		return err
	}
	return nil
}
