package app

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/Gabes-Tint/mega-agents/internal/engine"
	"github.com/Gabes-Tint/mega-agents/internal/gitops"
)

var placeholder = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_.-]+)\s*\}\}`)

var workspaceFields = map[string]func(gitops.Workspace) string{
	"path":         func(w gitops.Workspace) string { return w.Path },
	"branch":       func(w gitops.Workspace) string { return w.Branch },
	"base":         func(w gitops.Workspace) string { return w.Base },
	"baseRevision": func(w gitops.Workspace) string { return w.BaseRevision },
	"repository":   func(w gitops.Workspace) string { return w.Repository },
	"remote":       func(w gitops.Workspace) string { return w.Remote },
}

// templateSources is what a block's text templates may name: the workspace
// it receives, and the results of the blocks connected to it by the slug of
// their names.
type templateSources struct {
	workspace  bool
	resultFrom map[string]string
}

// sourcesOf reads a block's incoming needs.
func (planner runPlanner) sourcesOf(needs []engine.Need) templateSources {
	sources := templateSources{resultFrom: map[string]string{}}
	for _, need := range needs {
		switch need.Port {
		case "", sessionPort:
		case workspacePort:
			sources.workspace = true
		default:
			sources.resultFrom[newIdentifierSet().slugIdentifier(planner.nodeByID[need.TaskID].Name)] = need.TaskID
		}
	}
	return sources
}

// checkTemplate refuses placeholders the block will have nothing for.
func checkTemplate(text string, sources templateSources) error {
	for _, match := range placeholder.FindAllStringSubmatch(text, -1) {
		name := match[1]
		field, isWorkspace := strings.CutPrefix(name, "workspace.")
		blockName, isResult := strings.CutPrefix(name, "results.")
		switch {
		case isWorkspace && workspaceFields[field] != nil:
			if !sources.workspace {
				return fmt.Errorf("{{%s}} needs a workspace; connect a %s action to this block", name, gitActionLabels["worktree"])
			}
		case isResult:
			if sources.resultFrom[blockName] == "" {
				return fmt.Errorf("{{%s}} names no block connected to this one", name)
			}
		case name == "result":
			if len(sources.resultFrom) != 1 {
				return fmt.Errorf("{{result}} needs exactly one block connected to this one; name one with {{results.<block>}}")
			}
		default:
			return fmt.Errorf("unknown placeholder {{%s}}", name)
		}
	}
	return nil
}

// renderTemplate fills a checked template from a task's inputs: workspace
// fields as text, results as JSON.
func renderTemplate(text string, inputs []engine.Input, sources templateSources) string {
	workspace, _ := workspaceIn(inputs)
	results := map[string]any{}
	for _, input := range inputs {
		if input.Port != workspacePort && input.Port != sessionPort {
			results[input.TaskID] = input.Value
		}
	}
	return placeholder.ReplaceAllStringFunc(text, func(token string) string {
		name := placeholder.FindStringSubmatch(token)[1]
		if field, ok := strings.CutPrefix(name, "workspace."); ok {
			return workspaceFields[field](workspace)
		}
		source := ""
		if blockName, ok := strings.CutPrefix(name, "results."); ok {
			source = sources.resultFrom[blockName]
		} else {
			for _, id := range sources.resultFrom {
				source = id
			}
		}
		encoded, _ := json.Marshal(results[source])
		return string(encoded)
	})
}

// workspaceIn finds the workspace among a task's inputs.
func workspaceIn(inputs []engine.Input) (gitops.Workspace, bool) {
	for _, input := range inputs {
		if workspace, ok := input.Value.(gitops.Workspace); ok {
			return workspace, true
		}
	}
	return gitops.Workspace{}, false
}
