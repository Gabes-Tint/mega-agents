// Package cli is the mega-agents command line: it serves the editor, runs
// workflows headless, and inspects recorded runs and their logs.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"text/tabwriter"

	"github.com/Gabes-Tint/mega-agents/internal/app"
	"github.com/Gabes-Tint/mega-agents/internal/engine"
	"github.com/Gabes-Tint/mega-agents/internal/runs"
)

// Env is what the command line talks to.
type Env struct {
	Stdout io.Writer
	Stderr io.Writer
	// Serve starts the editor's web server and blocks while it runs.
	Serve func() error
	// Context bounds a run; nil stops it on an interrupt (Ctrl-C), which
	// records the run as cancelled.
	Context context.Context
}

const usage = `Usage: mega-agents [command]

Commands:
  serve                        serve the editor (the default)
  run <workflow.yaml|.json>    run a workflow file and record the run
  run <name>                   run a workflow saved from the editor
  workflows                    list workflows saved from the editor
  runs                         list recorded runs, newest first
  runs show <run-id>           show a run and each of its steps
  logs <run-id> [step]         print a run's logs, or one step's by id or name

Runs and logs are kept under $MEGA_AGENTS_HOME (default ~/.mega-agents).
`

// Main runs the command and returns the process exit code: 0 on success,
// 1 when a run or lookup failed, and 2 for a command that could not start.
func Main(args []string, env Env) int {
	if len(args) == 0 {
		args = []string{"serve"}
	}
	switch args[0] {
	case "serve":
		if err := env.Serve(); err != nil {
			fmt.Fprintln(env.Stderr, err)
			return 1
		}
		return 0
	case "run":
		return runWorkflow(args[1:], env)
	case "runs":
		return listOrShow(args[1:], env)
	case "workflows":
		return listWorkflows(env)
	case "logs":
		return printLogs(args[1:], env)
	case "help", "-h", "--help":
		fmt.Fprint(env.Stdout, usage)
		return 0
	}
	fmt.Fprintf(env.Stderr, "unknown command %q\n\n%s", args[0], usage)
	return 2
}

func store(env Env) (*runs.Store, bool) {
	store, err := runs.DefaultStore()
	if err != nil {
		fmt.Fprintln(env.Stderr, err)
		return nil, false
	}
	return store, true
}

func runWorkflow(args []string, env Env) int {
	if len(args) != 1 {
		fmt.Fprintln(env.Stderr, "usage: mega-agents run <workflow.yaml|workflow.json>")
		return 2
	}
	request, err := readWorkflow(args[0])
	if err != nil {
		fmt.Fprintln(env.Stderr, err)
		return 2
	}
	store, ok := store(env)
	if !ok {
		return 2
	}
	record, execute, err := app.Runs{Store: store}.Start(request)
	if err != nil {
		fmt.Fprintln(env.Stderr, err)
		return 2
	}
	fmt.Fprintf(env.Stdout, "Run %s started %s\n", record.ID, record.Workflow)
	reported := map[string]engine.Status{}
	ctx := env.Context
	if ctx == nil {
		var stop context.CancelFunc
		ctx, stop = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
	}
	finished := execute(ctx, func(current runs.Record) {
		for _, step := range current.Steps {
			if reported[step.TaskID] == step.Status {
				continue
			}
			reported[step.TaskID] = step.Status
			if line := stepLine(step); line != "" {
				fmt.Fprintln(env.Stdout, line)
			}
		}
	})
	fmt.Fprintf(env.Stdout, "Run %s %s; inspect it with: mega-agents logs %s\n", finished.ID, finished.Status, finished.ID)
	if finished.Status != engine.Succeeded {
		return 1
	}
	return 0
}

func stepLine(step engine.Step) string {
	switch step.Status {
	case engine.Running:
		return fmt.Sprintf("▶ %s running", step.Name)
	case engine.Succeeded:
		return fmt.Sprintf("✔ %s succeeded", step.Name)
	case engine.Failed:
		return fmt.Sprintf("✖ %s failed: %s", step.Name, step.Error)
	case engine.Skipped:
		return fmt.Sprintf("⏭ %s skipped: %s", step.Name, step.Error)
	}
	return ""
}

// readWorkflow accepts the YAML the editor exports and the JSON graph the
// editor posts.
func readWorkflow(path string) (app.WorkflowRequest, error) {
	if _, err := os.Stat(path); err != nil && filepath.Ext(path) == "" {
		store, storeErr := app.DefaultWorkflowStore()
		if storeErr != nil {
			return app.WorkflowRequest{}, storeErr
		}
		request, loadErr := store.Load(path)
		if loadErr != nil {
			return app.WorkflowRequest{}, fmt.Errorf("no workflow file or saved workflow named %s: %w", path, loadErr)
		}
		return request, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return app.WorkflowRequest{}, fmt.Errorf("cannot read %s: %w", path, err)
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return app.ParseWorkflowYAML(data)
	case ".json":
		var request app.WorkflowRequest
		if err := json.Unmarshal(data, &request); err != nil {
			return app.WorkflowRequest{}, fmt.Errorf("%s is not a valid workflow graph: %w", path, err)
		}
		return request, nil
	}
	return app.WorkflowRequest{}, fmt.Errorf("cannot read %s: use a .yaml, .yml or .json workflow", path)
}

func listOrShow(args []string, env Env) int {
	store, ok := store(env)
	if !ok {
		return 2
	}
	switch {
	case len(args) == 0:
		return listRuns(store, env)
	case args[0] == "show" && len(args) == 2:
		return showRun(store, args[1], env)
	case args[0] == "show":
		fmt.Fprintln(env.Stderr, "usage: mega-agents runs show <run-id>")
		return 2
	}
	fmt.Fprintln(env.Stderr, "usage: mega-agents runs [show <run-id>]")
	return 2
}

func listRuns(store *runs.Store, env Env) int {
	records, err := store.List()
	if err != nil {
		fmt.Fprintln(env.Stderr, err)
		return 1
	}
	if len(records) == 0 {
		fmt.Fprintln(env.Stdout, "No runs recorded yet.")
		return 0
	}
	table := tabwriter.NewWriter(env.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "RUN\tSTATUS\tSTARTED\tWORKFLOW")
	for _, record := range records {
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", record.ID, record.Status, record.StartedAt, record.Workflow)
	}
	_ = table.Flush()
	return 0
}

func loadRun(store *runs.Store, id string, env Env) (runs.Record, bool) {
	record, err := store.Load(id)
	if errors.Is(err, runs.ErrNotFound) {
		fmt.Fprintf(env.Stderr, "run %s not found\n", id)
		return runs.Record{}, false
	}
	if err != nil {
		fmt.Fprintln(env.Stderr, err)
		return runs.Record{}, false
	}
	return record, true
}

func showRun(store *runs.Store, id string, env Env) int {
	record, ok := loadRun(store, id, env)
	if !ok {
		return 1
	}
	fmt.Fprintf(env.Stdout, "Run %s  %s  %s\n", record.ID, record.Workflow, record.Status)
	fmt.Fprintf(env.Stdout, "Started %s", record.StartedAt)
	if record.FinishedAt != "" {
		fmt.Fprintf(env.Stdout, "  finished %s", record.FinishedAt)
	}
	fmt.Fprint(env.Stdout, "\n\n")
	for _, step := range record.Steps {
		fmt.Fprintf(env.Stdout, "%s  %s  %s  %s\n", step.TaskID, step.Name, step.Kind, step.Status)
		if step.Error != "" {
			fmt.Fprintf(env.Stdout, "  error: %s\n", step.Error)
		}
		keys := make([]string, 0, len(step.Details))
		for key := range step.Details {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Fprintf(env.Stdout, "  %s: %v\n", key, step.Details[key])
		}
	}
	return 0
}

func printLogs(args []string, env Env) int {
	if len(args) < 1 || len(args) > 2 {
		fmt.Fprintln(env.Stderr, "usage: mega-agents logs <run-id> [step]")
		return 2
	}
	store, ok := store(env)
	if !ok {
		return 2
	}
	record, ok := loadRun(store, args[0], env)
	if !ok {
		return 1
	}
	steps := record.Steps
	if len(args) == 2 {
		steps = nil
		for _, step := range record.Steps {
			if step.TaskID == args[1] || strings.EqualFold(step.Name, args[1]) {
				steps = append(steps, step)
			}
		}
		if len(steps) == 0 {
			fmt.Fprintf(env.Stderr, "run %s has no step %q\n", record.ID, args[1])
			return 1
		}
	}
	for _, step := range steps {
		text, err := store.ReadLog(record.ID, step.TaskID)
		if errors.Is(err, runs.ErrNotFound) {
			text = "(no log)\n"
		} else if err != nil {
			fmt.Fprintln(env.Stderr, err)
			return 1
		}
		if len(args) == 1 {
			fmt.Fprintf(env.Stdout, "== %s (%s) %s ==\n", step.Name, step.TaskID, step.Status)
		}
		fmt.Fprint(env.Stdout, text)
	}
	return 0
}

func listWorkflows(env Env) int {
	store, err := app.DefaultWorkflowStore()
	if err != nil {
		fmt.Fprintln(env.Stderr, err)
		return 2
	}
	workflows, err := store.List()
	if err != nil {
		fmt.Fprintln(env.Stderr, err)
		return 1
	}
	if len(workflows) == 0 {
		fmt.Fprintln(env.Stdout, "No workflows saved yet.")
		return 0
	}
	table := tabwriter.NewWriter(env.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "WORKFLOW\tUPDATED\tFILE")
	for _, workflow := range workflows {
		fmt.Fprintf(table, "%s\t%s\t%s\n", workflow.Name, workflow.UpdatedAt, store.Path(workflow.Name))
	}
	_ = table.Flush()
	return 0
}
