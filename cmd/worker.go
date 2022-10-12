package cmd

import (
	"context"
	"log"

	"github.com/metal-toolbox/flasher/internal/app"
	"github.com/metal-toolbox/flasher/internal/inventory"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/store"
	"github.com/spf13/cobra"
)

type workerFlags struct {
	installMethod     string
	inventorySource   string
	fwConfigFile      string
	inventoryYamlFile string
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

	cache := store.NewCacheStore()
	var inv inventory.Inventory

	switch flasher.Config.AppKind {
	case model.InventorySourceYaml:
		//inv, err = inventory.NewYamlInventory(workerFlagSet.inventoryYamlFile)
	case model.InventorySourceServerservice:

	}

	if err != nil {
		log.Fatal(err)
	}

	// - figure firmware configuration
	//  - on failure
	//   - serverservice
	//    - update flasher attribute in server
	//   - Yaml

	// - add device to store in state queued

	flasher.Logger.Trace("wait for goroutines..")
	flasher.SyncWG.Wait()
}

func init() {
	cmdWorker.PersistentFlags().StringVar(&workerFlagSet.inventorySource, "inventory-source", "", "inventory source to lookup devices for update - 'serverService' or 'Yaml'")
	cmdWorker.MarkPersistentFlagRequired("inventory-source")

	cmdWorker.PersistentFlags().StringVar(&workerFlagSet.inventorySource, "inventory-yaml", "", "inventory YAML containing devices and firmware configuration")

	rootCmd.AddCommand(cmdWorker)
}
