package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/store"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.hollow.sh/toolbox/events"

	cpv1types "github.com/metal-toolbox/conditionorc/pkg/api/v1/types"
	cptypes "github.com/metal-toolbox/conditionorc/pkg/types"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
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

	errEventAttributes   = errors.New("error in event attributes")
	errTaskFirmwareParam = errors.New("error in task firmware parameters")
	errInitTask          = errors.New("error initializing new task from event")
)

const (
	// urnNamespace defines the event message namespace value that this worker will process.
	urnNamespace = "hollow-controllers"
)

// Worker holds attributes for firmware install routines.
type Worker struct {
	stream         events.Stream
	store          store.Repository
	syncWG         *sync.WaitGroup
	logger         *logrus.Logger
	id             string
	facilityCode   string
	concurrency    int
	dispatched     int32
	dryrun         bool
	faultInjection bool
}

// NewOutofbandWorker returns a out of band firmware install worker instance
func New(
	facilityCode string,
	dryrun bool,
	faultInjection bool,
	concurrency int,
	stream events.Stream,
	repository store.Repository,
	logger *logrus.Logger,
) *Worker {
	id, _ := os.Hostname()

	return &Worker{
		id:             id,
		facilityCode:   facilityCode,
		dryrun:         dryrun,
		faultInjection: faultInjection,
		concurrency:    concurrency,
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

	o.logger.WithFields(
		logrus.Fields{
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
	if err != nil {
		o.logger.WithFields(
			logrus.Fields{"err": err.Error()},
		).Debug("error fetching work")
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

			o.processEvent(ctx, msg)
		}(msg)
	}
}

func (o *Worker) concurrencyLimit() bool {
	return int(o.dispatched) >= o.concurrency
}

func (o *Worker) eventAckInprogress(event events.Message) {
	if err := event.InProgress(); err != nil {
		o.logger.WithError(err).Warn("event Ack Inprogress error")
	}
}

func (o *Worker) eventAckComplete(event events.Message) {
	if err := event.Ack(); err != nil {
		o.logger.WithError(err).Warn("event Ack error")
	}
}

func (o *Worker) eventNak(event events.Message) {
	if err := event.Nak(); err != nil {
		o.logger.WithError(err).Warn("event Nak error")
	}
}

func newTask(conditionID uuid.UUID, params *model.TaskParameters) (model.Task, error) {
	task := model.Task{ID: conditionID}

	if err := task.SetState(model.StatePending); err != nil {
		return task, err
	}

	task.Parameters.AssetID = params.AssetID
	task.Parameters.ForceInstall = params.ForceInstall
	task.Parameters.ResetBMCBeforeInstall = params.ResetBMCBeforeInstall

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

func (o *Worker) processEvent(ctx context.Context, e events.Message) {
	defer o.eventAckComplete(e)

	data, err := e.Data()
	if err != nil {
		o.logger.WithFields(
			logrus.Fields{"err": err.Error(), "subject": e.Subject()},
		).Error("data unpack error")

		return
	}

	urn, err := e.SubjectURN(data)
	if err != nil {
		o.logger.WithFields(
			logrus.Fields{"err": err.Error(), "subject": e.Subject()},
		).Error("error parsing subject URN in msg")

		return
	}

	if urn.ResourceType != cptypes.ServerResourceType {
		o.logger.WithError(errEventAttributes).Warn("unsupported resourceType: " + urn.ResourceType)

		return
	}

	if data.EventType != string(cptypes.FirmwareInstallOutofband.EventType()) {
		o.logger.WithError(errEventAttributes).Warn("unsupported eventType: " + data.EventType)

		return
	}

	if urn.Namespace != urnNamespace {
		o.logger.WithError(errEventAttributes).Warn("unsupported URN Namespace: " + urn.Namespace)

		return
	}

	condition, err := conditionFromEvent(e)
	if err != nil {
		o.logger.WithError(errEventAttributes).Warn("error in Condition data" + urn.Namespace)

		return
	}

	task, err := newTaskFromCondition(condition, o.faultInjection)
	if err != nil {
		o.logger.WithError(err).Warn("error initializing task from condition")

		return
	}

	// first try to fetch asset inventory from inventory store
	//
	// error ignored on purpose
	asset, err := o.store.AssetByID(ctx, task.Parameters.AssetID.String())
	if err != nil {
		o.logger.WithFields(logrus.Fields{
			"assetID":     task.Parameters.AssetID.String(),
			"conditionID": condition.ID,
		}).Warn("error initializing task from condition")

		return
	}

	streamEvent := &model.StreamEvent{
		Msg:       e,
		Condition: condition,
		Urn:       urn,
	}

	taskCtx, cancel := context.WithTimeout(ctx, taskTimeout)
	defer cancel()

	o.runTaskWithMonitor(taskCtx, task, asset, streamEvent)
}

func (o *Worker) runTaskWithMonitor(ctx context.Context, task *model.Task, asset *model.Asset, streamEvent *model.StreamEvent) {
	// the runTask method is expected to close this channel to indicate its done
	doneCh := make(chan bool)

	// monitor sends in progress ack's until the task statemachine returns.
	monitor := func() {
		defer o.syncWG.Done()

		ticker := time.NewTicker(taskInprogressTick)
		defer ticker.Stop()

	Loop:
		for {
			select {
			case <-ticker.C:
				o.eventAckInprogress(streamEvent.Msg)
			case <-doneCh:
				break Loop
			}
		}
	}

	o.syncWG.Add(1)

	go monitor()

	// setup state machine task handler
	handler := &taskHandler{}

	// setup state machine task handler context
	l := logrus.New()
	l.Formatter = o.logger.Formatter
	l.Level = o.logger.Level

	handlerCtx := &sm.HandlerContext{
		WorkerID:     o.id,
		Dryrun:       o.dryrun,
		Task:         task,
		Publisher:    &statusEmitter{o.stream, o.logger},
		Ctx:          ctx,
		Store:        o.store,
		Data:         make(map[string]string),
		Asset:        asset,
		FacilityCode: o.facilityCode,
		Logger: l.WithFields(
			logrus.Fields{
				"workerID":    o.id,
				"conditionID": task.ID,
				"assetID":     asset.ID.String(),
				"bmc":         asset.BmcAddress.String(),
			},
		),
	}

	o.runTaskStatemachine(handler, handlerCtx, doneCh)
	<-doneCh
}

func (o *Worker) runTaskStatemachine(handler *taskHandler, handlerCtx *sm.HandlerContext, doneCh chan bool) {
	defer close(doneCh)

	startTS := time.Now()

	o.logger.WithFields(logrus.Fields{
		"deviceID":    handlerCtx.Task.Parameters.AssetID.String(),
		"conditionID": handlerCtx.Task.ID,
	}).Info("running task for device")

	// init state machine for task
	stateMachine, err := sm.NewTaskStateMachine(handler)
	if err != nil {
		o.logger.Error(err)

		return
	}

	// run task state machine
	if err := stateMachine.Run(handlerCtx.Task, handlerCtx); err != nil {
		o.logger.WithFields(
			logrus.Fields{
				"deviceID":    handlerCtx.Task.Parameters.AssetID,
				"conditionID": handlerCtx.Task.ID.String(),
				"err":         err.Error(),
			},
		).Warn("task for device failed")

		return
	}

	o.logger.WithFields(logrus.Fields{
		"deviceID":    handlerCtx.Task.Parameters.AssetID.String(),
		"conditionID": handlerCtx.Task.ID,
		"elapsed":     time.Since(startTS).String(),
	}).Info("task for device completed")
}

func conditionFromEvent(e events.Message) (*cptypes.Condition, error) {
	data, err := e.Data()
	if err != nil {
		return nil, err
	}

	value, exists := data.AdditionalData["data"]
	if !exists {
		return nil, errors.New("data field empty")
	}

	// we do this marshal, unmarshal dance here
	// since value is of type map[string]interface{} and unpacking this
	// into a known type isn't easily feasible (or atleast I'd be happy to find out otherwise).
	cbytes, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	condition := &cptypes.Condition{}
	if err := json.Unmarshal(cbytes, condition); err != nil {
		return nil, err
	}

	return condition, nil
}

// newTaskFromMsg returns a new task object with the given parameters
func newTaskFromCondition(condition *cptypes.Condition, faultInjection bool) (*model.Task, error) {
	parameters := &model.TaskParameters{}
	if err := json.Unmarshal(condition.Parameters, parameters); err != nil {
		return nil, errors.Wrap(errInitTask, "Task parameters error: "+err.Error())
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

func sortFirmwareByInstallOrder(firmwares []*model.Firmware) {
	sort.Slice(firmwares, func(i, j int) bool {
		slugi := strings.ToLower(firmwares[i].Component)
		slugj := strings.ToLower(firmwares[j].Component)
		return model.FirmwareInstallOrder[slugi] < model.FirmwareInstallOrder[slugj]
	})
}

// statusEmitter implements the statemachine.Publisher interface
type statusEmitter struct {
	stream events.Stream
	logger *logrus.Logger
}

func statusInfoJSON(s string) json.RawMessage {
	return []byte(fmt.Sprintf("{%q: %q}", "msg", s))
}

func (e *statusEmitter) Publish(ctx context.Context, task *model.Task) {
	update := &cpv1types.ConditionUpdateEvent{
		Kind: cptypes.FirmwareInstallOutofband,
		ConditionUpdate: cpv1types.ConditionUpdate{
			ID:     task.ID,
			State:  cptypes.ConditionState(task.State()),
			Status: statusInfoJSON(task.Status),
		},
	}

	if err := e.stream.PublishAsyncWithContext(
		ctx,
		events.ResourceType(cptypes.ServerResourceType),
		cptypes.ConditionUpdateEvent,
		task.Parameters.AssetID.String(),
		update,
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
