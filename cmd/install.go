package cmd

import (
	"context"
	"log"

	"github.com/metal-toolbox/flasher/internal/install"
	"github.com/sirupsen/logrus"
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
	l := logrus.New()
	l.Level = logrus.TraceLevel + 1

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
	}

	installer := install.New(l)

	installer.Install(ctx, p)
}

func init() {
	cmdInstall.PersistentFlags().BoolVarP(&dryrun, "dry-run", "", false, "dry run install")
	cmdInstall.PersistentFlags().BoolVarP(&force, "force", "", false, "force install, skip checking existing version")
	cmdInstall.PersistentFlags().StringVar(&fwversion, "version", "", "The version of the firmware being installed")
	cmdInstall.PersistentFlags().StringVar(&file, "file", "", "The firmware file")
	cmdInstall.PersistentFlags().StringVar(&addr, "addr", "", "BMC host address")
	cmdInstall.PersistentFlags().StringVar(&user, "user", "", "BMC user")
	cmdInstall.PersistentFlags().StringVar(&fwvendor, "vendor", "", "Component vendor")
	cmdInstall.PersistentFlags().StringVar(&fwmodel, "model", "", "Component model")
	cmdInstall.PersistentFlags().StringVar(&pass, "pass", "", "BMC user password")
	cmdInstall.PersistentFlags().StringVar(&component, "component", "", "The component slug the firmware applies to")

	required := []string{"version", "file", "component", "addr", "user", "pass", "vendor", "model"}
	for _, r := range required {
		if err := cmdInstall.MarkPersistentFlagRequired(r); err != nil {
			log.Fatal(err)
		}
	}

	rootCmd.AddCommand(cmdInstall)
}
