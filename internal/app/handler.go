package app

import (
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/Gabes-Tint/mega-agents/internal/agents"
	"github.com/Gabes-Tint/mega-agents/internal/runs"
)

type StatusResponse struct {
	Message string `json:"message"`
}

type DirectoriesResponse struct {
	Path        string   `json:"path"`
	Directories []string `json:"directories"`
}

// NewHandler serves the editor and its API, recording runs under the Mega
// Agents home. It never checks the agent backends: the server starts that
// check and hands it to NewHandlerWithHealth.
func NewHandler(assets fs.FS) http.Handler {
	return NewHandlerWithHealth(assets, agents.NewHealthChecker())
}

// NewHandlerWithHealth serves the editor, reporting the given agent check.
func NewHandlerWithHealth(assets fs.FS, health *agents.HealthChecker) http.Handler {
	store, err := runs.DefaultStore()
	if err != nil {
		// Without a home there is nowhere to record runs; starting one then
		// fails with this explanation instead of the server refusing to start.
		store = &runs.Store{Root: filepath.Join(os.TempDir(), "mega-agents-runs-unavailable")}
		log.Printf("recording runs under %s: %v", store.Root, err)
	}
	workflows, err := DefaultWorkflowStore()
	if err != nil {
		workflows = WorkflowStore{Root: filepath.Join(os.TempDir(), "mega-agents-workflows-unavailable")}
		log.Printf("saving workflows under %s: %v", workflows.Root, err)
	}
	return newHandler(assets, Runs{Store: store}, workflows, health)
}

func NewHandlerWithRuns(assets fs.FS, service Runs, workflows WorkflowStore) http.Handler {
	return newHandler(assets, service, workflows, agents.NewHealthChecker())
}

func newHandler(assets fs.FS, service Runs, workflows WorkflowStore, health *agents.HealthChecker) http.Handler {
	mux := http.NewServeMux()
	registerAgentHealthHandler(mux, health)
	registerWorkflowYAMLHandler(mux)
	registerProblemsHandler(mux)
	registerWorkflowStoreHandler(mux, workflows)
	registerTemplateHandler(mux)
	registerRunHandler(mux, service)
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(StatusResponse{Message: "Mega Agents backend is running"})
	})
	mux.HandleFunc("GET /api/directories", func(w http.ResponseWriter, r *http.Request) {
		target := r.URL.Query().Get("path")
		if target == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				http.Error(w, "cannot resolve home directory", http.StatusInternalServerError)
				return
			}
			target = home
		}
		absolute, err := filepath.Abs(filepath.Clean(target))
		if err != nil {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}
		entries, err := os.ReadDir(absolute)
		if err != nil {
			http.Error(w, "path is not a readable directory", http.StatusBadRequest)
			return
		}
		directories := []string{}
		for _, entry := range entries {
			if entry.IsDir() {
				directories = append(directories, entry.Name())
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DirectoriesResponse{Path: absolute, Directories: directories})
	})
	mux.Handle("/", http.FileServer(http.FS(assets)))
	return mux
}
