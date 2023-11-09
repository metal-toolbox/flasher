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
	fwversion string
	component string
	file      string
	addr      string
	user      string
	pass      string
	force     bool
)

func runInstall(ctx context.Context) {
	installer := install.New(logrus.New())
	installer.Install(ctx, addr, user, pass, component, file, fwversion, dryrun)
}

func init() {
	cmdInstall.PersistentFlags().BoolVarP(&dryrun, "dry-run", "", false, "dry run install")
	cmdInstall.PersistentFlags().StringVar(&fwversion, "version", "", "The version of the firmware being installed")
	cmdInstall.PersistentFlags().StringVar(&file, "file", "", "The firmware file")
	cmdInstall.PersistentFlags().StringVar(&addr, "addr", "", "BMC host address")
	cmdInstall.PersistentFlags().StringVar(&user, "user", "", "BMC user")
	cmdInstall.PersistentFlags().StringVar(&pass, "pass", "", "BMC user password")
	cmdInstall.PersistentFlags().StringVar(&component, "component", "", "The component slug the firmware applies to")

	if err := cmdInstall.MarkPersistentFlagRequired("version"); err != nil {
		log.Fatal(err)
	}

	if err := cmdInstall.MarkPersistentFlagRequired("file"); err != nil {
		log.Fatal(err)
	}

	if err := cmdInstall.MarkPersistentFlagRequired("component"); err != nil {
		log.Fatal(err)
	}

	if err := cmdInstall.MarkPersistentFlagRequired("addr"); err != nil {
		log.Fatal(err)
	}

	if err := cmdInstall.MarkPersistentFlagRequired("user"); err != nil {
		log.Fatal(err)
	}

	if err := cmdInstall.MarkPersistentFlagRequired("pass"); err != nil {
		log.Fatal(err)
	}

	rootCmd.AddCommand(cmdInstall)
}
