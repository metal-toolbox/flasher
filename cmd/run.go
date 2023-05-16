package cmd

import (
	"context"
	"log"
	"net/http"
	"strings"

	"github.com/metal-toolbox/flasher/internal/app"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/store"
	"github.com/metal-toolbox/flasher/internal/worker"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"go.hollow.sh/toolbox/events"

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
	dryrun         bool
	faultInjection bool
	storeKind      string
)

var (
	ErrInventoryStore = errors.New("inventory store error")
)

func runWorker(ctx context.Context) {
	go func() {
		// nolint:gosec // timeouts aren't a real concern when dealing with this endpoint.
		log.Println(http.ListenAndServe("localhost:9091", nil))
	}()

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

	// Setup cancel context with cancel func.
	ctx, cancelFunc := context.WithCancel(ctx)

	// routine listens for termination signal and cancels the context
	go func() {
		<-termCh
		flasher.Logger.Info("got TERM signal, exiting...")
		cancelFunc()
	}()

	inv, err := initInventory(flasher.Config, flasher.Logger)
	if err != nil {
		flasher.Logger.Fatal(err)
	}

	stream, err := events.NewStream(*flasher.Config.NatsOptions)
	if err != nil {
		flasher.Logger.Fatal(err)
	}

	w := worker.New(
		flasher.Config.FacilityCode,
		dryrun,
		faultInjection,
		flasher.Config.Concurrency,
		stream,
		inv,
		flasher.Logger,
	)

	w.Run(ctx)
}

func initInventory(config *app.Configuration, logger *logrus.Logger) (store.Repository, error) {
	switch {
	// from CLI flags
	case strings.HasSuffix(storeKind, ".yml"), strings.HasSuffix(storeKind, ".yaml"):
		return store.NewYamlInventory(storeKind)
	case storeKind == string(model.InventoryStoreServerservice):
		return store.NewServerserviceStore(config.ServerserviceOptions, logger)
	}

	return nil, errors.Wrap(ErrInventoryStore, "expected a valid inventory store parameter")
}

func init() {
	cmdRun.PersistentFlags().StringVar(&storeKind, "store", "", "inventory store to lookup devices for update - 'serverservice' or an inventory file with a .yml/.yaml extenstion")
	cmdRun.PersistentFlags().BoolVarP(&dryrun, "dry-run", "", false, "In dryrun mode, the worker actions the task without installing firmware")
	cmdRun.PersistentFlags().BoolVarP(&faultInjection, "fault-injection", "", false, "Tasks can include a Fault attribute to allow fault injection for development purposes")

	if err := cmdRun.MarkPersistentFlagRequired("store"); err != nil {
		log.Fatal(err)
	}

	rootCmd.AddCommand(cmdRun)
}
