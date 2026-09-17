package app

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gabes-Tint/mega-agents/internal/engine"
)

// retryFlow: a worktree, an agent working in it, and a gate that passes
// only once a marker file exists in the project folder.
func retryFlow(t *testing.T, marker string) string {
	t.Helper()
	clone, _ := projectClone(t)
	worktree := filepath.Join(t.TempDir(), "retry")
	return fmt.Sprintf(`{"name": "retry-me", "nodes": [
		{"id": "p1", "type": "project", "name": "api", "path": %q},
		{"id": "g1", "type": "github", "name": "GitHub 1", "parentId": "p1", "start": true},
		{"id": "w1", "type": "action", "action": "worktree", "name": "Create worktree", "parentId": "g1", "start": true, "branch": "retry", "worktreePath": %q},
		{"id": "a1", "type": "agent", "name": "Coder", "parentId": "p1", "backend": "claude", "prompt": "Code"},
		{"id": "c1", "type": "command", "name": "Gate", "parentId": "p1", "command": "pwd && test -e %s"}
	], "edges": [{"id": "e1", "from": "w1", "to": "a1"}, {"id": "e2", "from": "a1", "to": "c1"}]}`, clone, worktree, marker)
}

func postRetry(t *testing.T, handler http.Handler, id string) (int, string) {
	t.Helper()
	response := workflowRequest(t, handler, http.MethodPost, "/api/runs/"+id+"/retry", "{}", "application/json")
	return response.Code, response.Body.String()
}

func TestARetryReusesTheStepsThatSucceededAndRunsTheRest(t *testing.T) {
	bin := fakeClaude(t)
	marker := filepath.Join(t.TempDir(), "fixed")
	failed, handler := finishedRun(t, retryFlow(t, marker))
	if failed.Status != engine.Failed || failed.Steps[2].Status != engine.Failed {
		t.Fatalf("first run = %+v", failed)
	}
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	code, body := postRetry(t, handler, failed.ID)

	if code != http.StatusAccepted {
		t.Fatalf("retry = %d %s", code, body)
	}
	retried := awaitRun(t, handler, decodeRecordText(t, body).ID)
	if retried.Status != engine.Succeeded || retried.RetryOf != failed.ID || retried.Workflow != "retry-me" {
		t.Fatalf("retried = %+v", retried)
	}
	for _, step := range retried.Steps[:2] {
		if step.Details["reusedFrom"] != failed.ID {
			t.Errorf("%s was not reused: %+v", step.Name, step)
		}
	}
	if recorded(t, bin, "count") != "1" {
		t.Fatalf("the agent ran again: %s turns", recorded(t, bin, "count"))
	}
	if output := retried.Steps[2].Details["output"].(string); !strings.Contains(output, "retry") {
		t.Fatalf("the gate did not run in the reused worktree: %q", output)
	}
	log := get(t, handler, "/api/runs/"+retried.ID+"/logs/a1").Body.String()
	if !strings.Contains(log, "Reused the result of Coder from run "+failed.ID) {
		t.Fatalf("log = %q", log)
	}
}

func TestRetryRefusesRunsItCannotRetry(t *testing.T) {
	fakeClaude(t)
	succeeded, handler := finishedRun(t, retryFlow(t, "/"))

	if code, body := postRetry(t, handler, succeeded.ID); code != http.StatusConflict || !strings.Contains(body, "succeeded; there is nothing to retry") {
		t.Fatalf("retry succeeded run = %d %s", code, body)
	}
	if code, _ := postRetry(t, handler, "20000101T000000Z-000000"); code != http.StatusNotFound {
		t.Fatalf("retry unknown = %d", code)
	}
	response := workflowRequest(t, handler, http.MethodPost, "/api/runs/"+succeeded.ID+"/retry", "{}", "text/plain")
	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("text/plain = %d", response.Code)
	}
}

func decodeRecordText(t *testing.T, body string) struct{ ID string } {
	t.Helper()
	var record struct{ ID string }
	if err := jsonUnmarshal(body, &record); err != nil {
		t.Fatal(err)
	}
	return record
}
