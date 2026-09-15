package app

import (
	"encoding/json"
	"io/fs"
	"net/http"
)

type StatusResponse struct {
	Message string `json:"message"`
}

func NewHandler(assets fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(StatusResponse{Message: "Mega Agents backend is running"})
	})
	mux.Handle("/", http.FileServer(http.FS(assets)))
	return mux
}
