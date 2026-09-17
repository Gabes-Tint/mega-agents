package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Gabes-Tint/mega-agents/internal/megahome"
)

var workflowName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

var errWorkflowNotFound = errors.New("workflow not found")

// WorkflowStore keeps workflows as the YAML files the editor exports, one
// per name under Root, so they can be committed, diffed and run headless.
type WorkflowStore struct {
	Root string
}

type WorkflowSummary struct {
	Name      string `json:"name"`
	UpdatedAt string `json:"updatedAt"`
}

// DefaultWorkflowStore keeps workflows under the Mega Agents home.
func DefaultWorkflowStore() (WorkflowStore, error) {
	home, err := megahome.Dir()
	if err != nil {
		return WorkflowStore{}, err
	}
	return WorkflowStore{Root: filepath.Join(home, "workflows")}, nil
}

func checkWorkflowName(name string) error {
	if !workflowName.MatchString(name) {
		return fmt.Errorf("workflow name %q: use lowercase letters, digits and -", name)
	}
	return nil
}

// Path is where the named workflow lives.
func (store WorkflowStore) Path(name string) string {
	return filepath.Join(store.Root, name+".yaml")
}

// Save validates the graph and writes it as YAML under the name.
func (store WorkflowStore) Save(name string, request WorkflowRequest) error {
	if err := checkWorkflowName(name); err != nil {
		return err
	}
	request.Name = name
	document, err := BuildWorkflowYAML(request)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(store.Root, 0o750); err != nil {
		return fmt.Errorf("cannot create the workflows folder: %w", err)
	}
	temporary, err := os.CreateTemp(store.Root, "."+name+"-*.yaml")
	if err != nil {
		return fmt.Errorf("cannot save workflow %s: %w", name, err)
	}
	defer os.Remove(temporary.Name())
	if _, err := io.WriteString(temporary, document); err != nil {
		temporary.Close()
		return fmt.Errorf("cannot save workflow %s: %w", name, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("cannot save workflow %s: %w", name, err)
	}
	return os.Rename(temporary.Name(), store.Path(name))
}

// Load reads the named workflow back into the editor's graph.
func (store WorkflowStore) Load(name string) (WorkflowRequest, error) {
	if err := checkWorkflowName(name); err != nil {
		return WorkflowRequest{}, errWorkflowNotFound
	}
	document, err := os.ReadFile(store.Path(name))
	if errors.Is(err, fs.ErrNotExist) {
		return WorkflowRequest{}, errWorkflowNotFound
	}
	if err != nil {
		return WorkflowRequest{}, fmt.Errorf("cannot read workflow %s: %w", name, err)
	}
	return ParseWorkflowYAML(document)
}

// List returns saved workflows by name.
func (store WorkflowStore) List() ([]WorkflowSummary, error) {
	entries, err := os.ReadDir(store.Root)
	if errors.Is(err, fs.ErrNotExist) {
		return []WorkflowSummary{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot list workflows: %w", err)
	}
	workflows := []WorkflowSummary{}
	for _, entry := range entries {
		name, isYAML := strings.CutSuffix(entry.Name(), ".yaml")
		if !isYAML || checkWorkflowName(name) != nil {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		workflows = append(workflows, WorkflowSummary{Name: name, UpdatedAt: info.ModTime().UTC().Format(time.RFC3339)})
	}
	slices.SortFunc(workflows, func(a, b WorkflowSummary) int { return strings.Compare(a.Name, b.Name) })
	return workflows, nil
}

func registerWorkflowStoreHandler(mux *http.ServeMux, store WorkflowStore) {
	mux.HandleFunc("GET /api/workflows", func(w http.ResponseWriter, _ *http.Request) {
		workflows, err := store.List()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, workflows)
	})

	mux.HandleFunc("GET /api/workflows/{name}", func(w http.ResponseWriter, r *http.Request) {
		request, err := store.Load(r.PathValue("name"))
		if errors.Is(err, errWorkflowNotFound) {
			http.Error(w, "workflow "+r.PathValue("name")+" not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, request)
	})

	// Saving writes a file, so like runs it only accepts JSON, which
	// browsers must preflight cross-origin.
	mux.HandleFunc("PUT /api/workflows/{name}", func(w http.ResponseWriter, r *http.Request) {
		mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if mediaType != "application/json" {
			http.Error(w, "saving a workflow requires a JSON request", http.StatusUnsupportedMediaType)
			return
		}
		var request WorkflowRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, workflowMaxBodyBytes)).Decode(&request); err != nil {
			http.Error(w, "invalid workflow request", http.StatusBadRequest)
			return
		}
		if err := store.Save(r.PathValue("name"), request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, WorkflowSummary{Name: r.PathValue("name"), UpdatedAt: time.Now().UTC().Format(time.RFC3339)})
	})

	// Import only translates a YAML document into the graph; it saves nothing.
	mux.HandleFunc("POST /api/workflows/import", func(w http.ResponseWriter, r *http.Request) {
		document, err := io.ReadAll(http.MaxBytesReader(w, r.Body, workflowMaxBodyBytes))
		if err != nil {
			http.Error(w, "the workflow is too large", http.StatusBadRequest)
			return
		}
		request, err := ParseWorkflowYAML(document)
		if err == nil {
			_, err = validateGraph(request)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, request)
	})
}
