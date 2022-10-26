package cmd

import (
	"context"
	"log"
	"strings"

	"github.com/metal-toolbox/flasher/internal/app"
	"github.com/metal-toolbox/flasher/internal/inventory"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/store"
	"github.com/metal-toolbox/flasher/internal/worker"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var cmdRun = &cobra.Command{
	Use:   "run",
	Short: "Run flasher worker",
	Run: func(cmd *cobra.Command, args []string) {
		runWorker(cmd.Context())
	},
}

// run worker command
type workerRunFlags struct {
	dryrun          bool
	inventorySource string
}

var (
	workerRunFlagSet            = &workerRunFlags{}
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
	var logLevel int

	switch {
	case debug:
		logLevel = model.LogLevelDebug
	case trace:
		logLevel = model.LogLevelTrace
	default:
		logLevel = model.LogLevelInfo
	}

	flasher, err := app.New(ctx, model.AppKindWorker, workerRunFlagSet.inventorySource, cfgFile, logLevel)
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
		log.Fatal(err)
	}

	concurrency := 2
	w := worker.New(workerRunFlagSet.dryrun, concurrency, flasher.SyncWG, store.NewMemStore(), inv, flasher.Logger)

	flasher.SyncWG.Add(1)

	go func() {
		defer flasher.SyncWG.Done()

		w.Run(ctx)
	}()

	flasher.Logger.Trace("wait for goroutines..")
	flasher.SyncWG.Wait()
}

func initInventory(ctx context.Context, config *model.Config, logger *logrus.Logger) (inventory.Inventory, error) {
	switch {
	// from CLI flags
	case strings.HasSuffix(workerRunFlagSet.inventorySource, ".yml"), strings.HasSuffix(workerRunFlagSet.inventorySource, ".yaml"):
		return inventory.NewYamlInventory(workerRunFlagSet.inventorySource)
	case workerRunFlagSet.inventorySource == model.InventorySourceServerservice:
		return inventory.NewServerserviceInventory(ctx, config, logger)
	// from config file
	case strings.HasSuffix(config.InventorySource, ".yml"), strings.HasSuffix(config.InventorySource, ".yaml"):
		return inventory.NewYamlInventory(workerRunFlagSet.inventorySource)
	case config.InventorySource == model.InventorySourceServerservice:
		return inventory.NewServerserviceInventory(ctx, config, logger)
	default:

	}

	return nil, errors.Wrap(ErrInventorySourceUndefined, "expected a valid parameter through CLI or configuration file")
}

func init() {
	cmdRunWorker.PersistentFlags().StringVar(&workerRunFlagSet.inventorySource, "inventory-source", "", "inventory source to lookup devices for update - 'serverservice' or an inventory file with a .yml/.yaml extenstion")
	cmdRunWorker.PersistentFlags().BoolVarP(&workerRunFlagSet.dryrun, "dry-run", "", false, "In dryrun mode, the worker actions the task without installing firmware")

	if err := cmdRunWorker.MarkPersistentFlagRequired("inventory-source"); err != nil {
		log.Fatal(err)
	}

	cmdRun.AddCommand(cmdRunWorker)
	rootCmd.AddCommand(cmdRun)
}
