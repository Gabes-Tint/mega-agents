package main

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"os"
)

//go:embed all:internal/web/dist
var frontend embed.FS

func main() {
	assets, err := fs.Sub(frontend, "internal/web/dist")
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "Mega Agents backend is running"})
	})
	mux.Handle("/", http.FileServer(http.FS(assets)))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port
	log.Printf("listening on http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(addr, mux))
}
