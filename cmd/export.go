package cmd

import (
	"fmt"
	"log"

	"github.com/metal-toolbox/flasher/internal/outofband"
	"github.com/metal-toolbox/flasher/internal/runner"
	"github.com/spf13/cobra"

	"github.com/emicklei/dot"
)

var cmdExportFlowDiagram = &cobra.Command{
	Use:   "export-diagram",
	Short: "Export mermaidjs flowchart for flasher task transitions",
	Run: func(cmd *cobra.Command, _ []string) {
		g := runner.Graph()
		if err := outofband.GraphSteps(cmd.Context(), g); err != nil {
			log.Fatal(err)
		}

		fmt.Println(dot.MermaidGraph(g, dot.MermaidTopDown))
	},
}

func init() {
	rootCmd.AddCommand(cmdExportFlowDiagram)
}
