package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Gabes-Tint/mega-agents/internal/agents"
	"github.com/Gabes-Tint/mega-agents/internal/engine"
	"github.com/Gabes-Tint/mega-agents/internal/schema"
)

const (
	resultPort           = "result"
	defaultAgentRetries  = 2
	maxAgentRetries      = 5
	defaultAgentTimeout  = 30.0
	maxAgentTimeout      = 240.0
	maxReplyDetailLength = 8000
)

// agentTask plans an Agent block: one conversation with a coding-agent CLI.
// It works in the workspace it receives, or in its project's folder, and
// its prompt may name that workspace and the results of the agents
// connected to it. With an output schema, the reply is held to it.
func (planner runPlanner) agentTask(node WorkflowNodeInput) (engine.Task, error) {
	fail := func(format string, args ...any) (engine.Task, error) {
		return engine.Task{}, fmt.Errorf("%s: %s", node.Name, fmt.Sprintf(format, args...))
	}
	backend, err := agents.Lookup(node.Backend)
	if err != nil {
		return fail("%v", err)
	}
	if strings.TrimSpace(node.Prompt) == "" {
		return fail("write the prompt the agent receives")
	}
	retries := defaultAgentRetries
	if node.Retries != nil {
		retries = *node.Retries
	}
	if retries < 0 || retries > maxAgentRetries {
		return fail("retries must be between 0 and %d", maxAgentRetries)
	}
	timeout := defaultAgentTimeout
	if node.TimeoutMinutes != nil {
		timeout = *node.TimeoutMinutes
	}
	if timeout <= 0 || timeout > maxAgentTimeout {
		return fail("the timeout must be more than 0 and at most %g minutes", maxAgentTimeout)
	}
	var outputSchema *schema.Schema
	if strings.TrimSpace(node.OutputSchema) != "" {
		if outputSchema, err = schema.CompileStrict(node.OutputSchema); err != nil {
			return fail("output schema: %v", err)
		}
	}
	needs, err := planner.incomingNeeds(node)
	if err != nil {
		return engine.Task{}, err
	}
	sources := planner.sourcesOf(needs)
	if err := checkTemplate(node.Prompt, sources); err != nil {
		return fail("%v", err)
	}
	return engine.Task{
		ID: node.ID, Name: node.Name, Kind: "agent", Needs: needs,
		Run: func(ctx context.Context, inputs []engine.Input, log io.Writer) (engine.Result, error) {
			details := map[string]any{"backend": node.Backend, "model": node.Model, "effort": node.Effort}
			workspace, hasWorkspace := workspaceIn(inputs)
			dir := workspace.Path
			if !hasWorkspace {
				project := planner.projectOf(node)
				if project.Path == "" {
					return engine.Result{Details: details}, fmt.Errorf("set the path on the project that contains %s", node.Name)
				}
				var err error
				if dir, err = projectDir(project); err != nil {
					return engine.Result{Details: details}, err
				}
			}
			prompt := renderTemplate(node.Prompt, inputs, sources)
			ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout*float64(time.Minute)))
			defer cancel()
			fmt.Fprintf(log, "Working in %s\nPrompt:\n%s\n", dir, prompt)
			conversation, err := agents.Talk(ctx, agents.Runner{}, backend, agents.Turn{
				Prompt: prompt, Dir: dir, Model: node.Model, Effort: node.Effort, Schema: outputSchema,
			}, retries, log)
			details["attempts"] = conversation.Attempts
			if conversation.Reply.SessionID != "" {
				details["sessionId"] = conversation.Reply.SessionID
			}
			reply := conversation.Reply.Text
			if len(reply) > maxReplyDetailLength {
				reply = reply[:maxReplyDetailLength] + "…"
			}
			details["reply"] = reply
			fmt.Fprintf(log, "Reply:\n%s\n", conversation.Reply.Text)
			if err != nil {
				var invalid *agents.InvalidReplyError
				if errors.As(err, &invalid) {
					details["schemaErrors"] = invalid.Errors
				}
				return engine.Result{Details: details}, err
			}
			result := map[string]any{"text": conversation.Reply.Text}
			if outputSchema != nil {
				result = conversation.Value
			}
			details["result"] = result
			outputs := map[string]any{resultPort: result}
			if hasWorkspace {
				outputs[workspacePort] = workspace
			}
			return engine.Result{Outputs: outputs, Details: details}, nil
		},
	}, nil
}

// projectOf finds the project an agent sits in, through any agents that
// contain it.
func (planner runPlanner) projectOf(node WorkflowNodeInput) WorkflowNodeInput {
	for current := node; current.ParentID != ""; {
		current = planner.nodeByID[current.ParentID]
		if current.Type == "project" {
			return current
		}
	}
	return WorkflowNodeInput{}
}
