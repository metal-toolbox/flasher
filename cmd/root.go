/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/spf13/cobra"
)

var (
	logLevel int
	trace    bool
	debug    bool
	cfgFile  string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "flasher",
	Short: "flasher installs firmware",
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	switch {
	case debug:
		logLevel = model.LogLevelDebug
	case trace:
		logLevel = model.LogLevelTrace
	default:
		logLevel = model.LogLevelInfo
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.flasher.yml)")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug level logging")
	rootCmd.PersistentFlags().BoolVar(&trace, "trace", false, "enable trace level logging")
}
