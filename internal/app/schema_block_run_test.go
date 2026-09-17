package app

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/Gabes-Tint/mega-agents/internal/engine"
)

const replySchema = `{\"type\":\"object\",\"required\":[\"text\"],\"properties\":{\"text\":{\"type\":\"string\",\"pattern\":\"^done$\"}}}`

// validationFlow is an agent answering "done" (the fake CLI's default),
// with a schema block inside it checking that reply, and whatever blocks and
// edges follow it.
func validationFlow(t *testing.T, schemaText string, more string, edges string) string {
	t.Helper()
	return fmt.Sprintf(`{"nodes": [
		{"id": "p1", "type": "project", "name": "api", "path": %q},
		{"id": "a1", "type": "agent", "name": "Writer", "parentId": "p1", "start": true, "backend": "claude", "prompt": "Write"},
		{"id": "s1", "type": "jsonschema", "name": "Check reply", "parentId": "a1", "schema": "%s"}
		%s
	], "edges": [%s]}`, t.TempDir(), schemaText, more, strings.TrimPrefix(edges, ","))
}

func TestASchemaBlockPassesAValidValueOn(t *testing.T) {
	bin := fakeClaude(t)
	body := validationFlow(t, replySchema,
		`, {"id": "n1", "type": "agent", "name": "Next", "parentId": "p1", "backend": "claude", "prompt": "Got {{results.check-reply}}"}`,
		`, {"id": "e2", "from": "s1", "to": "n1"}`)

	record := finishedRunOnly(t, body)

	if record.Status != engine.Succeeded {
		t.Fatalf("record = %+v", record)
	}
	check := record.Steps[1]
	if check.Kind != "jsonschema" || check.Details["valid"] != true || check.Details["value"].(map[string]any)["text"] != "done" {
		t.Fatalf("check = %+v", check)
	}
	if got := recorded(t, bin, "prompt-2"); got != `Got {"text":"done"}` {
		t.Fatalf("next prompt = %q", got)
	}
}

func TestAnInvalidValueFailsTheSchemaBlockWhenNothingHandlesIt(t *testing.T) {
	fakeClaude(t)
	strict := strings.Replace(replySchema, `^done$`, `^approved$`, 1)
	body := validationFlow(t, strict,
		`, {"id": "n1", "type": "agent", "name": "Next", "parentId": "p1", "backend": "claude", "prompt": "Go"}`,
		`, {"id": "e2", "from": "s1", "to": "n1"}`)

	record := finishedRunOnly(t, body)

	check := record.Steps[1]
	if record.Status != engine.Failed || check.Status != engine.Failed || record.Steps[2].Status != engine.Skipped {
		t.Fatalf("record = %+v", record)
	}
	if !strings.Contains(check.Error, "the value did not satisfy the schema: $.text: ") || check.Details["valid"] != false {
		t.Fatalf("check = %+v", check)
	}
	errs, _ := check.Details["errors"].([]any)
	if len(errs) != 1 || errs[0].(map[string]any)["path"] != "$.text" {
		t.Fatalf("errors = %+v", check.Details["errors"])
	}
}

func TestAnInvalidValueTakesTheInvalidBranchWhenOneIsConnected(t *testing.T) {
	bin := fakeClaude(t)
	strict := strings.Replace(replySchema, `^done$`, `^approved$`, 1)
	body := validationFlow(t, strict,
		`, {"id": "ok", "type": "agent", "name": "Ship", "parentId": "p1", "backend": "claude", "prompt": "Ship"},
		   {"id": "fix", "type": "agent", "name": "Fix", "parentId": "p1", "backend": "claude", "prompt": "Fix {{result}}"}`,
		`, {"id": "e2", "from": "s1", "to": "ok", "fromPort": "valid"}, {"id": "e3", "from": "s1", "to": "fix", "fromPort": "invalid"}`)

	record := finishedRunOnly(t, body)

	statuses := map[string]engine.Status{}
	for _, step := range record.Steps {
		statuses[step.TaskID] = step.Status
	}
	if record.Status != engine.Succeeded || statuses["s1"] != engine.Succeeded || statuses["ok"] != engine.Skipped || statuses["fix"] != engine.Succeeded {
		t.Fatalf("statuses = %v, record = %+v", statuses, record)
	}
	prompt := recorded(t, bin, "prompt-2")
	if !strings.HasPrefix(prompt, `Fix {"errors":[{"path":"$.text","message":`) || !strings.Contains(prompt, `"value":{"text":"done"}`) {
		t.Fatalf("fix prompt = %q", prompt)
	}
}

func TestSchemaBlockPlansAreCheckedBeforeRunning(t *testing.T) {
	flow := func(schemaText string, extraNodes string, edges string) string {
		return fmt.Sprintf(`{"nodes": [
			{"id": "p1", "type": "project", "name": "api", "path": "/tmp"},
			{"id": "a1", "type": "agent", "name": "Writer", "parentId": "p1", "start": true, "backend": "claude", "prompt": "Write"},
			{"id": "s1", "type": "jsonschema", "name": "Check", "parentId": "a1", "schema": "%s"}
			%s
		], "edges": [%s]}`, schemaText, extraNodes, edges)
	}
	cases := map[string]struct {
		body string
		want string
	}{
		"invalid schema": {
			body: flow(`{\"type\": 3}`, "", ""),
			want: "Check: the schema is not a valid JSON Schema",
		},
		"no schema": {
			body: flow(``, "", ""),
			want: "Check: write the JSON Schema the value must satisfy",
		},
		"an arrow into it": {
			body: flow(replySchema, `, {"id": "a2", "type": "agent", "name": "Other", "parentId": "p1", "start": false, "backend": "claude", "prompt": "x"}`,
				`{"id": "e0", "from": "a1", "to": "a2"}, {"id": "e2", "from": "a2", "to": "s1"}`),
			want: "Check checks the reply of Writer, the agent it sits in; remove the arrows into it",
		},
		"unknown port": {
			body: flow(replySchema, `, {"id": "n1", "type": "agent", "name": "Next", "parentId": "p1", "backend": "claude", "prompt": "x"}`,
				`{"id": "e2", "from": "s1", "to": "n1", "fromPort": "maybe"}`),
			want: `Check has no output "maybe"; use valid or invalid`,
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			response, _ := postRun(t, testCase.body)

			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), testCase.want) {
				t.Fatalf("response = %d %q, want %q", response.Code, response.Body.String(), testCase.want)
			}
		})
	}
}

func TestSchemaBlocksLiveOnlyInsideAgents(t *testing.T) {
	for _, parent := range []string{"root", "project", "loop"} {
		t.Run(parent, func(t *testing.T) {
			body := fmt.Sprintf(`{"nodes": [
				{"id": "p1", "type": "project", "name": "api", "path": "/tmp"},
				{"id": "loop", "type": "loop", "name": "Loop", "parentId": "p1"},
				{"id": "s1", "type": "jsonschema", "name": "Check", "parentId": %q, "schema": "%s"}
			]}`, map[string]string{"root": "", "project": "p1", "loop": "loop"}[parent], replySchema)

			response, _ := postRun(t, body)

			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "jsonschema cannot be placed inside "+parent) {
				t.Fatalf("response = %d %q", response.Code, response.Body.String())
			}
		})
	}
}

func TestASchemaBlockInsideAnAgentInALoopCanEndTheLoop(t *testing.T) {
	bin := fakeClaude(t)
	body := fmt.Sprintf(`{"nodes": [
		{"id": "p1", "type": "project", "name": "api", "path": %q},
		{"id": "a0", "type": "agent", "name": "Planner", "parentId": "p1", "start": true, "backend": "claude", "prompt": "Plan"},
		{"id": "l1", "type": "loop", "name": "Until valid", "parentId": "p1", "maxIterations": 2, "untilNode": "s1", "untilPort": "valid"},
		{"id": "w1", "type": "agent", "name": "Writer", "parentId": "l1", "backend": "claude", "prompt": "Write"},
		{"id": "s1", "type": "jsonschema", "name": "Check", "parentId": "w1", "schema": "%s"},
		{"id": "n1", "type": "agent", "name": "Ship", "parentId": "p1", "backend": "claude", "prompt": "Ship {{result}}"}
	], "edges": [
		{"id": "e1", "from": "a0", "to": "l1"},
		{"id": "e2", "from": "l1", "to": "n1", "fromPort": "done"}
	]}`, t.TempDir(), replySchema)

	record := finishedRunOnly(t, body)

	if record.Status != engine.Succeeded {
		t.Fatalf("record = %+v", record)
	}
	for _, step := range record.Steps {
		if step.TaskID == "s1" && (step.Loop != "l1" || step.Status != engine.Succeeded) {
			t.Fatalf("check = %+v", step)
		}
	}
	if got := recorded(t, bin, "prompt-3"); got != `Ship {"text":"done"}` {
		t.Fatalf("ship prompt = %q", got)
	}
}

func TestASchemaBlockInsideAReachedAgentRuns(t *testing.T) {
	problems := CheckWorkflow(WorkflowRequest{Nodes: []WorkflowNodeInput{
		{ID: "p1", Type: "project", Name: "api", Path: "/work/api"},
		{ID: "a1", Type: "agent", Name: "Writer", ParentID: "p1", Start: true, Backend: "claude", Prompt: "Write"},
		{ID: "s1", Type: "jsonschema", Name: "Check", ParentID: "a1", Schema: `{"type":"object"}`},
	}})

	if len(problems) != 0 {
		t.Fatalf("problems = %+v", problems)
	}
}
