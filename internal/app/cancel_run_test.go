package app

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Gabes-Tint/mega-agents/internal/engine"
)

// slowClaude is a claude that takes far longer than any test waits.
func slowClaude(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\nsleep 30\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func slowAgentFlow(t *testing.T) string {
	return fmt.Sprintf(`{"nodes": [
		{"id": "p1", "type": "project", "name": "api", "path": %q},
		{"id": "a1", "type": "agent", "name": "Slow", "parentId": "p1", "start": true, "backend": "claude", "prompt": "Think"},
		{"id": "a2", "type": "agent", "name": "After", "parentId": "p1", "backend": "claude", "prompt": "Next"}
	], "edges": [{"id": "e1", "from": "a1", "to": "a2"}]}`, t.TempDir())
}

func cancelRun(t *testing.T, handler http.Handler, id string, contentType string) int {
	t.Helper()
	return workflowRequest(t, handler, http.MethodPost, "/api/runs/"+id+"/cancel", "{}", contentType).Code
}

func TestARunningRunCanBeCancelled(t *testing.T) {
	slowClaude(t)
	response, handler := postRun(t, slowAgentFlow(t))
	started := decodeRecord(t, response)
	waitForStep(t, handler, started.ID, engine.Running)
	began := time.Now()

	if code := cancelRun(t, handler, started.ID, "application/json"); code != http.StatusAccepted {
		t.Fatalf("cancel = %d", code)
	}
	record := awaitRun(t, handler, started.ID)

	if time.Since(began) > 5*time.Second {
		t.Fatal("cancelling did not stop the agent promptly")
	}
	if record.Status != Cancelled || record.Steps[0].Status != engine.Failed || record.Steps[1].Status != engine.Skipped {
		t.Fatalf("record = %+v", record)
	}
	if !strings.Contains(record.Steps[1].Error, "Slow failed") && !strings.Contains(record.Steps[1].Error, "cancelled") {
		t.Fatalf("skip reason = %q", record.Steps[1].Error)
	}
	if code := cancelRun(t, handler, started.ID, "application/json"); code != http.StatusConflict {
		t.Fatalf("cancelling a finished run = %d, want 409", code)
	}
}

func TestCancelRefusesUnknownRunsAndNonJSON(t *testing.T) {
	_, handler := postRun(t, `{}`)

	if code := cancelRun(t, handler, "20000101T000000Z-000000", "application/json"); code != http.StatusNotFound {
		t.Fatalf("unknown = %d", code)
	}
	if code := cancelRun(t, handler, "20000101T000000Z-000000", "text/plain"); code != http.StatusUnsupportedMediaType {
		t.Fatalf("text/plain = %d", code)
	}
}

func waitForStep(t *testing.T, handler http.Handler, id string, status engine.Status) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		record := decodeRecord(t, get(t, handler, "/api/runs/"+id))
		if record.Steps[0].Status == status {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("the first step of run %s never became %s", id, status)
}
