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
	inventoryStore string
)

var (
	ErrInventoryStore = errors.New("inventory store error")
)

func runWorker(ctx context.Context) {
	go func() {
		log.Println(http.ListenAndServe("localhost:9091", nil))
	}()

	flasher, termCh, err := app.New(
		model.AppKindWorker,
		model.StoreKind(inventoryStore),
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

	inv, err := initInventory(ctx, flasher.Config, flasher.Logger)
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
		flasher.Config.Concurrency,
		stream,
		inv,
		flasher.Logger,
	)

	w.Run(ctx)
}

func initInventory(ctx context.Context, config *app.Configuration, logger *logrus.Logger) (store.Repository, error) {
	switch {
	// from CLI flags
	case strings.HasSuffix(inventoryStore, ".yml"), strings.HasSuffix(inventoryStore, ".yaml"):
		return store.NewYamlInventory(inventoryStore)
	case inventoryStore == string(model.InventoryStoreServerservice):
		return store.NewServerserviceStore(ctx, config.ServerserviceOptions, logger)
	}

	return nil, errors.Wrap(ErrInventoryStore, "expected a valid inventory store parameter")
}

func init() {
	cmdRun.PersistentFlags().StringVar(&inventoryStore, "store", "", "inventory store to lookup devices for update - 'serverservice' or an inventory file with a .yml/.yaml extenstion")
	cmdRun.PersistentFlags().BoolVarP(&dryrun, "dry-run", "", false, "In dryrun mode, the worker actions the task without installing firmware")

	if err := cmdRun.MarkPersistentFlagRequired("store"); err != nil {
		log.Fatal(err)
	}

	rootCmd.AddCommand(cmdRun)
}
