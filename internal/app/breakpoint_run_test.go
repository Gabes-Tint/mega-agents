package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Gabes-Tint/mega-agents/internal/engine"
	"github.com/Gabes-Tint/mega-agents/internal/runs"
)

// breakpointFlow is a project whose agent carries a breakpoint, followed by
// a second agent that only runs once the first one has.
func breakpointFlow(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf(`{"name": "review", "nodes": [
		{"id": "p1", "type": "project", "name": "api", "path": %q},
		{"id": "a1", "type": "agent", "name": "Coder", "parentId": "p1", "start": true,
		 "backend": "claude", "prompt": "Write the fix", "breakpoint": true},
		{"id": "a2", "type": "agent", "name": "Reviewer", "parentId": "p1",
		 "backend": "claude", "prompt": "Review {{result}}"}
	], "edges": [{"id": "e1", "from": "a1", "to": "a2"}]}`, t.TempDir())
}

// awaitPaused follows a run until it waits before the block.
func awaitPaused(t *testing.T, handler http.Handler, id string, nodeID string) runs.Record {
	t.Helper()
	return awaitPauseWhere(t, handler, id, nodeID, func(engine.Pause) bool { return true })
}

// awaitPauseWhere follows a run until it waits before the block at a pause
// the test is looking for, such as a given repeat of a loop.
func awaitPauseWhere(t *testing.T, handler http.Handler, id string, nodeID string, match func(engine.Pause) bool) runs.Record {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		record := decodeRecord(t, get(t, handler, "/api/runs/"+id))
		for _, at := range record.Paused {
			if at.TaskID == nodeID && match(at) {
				return record
			}
		}
		if record.Status != engine.Running && record.Status != Paused {
			t.Fatalf("run %s ended %s without pausing before %s", id, record.Status, nodeID)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("run %s never paused before %s", id, nodeID)
	return runs.Record{}
}

func TestARunPausesBeforeTheBlockWithABreakpointAndContinues(t *testing.T) {
	bin := fakeClaude(t)
	response, handler := postRun(t, breakpointFlow(t))
	started := decodeRecord(t, response)

	record := awaitPaused(t, handler, started.ID, "a1")

	if record.Status != engine.Paused {
		t.Fatalf("status = %s, want the run to show as paused", record.Status)
	}
	at := record.Paused[0]
	if at.TaskID != "a1" || at.Name != "Coder" || at.Kind != "agent" || at.At == "" {
		t.Fatalf("paused = %+v", at)
	}
	if at.Resolved.Prompt != "Write the fix" {
		t.Fatalf("resolved = %+v, want the prompt the block is about to run", at.Resolved)
	}
	if record.Steps[0].Status != engine.Paused || record.Steps[0].StartedAt != "" {
		t.Fatalf("step = %+v, want a block that has not started", record.Steps[0])
	}

	if code := resume(t, handler, started.ID, `{"action": "continue"}`); code != http.StatusAccepted {
		t.Fatalf("resume = %d", code)
	}
	finished := awaitRun(t, handler, started.ID)

	if finished.Status != engine.Succeeded || len(finished.Paused) != 0 {
		t.Fatalf("record = %+v", finished)
	}
	if recorded(t, bin, "prompt-1") != "Write the fix" {
		t.Fatalf("the agent ran with %q", recorded(t, bin, "prompt-1"))
	}
	if finished.Steps[0].Edited != nil || finished.Steps[0].StartedAt == "" {
		t.Fatalf("step = %+v, want a block that ran unedited", finished.Steps[0])
	}
}

func TestAnEditAtABreakpointRunsThatRunOnlyAndKeepsTheOriginal(t *testing.T) {
	bin := fakeClaude(t)
	body := breakpointFlow(t)
	response, handler := postRun(t, body)
	started := decodeRecord(t, response)
	awaitPaused(t, handler, started.ID, "a1")

	code := resume(t, handler, started.ID, `{"nodeId": "a1", "action": "continue", "prompt": "Write the smallest fix"}`)
	if code != http.StatusAccepted {
		t.Fatalf("resume = %d", code)
	}
	record := awaitRun(t, handler, started.ID)

	if recorded(t, bin, "prompt-1") != "Write the smallest fix" {
		t.Fatalf("the agent ran with %q, want the edit made at the breakpoint", recorded(t, bin, "prompt-1"))
	}
	edited := record.Steps[0].Edited
	if edited == nil || edited.Ran.Prompt != "Write the smallest fix" || edited.Original.Prompt != "Write the fix" {
		t.Fatalf("edited = %+v, want what ran and what the workflow holds", edited)
	}
	var graph WorkflowRequest
	if err := json.Unmarshal(record.Graph, &graph); err != nil {
		t.Fatal(err)
	}
	for _, node := range graph.Nodes {
		if node.ID == "a1" && node.Prompt != "Write the fix" {
			t.Fatalf("the recorded workflow holds %q; an edit must never be written back", node.Prompt)
		}
	}
}

func TestSkippingAtABreakpointSkipsTheBlockAndWhatFollowsIt(t *testing.T) {
	fakeClaude(t)
	response, handler := postRun(t, breakpointFlow(t))
	started := decodeRecord(t, response)
	awaitPaused(t, handler, started.ID, "a1")

	if code := resume(t, handler, started.ID, `{"action": "skip"}`); code != http.StatusAccepted {
		t.Fatalf("resume = %d", code)
	}
	record := awaitRun(t, handler, started.ID)

	if record.Steps[0].Status != engine.Skipped || record.Steps[1].Status != engine.Skipped {
		t.Fatalf("steps = %+v", record.Steps)
	}
	if !strings.Contains(record.Steps[0].Error, "breakpoint") {
		t.Fatalf("skip reason = %q", record.Steps[0].Error)
	}
}

func TestSteppingAtABreakpointPausesBeforeTheNextBlock(t *testing.T) {
	fakeClaude(t)
	response, handler := postRun(t, breakpointFlow(t))
	started := decodeRecord(t, response)
	awaitPaused(t, handler, started.ID, "a1")

	if code := resume(t, handler, started.ID, `{"action": "step"}`); code != http.StatusAccepted {
		t.Fatalf("resume = %d", code)
	}
	record := awaitPaused(t, handler, started.ID, "a2")

	if record.Paused[0].TaskID != "a2" {
		t.Fatalf("paused = %+v, want the block after the one that was stepped", record.Paused)
	}
	if record.Paused[0].Resolved.Prompt != `Review {"text":"done"}` {
		t.Fatalf("resolved = %q, want the prompt with what arrived filled in", record.Paused[0].Resolved.Prompt)
	}
	if code := resume(t, handler, started.ID, `{"action": "continue"}`); code != http.StatusAccepted {
		t.Fatalf("resume = %d", code)
	}
	if finished := awaitRun(t, handler, started.ID); finished.Status != engine.Succeeded {
		t.Fatalf("record = %+v", finished)
	}
}

func TestABreakpointPausesOnlyItsOwnBlockWhileTheOthersRun(t *testing.T) {
	fakeClaude(t)
	body := fmt.Sprintf(`{"nodes": [
		{"id": "p1", "type": "project", "name": "api", "path": %q},
		{"id": "a1", "type": "agent", "name": "Coder", "parentId": "p1", "start": true,
		 "backend": "claude", "prompt": "Write the fix", "breakpoint": true},
		{"id": "a2", "type": "agent", "name": "Docs", "parentId": "p1", "start": true,
		 "backend": "claude", "prompt": "Write the notes"}
	]}`, t.TempDir())
	response, handler := postRun(t, body)
	started := decodeRecord(t, response)

	// The block beside the breakpoint runs to the end while the run waits.
	deadline := time.Now().Add(20 * time.Second)
	for {
		record := decodeRecord(t, get(t, handler, "/api/runs/"+started.ID))
		if record.Steps[1].Status == engine.Succeeded {
			if len(record.Paused) != 1 || record.Paused[0].TaskID != "a1" {
				t.Fatalf("paused = %+v, want the run still waiting before the block with the breakpoint", record.Paused)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the block beside the breakpoint never finished: %+v", record.Steps)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if code := resume(t, handler, started.ID, `{"action": "continue"}`); code != http.StatusAccepted {
		t.Fatalf("resume = %d", code)
	}
	if record := awaitRun(t, handler, started.ID); record.Status != engine.Succeeded {
		t.Fatalf("record = %+v", record)
	}
}

func TestABreakpointInsideALoopPausesOnEveryRepeat(t *testing.T) {
	fakeClaude(t)
	body := strings.Replace(
		loopFlow(t, 2, 3, "", ""),
		`{"id": "c1", "type": "command", "name": "Gate", "parentId": "l1"`,
		`{"id": "c1", "type": "command", "name": "Gate", "parentId": "l1", "breakpoint": true`, 1)
	response, handler := postRun(t, body)
	started := decodeRecord(t, response)

	for repeat := 1; repeat <= 2; repeat++ {
		record := awaitPauseWhere(t, handler, started.ID, "c1", func(at engine.Pause) bool {
			return at.Iteration == repeat
		})
		at := record.Paused[0]
		if at.Loop != "l1" || at.Iteration != repeat {
			t.Fatalf("paused = %+v, want repeat %d of the loop", at, repeat)
		}
		if !strings.Contains(at.Resolved.Command, "echo") {
			t.Fatalf("resolved = %+v, want the command the block is about to run", at.Resolved)
		}
		if code := resume(t, handler, started.ID, `{"action": "continue"}`); code != http.StatusAccepted {
			t.Fatalf("resume = %d", code)
		}
	}
	record := awaitRun(t, handler, started.ID)

	if record.Status != engine.Succeeded {
		t.Fatalf("record = %+v", record)
	}
}

func TestCancellingAPausedRunStopsIt(t *testing.T) {
	fakeClaude(t)
	response, handler := postRun(t, breakpointFlow(t))
	started := decodeRecord(t, response)
	awaitPaused(t, handler, started.ID, "a1")

	if code := cancelRun(t, handler, started.ID, "application/json"); code != http.StatusAccepted {
		t.Fatalf("cancel = %d", code)
	}
	record := awaitRun(t, handler, started.ID)

	if record.Status != Cancelled || len(record.Paused) != 0 {
		t.Fatalf("record = %+v", record)
	}
	if record.Steps[0].Status != engine.Skipped {
		t.Fatalf("step = %+v", record.Steps[0])
	}
}

func TestResumeRefusesWhatItCannotAnswer(t *testing.T) {
	fakeClaude(t)
	response, handler := postRun(t, breakpointFlow(t))
	started := decodeRecord(t, response)
	paused := awaitPaused(t, handler, started.ID, "a1")

	cases := map[string]struct {
		id          string
		body        string
		contentType string
		want        int
	}{
		"not JSON":       {id: started.ID, body: `{"action": "continue"}`, contentType: "text/plain", want: http.StatusUnsupportedMediaType},
		"unknown run":    {id: "20000101T000000Z-000000", body: `{"action": "continue"}`, contentType: "application/json", want: http.StatusNotFound},
		"unknown action": {id: started.ID, body: `{"action": "rewind"}`, contentType: "application/json", want: http.StatusBadRequest},
		"unknown block":  {id: started.ID, body: `{"nodeId": "a2", "action": "continue"}`, contentType: "application/json", want: http.StatusConflict},
		"invalid body":   {id: started.ID, body: `not json`, contentType: "application/json", want: http.StatusBadRequest},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			got := workflowRequest(t, handler, http.MethodPost, "/api/runs/"+testCase.id+"/resume", testCase.body, testCase.contentType)
			if got.Code != testCase.want {
				t.Fatalf("resume = %d, want %d: %s", got.Code, testCase.want, got.Body.String())
			}
		})
	}
	if paused.Paused[0].TaskID != "a1" {
		t.Fatalf("paused = %+v", paused.Paused)
	}

	if code := resume(t, handler, started.ID, `{"action": "continue"}`); code != http.StatusAccepted {
		t.Fatalf("resume = %d", code)
	}
	finished := awaitRun(t, handler, started.ID)
	if code := resume(t, handler, finished.ID, `{"action": "continue"}`); code != http.StatusConflict {
		t.Fatalf("resuming a run that is not paused = %d, want 409", code)
	}
}

func resume(t *testing.T, handler http.Handler, id string, body string) int {
	t.Helper()
	return workflowRequest(t, handler, http.MethodPost, "/api/runs/"+id+"/resume", body, "application/json").Code
}
