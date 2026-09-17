package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"time"

	"github.com/Gabes-Tint/mega-agents/internal/engine"
	"github.com/Gabes-Tint/mega-agents/internal/gitops"
)

// runStepTimeout bounds one Git action so an unreachable remote cannot hold
// the request open indefinitely.
const runStepTimeout = 2 * time.Minute

const workspacePort = "workspace"

var errNoStartingGitHub = errors.New(
	"flag a GitHub block as the starting point to run the flow",
)

// gitActionLabels names each Git action a GitHub block can hold.
var gitActionLabels = map[string]string{
	"fetch":    "Fetch",
	"worktree": "Create worktree",
	"rebase":   "Rebase",
}

// RunWorkflow plans the flow and executes it. A GitHub block flagged as a
// starting point runs its own actions from its starting action along their
// arrows, or a plain fetch when it holds no actions. Actions not reachable
// from a starting action do not run.
func RunWorkflow(ctx context.Context, request WorkflowRequest) (engine.Run, error) {
	tasks, err := planRun(request)
	if err != nil {
		return engine.Run{}, err
	}
	return engine.Execute(ctx, tasks, nil)
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
		if node.Type != "github" || !node.Start {
			continue
		}
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
	if len(tasks) == 0 && len(included) == 0 {
		return nil, errNoStartingGitHub
	}
	for _, node := range request.Nodes {
		if !included[node.ID] {
			continue
		}
		task, err := planner.actionTask(node, included)
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

// include marks the node and every executable node its arrows reach.
func (planner runPlanner) include(id string, included map[string]bool) {
	if included[id] || planner.nodeByID[id].Type != "action" {
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
		need := engine.Need{TaskID: edge.From}
		if producesWorkspace(planner.nodeByID[edge.From]) && node.Action == "rebase" {
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
		Run: func(ctx context.Context, inputs []engine.Input) (engine.Result, error) {
			ctx, cancel := context.WithTimeout(ctx, runStepTimeout)
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
		Run: func(ctx context.Context, _ []engine.Input) (engine.Result, error) {
			if !github.Authenticated {
				return engine.Result{}, errors.New(
					`check "Already authenticated" on this GitHub block; token authentication is not supported yet`,
				)
			}
			dir, err := projectDir(planner.nodeByID[github.ParentID])
			if err != nil {
				return engine.Result{}, err
			}
			ctx, cancel := context.WithTimeout(ctx, runStepTimeout)
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

func registerRunHandler(mux *http.ServeMux) {
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
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(detected)
	})

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
		run, err := RunWorkflow(r.Context(), request)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(run)
	})
}
