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
type workerFlags struct {
	inventorySource string
}

var (
	workerFlagSet = &workerFlags{}
)

var cmdWorker = &cobra.Command{
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

	flasher, err := app.New(ctx, model.AppKindWorker, workerFlagSet.inventorySource, cfgFile, logLevel)
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

	var inv inventory.Inventory

	switch {
	case strings.HasSuffix(workerFlagSet.inventorySource, ".yml"), strings.HasSuffix(workerFlagSet.inventorySource, ".yaml"):
		inv, err = inventory.NewYamlInventory(workerFlagSet.inventorySource)
		if err != nil {
			log.Fatal(err)
		}
	case workerFlagSet.inventorySource == model.InventorySourceServerservice:
		inv, err = inventory.NewServerserviceInventory(flasher.Config)
		if err != nil {
			log.Fatal(err)
		}
	}

	if err != nil {
		log.Fatal(err)
	}

	concurrency := 2
	w := worker.New(concurrency, flasher.SyncWG, store.NewMemStore(), inv, flasher.Logger)

	flasher.SyncWG.Add(1)

	go func() {
		defer flasher.SyncWG.Done()

		w.Run(ctx)
	}()

	flasher.Logger.Trace("wait for goroutines..")
	flasher.SyncWG.Wait()
}

func init() {
	cmdWorker.PersistentFlags().StringVar(&workerFlagSet.inventorySource, "inventory-source", "", "inventory source to lookup devices for update - 'serverservice' or an inventory file with a .yml/.yaml extenstion")

	if err := cmdWorker.MarkPersistentFlagRequired("inventory-source"); err != nil {
		log.Fatal(err)
	}

	cmdRun.AddCommand(cmdWorker)
	rootCmd.AddCommand(cmdRun)
}
