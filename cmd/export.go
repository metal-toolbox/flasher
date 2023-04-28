package cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	sw "github.com/filanov/stateswitch"
	"github.com/metal-toolbox/flasher/internal/outofband"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
	"github.com/spf13/cobra"

	"github.com/emicklei/dot"
)

var cmdExport = &cobra.Command{
	Use:   "export",
	Short: "export resources [statemachine]",
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

type exportFlags struct {
	action bool
	task   bool
	json   bool
}

var (
	exportFlagSet = &exportFlags{}
)

var cmdExportStatemachine = &cobra.Command{
	Use:   "statemachine --task|--action",
	Short: "Export a JSON dump of flasher statemachine(s) - writes to a file task-statemachine.json",
	Run: func(cmd *cobra.Command, args []string) {
		exportStatemachine()
	},
}

func exportAsDot(data []byte) (string, error) {
	smJSON := &sw.StateMachineJSON{}

	if err := json.Unmarshal(data, smJSON); err != nil {
		return "", err
	}

	g := dot.NewGraph(dot.Directed)
	nodes := map[string]dot.Node{}

	for _, transition := range smJSON.TransitionRules {
		_, exists := nodes[transition.DestinationState]
		if !exists {
			nodes[transition.DestinationState] = g.Node(transition.DestinationState)
		}

		for _, sourceState := range transition.SourceStates {
			_, exists := nodes[sourceState]
			if !exists {
				nodes[sourceState] = g.Node(sourceState)
			}

			g.Edge(nodes[sourceState], nodes[transition.DestinationState], transition.Name)
		}
	}

	return g.String(), nil
}

func exportStatemachine() {
	if exportFlagSet.task {
		exportTaskStatemachine()
		os.Exit(0)
	}

	if exportFlagSet.action {
		exportOutofbandActionStatemachine()
	}
}

func exportTaskStatemachine() {
	handler := &sm.MockTaskHandler{}

	m, err := sm.NewTaskStateMachine(handler)
	if err != nil {
		log.Fatal(err)
	}

	data, err := m.DescribeAsJSON()
	if err != nil {
		log.Fatal(err)
	}

	if exportFlagSet.json {
		fmt.Println(string(data))
		os.Exit(0)
	}

	val, err := exportAsDot(data)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(val)
}

func exportOutofbandActionStatemachine() {
	m, err := outofband.NewActionStateMachine("dummy")
	if err != nil {
		log.Fatal(err)
	}

	data, err := m.DescribeAsJSON()
	if err != nil {
		log.Fatal(err)
	}

	if exportFlagSet.json {
		fmt.Println(string(data))
		os.Exit(0)
	}

	val, err := exportAsDot(data)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(val)
}

func init() {
	cmdExportStatemachine.PersistentFlags().BoolVarP(&exportFlagSet.action, "action", "", false, "export action statemachine in the Graphviz Dot format")
	cmdExportStatemachine.PersistentFlags().BoolVarP(&exportFlagSet.task, "task", "", false, "export task statemachine in the Graphviz Dot format")
	cmdExportStatemachine.PersistentFlags().BoolVarP(&exportFlagSet.json, "json", "", false, "export task statemachine in the JSON format")

	cmdExport.AddCommand(cmdExportStatemachine)
	rootCmd.AddCommand(cmdExport)
}
