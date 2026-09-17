package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Gabes-Tint/mega-agents/internal/app"
	"github.com/Gabes-Tint/mega-agents/internal/cli"
)

//go:embed all:internal/web/dist
var frontend embed.FS

func main() {
	os.Exit(cli.Main(os.Args[1:], cli.Env{Stdout: os.Stdout, Stderr: os.Stderr, Serve: serve}))
}

func serve() error {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port
	log.Printf("listening on http://localhost:%s", port)
	assets, err := fs.Sub(frontend, "internal/web/dist")
	if err != nil {
		return err
	}
	// Each installed agent backend gets one tiny prompt in the background,
	// so the status bar can show which ones answer.
	health := app.AgentHealthFromEnvironment()
	health.Start()
	server := &http.Server{
		Addr:              addr,
		Handler:           app.NewHandlerWithHealth(assets, health),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return server.ListenAndServe()
}
