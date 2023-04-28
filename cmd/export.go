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
	Run: func(cmd *cobra.Command, args []string) {
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

func taskStateMachine() {
	handler := &sm.MockTaskHandler{}

	m, err := sm.NewTaskStateMachine(handler)
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

func outofbandActionStatemachine() {
	m, err := outofband.NewActionStateMachine("dummy")
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
		taskStateMachine()
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
