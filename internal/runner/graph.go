package runner

import (
	"github.com/emicklei/dot"
	model "github.com/metal-toolbox/flasher/internal/model"
)

func Graph() *dot.Graph {
	g := dot.NewGraph(dot.Directed)
	// add task states as nodes
	pending := g.Node(string(model.StatePending))
	active := g.Node(string(model.StateActive))
	succeeded := g.Node(string(model.StateSucceeded))
	failed := g.Node(string(model.StateFailed))

	// draw edges between states
	g.Edge(pending, active, "Task active")
	g.Edge(active, failed, "Invalid task parameters")

	init := g.Node("Initialize")
	query := g.Node("Query")
	plan := g.Node("Plan")
	run := g.Node("Run")

	g.Edge(active, init)
	g.Edge(init, query)
	g.Edge(query, succeeded, "Installed firmware equals expected")
	g.Edge(query, plan, "Query for installed firmware")
	g.Edge(plan, run, "Plan actions")

	return g
}
