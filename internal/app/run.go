package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	stdlog "log"
	"mime"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Gabes-Tint/mega-agents/internal/engine"
	"github.com/Gabes-Tint/mega-agents/internal/gitops"
	"github.com/Gabes-Tint/mega-agents/internal/router"
	"github.com/Gabes-Tint/mega-agents/internal/runs"
)

// runStepTimeout bounds one Git action so an unreachable remote cannot hold
// the request open indefinitely.
const runStepTimeout = 2 * time.Minute

const workspacePort = "workspace"

var errNoStartingBlock = errors.New(
	"flag a GitHub block or an agent as the starting point to run the flow",
)

// gitActionLabels names each Git action a GitHub block can hold.
var gitActionLabels = map[string]string{
	"fetch":    "Fetch",
	"worktree": "Create worktree",
	"rebase":   "Rebase",
}

// Runs plans workflows and executes them as recorded runs: every run keeps
// its record and the log of every step in the store.
type Runs struct {
	Store *runs.Store
}

// Execution runs a started run to its end, calling observe (when not nil)
// with the record each time a step changes, and returns the finished record.
type Execution func(ctx context.Context, observe func(runs.Record)) runs.Record

// Start plans the flow and records a run of it with every step pending. It
// returns that record and the execution that runs it. A GitHub block
// flagged as a starting point runs its own actions from its starting action
// along their arrows, or a plain fetch when it holds no actions. Actions not
// reachable from a starting action do not run.
func (service Runs) Start(request WorkflowRequest) (runs.Record, Execution, error) {
	tasks, err := planRun(request)
	if err != nil {
		return runs.Record{}, nil, err
	}
	if err := engine.Validate(tasks); err != nil {
		return runs.Record{}, nil, err
	}
	steps := make([]engine.Step, len(tasks))
	for i, task := range tasks {
		steps[i] = engine.Step{TaskID: task.ID, Name: task.Name, Kind: task.Kind, Status: engine.Pending}
	}
	name := request.Name
	if name == "" {
		name = "workflow"
	}
	record, err := service.Store.Create(name, steps)
	if err != nil {
		return runs.Record{}, nil, err
	}
	execute := func(ctx context.Context, observe func(runs.Record)) runs.Record {
		return service.execute(ctx, record, tasks, observe)
	}
	return record, execute, nil
}

func (service Runs) execute(
	ctx context.Context, record runs.Record, tasks []engine.Task, observe func(runs.Record),
) runs.Record {
	var logs []io.Closer
	defer func() {
		for _, log := range logs {
			_ = log.Close()
		}
	}()
	save := func() {
		if err := service.Store.Save(record); err != nil {
			stdlog.Printf("run %s: %v", record.ID, err)
		}
		if observe != nil {
			observe(record)
		}
	}
	finished, err := engine.Execute(ctx, tasks, engine.Options{
		Observe: func(run engine.Run) {
			record.Steps = run.Steps
			save()
		},
		Log: func(taskID string) io.Writer {
			log, err := service.Store.OpenLog(record.ID, taskID)
			if err != nil {
				stdlog.Printf("run %s: %v", record.ID, err)
				return nil
			}
			logs = append(logs, log)
			return log
		},
	})
	if err != nil {
		// Planning already accepted these tasks, so this cannot happen; the
		// record still ends failed rather than running forever.
		finished.Status = engine.Failed
	}
	if finished.Steps != nil {
		record.Steps = finished.Steps
	}
	record.Status = finished.Status
	record.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	save()
	return record
}

type runPlanner struct {
	nodeByID   map[string]WorkflowNodeInput
	childrenOf map[string][]WorkflowNodeInput
	request    WorkflowRequest
}

func planRun(request WorkflowRequest) ([]engine.Task, error) {
	nodeByID, err := validateGraph(request)
	if err != nil {
		return nil, err
	}
	planner := runPlanner{nodeByID: nodeByID, childrenOf: map[string][]WorkflowNodeInput{}, request: request}
	for _, node := range request.Nodes {
		if node.ParentID != "" {
			planner.childrenOf[node.ParentID] = append(planner.childrenOf[node.ParentID], node)
		}
	}
	included := map[string]bool{}
	var tasks []engine.Task
	for _, node := range request.Nodes {
		switch {
		case node.Type == "agent" && node.Start:
			planner.include(node.ID, included)
		case node.Type == "github" && node.Start:
			start, hasActions, err := planner.startingAction(node)
			if err != nil {
				return nil, err
			}
			if !hasActions {
				tasks = append(tasks, planner.fetchTask(node, node))
				continue
			}
			planner.include(start.ID, included)
		}
	}
	if len(tasks) == 0 && len(included) == 0 {
		return nil, errNoStartingBlock
	}
	for _, node := range request.Nodes {
		if !included[node.ID] {
			continue
		}
		var task engine.Task
		switch node.Type {
		case "agent":
			task, err = planner.agentTask(node, included)
		case "jsonschema":
			task, err = planner.schemaTask(node, included)
		case "router":
			task, err = planner.routerTask(node, included)
		default:
			task, err = planner.actionTask(node, included)
		}
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

// startingAction finds the action a GitHub block's sequence starts from.
func (planner runPlanner) startingAction(github WorkflowNodeInput) (WorkflowNodeInput, bool, error) {
	var actions []WorkflowNodeInput
	for _, child := range planner.childrenOf[github.ID] {
		if child.Type == "action" {
			actions = append(actions, child)
		}
	}
	for _, action := range actions {
		if action.Start {
			return action, true, nil
		}
	}
	if len(actions) > 0 {
		return WorkflowNodeInput{}, true, fmt.Errorf("flag the first action of %s as its starting point", github.Name)
	}
	return WorkflowNodeInput{}, false, nil
}

// executableTypes are the blocks a run executes when a start reaches them.
var executableTypes = map[string]bool{"action": true, "agent": true, "jsonschema": true, "router": true}

// include marks the node and every executable node its arrows reach.
func (planner runPlanner) include(id string, included map[string]bool) {
	if included[id] || !executableTypes[planner.nodeByID[id].Type] {
		return
	}
	included[id] = true
	for _, edge := range planner.request.Edges {
		if edge.From == id {
			planner.include(edge.To, included)
		}
	}
}

func (planner runPlanner) actionTask(node WorkflowNodeInput, included map[string]bool) (engine.Task, error) {
	if _, known := gitActionLabels[node.Action]; !known {
		return engine.Task{}, fmt.Errorf("unknown Git action %q on %s", node.Action, node.Name)
	}
	github := planner.nodeByID[node.ParentID]
	var needs []engine.Need
	receivesWorkspace := false
	for _, edge := range planner.request.Edges {
		if edge.To != node.ID || !included[edge.From] {
			continue
		}
		port, err := planner.sourcePort(edge)
		if err != nil {
			return engine.Task{}, err
		}
		need := engine.Need{TaskID: edge.From}
		if port == workspacePort && node.Action == "rebase" {
			need.Port = workspacePort
			receivesWorkspace = true
		}
		needs = append(needs, need)
	}
	switch node.Action {
	case "fetch":
		task := planner.fetchTask(node, github)
		task.Needs = needs
		return task, nil
	case "rebase":
		if !receivesWorkspace {
			return engine.Task{}, fmt.Errorf(
				"%s needs a workspace; connect a %s action before it", node.Name, gitActionLabels["worktree"],
			)
		}
	}
	return engine.Task{
		ID: node.ID, Name: node.Name, Kind: node.Action, Needs: needs,
		Run: func(ctx context.Context, inputs []engine.Input, log io.Writer) (engine.Result, error) {
			ctx, cancel := context.WithTimeout(gitops.WithLog(ctx, log), runStepTimeout)
			defer cancel()
			if node.Action == "rebase" {
				workspace, _ := inputs[0].Value.(gitops.Workspace)
				return workspaceResult(gitops.Rebase(ctx, workspace, node.Onto))
			}
			dir, err := projectDir(planner.nodeByID[github.ParentID])
			if err != nil {
				return engine.Result{}, err
			}
			return workspaceResult(gitops.CreateWorktree(ctx, gitops.WorktreeRequest{
				Dir: dir, Repository: github.Repository, Branch: node.Branch,
				Base: node.Base, Path: node.WorktreePath,
			}))
		},
	}, nil
}

// sourcePort names the output an arrow carries from its source block, or
// returns "" for an arrow that only orders the two blocks.
func (planner runPlanner) sourcePort(edge WorkflowEdgeInput) (string, error) {
	source := planner.nodeByID[edge.From]
	switch {
	case producesWorkspace(source):
		return workspacePort, nil
	case source.Type == "agent":
		return resultPort, nil
	case source.Type == "jsonschema":
		switch edge.FromPort {
		case "":
			return validPort, nil
		case validPort, invalidPort:
			return edge.FromPort, nil
		}
		return "", fmt.Errorf("%s has no output %q; use %s or %s", source.Name, edge.FromPort, validPort, invalidPort)
	case source.Type == "router":
		routes := []string{}
		for _, routeCase := range source.Cases {
			routes = append(routes, routeCase.Name)
		}
		routes = append(routes, router.Default)
		if edge.FromPort == "" {
			return "", fmt.Errorf("choose which route the arrow from %s to %s takes", source.Name, planner.nodeByID[edge.To].Name)
		}
		if !slices.Contains(routes, edge.FromPort) {
			return "", fmt.Errorf("%s has no route %q; use %s", source.Name, edge.FromPort, joinChoices(routes))
		}
		return edge.FromPort, nil
	}
	return "", nil
}

// joinChoices spells a list as "a, b or c".
func joinChoices(choices []string) string {
	if len(choices) == 1 {
		return choices[0]
	}
	return strings.Join(choices[:len(choices)-1], ", ") + " or " + choices[len(choices)-1]
}

// valueNeeds collects the arrows into a block that takes exactly one value,
// such as an agent's result, and nothing else.
func (planner runPlanner) valueNeeds(node WorkflowNodeInput, included map[string]bool, verb string) ([]engine.Need, error) {
	var needs []engine.Need
	values := 0
	for _, edge := range planner.request.Edges {
		if edge.To != node.ID || !included[edge.From] {
			continue
		}
		port, err := planner.sourcePort(edge)
		if err != nil {
			return nil, err
		}
		if port != "" && port != workspacePort {
			values++
		} else {
			values += 2 // Only a value counts.
		}
		needs = append(needs, engine.Need{TaskID: edge.From, Port: port})
	}
	if values != 1 {
		return nil, fmt.Errorf("%s: %s one value; connect exactly one block to it", node.Name, verb)
	}
	return needs, nil
}

func producesWorkspace(node WorkflowNodeInput) bool {
	return node.Type == "action" && (node.Action == "worktree" || node.Action == "rebase")
}

func workspaceResult(workspace gitops.Workspace, err error) (engine.Result, error) {
	if err != nil {
		return engine.Result{}, err
	}
	return engine.Result{
		Outputs: map[string]any{workspacePort: workspace},
		Details: map[string]any{
			"repository": workspace.Repository, "remote": workspace.Remote, "path": workspace.Path,
			"branch": workspace.Branch, "base": workspace.Base, "baseRevision": workspace.BaseRevision,
		},
	}, nil
}

// fetchTask fetches the repository of the GitHub block inside its project's
// clone. The task belongs to node, which is the GitHub block itself when it
// holds no actions.
func (planner runPlanner) fetchTask(node WorkflowNodeInput, github WorkflowNodeInput) engine.Task {
	return engine.Task{
		ID: node.ID, Name: node.Name, Kind: "fetch",
		Run: func(ctx context.Context, _ []engine.Input, log io.Writer) (engine.Result, error) {
			if !github.Authenticated {
				return engine.Result{}, errors.New(
					`check "Already authenticated" on this GitHub block; token authentication is not supported yet`,
				)
			}
			dir, err := projectDir(planner.nodeByID[github.ParentID])
			if err != nil {
				return engine.Result{}, err
			}
			ctx, cancel := context.WithTimeout(gitops.WithLog(ctx, log), runStepTimeout)
			defer cancel()
			result, err := gitops.Fetch(ctx, dir, github.Repository)
			if err != nil {
				return engine.Result{}, err
			}
			return engine.Result{Details: map[string]any{
				"repository": result.Repository, "remote": result.Remote, "output": result.Output,
			}}, nil
		},
	}
}

func projectDir(project WorkflowNodeInput) (string, error) {
	switch {
	case project.Path == "":
		return "", errors.New("set the path on the project that contains this GitHub block")
	case !filepath.IsAbs(project.Path):
		return "", fmt.Errorf("the project path %s must be absolute", project.Path)
	}
	return project.Path, nil
}

func registerRunHandler(mux *http.ServeMux, service Runs) {
	// The editor asks for the repository when a GitHub block lands in a
	// project, so the field can be prefilled from the project's clone.
	mux.HandleFunc("GET /api/git/repository", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		if !filepath.IsAbs(path) {
			http.Error(w, "the project path must be absolute", http.StatusNotFound)
			return
		}
		detected, err := gitops.DetectGitHubRepository(r.Context(), path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, detected)
	})

	// Starting a run answers at once with the recorded run; it executes in
	// the background, and the editor follows it through GET /api/runs/{id}.
	mux.HandleFunc("POST /api/runs", func(w http.ResponseWriter, r *http.Request) {
		// Runs execute local commands, so only JSON is accepted: browsers must
		// preflight it cross-origin, and this server never approves a preflight.
		mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if mediaType != "application/json" {
			http.Error(w, "runs require a JSON request", http.StatusUnsupportedMediaType)
			return
		}
		var request WorkflowRequest
		body := http.MaxBytesReader(w, r.Body, workflowMaxBodyBytes)
		if err := json.NewDecoder(body).Decode(&request); err != nil {
			http.Error(w, "invalid workflow request", http.StatusBadRequest)
			return
		}
		record, execute, err := service.Start(request)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		go execute(context.WithoutCancel(r.Context()), nil)
		writeJSON(w, http.StatusAccepted, record)
	})

	mux.HandleFunc("GET /api/runs", func(w http.ResponseWriter, _ *http.Request) {
		records, err := service.Store.List()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, records)
	})

	mux.HandleFunc("GET /api/runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		record, err := service.Store.Load(r.PathValue("id"))
		if err != nil {
			storeError(w, "run", err)
			return
		}
		writeJSON(w, http.StatusOK, record)
	})

	mux.HandleFunc("GET /api/runs/{id}/logs/{step}", func(w http.ResponseWriter, r *http.Request) {
		text, err := service.Store.ReadLog(r.PathValue("id"), r.PathValue("step"))
		if err != nil {
			storeError(w, "log", err)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, text)
	})
}

func storeError(w http.ResponseWriter, what string, err error) {
	if errors.Is(err, runs.ErrNotFound) {
		http.Error(w, what+" not found", http.StatusNotFound)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
