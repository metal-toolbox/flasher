package cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	bconsts "github.com/bmc-toolbox/bmclib/v2/constants"
	sw "github.com/filanov/stateswitch"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/outofband"
	"github.com/spf13/cobra"

	"github.com/emicklei/dot"
)

type exportFlags struct {
	actionSM bool
	taskSM   bool
	mermaid  bool
	json     bool
}

var (
	exportFlagSet = &exportFlags{}
)

var cmdExportStatemachine = &cobra.Command{
	Use:   "export-statemachine --task|--action [--json|--mermaid]",
	Short: "Export a JSON dump of flasher statemachine(s) - writes to a file task-statemachine.json",
	Run: func(_ *cobra.Command, _ []string) {
		exportStatemachine()
	},
}

func asGraph(s *sw.StateMachineJSON) *dot.Graph {
	g := dot.NewGraph(dot.Directed)
	nodes := map[string]dot.Node{}

	for _, transition := range s.TransitionRules {
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

	return g
}

func conditionStateMachine() {
	g := dot.NewGraph(dot.Directed)

	pending := g.Node(string(model.StatePending))
	active := g.Node(string(model.StateActive))
	succeeded := g.Node(string(model.StateSucceeded))
	failed := g.Node(string(model.StateFailed))

	g.Edge(pending, active, "Task active")
	g.Edge(active, succeeded, "Task successful")
	g.Edge(active, failed, "Task failed")

	fmt.Println(dot.MermaidGraph(g, dot.MermaidTopDown))
}

func outofbandActionStatemachine() {
	steps := []bconsts.FirmwareInstallStep{
		bconsts.FirmwareInstallStepUploadInitiateInstall,
		bconsts.FirmwareInstallStepInstallStatus,
	}

	m, err := outofband.NewActionStateMachine("dummy", steps, true)
	if err != nil {
		log.Fatal(err)
	}

	j, err := m.DescribeAsJSON()
	if err != nil {
		log.Fatal(err)
	}

	if exportFlagSet.json {
		fmt.Println(string(j))
		os.Exit(0)
	}

	t := &sw.StateMachineJSON{}
	if err := json.Unmarshal(j, t); err != nil {
		log.Fatal(err)
	}

	fmt.Println(dot.MermaidGraph(asGraph(t), dot.MermaidTopDown))
}

func exportStatemachine() {
	if exportFlagSet.taskSM {
		conditionStateMachine()

		return
	}

	if exportFlagSet.actionSM {
		outofbandActionStatemachine()

		return
	}

	log.Println("expected --task OR --action flag")
	os.Exit(1)
}

func init() {
	cmdExportStatemachine.PersistentFlags().BoolVarP(&exportFlagSet.taskSM, "task", "", false, "export Task main statemachine")
	cmdExportStatemachine.PersistentFlags().BoolVarP(&exportFlagSet.actionSM, "action", "", false, "export action sub-statemachine")
	cmdExportStatemachine.PersistentFlags().BoolVarP(&exportFlagSet.mermaid, "mermaid", "", true, "export statemachine in mermaid format")
	cmdExportStatemachine.PersistentFlags().BoolVarP(&exportFlagSet.json, "json", "", false, "export task statemachine in the JSON format")

	rootCmd.AddCommand(cmdExportStatemachine)
}
