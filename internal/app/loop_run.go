package app

import (
	"fmt"
	"slices"

	"github.com/Gabes-Tint/mega-agents/internal/engine"
)

const (
	donePort      = "done"
	exhaustedPort = "exhausted"

	defaultLoopIterations = 3
	maxLoopIterations     = 20
)

// loopTask repeats the blocks inside a loop until the block chosen to end it
// takes the chosen output, at most its number of iterations. Each iteration
// receives the arrows into the loop afresh; the loop sends the ending value
// on done, or the last value on exhausted, and passes a workspace on. The
// failures of the blocks inside are collected with theirs.
func (planner runPlanner) loopTask(node WorkflowNodeInput, failures *[]planFailure) (engine.Task, error) {
	fail := func(format string, args ...any) (engine.Task, error) {
		return engine.Task{}, fmt.Errorf("%s: %s", node.Name, fmt.Sprintf(format, args...))
	}
	var children []WorkflowNodeInput
	for _, child := range planner.childrenOf[node.ID] {
		if executableTypes[child.Type] {
			children = append(children, child)
		}
	}
	if len(children) == 0 {
		return fail("put the blocks to repeat inside the loop")
	}
	iterations := defaultLoopIterations
	if node.MaxIterations != nil {
		iterations = *node.MaxIterations
	}
	if iterations < 1 || iterations > maxLoopIterations {
		return fail("repeat between 1 and %d times", maxLoopIterations)
	}
	until := planner.nodeByID[node.UntilNode]
	if until.ParentID != node.ID || node.UntilPort == "" {
		return fail("choose the block and output that end the loop")
	}
	if ports := outputPortsOf(until); !slices.Contains(ports, node.UntilPort) {
		return fail("%s has no output %q; use %s", until.Name, node.UntilPort, joinChoices(ports))
	}
	needs, err := planner.incomingNeeds(node)
	if err != nil {
		return engine.Task{}, err
	}
	var body []engine.Task
	for _, child := range children {
		var task engine.Task
		switch child.Type {
		case "agent":
			task, err = planner.agentTask(child)
		case "jsonschema":
			task, err = planner.schemaTask(child)
		case "router":
			task, err = planner.routerTask(child)
		default:
			task, err = planner.commandTask(child)
		}
		if err != nil {
			*failures = append(*failures, planFailure{nodeID: child.ID, err: err})
			continue
		}
		body = append(body, task)
	}
	handlesExhausted := false
	for _, edge := range planner.request.Edges {
		if edge.From == node.ID {
			port, _ := planner.sourcePort(edge)
			handlesExhausted = handlesExhausted || port == exhaustedPort
		}
	}
	return engine.Task{
		ID: node.ID, Name: node.Name, Kind: "loop", Needs: needs,
		Loop: &engine.Loop{
			Body: body, MaxIterations: iterations, Until: engine.Need{TaskID: until.ID, Port: node.UntilPort},
			Finish: func(inputs []engine.Input, outcome engine.LoopOutcome) (engine.Result, error) {
				details := map[string]any{"maxIterations": iterations}
				outputs := map[string]any{}
				if workspace, ok := workspaceIn(inputs); ok {
					outputs[workspacePort] = workspace
				}
				if outcome.Ended {
					outputs[donePort] = outcome.Outputs[node.UntilPort]
					return engine.Result{Outputs: outputs, Details: details}, nil
				}
				if !handlesExhausted {
					return engine.Result{Details: details}, fmt.Errorf(
						"%s did not end after %d iterations: %s never took %s", node.Name, outcome.Iterations, until.Name, node.UntilPort,
					)
				}
				outputs[exhaustedPort] = lastValue(outcome.Outputs, node.UntilPort)
				return engine.Result{Outputs: outputs, Details: details}, nil
			},
		},
	}, nil
}

// lastValue is what the block ending a loop put out instead of the ending
// output, such as a gate's failure.
func lastValue(outputs map[string]any, untilPort string) any {
	for port, value := range outputs {
		if port != workspacePort && port != untilPort {
			return value
		}
	}
	return nil
}

// outputPortsOf names the outputs a block chooses between.
func outputPortsOf(node WorkflowNodeInput) []string {
	switch node.Type {
	case "jsonschema":
		return []string{validPort, invalidPort}
	case "command":
		return []string{passedPort, failedPort}
	case "router":
		var routes []string
		for _, routeCase := range node.Cases {
			routes = append(routes, routeCase.Name)
		}
		return append(routes, "default")
	}
	return []string{}
}
