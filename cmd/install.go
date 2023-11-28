package cmd

import (
	"context"
	"log"

	"github.com/metal-toolbox/flasher/internal/app"
	"github.com/metal-toolbox/flasher/internal/install"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/spf13/cobra"
)

var cmdInstall = &cobra.Command{
	Use:   "install",
	Short: "Install given firmware for a component",
	Run: func(cmd *cobra.Command, args []string) {
		runInstall(cmd.Context())
	},
}

var (
	fwvendor  string
	fwmodel   string
	fwversion string
	component string
	file      string
	addr      string
	user      string
	pass      string
	force     bool
)

func runInstall(ctx context.Context) {
	flasher, termCh, err := app.New(
		model.AppKindCLI,
		"",
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

	p := &install.Params{
		DryRun:    dryrun,
		Version:   fwversion,
		File:      file,
		Component: component,
		Model:     fwmodel,
		Vendor:    fwvendor,
		User:      user,
		Pass:      pass,
		BmcAddr:   addr,
		Force:     force,
	}

	installer := install.New(flasher.Logger)

	installer.Install(ctx, p)
}

func init() {
	cmdInstall.Flags().BoolVarP(&dryrun, "dry-run", "", false, "dry run install")
	cmdInstall.Flags().BoolVarP(&force, "force", "", false, "force install, skip checking existing version")
	cmdInstall.Flags().StringVar(&fwversion, "version", "", "The version of the firmware being installed")
	cmdInstall.Flags().StringVar(&file, "file", "", "The firmware file")
	cmdInstall.Flags().StringVar(&addr, "addr", "", "BMC host address")
	cmdInstall.Flags().StringVar(&user, "user", "", "BMC user")
	cmdInstall.Flags().StringVar(&fwvendor, "vendor", "", "Component vendor")
	cmdInstall.Flags().StringVar(&fwmodel, "model", "", "Component model")
	cmdInstall.Flags().StringVar(&pass, "pass", "", "BMC user password")
	cmdInstall.Flags().StringVar(&component, "component", "", "The component slug the firmware applies to")

	required := []string{"version", "file", "component", "addr", "user", "pass", "vendor", "model"}
	for _, r := range required {
		if err := cmdInstall.MarkFlagRequired(r); err != nil {
			log.Fatal(err)
		}
	}

	rootCmd.AddCommand(cmdInstall)
}
