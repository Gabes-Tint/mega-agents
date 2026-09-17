package app

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Gabes-Tint/mega-agents/internal/engine"
	"github.com/Gabes-Tint/mega-agents/internal/schema"
)

const (
	validPort   = "valid"
	invalidPort = "invalid"
)

// schemaTask plans a JSON Schema block (output/json-schema@v1): it checks
// the one value it receives against its schema. A valid value leaves on
// the valid output. An invalid one leaves on the invalid output, carrying
// the value and its field errors, when an arrow takes that branch;
// otherwise the block fails, so an unhandled invalid value stops the run.
func (planner runPlanner) schemaTask(node WorkflowNodeInput) (engine.Task, error) {
	fail := func(format string, args ...any) (engine.Task, error) {
		return engine.Task{}, fmt.Errorf("%s: %s", node.Name, fmt.Sprintf(format, args...))
	}
	if strings.TrimSpace(node.Schema) == "" {
		return fail("write the JSON Schema the value must satisfy")
	}
	compiled, err := schema.Compile(node.Schema)
	if err != nil {
		return fail("%v", err)
	}
	needs, err := planner.valueNeeds(node, "validates")
	if err != nil {
		return engine.Task{}, err
	}
	handlesInvalid := false
	for _, edge := range planner.request.Edges {
		if edge.From == node.ID {
			port, err := planner.sourcePort(edge)
			if err != nil {
				return engine.Task{}, err
			}
			handlesInvalid = handlesInvalid || port == invalidPort
		}
	}
	handlesInvalid = handlesInvalid || planner.loopOf(node).UntilNode == node.ID
	return engine.Task{
		ID: node.ID, Name: node.Name, Kind: "jsonschema", Needs: needs,
		Run: func(_ context.Context, inputs []engine.Input, log io.Writer) (engine.Result, error) {
			started := time.Now()
			value, err := schema.Normalize(inputs[0].Value)
			if err != nil {
				return engine.Result{}, fmt.Errorf("the value cannot be read as JSON: %w", err)
			}
			errs := compiled.Validate(value)
			details := map[string]any{
				"valid": len(errs) == 0, "value": inputs[0].Value, "schema": compiled.Text,
				"durationMs": time.Since(started).Milliseconds(),
			}
			if len(errs) == 0 {
				fmt.Fprintln(log, "✅ The value satisfies the schema")
				return engine.Result{Outputs: map[string]any{validPort: inputs[0].Value}, Details: details}, nil
			}
			details["errors"] = errs
			messages := make([]string, len(errs))
			for i, fieldError := range errs {
				messages[i] = fieldError.String()
				fmt.Fprintln(log, "❌", fieldError.String())
			}
			if !handlesInvalid {
				return engine.Result{Details: details}, fmt.Errorf(
					"the value did not satisfy the schema: %s", strings.Join(messages, "; "),
				)
			}
			fmt.Fprintln(log, "⚠️ The value takes the invalid branch")
			invalid := map[string]any{"value": inputs[0].Value, "errors": errs}
			return engine.Result{Outputs: map[string]any{invalidPort: invalid}, Details: details}, nil
		},
	}, nil
}
