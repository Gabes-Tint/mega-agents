package app

import (
	"context"
	"fmt"
	"io"

	"github.com/Gabes-Tint/mega-agents/internal/engine"
	"github.com/Gabes-Tint/mega-agents/internal/router"
	"github.com/Gabes-Tint/mega-agents/internal/schema"
)

// routerTask plans a Router block: its CEL cases are checked now, and at
// run time the one value it receives leaves on the first route whose case
// holds, or on default, as {"case": route, "value": value}. Arrows on the
// other routes are skipped.
func (planner runPlanner) routerTask(node WorkflowNodeInput) (engine.Task, error) {
	compiled, err := router.Compile(node.Cases)
	if err != nil {
		return engine.Task{}, fmt.Errorf("%s: %w", node.Name, err)
	}
	needs, err := planner.valueNeeds(node, "routes")
	if err != nil {
		return engine.Task{}, err
	}
	return engine.Task{
		ID: node.ID, Name: node.Name, Kind: "router", Needs: needs,
		Run: func(_ context.Context, inputs []engine.Input, log io.Writer) (engine.Result, error) {
			value, err := schema.Normalize(inputs[0].Value)
			if err != nil {
				return engine.Result{}, fmt.Errorf("the value cannot be read as JSON: %w", err)
			}
			for _, routeCase := range node.Cases {
				fmt.Fprintf(log, "case %s: %s\n", routeCase.Name, routeCase.Expression)
			}
			route, err := compiled.Route(value)
			details := map[string]any{"value": inputs[0].Value}
			if err != nil {
				return engine.Result{Details: details}, err
			}
			details["case"] = route
			fmt.Fprintf(log, "Routed to %s\n", route)
			routed := map[string]any{"case": route, "value": inputs[0].Value}
			return engine.Result{Outputs: map[string]any{route: routed}, Details: details}, nil
		},
	}, nil
}
