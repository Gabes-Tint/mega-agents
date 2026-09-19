package main

import (
	"context"
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
	os.Exit(cli.Main(os.Args[1:], cli.Env{
		Stdout: os.Stdout, Stderr: os.Stderr, Serve: serve,
		Stdin: os.Stdin, Interactive: isTerminal(os.Stdin),
	}))
}

// isTerminal reports whether someone is there to answer a question, such as
// what a run does at a breakpoint. A piped or redirected run has nobody.
func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
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
	handler, scheduler := app.NewEditor(assets, health)
	// The schedule is kept for as long as the server is up: workflows with
	// a cron expression start themselves, and stop when the server stops.
	keeping, stop := context.WithCancel(context.Background())
	defer stop()
	go scheduler.Run(keeping)
	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return server.ListenAndServe()
}
