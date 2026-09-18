package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/Gabes-Tint/mega-agents/internal/engine"
)

// Breakpoints holds the runs this process has paused at a breakpoint, so
// the resume endpoint can answer the block each one waits before. A run
// waits for as long as it takes; nothing expires it.
type Breakpoints struct {
	mu sync.Mutex
	// waiting maps a run to the blocks it waits before, each with the
	// channel its pause listens on.
	waiting map[string]map[string]chan engine.Resume
}

var errNotPaused = errors.New("is not paused here")

// Wait records the run as waiting before the block and blocks until someone
// resumes it, or until the run is cancelled.
func (breakpoints *Breakpoints) Wait(ctx context.Context, runID string, at engine.Pause) engine.Resume {
	answer := make(chan engine.Resume, 1)
	breakpoints.mu.Lock()
	if breakpoints.waiting == nil {
		breakpoints.waiting = map[string]map[string]chan engine.Resume{}
	}
	if breakpoints.waiting[runID] == nil {
		breakpoints.waiting[runID] = map[string]chan engine.Resume{}
	}
	breakpoints.waiting[runID][at.TaskID] = answer
	breakpoints.mu.Unlock()
	defer breakpoints.forget(runID, at.TaskID)
	select {
	case resume := <-answer:
		return resume
	case <-ctx.Done():
		return engine.Resume{}
	}
}

func (breakpoints *Breakpoints) forget(runID string, nodeID string) {
	breakpoints.mu.Lock()
	defer breakpoints.mu.Unlock()
	delete(breakpoints.waiting[runID], nodeID)
	if len(breakpoints.waiting[runID]) == 0 {
		delete(breakpoints.waiting, runID)
	}
}

// Resume answers the block a run waits before. An empty nodeID answers the
// only block it waits before, and names them all when there are several.
func (breakpoints *Breakpoints) Resume(runID string, nodeID string, resume engine.Resume) error {
	breakpoints.mu.Lock()
	defer breakpoints.mu.Unlock()
	waiting := breakpoints.waiting[runID]
	if len(waiting) == 0 {
		return fmt.Errorf("run %s %w", runID, errNotPaused)
	}
	if nodeID == "" {
		if len(waiting) > 1 {
			blocks := make([]string, 0, len(waiting))
			for id := range waiting {
				blocks = append(blocks, id)
			}
			slices.Sort(blocks)
			return fmt.Errorf("run %s waits at %s; name the block to resume", runID, strings.Join(blocks, ", "))
		}
		for id := range waiting {
			nodeID = id
		}
	}
	answer, paused := waiting[nodeID]
	if !paused {
		return fmt.Errorf("run %s %w before %s", runID, errNotPaused, nodeID)
	}
	answer <- resume
	delete(waiting, nodeID)
	return nil
}

// resumeRequest is what the editor sends to take a paused run on. The
// prompt and command are the edit the block runs with, for this run only.
type resumeRequest struct {
	NodeID  string        `json:"nodeId"`
	Action  engine.Action `json:"action"`
	Prompt  string        `json:"prompt"`
	Command string        `json:"command"`
}

var resumeActions = []engine.Action{engine.ResumeContinue, engine.ResumeStep, engine.ResumeSkip}

func registerResumeHandler(mux *http.ServeMux, service Runs, breakpoints *Breakpoints) {
	mux.HandleFunc("POST /api/runs/{id}/resume", func(w http.ResponseWriter, r *http.Request) {
		mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if mediaType != "application/json" {
			http.Error(w, "resuming a run requires a JSON request", http.StatusUnsupportedMediaType)
			return
		}
		var request resumeRequest
		body := http.MaxBytesReader(w, r.Body, workflowMaxBodyBytes)
		if err := json.NewDecoder(body).Decode(&request); err != nil {
			http.Error(w, "invalid resume request", http.StatusBadRequest)
			return
		}
		if !slices.Contains(resumeActions, request.Action) {
			http.Error(w, fmt.Sprintf(
				"unknown action %q; use %s, %s or %s",
				request.Action, engine.ResumeContinue, engine.ResumeStep, engine.ResumeSkip,
			), http.StatusBadRequest)
			return
		}
		id := r.PathValue("id")
		if _, err := service.Store.Load(id); err != nil {
			storeError(w, "run", err)
			return
		}
		resume := engine.Resume{
			Action: request.Action,
			Edit:   engine.Edit{Prompt: request.Prompt, Command: request.Command},
		}
		if err := breakpoints.Resume(id, request.NodeID, resume); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{
			"id": id, "nodeId": request.NodeID, "action": string(request.Action),
		})
	})
}

// editedPrompt is the prompt a block runs with: the edit made at its
// breakpoint, or its own text with the placeholders filled in. An edit is
// what the person saw and changed, so it runs as written.
func editedPrompt(ctx context.Context, text string, inputs []engine.Input, sources templateSources) string {
	if edit, edited := engine.EditIn(ctx); edited && edit.Prompt != "" {
		return edit.Prompt
	}
	return renderTemplate(text, inputs, sources)
}

// editedCommand is the command a block runs: the edit made at its
// breakpoint, or the command the workflow holds.
func editedCommand(ctx context.Context, command string) string {
	if edit, edited := engine.EditIn(ctx); edited && edit.Command != "" {
		return edit.Command
	}
	return command
}
