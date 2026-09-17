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
	"sync"
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

// Cancelled is the status of a run someone stopped.
const Cancelled = runs.Cancelled

var errNoStartingBlock = errors.New(
	"flag a GitHub block or an agent as the starting point to run the flow",
)

// gitActionLabels names each Git action a GitHub block can hold.
var gitActionLabels = map[string]string{
	"fetch":       "Fetch",
	"worktree":    "Create worktree",
	"rebase":      "Rebase",
	"issue":       "Read issue",
	"commit":      "Commit",
	"push":        "Push",
	"pullrequest": "Open pull request",
}

const (
	pullRequestPort = "pullRequest"
	issuePort       = "issue"
)

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
	if errors.Is(ctx.Err(), context.Canceled) {
		record.Status = Cancelled
	}
	record.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	save()
	return record
}

type runPlanner struct {
	nodeByID   map[string]WorkflowNodeInput
	childrenOf map[string][]WorkflowNodeInput
	request    WorkflowRequest
	// included are the blocks this run executes.
	included map[string]bool
}

func planRun(request WorkflowRequest) ([]engine.Task, error) {
	nodeByID, err := validateGraph(request)
	if err != nil {
		return nil, err
	}
	included := map[string]bool{}
	planner := runPlanner{nodeByID: nodeByID, childrenOf: map[string][]WorkflowNodeInput{}, request: request, included: included}
	for _, node := range request.Nodes {
		if node.ParentID != "" {
			planner.childrenOf[node.ParentID] = append(planner.childrenOf[node.ParentID], node)
		}
	}
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
			task, err = planner.agentTask(node)
		case "jsonschema":
			task, err = planner.schemaTask(node)
		case "router":
			task, err = planner.routerTask(node)
		case "command":
			task, err = planner.commandTask(node)
		default:
			task, err = planner.actionTask(node)
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
var executableTypes = map[string]bool{"action": true, "agent": true, "jsonschema": true, "router": true, "command": true}

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

// needsWorkspace are the Git actions that work inside a workspace.
var needsWorkspace = map[string]bool{"rebase": true, "commit": true, "push": true, "pullrequest": true}

func (planner runPlanner) actionTask(node WorkflowNodeInput) (engine.Task, error) {
	if _, known := gitActionLabels[node.Action]; !known {
		return engine.Task{}, fmt.Errorf("unknown Git action %q on %s", node.Action, node.Name)
	}
	fail := func(format string, args ...any) (engine.Task, error) {
		return engine.Task{}, fmt.Errorf("%s: %s", node.Name, fmt.Sprintf(format, args...))
	}
	github := planner.nodeByID[node.ParentID]
	needs, err := planner.incomingNeeds(node)
	if err != nil {
		return engine.Task{}, err
	}
	sources := planner.sourcesOf(needs)
	if needsWorkspace[node.Action] && !sources.workspace {
		return engine.Task{}, fmt.Errorf(
			"%s needs a workspace; connect a %s action before it", node.Name, gitActionLabels["worktree"],
		)
	}
	for _, text := range []string{node.Message, node.Title, node.Body} {
		if err := checkTemplate(text, sources); err != nil {
			return fail("%v", err)
		}
	}
	if node.Action == "issue" && node.Issue <= 0 {
		return fail("set the issue number")
	}
	if node.Action == "fetch" {
		task := planner.fetchTask(node, github)
		task.Needs = needs
		return task, nil
	}
	return engine.Task{
		ID: node.ID, Name: node.Name, Kind: node.Action, Needs: needs,
		Run: func(ctx context.Context, inputs []engine.Input, log io.Writer) (engine.Result, error) {
			ctx, cancel := context.WithTimeout(gitops.WithLog(ctx, log), runStepTimeout)
			defer cancel()
			workspace, _ := workspaceIn(inputs)
			networked := node.Action == "push" || node.Action == "pullrequest" || node.Action == "issue"
			if networked && !github.Authenticated {
				return engine.Result{}, errors.New(
					`check "Already authenticated" on this GitHub block; token authentication is not supported yet`,
				)
			}
			switch node.Action {
			case "rebase":
				return workspaceResult(gitops.Rebase(ctx, workspace, node.Onto))
			case "commit":
				message := renderTemplate(node.Message, inputs, sources)
				if strings.TrimSpace(node.Message) == "" {
					message = "Changes from Mega Agents run"
				}
				commit, err := gitops.Commit(ctx, workspace, message)
				result, _ := workspaceResult(workspace, err)
				if err == nil {
					result.Details["commit"] = commit
				}
				return result, err
			case "push":
				return workspaceResult(workspace, gitops.Push(ctx, workspace))
			case "pullrequest":
				pull, err := gitops.OpenPullRequest(ctx, workspace, gitops.PullRequest{
					Title: renderTemplate(node.Title, inputs, sources),
					Body:  renderTemplate(node.Body, inputs, sources), Base: node.Base,
				})
				if err != nil {
					return engine.Result{}, err
				}
				value := map[string]any{"number": pull.Number, "url": pull.URL, "base": pull.Base, "title": pull.Title}
				result, _ := workspaceResult(workspace, nil)
				result.Outputs[pullRequestPort] = value
				result.Details["url"], result.Details["number"] = pull.URL, pull.Number
				return result, nil
			case "issue":
				repository := github.Repository
				if strings.TrimSpace(repository) == "" {
					dir, err := projectDir(planner.nodeByID[github.ParentID])
					if err != nil {
						return engine.Result{}, err
					}
					detected, err := gitops.DetectGitHubRepository(ctx, dir)
					if err != nil {
						return engine.Result{}, err
					}
					repository = detected.Repository
				}
				issue, err := gitops.ReadIssue(ctx, repository, node.Issue)
				if err != nil {
					return engine.Result{}, err
				}
				return engine.Result{Outputs: map[string]any{issuePort: issue}, Details: map[string]any{"issue": issue}}, nil
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

// incomingNeeds lists what a block receives from the executed blocks with
// arrows into it: each arrow's output, and the workspace a block passes on
// alongside its own output.
func (planner runPlanner) incomingNeeds(node WorkflowNodeInput) ([]engine.Need, error) {
	var needs []engine.Need
	for _, edge := range planner.request.Edges {
		if edge.To != node.ID || !planner.included[edge.From] {
			continue
		}
		port, err := planner.sourcePort(edge)
		if err != nil {
			return nil, err
		}
		needs = append(needs, engine.Need{TaskID: edge.From, Port: port})
		if port != workspacePort && planner.passesWorkspace(edge.From, map[string]bool{}) {
			needs = append(needs, engine.Need{TaskID: edge.From, Port: workspacePort})
		}
	}
	return needs, nil
}

// passesWorkspace reports whether a block outputs a workspace: the Git
// actions that prepare or keep one, and agents and commands that receive one.
func (planner runPlanner) passesWorkspace(id string, visiting map[string]bool) bool {
	node := planner.nodeByID[id]
	switch node.Type {
	case "action":
		return node.Action == "worktree" || needsWorkspace[node.Action]
	case "agent", "command":
		if visiting[id] {
			return false
		}
		visiting[id] = true
		for _, edge := range planner.request.Edges {
			if edge.To == id && planner.included[edge.From] && planner.passesWorkspace(edge.From, visiting) {
				return true
			}
		}
	}
	return false
}

// sourcePort names the output an arrow carries from its source block, or
// returns "" for an arrow that only orders the two blocks.
func (planner runPlanner) sourcePort(edge WorkflowEdgeInput) (string, error) {
	source := planner.nodeByID[edge.From]
	switch {
	case source.Type == "action" && source.Action == "pullrequest":
		return pullRequestPort, nil
	case source.Type == "action" && source.Action == "issue":
		return issuePort, nil
	case source.Type == "action" && (source.Action == "worktree" || needsWorkspace[source.Action]):
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
	case source.Type == "command":
		switch edge.FromPort {
		case "":
			return passedPort, nil
		case passedPort, failedPort:
			return edge.FromPort, nil
		}
		return "", fmt.Errorf("%s has no output %q; use %s or %s", source.Name, edge.FromPort, passedPort, failedPort)
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
func (planner runPlanner) valueNeeds(node WorkflowNodeInput, verb string) ([]engine.Need, error) {
	incoming, err := planner.incomingNeeds(node)
	if err != nil {
		return nil, err
	}
	var needs []engine.Need
	for _, need := range incoming {
		if need.Port != workspacePort {
			needs = append(needs, need)
		}
	}
	if len(needs) != 1 || needs[0].Port == "" {
		return nil, fmt.Errorf("%s: %s one value; connect exactly one block to it", node.Name, verb)
	}
	return needs, nil
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
	// Cancel functions of the runs this server is executing, by run id.
	var active sync.Map

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
		ctx, cancel := context.WithCancel(context.WithoutCancel(r.Context()))
		active.Store(record.ID, cancel)
		go func() {
			defer active.Delete(record.ID)
			defer cancel()
			execute(ctx, nil)
		}()
		writeJSON(w, http.StatusAccepted, record)
	})

	mux.HandleFunc("POST /api/runs/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if mediaType != "application/json" {
			http.Error(w, "cancelling a run requires a JSON request", http.StatusUnsupportedMediaType)
			return
		}
		id := r.PathValue("id")
		if cancel, running := active.Load(id); running {
			cancel.(context.CancelFunc)()
			writeJSON(w, http.StatusAccepted, map[string]string{"id": id, "status": "cancelling"})
			return
		}
		if _, err := service.Store.Load(id); err != nil {
			storeError(w, "run", err)
			return
		}
		http.Error(w, "run "+id+" is not running here", http.StatusConflict)
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
