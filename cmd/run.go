package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/go-logr/zapr"
	"github.com/google/uuid"
	"github.com/metal-toolbox/ctrl"
	"github.com/metal-toolbox/flasher/internal/app"
	"github.com/metal-toolbox/flasher/internal/metrics"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/otel"
	"github.com/metal-toolbox/flasher/internal/store"
	"github.com/metal-toolbox/flasher/internal/worker"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	rctypes "github.com/metal-toolbox/rivets/v2/condition"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	// nolint:gosec // profiling endpoint listens on localhost.
	_ "net/http/pprof"
)

var cmdRun = &cobra.Command{
	Use:   "run",
	Short: "Run flasher service to listen for events and install firmware",
	Run: func(cmd *cobra.Command, _ []string) {
		var mode model.RunMode
		if runsInband {
			mode = model.RunInband
		} else {
			mode = model.RunOutofband
		}

		runWorker(cmd.Context(), mode)
	},
}

// run worker command
var (
	dryrun         bool
	runsInband     bool
	runsOutofband  bool
	faultInjection bool
	facilityCode   string
	storeKind      string
	inbandServerID string
)

var (
	ErrInventoryStore = errors.New("inventory store error")
)

func runWorker(ctx context.Context, mode model.RunMode) {
	flasher, termCh, err := app.New(
		model.AppKindWorker,
		model.StoreKind(storeKind),
		cfgFile,
		logLevel,
		enableProfiling,
		mode,
	)
	if err != nil {
		log.Fatal(err)
	}

	// serve metrics endpoint
	metrics.ListenAndServe()

	isEnv := os.Getenv("OTEL_EXPORTER_OTLP_INSECURE")
	insecure, err := strconv.ParseBool(isEnv)
	if err != nil {
		insecure = false
		flasher.Logger.Error(err, "Invalid boolean value in OTEL_EXPORTER_OTLP_INSECURE. Try true or false.")
	}

	config := zap.NewProductionConfig()
	config.OutputPaths = []string{"stdout"}
	switch logLevel {
	case "debug":
		config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	default:
		config.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	}
	zapLogger, err := config.Build()
	if err != nil {
		panic(fmt.Sprintf("who watches the watchmen (%v)?", err))
	}
	lg := zapr.NewLogger(zapLogger)

	oCfg := otel.Config{
		Servicename: "flasher-" + string(mode),
		Endpoint:    os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		Insecure:    insecure,
		Logger:      lg,
	}
	ctx, otelShutdown, err := otel.Init(ctx, oCfg)
	if err != nil {
		flasher.Logger.Error(err, "failed to initialize OpenTelemetry")
	}
	defer otelShutdown()

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

	switch mode {
	case model.RunInband:
		runInband(ctx, flasher, repository)
		return
	case model.RunOutofband:
		runOutofband(ctx, flasher, repository)
		return
	default:
		flasher.Logger.Fatal("unsupported run mode: " + mode)
	}
}

func runOutofband(ctx context.Context, flasher *app.App, repository store.Repository) {
	natsCfg, err := flasher.NatsParams()
	if err != nil {
		flasher.Logger.Fatal(err)
	}

	nc := ctrl.NewNatsController(
		model.AppName,
		facilityCode,
		string(rctypes.FirmwareInstall),
		natsCfg.NatsURL,
		natsCfg.CredsFile,
		rctypes.FirmwareInstall,
		ctrl.WithConcurrency(flasher.Config.Concurrency),
		ctrl.WithKVReplicas(natsCfg.KVReplicas),
		ctrl.WithLogger(flasher.Logger),
		ctrl.WithConnectionTimeout(natsCfg.ConnectTimeout),
	)

	if err := nc.Connect(ctx); err != nil {
		flasher.Logger.Fatal(err)
	}

	worker.RunOutofband(
		ctx,
		dryrun,
		faultInjection,
		repository,
		nc,
		flasher.Logger,
	)
}

func runInband(ctx context.Context, flasher *app.App, repository store.Repository) {
	cfgOrcAPI := flasher.Config.OrchestratorAPIParams
	orcConfig := &ctrl.OrchestratorAPIConfig{
		Endpoint:             cfgOrcAPI.Endpoint,
		AuthDisabled:         cfgOrcAPI.AuthDisabled,
		OidcIssuerEndpoint:   cfgOrcAPI.OidcIssuerEndpoint,
		OidcAudienceEndpoint: cfgOrcAPI.OidcAudienceEndpoint,
		OidcClientSecret:     cfgOrcAPI.OidcClientSecret,
		OidcClientID:         cfgOrcAPI.OidcClientID,
		OidcClientScopes:     cfgOrcAPI.OidcClientScopes,
	}

	nc, err := ctrl.NewHTTPController(
		"flasher-inband",
		facilityCode,
		uuid.MustParse(flasher.Config.ServerID),
		rctypes.FirmwareInstallInband,
		orcConfig,
		ctrl.WithNATSHTTPLogger(flasher.Logger),
	)
	if err != nil {
		flasher.Logger.Fatal(err)
	}

	worker.RunInband(
		ctx,
		dryrun,
		faultInjection,
		facilityCode,
		repository,
		nc,
		flasher.Logger,
	)
}

func initStore(ctx context.Context, config *app.Configuration, logger *logrus.Logger) (store.Repository, error) {
	if storeKind == string(model.InventoryStoreServerservice) {
		return store.NewServerserviceStore(ctx, config.FleetDBAPIOptions, logger)
	}

	return nil, errors.Wrap(ErrInventoryStore, "expected a valid inventory store parameter")
}

func init() {
	cmdRun.PersistentFlags().StringVar(&storeKind, "store", "", "Inventory store to lookup devices for update - fleetdb.")
	cmdRun.PersistentFlags().StringVar(&inbandServerID, "server-id", "", "ServerID when running inband")
	cmdRun.PersistentFlags().BoolVarP(&dryrun, "dry-run", "", false, "In dryrun mode, the worker actions the task without installing firmware")
	cmdRun.PersistentFlags().BoolVarP(&runsInband, "inband", "", false, "Runs worker in inband firmware install mode")
	cmdRun.PersistentFlags().BoolVarP(&runsOutofband, "outofband", "", false, "Runs worker in out-of-band firmware install mode")
	cmdRun.PersistentFlags().BoolVarP(&faultInjection, "fault-injection", "", false, "Tasks can include a Fault attribute to allow fault injection for development purposes")
	cmdRun.PersistentFlags().StringVar(&facilityCode, "facility-code", "", "The facility code this flasher instance is associated with")

	if err := cmdRun.MarkPersistentFlagRequired("store"); err != nil {
		log.Fatal(err)
	}

	cmdRun.MarkFlagsMutuallyExclusive("inband", "outofband")
	cmdRun.MarkFlagsOneRequired("inband", "outofband")

	rootCmd.AddCommand(cmdRun)
}
