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
	// anyOne is set for a block that runs on whichever connected block
	// arrives, whose {{result}} is that block's.
	anyOne bool
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
			if len(sources.resultFrom) != 1 && !(sources.anyOne && len(sources.resultFrom) > 0) {
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
	return fillTemplate(text, inputs, sources, func(value, _ string) string { return value })
}

// renderShellTemplate fills a checked template that a shell is about to run,
// so every value is text the command reads and never shell the command runs.
func renderShellTemplate(text string, inputs []engine.Input, sources templateSources) string {
	return fillTemplate(text, inputs, sources, shellValue)
}

// shellValue writes a value where the command puts it, which the command up
// to that point gives: between single quotes it closes and reopens them
// around every quote the value holds, between double quotes it escapes what
// the shell still reads there, and outside both it becomes one quoted word,
// so a path holding a space stays one path.
func shellValue(value, before string) string {
	switch quotesOpenIn(before) {
	case singleQuoted:
		return strings.ReplaceAll(value, "'", `'\''`)
	case doubleQuoted:
		return insideDoubleQuotes.Replace(value)
	default:
		return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
	}
}

// insideDoubleQuotes escapes the characters a shell still reads between
// double quotes.
var insideDoubleQuotes = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", "$", `\$`)

type quoting int

const (
	unquoted quoting = iota
	singleQuoted
	doubleQuoted
)

// quotesOpenIn reads the command before a placeholder for the quotes it
// opened and has not closed.
func quotesOpenIn(before string) quoting {
	open := unquoted
	for index := 0; index < len(before); index++ {
		switch before[index] {
		case '\\':
			// A backslash escapes the character after it everywhere but
			// between single quotes, where it is a character of its own.
			if open != singleQuoted {
				index++
			}
		case '\'':
			switch open {
			case unquoted:
				open = singleQuoted
			case singleQuoted:
				open = unquoted
			}
		case '"':
			switch open {
			case unquoted:
				open = doubleQuoted
			case doubleQuoted:
				open = unquoted
			}
		}
	}
	return open
}

// fillTemplate replaces every placeholder with its value, written as the
// field's language needs it where it lands.
func fillTemplate(text string, inputs []engine.Input, sources templateSources, write func(value, before string) string) string {
	workspace, _ := workspaceIn(inputs)
	results := map[string]any{}
	for _, input := range inputs {
		if input.Port != workspacePort && input.Port != sessionPort {
			results[input.TaskID] = input.Value
		}
	}
	var filled strings.Builder
	end := 0
	for _, match := range placeholder.FindAllStringSubmatchIndex(text, -1) {
		value := ""
		name := text[match[2]:match[3]]
		if field, ok := strings.CutPrefix(name, "workspace."); ok {
			value = workspaceFields[field](workspace)
		} else {
			source := ""
			if blockName, ok := strings.CutPrefix(name, "results."); ok {
				source = sources.resultFrom[blockName]
			} else {
				for _, id := range sources.resultFrom {
					if _, arrived := results[id]; arrived {
						source = id
					}
				}
			}
			encoded, _ := json.Marshal(results[source])
			value = string(encoded)
		}
		filled.WriteString(text[end:match[0]])
		filled.WriteString(write(value, filled.String()))
		end = match[1]
	}
	filled.WriteString(text[end:])
	return filled.String()
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
