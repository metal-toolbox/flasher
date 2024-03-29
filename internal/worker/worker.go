package worker

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/metal-toolbox/flasher/internal/metrics"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/runner"
	"github.com/metal-toolbox/flasher/internal/store"
	"github.com/metal-toolbox/flasher/internal/version"
	"github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"go.hollow.sh/toolbox/events"
	"go.hollow.sh/toolbox/events/pkg/kv"
	"go.hollow.sh/toolbox/events/registry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	cpv1types "github.com/metal-toolbox/conditionorc/pkg/api/v1/types"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
	rctypes "github.com/metal-toolbox/rivets/condition"
)

const (
	pkgName = "internal/worker"
)

var (
	fetchEventsInterval = 10 * time.Second

	// taskTimeout defines the time after which a task will be canceled.
	taskTimeout = 180 * time.Minute

	// taskInprogressTicker is the interval at which tasks in progress
	// will ack themselves as in progress on the event stream.
	//
	// This value should be set to less than the event stream Ack timeout value.
	taskInprogressTick = 3 * time.Minute

	errConditionDeserialize = errors.New("unable to deserialize condition")
	errTaskFirmwareParam    = errors.New("error in task firmware parameters")
	errInitTask             = errors.New("error initializing new task from event")
)

// Worker holds attributes for firmware install routines.
type Worker struct {
	stream         events.Stream
	store          store.Repository
	syncWG         *sync.WaitGroup
	logger         *logrus.Logger
	name           string
	id             registry.ControllerID // assigned when this worker registers itself
	facilityCode   string
	concurrency    int
	dispatched     int32
	dryrun         bool
	faultInjection bool
	useStatusKV    bool
	replicaCount   int
}

// NewOutofbandWorker returns a out of band firmware install worker instance
func New(
	facilityCode string,
	dryrun,
	useStatusKV,
	faultInjection bool,
	concurrency,
	replicaCount int,
	stream events.Stream,
	repository store.Repository,
	logger *logrus.Logger,
) *Worker {
	id, _ := os.Hostname()

	return &Worker{
		name:           id,
		facilityCode:   facilityCode,
		dryrun:         dryrun,
		useStatusKV:    useStatusKV,
		faultInjection: faultInjection,
		concurrency:    concurrency,
		replicaCount:   replicaCount,
		syncWG:         &sync.WaitGroup{},
		stream:         stream,
		store:          repository,
		logger:         logger,
	}
}

// Run runs the firmware install worker which listens for events to action.
func (o *Worker) Run(ctx context.Context) {
	tickerFetchEvents := time.NewTicker(fetchEventsInterval).C

	if err := o.stream.Open(); err != nil {
		o.logger.WithError(err).Error("event stream connection error")
		return
	}

	// returned channel ignored, since this is a Pull based subscription.
	_, err := o.stream.Subscribe(ctx)
	if err != nil {
		o.logger.WithError(err).Error("event stream subscription error")
		return
	}

	o.logger.Info("connected to event stream.")

	o.startWorkerLivenessCheckin(ctx)

	v := version.Current()
	o.logger.WithFields(
		logrus.Fields{
			"version":         v.AppVersion,
			"commit":          v.GitCommit,
			"branch":          v.GitBranch,
			"replica-count":   o.replicaCount,
			"concurrency":     o.concurrency,
			"dry-run":         o.dryrun,
			"fault-injection": o.faultInjection,
		},
	).Info("flasher worker running")

Loop:
	for {
		select {
		case <-tickerFetchEvents:
			if o.concurrencyLimit() {
				continue
			}

			o.processEvents(ctx)

		case <-ctx.Done():
			if o.dispatched > 0 {
				continue
			}

			break Loop
		}
	}
}

func (o *Worker) processEvents(ctx context.Context) {
	// XXX: consider having a separate context for message retrieval
	msgs, err := o.stream.PullMsg(ctx, 1)
	switch {
	case err == nil:
	case errors.Is(err, nats.ErrTimeout):
		o.logger.WithFields(
			logrus.Fields{"err": err.Error()},
		).Trace("no new events")
	default:
		o.logger.WithFields(
			logrus.Fields{"err": err.Error()},
		).Warn("retrieving new messages")
		metrics.NATSError("pull-msg")
	}

	for _, msg := range msgs {
		if ctx.Err() != nil || o.concurrencyLimit() {
			o.eventNak(msg)

			return
		}

		// spawn msg process handler
		o.syncWG.Add(1)

		go func(msg events.Message) {
			defer o.syncWG.Done()

			atomic.AddInt32(&o.dispatched, 1)
			defer atomic.AddInt32(&o.dispatched, -1)

			o.processSingleEvent(ctx, msg)
		}(msg)
	}
}

func (o *Worker) concurrencyLimit() bool {
	return int(o.dispatched) >= o.concurrency
}

func (o *Worker) eventAckInProgress(event events.Message) {
	if err := event.InProgress(); err != nil {
		metrics.NATSError("ack-in-progress")
		o.logger.WithError(err).Warn("event Ack Inprogress error")
	}
}

func (o *Worker) eventAckComplete(event events.Message) {
	if err := event.Ack(); err != nil {
		metrics.NATSError("ack")
		o.logger.WithError(err).Warn("event Ack error")
	}
}

func (o *Worker) eventNak(event events.Message) {
	if err := event.Nak(); err != nil {
		metrics.NATSError("nak")
		o.logger.WithError(err).Warn("event Nak error")
	}
}

func newTask(conditionID uuid.UUID, params *rctypes.FirmwareInstallTaskParameters) (model.Task, error) {
	task := model.Task{
		ID:         conditionID,
		Parameters: *params,
		Status:     model.NewTaskStatusRecord("initialized task"),
	}

	//nolint:errcheck // this method returns nil unconditionally
	task.SetState(model.StatePending)

	if len(params.Firmwares) > 0 {
		task.Parameters.Firmwares = params.Firmwares
		task.FirmwarePlanMethod = model.FromRequestedFirmware

		return task, nil
	}

	if params.FirmwareSetID != uuid.Nil {
		task.Parameters.FirmwareSetID = params.FirmwareSetID
		task.FirmwarePlanMethod = model.FromFirmwareSet

		return task, nil
	}

	return task, errors.Wrap(errTaskFirmwareParam, "no firmware list or firmwareSetID specified")
}

func (o *Worker) registerEventCounter(valid bool, response string) {
	metrics.EventsCounter.With(
		prometheus.Labels{
			"valid":    strconv.FormatBool(valid),
			"response": response,
		}).Inc()
}

func (o *Worker) processSingleEvent(ctx context.Context, e events.Message) {
	// extract parent trace context from the event if any.
	ctx = e.ExtractOtelTraceContext(ctx)

	ctx, span := otel.Tracer(pkgName).Start(
		ctx,
		"worker.processSingleEvent",
	//	trace.WithSpanKind(trace.SpanKindConsumer),
	)
	defer span.End()

	condition, err := conditionFromEvent(e)
	if err != nil {
		o.logger.WithError(err).WithField(
			"subject", e.Subject()).Warn("unable to retrieve condition from message")

		o.registerEventCounter(false, "ack")
		o.eventAckComplete(e)

		return
	}

	span.SetAttributes(attribute.KeyValue{Key: "conditionKind", Value: attribute.StringValue(condition.ID.String())})

	// check and see if the task is or has-been handled by another worker
	currentState := o.taskInProgress(condition.ID.String())
	switch currentState {
	case inProgress:
		o.logger.WithField("condition_id", condition.ID.String()).Info("condition is already in progress")
		o.eventAckInProgress(e)
		metrics.RegisterSpanEvent(span, condition, o.id.String(), "", "ackInProgress")

		return

	case complete:
		o.logger.WithField("condition_id", condition.ID.String()).Info("condition is complete")
		o.eventAckComplete(e)
		metrics.RegisterSpanEvent(span, condition, o.id.String(), "", "ackComplete")

		return

	case orphaned:
		o.logger.WithField("condition_id", condition.ID.String()).Warn("restarting this condition")
		metrics.RegisterSpanEvent(span, condition, o.id.String(), "", "restarting condition")

		// we need to restart this event
	case notStarted:
		o.logger.WithField("condition_id", condition.ID.String()).Info("starting new condition")
		metrics.RegisterSpanEvent(span, condition, o.id.String(), "", "start new condition")

		// break out here, this is a new event
	case indeterminate:
		o.logger.WithField("condition_id", condition.ID.String()).Warn("unable to determine state of this condition")
		// send it back to NATS to try again
		o.eventNak(e)
		metrics.RegisterSpanEvent(span, condition, o.id.String(), "", "sent nack, indeterminate state")

		return
	}

	task, err := newTaskFromCondition(condition, o.faultInjection)
	if err != nil {
		o.logger.WithError(err).Warn("error initializing task from condition")

		o.registerEventCounter(false, "ack")
		o.eventAckComplete(e)
		metrics.RegisterSpanEvent(span, condition, o.id.String(), "", "sent ack, error task init")

		return
	}

	// first try to fetch asset inventory from inventory store
	asset, err := o.store.AssetByID(ctx, task.Parameters.AssetID.String())
	if err != nil {
		o.logger.WithFields(logrus.Fields{
			"assetID":     task.Parameters.AssetID.String(),
			"conditionID": condition.ID,
			"err":         err.Error(),
		}).Warn("asset lookup error")

		o.registerEventCounter(true, "nack")
		o.eventNak(e) // have the message bus re-deliver the message
		metrics.RegisterSpanEvent(
			span,
			condition,
			o.id.String(),
			task.Parameters.AssetID.String(),
			"sent nack, store query error",
		)

		return
	}

	taskCtx, cancel := context.WithTimeout(ctx, taskTimeout)
	defer cancel()

	defer o.registerEventCounter(true, "ack")
	defer o.eventAckComplete(e)
	metrics.RegisterSpanEvent(
		span,
		condition,
		o.id.String(),
		task.Parameters.AssetID.String(),
		"sent ack, condition fulfilled",
	)

	o.runTaskWithMonitor(taskCtx, task, asset, e)
}

func (o *Worker) runTaskWithMonitor(ctx context.Context, task *model.Task, asset *model.Asset, e events.Message) {
	// the runTask method is expected to close this channel to indicate its done
	doneCh := make(chan bool)

	// monitor sends in progress ack's until the task handler returns.
	monitor := func() {
		defer o.syncWG.Done()

		ticker := time.NewTicker(taskInprogressTick)
		defer ticker.Stop()

	Loop:
		for {
			select {
			case <-ticker.C:
				o.eventAckInProgress(e)
			case <-doneCh:
				break Loop
			}
		}
	}

	o.syncWG.Add(1)

	go monitor()

	o.runTaskHandler(ctx, asset, task, doneCh)

	<-doneCh
}

func (o *Worker) getStatusPublisher() sm.Publisher {
	if o.useStatusKV {
		var opts []kv.Option
		if o.replicaCount > 1 {
			opts = append(opts, kv.WithReplicas(o.replicaCount))
		}
		return NewStatusKVPublisher(o.stream, o.logger, opts...)
	}
	return &statusEmitter{o.stream, o.logger}
}

func (o *Worker) registerConditionMetrics(startTS time.Time, state string) {
	metrics.ConditionRunTimeSummary.With(
		prometheus.Labels{
			"condition": string(rctypes.FirmwareInstall),
			"state":     state,
		},
	).Observe(time.Since(startTS).Seconds())
}

func (o *Worker) runTaskHandler(ctx context.Context, asset *model.Asset, task *model.Task, doneCh chan bool) {
	defer close(doneCh)

	// prepare logger
	l := logrus.New()
	l.Formatter = o.logger.Formatter
	l.Level = o.logger.Level
	hLogger := l.WithFields(
		logrus.Fields{
			"workerID":    o.id.String(),
			"conditionID": task.ID.String(),
			"assetID":     asset.ID.String(),
			"bmc":         asset.BmcAddress.String(),
		},
	)

	// init handler
	handler := newHandler(
		ctx,
		o.dryrun,
		o.id,
		o.facilityCode,
		task,
		asset,
		o.store,
		o.getStatusPublisher(),
		hLogger,
	)

	// init runner
	r := runner.New(hLogger)
	startTS := time.Now()

	o.logger.WithFields(logrus.Fields{
		"deviceID":    task.Parameters.AssetID.String(),
		"conditionID": task.ID,
	}).Info("running task for device")

	// run task handler
	if err := r.RunTask(ctx, task, handler); err != nil {
		o.logger.WithFields(
			logrus.Fields{
				"deviceID":    task.Parameters.AssetID,
				"conditionID": task.ID.String(),
				"err":         err.Error(),
			},
		).Warn("task for device failed")

		o.registerConditionMetrics(startTS, string(rctypes.Failed))
		return
	}

	o.registerConditionMetrics(startTS, string(rctypes.Succeeded))

	o.logger.WithFields(logrus.Fields{
		"deviceID":    task.Parameters.AssetID.String(),
		"conditionID": task.ID,
		"elapsed":     time.Since(startTS).String(),
	}).Info("task for device completed")
}

func conditionFromEvent(e events.Message) (*rctypes.Condition, error) {
	data := e.Data()
	if data == nil {
		return nil, errors.New("data field empty")
	}

	condition := &rctypes.Condition{}
	if err := json.Unmarshal(data, condition); err != nil {
		return nil, errors.Wrap(errConditionDeserialize, err.Error())
	}

	return condition, nil
}

// newTaskFromMsg returns a new task object with the given parameters
func newTaskFromCondition(condition *rctypes.Condition, faultInjection bool) (*model.Task, error) {
	parameters := &rctypes.FirmwareInstallTaskParameters{}
	if err := json.Unmarshal(condition.Parameters, parameters); err != nil {
		return nil, errors.Wrap(errInitTask, "Firmware install task parameters error: "+err.Error())
	}

	task, err := newTask(condition.ID, parameters)
	if err != nil {
		return nil, err
	}

	if faultInjection && condition.Fault != nil {
		task.Fault = condition.Fault
	}

	return &task, nil
}

// statusEmitter implements the statemachine.Publisher interface
type statusEmitter struct {
	stream events.Stream
	logger *logrus.Logger
}

func (e *statusEmitter) Publish(hCtx *sm.HandlerContext) {
	ctx, span := otel.Tracer(pkgName).Start(
		hCtx.Ctx,
		"worker.Publish.Event",
		trace.WithSpanKind(trace.SpanKindConsumer),
	)
	defer span.End()

	task := hCtx.Task
	update := &cpv1types.ConditionUpdateEvent{
		Kind: rctypes.FirmwareInstall,
		ConditionUpdate: cpv1types.ConditionUpdate{
			ConditionID: task.ID,
			ServerID:    task.Parameters.AssetID,
			State:       rctypes.State(task.State()),
			Status:      task.Status.MustMarshal(),
		},
	}

	// XXX: This ought to be a method on ConditionUpdate like we have for Condition in
	// ConditionOrc
	byt, err := json.Marshal(update)
	if err != nil {
		panic("unable to marshal a condition update" + err.Error())
	}

	if err := e.stream.Publish(
		ctx,
		string(rctypes.ConditionUpdateEvent),
		byt,
	); err != nil {
		e.logger.WithError(err).Error("error publishing condition update")
	}

	e.logger.WithFields(
		logrus.Fields{
			"state":       update.ConditionUpdate.State,
			"status":      update.ConditionUpdate.Status,
			"assetID":     task.Parameters.AssetID,
			"conditionID": task.ID,
		},
	).Trace("condition update published")
}
