package app

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
)

type StatusResponse struct {
	Message string `json:"message"`
}

type DirectoriesResponse struct {
	Path        string   `json:"path"`
	Directories []string `json:"directories"`
}

func NewHandler(assets fs.FS) http.Handler {
	mux := http.NewServeMux()
	registerWorkflowYAMLHandler(mux)
	registerRunHandler(mux)
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
