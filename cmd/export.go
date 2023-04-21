package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	sw "github.com/filanov/stateswitch"
	"github.com/metal-toolbox/flasher/internal/model"
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
		exportStatemachine(cmd.Context())
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

func exportOutofbandActionStatemachine(ctx context.Context) {
	m, err := outofband.NewActionStateMachine(ctx, "dummy")
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

func (h *mockTaskHandler) PublishStatus(t sw.StateSwitch, args sw.TransitionArgs) error {
	return nil
}
