package app

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/Gabes-Tint/mega-agents/internal/engine"
)

// joinFlow: a reviewer's verdict routes to one of two commands, and both
// lead to Next, which waits for any of them when any is true, through a
// loop around it when looped is true.
func joinFlow(t *testing.T, any bool, looped bool) string {
	t.Helper()
	next := fmt.Sprintf(`{"id": "n1", "type": "agent", "name": "Next", "parentId": "p1", "backend": "claude", "prompt": "Got {{result}}", "waitForAny": %t}`, any)
	into := "n1"
	if looped {
		next = fmt.Sprintf(`{"id": "l1", "type": "loop", "name": "Again", "parentId": "p1", "maxIterations": 2, "untilNode": "g1", "untilPort": "passed", "waitForAny": %t},
			{"id": "g1", "type": "command", "name": "Gate", "parentId": "l1", "command": "echo checked"}`, any)
		into = "l1"
	}
	return fmt.Sprintf(`{"nodes": [
		{"id": "p1", "type": "project", "name": "api", "path": %q},
		{"id": "r1", "type": "agent", "name": "Reviewer", "parentId": "p1", "start": true, "backend": "claude", "prompt": "Review"},
		{"id": "x1", "type": "router", "name": "Route", "parentId": "p1", "cases": [{"name": "approved", "expression": "value.text.contains('approve')"}]},
		{"id": "a1", "type": "command", "name": "Ship", "parentId": "p1", "command": "echo shipped"},
		{"id": "b1", "type": "command", "name": "Hold", "parentId": "p1", "command": "echo held"},
		%s
	], "edges": [
		{"id": "e1", "from": "r1", "to": "x1"},
		{"id": "e2", "from": "x1", "to": "a1", "fromPort": "approved"},
		{"id": "e3", "from": "x1", "to": "b1", "fromPort": "default"},
		{"id": "e4", "from": "a1", "to": %q},
		{"id": "e5", "from": "b1", "to": %q}
	]}`, t.TempDir(), next, into, into)
}

func TestABlockWaitingForAnyRunsOnTheBranchTaken(t *testing.T) {
	bin := fakeClaude(t)
	writeReply(t, bin, 1, `{"type":"result","session_id":"s1","result":"not yet"}`)

	record := finishedRunOnly(t, joinFlow(t, true, false))

	if record.Status != engine.Succeeded {
		t.Fatalf("record = %+v", record)
	}
	if prompt := recorded(t, bin, "prompt-2"); !strings.HasPrefix(prompt, `Got {"exitCode":0,"output":"held`) {
		t.Fatalf("next prompt = %q", prompt)
	}
}

func TestALoopWaitingForAnyRunsOnTheBranchTaken(t *testing.T) {
	bin := fakeClaude(t)
	writeReply(t, bin, 1, `{"type":"result","session_id":"s1","result":"approve"}`)

	record := finishedRunOnly(t, joinFlow(t, true, true))

	for _, step := range record.Steps {
		if step.Name == "Gate" && step.Status != engine.Succeeded {
			t.Fatalf("gate = %+v", step)
		}
	}
	if record.Status != engine.Succeeded {
		t.Fatalf("record = %+v", record)
	}
}

func TestWithoutWaitingForAnyTwoResultsNeedNames(t *testing.T) {
	response, _ := postRun(t, joinFlow(t, false, false))

	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "{{result}} needs exactly one block") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}
