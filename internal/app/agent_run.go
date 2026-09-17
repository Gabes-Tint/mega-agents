package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/Gabes-Tint/mega-agents/internal/agents"
	"github.com/Gabes-Tint/mega-agents/internal/engine"
	"github.com/Gabes-Tint/mega-agents/internal/gitops"
	"github.com/Gabes-Tint/mega-agents/internal/schema"
)

const (
	resultPort            = "result"
	defaultAgentRetries   = 2
	maxAgentRetries       = 5
	defaultAgentTimeout   = 30.0
	maxAgentTimeout       = 240.0
	maxReplyDetailLength  = 8000
	placeholderExpression = `\{\{\s*([A-Za-z0-9_.-]+)\s*\}\}`
)

var placeholder = regexp.MustCompile(placeholderExpression)

var workspaceFields = map[string]func(gitops.Workspace) string{
	"path":         func(w gitops.Workspace) string { return w.Path },
	"branch":       func(w gitops.Workspace) string { return w.Branch },
	"base":         func(w gitops.Workspace) string { return w.Base },
	"baseRevision": func(w gitops.Workspace) string { return w.BaseRevision },
	"repository":   func(w gitops.Workspace) string { return w.Repository },
	"remote":       func(w gitops.Workspace) string { return w.Remote },
}

// agentTask plans an Agent block: one conversation with a coding-agent CLI.
// It works in the workspace it receives, or in its project's folder, and
// its prompt may name that workspace and the results of the agents
// connected to it. With an output schema, the reply is held to it.
func (planner runPlanner) agentTask(node WorkflowNodeInput, included map[string]bool) (engine.Task, error) {
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
	var needs []engine.Need
	receivesWorkspace := false
	resultFrom := map[string]string{}
	for _, edge := range planner.request.Edges {
		if edge.To != node.ID || !included[edge.From] {
			continue
		}
		source := planner.nodeByID[edge.From]
		port, err := planner.sourcePort(edge)
		if err != nil {
			return engine.Task{}, err
		}
		need := engine.Need{TaskID: edge.From, Port: port}
		switch port {
		case "":
		case workspacePort:
			if receivesWorkspace {
				return fail("receives more than one workspace; connect only one")
			}
			receivesWorkspace = true
		default:
			resultFrom[newIdentifierSet().slugIdentifier(source.Name)] = source.ID
		}
		needs = append(needs, need)
	}
	for _, match := range placeholder.FindAllStringSubmatch(node.Prompt, -1) {
		name := match[1]
		field, isWorkspace := strings.CutPrefix(name, "workspace.")
		agentName, isResult := strings.CutPrefix(name, "results.")
		switch {
		case isWorkspace && workspaceFields[field] != nil:
			if !receivesWorkspace {
				return fail("{{%s}} needs a workspace; connect a %s action to this agent", name, gitActionLabels["worktree"])
			}
		case isResult:
			if resultFrom[agentName] == "" {
				return fail("{{%s}} names no agent connected to this one", name)
			}
		case name == "result":
			if len(resultFrom) != 1 {
				return fail("{{result}} needs exactly one agent connected to this one; name one with {{results.<agent>}}")
			}
		default:
			return fail("unknown placeholder {{%s}}", name)
		}
	}
	return engine.Task{
		ID: node.ID, Name: node.Name, Kind: "agent", Needs: needs,
		Run: func(ctx context.Context, inputs []engine.Input, log io.Writer) (engine.Result, error) {
			details := map[string]any{"backend": node.Backend, "model": node.Model, "effort": node.Effort}
			var workspace *gitops.Workspace
			results := map[string]any{}
			for _, input := range inputs {
				switch value := input.Value.(type) {
				case gitops.Workspace:
					workspace = &value
				default:
					results[input.TaskID] = value
				}
			}
			dir := ""
			if workspace != nil {
				dir = workspace.Path
			} else {
				project := planner.projectOf(node)
				if project.Path == "" {
					return engine.Result{Details: details}, fmt.Errorf("set the path on the project that contains %s", node.Name)
				}
				if dir, err = projectDir(project); err != nil {
					return engine.Result{Details: details}, err
				}
			}
			prompt := placeholder.ReplaceAllStringFunc(node.Prompt, func(token string) string {
				name := placeholder.FindStringSubmatch(token)[1]
				if field, ok := strings.CutPrefix(name, "workspace."); ok {
					return workspaceFields[field](*workspace)
				}
				source := ""
				if agentName, ok := strings.CutPrefix(name, "results."); ok {
					source = resultFrom[agentName]
				} else {
					for _, id := range resultFrom {
						source = id
					}
				}
				encoded, _ := json.Marshal(results[source])
				return string(encoded)
			})
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
			return engine.Result{Outputs: map[string]any{resultPort: result}, Details: details}, nil
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
