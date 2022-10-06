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

type outofbandFlags struct {
	inventorySource   string
	fwConfigFile      string
	inventoryYamlFile string
}

var (
	outofbandFlagsSet = &outofbandFlags{}
)

var cmdOutofband = &cobra.Command{
	Use:   "outofband",
	Short: "Install firmware out-of-band",
	Run: func(cmd *cobra.Command, args []string) {
		runOutofband(cmd.Context())
	},
}

func runOutofband(ctx context.Context) {
	flasher, err := app.New(ctx, model.AppKindOutofband, outofbandFlagsSet.inventorySource, cfgFile, store.NewCacheStore())
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
		//inv, err = inventory.NewYamlInventory(outofbandFlagsSet.inventoryYamlFile)
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
	cmdOutofband.PersistentFlags().StringVar(&outofbandFlagsSet.inventorySource, "inventory-source", "", "inventory source to lookup devices for update - 'serverService' or 'Yaml'")
	cmdOutofband.MarkPersistentFlagRequired("inventory-source")

	cmdOutofband.PersistentFlags().StringVar(&outofbandFlagsSet.inventorySource, "inventory-yaml", "", "inventory YAML containing devices and firmware configuration")

	rootCmd.AddCommand(cmdOutofband)
}
