package app

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"path/filepath"
	"time"

	"github.com/Gabes-Tint/mega-agents/internal/gitops"
)

// runStepTimeout bounds one Git action so an unreachable remote cannot hold
// the request open indefinitely.
const runStepTimeout = 2 * time.Minute

type RunStep struct {
	NodeID string `json:"nodeId"`
	Name   string `json:"name"`
	Action string `json:"action"`
	Status string `json:"status"`
	Remote string `json:"remote,omitempty"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

type RunResponse struct {
	Status string    `json:"status"`
	Steps  []RunStep `json:"steps"`
}

var errNoStartingGitHub = errors.New(
	"flag a GitHub block as the starting point to run the flow",
)

// RunWorkflow executes the flow's runnable starting points. In this first
// slice only a GitHub block flagged as a starting point runs, and its action
// is a fetch inside the path of the project that contains it. Downstream
// nodes are not executed yet.
func RunWorkflow(ctx context.Context, request WorkflowRequest) (RunResponse, error) {
	nodeByID, err := validateGraph(request)
	if err != nil {
		return RunResponse{}, err
	}
	run := RunResponse{Status: "succeeded", Steps: []RunStep{}}
	for _, node := range request.Nodes {
		if node.Type != "github" || !node.Start {
			continue
		}
		step := fetchGitHubBlock(ctx, node, nodeByID[node.ParentID])
		if step.Status != "succeeded" {
			run.Status = "failed"
		}
		run.Steps = append(run.Steps, step)
	}
	if len(run.Steps) == 0 {
		return RunResponse{}, errNoStartingGitHub
	}
	return run, nil
}

func fetchGitHubBlock(ctx context.Context, node WorkflowNodeInput, project WorkflowNodeInput) RunStep {
	step := RunStep{NodeID: node.ID, Name: node.Name, Action: "fetch", Status: "failed"}
	switch {
	case !node.Authenticated:
		step.Error = `Check "Already authenticated" on this GitHub block; token authentication is not supported yet`
		return step
	case node.Repository == "":
		step.Error = "Set the repository on this GitHub block"
		return step
	case project.Path == "":
		step.Error = "Set the path on the project that contains this GitHub block"
		return step
	case !filepath.IsAbs(project.Path):
		step.Error = "The project path " + project.Path + " must be absolute"
		return step
	}
	ctx, cancel := context.WithTimeout(ctx, runStepTimeout)
	defer cancel()
	result, err := gitops.Fetch(ctx, project.Path, node.Repository)
	if err != nil {
		step.Error = err.Error()
		return step
	}
	step.Status = "succeeded"
	step.Remote = result.Remote
	step.Output = result.Output
	return step
}

func registerRunHandler(mux *http.ServeMux) {
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
