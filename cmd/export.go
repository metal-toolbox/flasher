package cmd

import (
	"context"
	"fmt"
	"log"
	"os"

	sw "github.com/filanov/stateswitch"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/outofband"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
	"github.com/nikolaydubina/jsonl-graph/dot"
	"github.com/nikolaydubina/jsonl-graph/graph"
	"github.com/spf13/cobra"
	// "github.com/nikolaydubina/jsonl-graph/graph"
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
}

var (
	exportFlagSet = &exportFlags{}
)

var cmdExportStatemachine = &cobra.Command{
	Use:   "statemachine --task|--action",
	Short: "Export a JSON dump of flasher statemachine(s) - writes to a file task-statemachine.json",
	Run: func(cmd *cobra.Command, args []string) {
		exportStatemachine(cmd.Context())
	},
}

func exportStatemachine(ctx context.Context) {

	if exportFlagSet.task {
		exportTaskStatemachine(ctx)
		os.Exit(0)
	}

	if exportFlagSet.action {
		exportOutofbandActionStatemachine(ctx)
	}
}

func exportTaskStatemachine(ctx context.Context) {
	task, err := model.NewTask("", nil)
	if err != nil {
		log.Fatal(err)
	}

	m, err := sm.NewTaskStateMachine(ctx, &task, &mockTaskHandler{})
	if err != nil {
		log.Fatal(err)
	}

	json, err := m.DescribeAsJSON()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(json))
}

func exportOutofbandActionStatemachine(ctx context.Context) {
	m, err := outofband.NewActionStateMachine(ctx, "dummy")
	if err != nil {
		log.Fatal(err)
	}

	sm := m.Describe()

	g := graph.NewGraph()
	for _, tnode := range sm.TransitionRuleNodes {

		//NodeData: NodeData(map[string]interface{}{"id": 123}),
		g.AddNode(graph.NodeData{tnode.ID: tnode.Description})
	}

	for _, tedge := range sm.TransitionRuleEdges {
		g.AddEdge(graph.EdgeData{"from": tedge.From, "to": tedge.To, "label": tedge.Name})
	}

	dotg := dot.NewBasicGraph(g, dot.TB)

	// TODO: add this graph as a subgraph to the task statemachine
	// Requires:
	// https://github.com/nikolaydubina/jsonl-graph/pull/16
	// https://github.com/nikolaydubina/jsonl-graph/issues/17

	fmt.Println(dotg.Render())
	//g, err := graph.NewGraphFromJSONL(bytes.NewReader(want))
	//if err != nil {
	//	log.Fatal(err)
	//}

	//fmt.Println(g.String())
}

func init() {
	cmdExportStatemachine.PersistentFlags().BoolVarP(&exportFlagSet.action, "action", "", false, "export action statemachine as JSON")
	cmdExportStatemachine.PersistentFlags().BoolVarP(&exportFlagSet.task, "task", "", false, "export task statemachine as JSON")

	cmdExport.AddCommand(cmdExportStatemachine)
	rootCmd.AddCommand(cmdExport)
}

// mockTaskHandler implements the TaskTransitioner interface
type mockTaskHandler struct{}

func (h *mockTaskHandler) Query(t sw.StateSwitch, args sw.TransitionArgs) error {
	return nil
}

func (h *mockTaskHandler) Plan(t sw.StateSwitch, args sw.TransitionArgs) error {
	return nil
}

// planFromFirmwareSet
func (h *mockTaskHandler) planFromFirmwareSet(tctx *sm.HandlerContext, task *model.Task, device model.Device) error {
	return nil
}

func (h *mockTaskHandler) ValidatePlan(t sw.StateSwitch, args sw.TransitionArgs) (bool, error) {
	return true, nil
}

func (h *mockTaskHandler) Run(t sw.StateSwitch, args sw.TransitionArgs) error {
	return nil
}

func (h *mockTaskHandler) TaskFailed(task sw.StateSwitch, args sw.TransitionArgs) error {
	return nil
}

func (h *mockTaskHandler) TaskSuccessful(task sw.StateSwitch, args sw.TransitionArgs) error {
	return nil
}

func (h *mockTaskHandler) PersistState(t sw.StateSwitch, args sw.TransitionArgs) error {
	return nil
}
