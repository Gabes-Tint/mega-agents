package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Gabes-Tint/mega-agents/internal/engine"
)

// loopFlow: a Coder hands its work to a loop that runs a Gate, failing until
// its passOn-th run, and a Fixer on each failure, at most maxIterations
// times; more nodes and edges follow, such as what takes done or exhausted.
func loopFlow(t *testing.T, passOn int, maxIterations int, more string, edges string) string {
	t.Helper()
	gate := fmt.Sprintf(`n=$(( $(cat runs 2>/dev/null || echo 0) + 1 )); echo $n > runs; echo "run $n"; [ $n -ge %d ]`, passOn)
	return fmt.Sprintf(`{"nodes": [
		{"id": "p1", "type": "project", "name": "api", "path": %q},
		{"id": "a1", "type": "agent", "name": "Coder", "parentId": "p1", "start": true, "backend": "claude", "prompt": "Code"},
		{"id": "l1", "type": "loop", "name": "Until green", "parentId": "p1", "maxIterations": %d, "untilNode": "c1", "untilPort": "passed"},
		{"id": "c1", "type": "command", "name": "Gate", "parentId": "l1", "command": %q},
		{"id": "f1", "type": "agent", "name": "Fixer", "parentId": "l1", "backend": "claude", "prompt": "Fix {{result}}", "continueSession": true}
		%s
	], "edges": [
		{"id": "e1", "from": "a1", "to": "l1"},
		{"id": "e2", "from": "c1", "to": "f1", "fromPort": "failed"}
		%s
	]}`, t.TempDir(), maxIterations, gate, more, edges)
}

func decodeJSON(t *testing.T, text string, into any) {
	t.Helper()
	if err := json.Unmarshal([]byte(text), into); err != nil {
		t.Fatal(err)
	}
}

func TestALoopFixesUntilTheGatePasses(t *testing.T) {
	bin := fakeClaude(t)
	body := loopFlow(t, 3, 4,
		`, {"id": "s1", "type": "agent", "name": "Ship", "parentId": "p1", "backend": "claude", "prompt": "Ship {{result}}"}`,
		`, {"id": "e3", "from": "l1", "to": "s1", "fromPort": "done"}`)

	record := finishedRunOnly(t, body)

	steps := map[string]engine.Step{}
	for _, step := range record.Steps {
		steps[step.Name] = step
	}
	if record.Status != engine.Succeeded {
		t.Fatalf("record = %+v", record)
	}
	if loop := steps["Until green"]; loop.Kind != "loop" || loop.Details["iterations"] != 3.0 {
		t.Fatalf("loop = %+v", loop)
	}
	if gate, fixer := steps["Gate"], steps["Fixer"]; gate.Loop != "l1" || gate.Iteration != 3 ||
		gate.Status != engine.Succeeded || fixer.Status != engine.Skipped {
		t.Fatalf("gate = %+v, fixer = %+v", gate, fixer)
	}
	if count := recorded(t, bin, "count"); count != "4" {
		t.Fatalf("agent turns = %s, want the coder, two fixes and the shipper", count)
	}
	if prompt := recorded(t, bin, "prompt-2"); !strings.HasPrefix(prompt, `Fix {"exitCode":1,"output":"run 1`) {
		t.Fatalf("first fix prompt = %q", prompt)
	}
	// The first fix works on a copy of the coder's conversation, the second
	// continues the first fix's own.
	if recorded(t, bin, "resume-2") != "session-1" || recorded(t, bin, "resume-3") != "session-2" {
		t.Fatalf("resumed %q then %q", recorded(t, bin, "resume-2"), recorded(t, bin, "resume-3"))
	}
	if _, err := os.Stat(filepath.Join(bin, "fork-3")); err == nil {
		t.Fatal("the second fix forked instead of continuing its own conversation")
	}
	if prompt := recorded(t, bin, "prompt-4"); !strings.HasPrefix(prompt, `Ship {"exitCode":0,"output":"run 3`) {
		t.Fatalf("ship prompt = %q", prompt)
	}
}

func TestALoopThatNeverEndsFailsUnlessSomethingTakesExhausted(t *testing.T) {
	fakeClaude(t)

	record := finishedRunOnly(t, loopFlow(t, 99, 2, "", ""))

	loop := record.Steps[1]
	if record.Status != engine.Failed || loop.Status != engine.Failed ||
		loop.Error != "Until green did not end after 2 iterations: Gate never took passed" {
		t.Fatalf("loop = %+v", loop)
	}
}

func TestAnExhaustedLoopHandsTheLastResultOn(t *testing.T) {
	bin := fakeClaude(t)
	body := loopFlow(t, 99, 2,
		`, {"id": "x1", "type": "agent", "name": "Escalate", "parentId": "p1", "backend": "claude", "prompt": "Help {{result}}"}`,
		`, {"id": "e3", "from": "l1", "to": "x1", "fromPort": "exhausted"}`)

	record := finishedRunOnly(t, body)

	if record.Status != engine.Succeeded {
		t.Fatalf("record = %+v", record)
	}
	if prompt := recorded(t, bin, "prompt-4"); !strings.HasPrefix(prompt, `Help {"exitCode":1,"output":"run 2`) {
		t.Fatalf("escalation prompt = %q", prompt)
	}
}

func TestLoopPlansAreCheckedBeforeRunning(t *testing.T) {
	flow := func(loop string, children string) string {
		return fmt.Sprintf(`{"nodes": [
			{"id": "p1", "type": "project", "name": "api", "path": "/tmp"},
			{"id": "a1", "type": "agent", "name": "Coder", "parentId": "p1", "start": true, "backend": "claude", "prompt": "Code"},
			{"id": "l1", "type": "loop", "name": "Repeat", "parentId": "p1" %s} %s
		], "edges": [{"id": "e1", "from": "a1", "to": "l1"}]}`, loop, children)
	}
	gate := `, {"id": "c1", "type": "command", "name": "Gate", "parentId": "l1", "command": "true"}`
	for name, testCase := range map[string]struct{ body, want string }{
		"empty":            {flow(`, "untilNode": "c1", "untilPort": "passed"`, ""), "Repeat: put the blocks to repeat inside the loop"},
		"no exit":          {flow("", gate), "Repeat: choose the block and output that end the loop"},
		"exit outside":     {flow(`, "untilNode": "a1", "untilPort": "result"`, gate), "Repeat: choose the block and output that end the loop"},
		"unknown port":     {flow(`, "untilNode": "c1", "untilPort": "green"`, gate), `Repeat: Gate has no output "green"; use passed or failed`},
		"too many":         {flow(`, "untilNode": "c1", "untilPort": "passed", "maxIterations": 21`, gate), "Repeat: repeat between 1 and 20 times"},
		"start inside":     {flow(`, "untilNode": "c1", "untilPort": "passed"`, gate+`, {"id": "a2", "type": "agent", "name": "Inner", "parentId": "l1", "start": true, "backend": "claude", "prompt": "x"}`), "Inner: blocks inside a loop start with the loop; clear its starting point"},
		"body block fails": {flow(`, "untilNode": "c1", "untilPort": "passed"`, `, {"id": "c1", "type": "command", "name": "Gate", "parentId": "l1"}`), "Gate: write the command to run"},
	} {
		t.Run(name, func(t *testing.T) {
			response, _ := postRun(t, testCase.body)

			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), testCase.want) {
				t.Fatalf("response = %d %q, want %q", response.Code, response.Body.String(), testCase.want)
			}
		})
	}
}

func TestTheBlocksInsideAReachedLoopRun(t *testing.T) {
	t.Parallel()
	var request WorkflowRequest
	decodeJSON(t, loopFlow(t, 1, 3, "", ""), &request)

	if problems := CheckWorkflow(request); len(problems) != 0 {
		t.Fatalf("problems = %+v", problems)
	}
}

func TestLoopsSurviveExportAndImport(t *testing.T) {
	t.Parallel()
	iterations := 5
	request := WorkflowRequest{Name: "loop", Nodes: []WorkflowNodeInput{
		{ID: "p1", Type: "project", Name: "api", Path: "/work"},
		{ID: "l1", Type: "loop", Name: "Until green", ParentID: "p1", MaxIterations: &iterations, UntilNode: "c1", UntilPort: "passed", WaitForAny: true},
		{ID: "c1", Type: "command", Name: "Gate", ParentID: "l1", Command: "make verify"},
	}}

	document, err := BuildWorkflowYAML(request)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := ParseWorkflowYAML([]byte(document))
	if err != nil {
		t.Fatalf("import: %v\n%s", err, document)
	}

	want := WorkflowNodeInput{ID: "until-green", Type: "loop", Name: "Until green", ParentID: "api",
		MaxIterations: &iterations, UntilNode: "gate", UntilPort: "passed", WaitForAny: true}
	if !reflect.DeepEqual(imported.Nodes[1], want) {
		t.Fatalf("loop = %+v\nwant %+v\n%s", imported.Nodes[1], want, document)
	}
}

func TestARetryReusesALoopThatEnded(t *testing.T) {
	bin := fakeClaude(t)
	marker := filepath.Join(t.TempDir(), "deployable")
	body := loopFlow(t, 2, 3,
		fmt.Sprintf(`, {"id": "d1", "type": "command", "name": "Deploy", "parentId": "p1", "command": "test -e %s"}`, marker),
		`, {"id": "e3", "from": "l1", "to": "d1", "fromPort": "done"}`)
	failed, handler := finishedRun(t, body)
	if failed.Status != engine.Failed {
		t.Fatalf("first run = %+v", failed)
	}
	turns := recorded(t, bin, "count")
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	code, text := postRetry(t, handler, failed.ID)

	if code != http.StatusAccepted {
		t.Fatalf("retry = %d %s", code, text)
	}
	retried := awaitRun(t, handler, decodeRecordText(t, text).ID)
	if retried.Status != engine.Succeeded || recorded(t, bin, "count") != turns {
		t.Fatalf("retried = %+v, agent turns %s then %s", retried, turns, recorded(t, bin, "count"))
	}
	for _, step := range retried.Steps {
		if step.Name == "Until green" && step.Details["reusedFrom"] != failed.ID {
			t.Fatalf("the loop ran again: %+v", step)
		}
	}
}

func TestAnAgentInALoopFedByNoAgentContinuesItsOwnConversation(t *testing.T) {
	bin := fakeClaude(t)
	gate := `n=$(( $(cat runs 2>/dev/null || echo 0) + 1 )); echo $n > runs; [ $n -ge 3 ]`
	body := fmt.Sprintf(`{"nodes": [
		{"id": "p1", "type": "project", "name": "api", "path": %q},
		{"id": "a0", "type": "agent", "name": "Start", "parentId": "p1", "start": true, "backend": "claude", "prompt": "Go"},
		{"id": "c0", "type": "command", "name": "Prepare", "parentId": "p1", "command": "true"},
		{"id": "l1", "type": "loop", "name": "Implement", "parentId": "p1", "maxIterations": 3, "untilNode": "c1", "untilPort": "passed"},
		{"id": "c1", "type": "command", "name": "Gate", "parentId": "l1", "command": %q},
		{"id": "f1", "type": "agent", "name": "Implementer", "parentId": "l1", "backend": "claude", "prompt": "Fix {{result}}", "continueSession": true}
	], "edges": [
		{"id": "e0", "from": "a0", "to": "c0"},
		{"id": "e1", "from": "c0", "to": "l1"},
		{"id": "e2", "from": "c1", "to": "f1", "fromPort": "failed"}
	]}`, t.TempDir(), gate)

	record := finishedRunOnly(t, body)

	if record.Status != engine.Succeeded {
		t.Fatalf("record = %+v", record)
	}
	if _, err := os.Stat(filepath.Join(bin, "resume-2")); err == nil {
		t.Fatal("the first implementation turn resumed a conversation")
	}
	if recorded(t, bin, "resume-3") != "session-2" {
		t.Fatalf("the second turn resumed %q, want its own session-2", recorded(t, bin, "resume-3"))
	}
}
