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
	Short: "Run flasher worker",
	Run: func(cmd *cobra.Command, args []string) {
		runWorker(cmd.Context())
	},
}

// run worker command
var (
	dryrun          bool
	inventorySource string
)

var (
	ErrInventorySourceUndefined = errors.New("An inventory source was not specified")
)

var cmdRunWorker = &cobra.Command{
	Use:   "worker",
	Short: "Run worker to identify and install firmware",
	Run: func(cmd *cobra.Command, args []string) {
		runWorker(cmd.Context())
	},
}

func runWorker(ctx context.Context) {
	go func() {
		log.Println(http.ListenAndServe("localhost:9091", nil))
	}()

	flasher, err := app.New(ctx, model.AppKindWorker, model.StoreKind(inventorySource), cfgFile, logLevel)
	if err != nil {
		log.Fatal(err)
	}

	// Setup cancel context with cancel func.
	// The context is used to
	ctx, cancelFunc := context.WithCancel(ctx)

	// routine listens for termination signal and cancels the context
	flasher.SyncWG.Add(1)

	go func() {
		defer flasher.SyncWG.Done()

		<-flasher.TermCh
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
		flasher.Config.FirmwareURLPrefix,
		flasher.Config.FacilityCode,
		dryrun,
		flasher.Config.Concurrency,
		stream,
		inv,
		flasher.Logger,
	)

	flasher.SyncWG.Add(1)

	go func() {
		defer flasher.SyncWG.Done()

		w.Run(ctx)
	}()

	flasher.Logger.Trace("wait for goroutines..")
	flasher.SyncWG.Wait()
}

func initInventory(ctx context.Context, config *app.Configuration, logger *logrus.Logger) (store.Repository, error) {
	switch {
	// from CLI flags
	case strings.HasSuffix(inventorySource, ".yml"), strings.HasSuffix(inventorySource, ".yaml"):
		return store.NewYamlInventory(inventorySource)
	case inventorySource == string(model.InventoryStoreServerservice):
		return store.NewServerserviceStore(ctx, config.ServerserviceOptions, logger)
	default:

	}

	return nil, errors.Wrap(ErrInventorySourceUndefined, "expected a valid parameter through CLI or configuration file")
}

func init() {
	cmdRunWorker.PersistentFlags().StringVar(&inventorySource, "inventory-source", "", "inventory source to lookup devices for update - 'serverservice' or an inventory file with a .yml/.yaml extenstion")
	cmdRunWorker.PersistentFlags().BoolVarP(&dryrun, "dry-run", "", false, "In dryrun mode, the worker actions the task without installing firmware")

	if err := cmdRunWorker.MarkPersistentFlagRequired("inventory-source"); err != nil {
		log.Fatal(err)
	}

	cmdRun.AddCommand(cmdRunWorker)
	rootCmd.AddCommand(cmdRun)
}
