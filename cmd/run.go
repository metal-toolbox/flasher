package cmd

import (
	"context"
	"log"
	"strings"

	"github.com/equinix-labs/otel-init-go/otelinit"
	"github.com/metal-toolbox/flasher/internal/app"
	"github.com/metal-toolbox/flasher/internal/metrics"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/store"
	"github.com/metal-toolbox/flasher/internal/worker"
	"github.com/metal-toolbox/rivets/events/controller"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	// nolint:gosec // profiling endpoint listens on localhost.
	_ "net/http/pprof"
)

var cmdRun = &cobra.Command{
	Use:   "run",
	Short: "Run flasher service to listen for events and install firmware",
	Run: func(cmd *cobra.Command, args []string) {
		runWorker(cmd.Context())
	},
}

// run worker command
var (
	useStatusKV    bool
	dryrun         bool
	faultInjection bool
	facilityCode   string
	storeKind      string
	replicas       int
)

var (
	ErrInventoryStore = errors.New("inventory store error")
)

func runWorker(ctx context.Context) {
	flasher, termCh, err := app.New(
		model.AppKindWorker,
		model.StoreKind(storeKind),
		cfgFile,
		logLevel,
		enableProfiling,
	)
	if err != nil {
		log.Fatal(err)
	}

	// serve metrics endpoint
	metrics.ListenAndServe()

	ctx, otelShutdown := otelinit.InitOpenTelemetry(ctx, "flasher")
	defer otelShutdown(ctx)

	// Setup cancel context with cancel func.
	ctx, cancelFunc := context.WithCancel(ctx)

	// routine listens for termination signal and cancels the context
	go func() {
		<-termCh
		flasher.Logger.Info("got TERM signal, exiting...")
		cancelFunc()
	}()

	repository, err := initStore(ctx, flasher.Config, flasher.Logger)
	if err != nil {
		flasher.Logger.Fatal(err)
	}

	if facilityCode == "" {
		flasher.Logger.Fatal("--facility-code parameter required")
	}

	natsURL, natsCreds, connectTimeout, err := flasher.NatsParams()
	if err != nil {
		flasher.Logger.Fatal(err)
	}

	nc := controller.NewNatsController(
		model.AppName,
		facilityCode,
		"firmwareInstall",
		natsURL,
		natsCreds,
		"firmwareInstall",
		controller.WithConcurrency(10),
		controller.WithKVReplicas(1),
		controller.WithLogger(flasher.Logger),
		controller.WithConnectionTimeout(connectTimeout),
	)

	if err := nc.Connect(ctx); err != nil {
		flasher.Logger.Fatal(err)
	}

	worker.Run(
		ctx,
		dryrun,
		faultInjection,
		repository,
		nc,
		flasher.Logger,
	)
}

func initStore(ctx context.Context, config *app.Configuration, logger *logrus.Logger) (store.Repository, error) {
	switch {
	// from CLI flags
	case strings.HasSuffix(storeKind, ".yml"), strings.HasSuffix(storeKind, ".yaml"):
		return store.NewYamlInventory(storeKind)
	case storeKind == string(model.InventoryStoreServerservice):
		return store.NewServerserviceStore(ctx, config.ServerserviceOptions, logger)
	}

	return nil, errors.Wrap(ErrInventoryStore, "expected a valid inventory store parameter")
}

func init() {
	cmdRun.PersistentFlags().StringVar(&storeKind, "store", "", "Inventory store to lookup devices for update - 'serverservice' or an inventory file with a .yml/.yaml extenstion")
	cmdRun.PersistentFlags().BoolVarP(&dryrun, "dry-run", "", false, "In dryrun mode, the worker actions the task without installing firmware")
	cmdRun.PersistentFlags().BoolVarP(&useStatusKV, "use-kv", "", false, "When this is true, flasher writes status to a NATS KV store instead of sending reply messages (requires --facility-code)")
	cmdRun.PersistentFlags().BoolVarP(&faultInjection, "fault-injection", "", false, "Tasks can include a Fault attribute to allow fault injection for development purposes")
	cmdRun.PersistentFlags().IntVarP(&replicas, "replica-count", "r", 3, "The number of replicas to use for NATS data")
	cmdRun.PersistentFlags().StringVar(&facilityCode, "facility-code", "", "The facility code this flasher instance is associated with")

	if err := cmdRun.MarkPersistentFlagRequired("store"); err != nil {
		log.Fatal(err)
	}

	rootCmd.AddCommand(cmdRun)
}
