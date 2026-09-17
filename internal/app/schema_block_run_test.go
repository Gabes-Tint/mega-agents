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
// checked by a schema block, with whatever blocks and edges follow it.
func validationFlow(t *testing.T, schemaText string, more string, edges string) string {
	t.Helper()
	return fmt.Sprintf(`{"nodes": [
		{"id": "p1", "type": "project", "name": "api", "path": %q},
		{"id": "a1", "type": "agent", "name": "Writer", "parentId": "p1", "start": true, "backend": "claude", "prompt": "Write"},
		{"id": "s1", "type": "jsonschema", "name": "Check reply", "parentId": "p1", "schema": "%s"}
		%s
	], "edges": [{"id": "e1", "from": "a1", "to": "s1"} %s]}`, t.TempDir(), schemaText, more, edges)
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
			{"id": "s1", "type": "jsonschema", "name": "Check", "parentId": "p1", "schema": "%s"}
			%s
		], "edges": [%s]}`, schemaText, extraNodes, edges)
	}
	link := `{"id": "e1", "from": "a1", "to": "s1"}`
	cases := map[string]struct {
		body string
		want string
	}{
		"invalid schema": {
			body: flow(`{\"type\": 3}`, "", link),
			want: "Check: the schema is not a valid JSON Schema",
		},
		"no schema": {
			body: flow(``, "", link),
			want: "Check: write the JSON Schema the value must satisfy",
		},
		"two inputs": {
			body: flow(replySchema, `, {"id": "a2", "type": "agent", "name": "Other", "parentId": "p1", "start": false, "backend": "claude", "prompt": "x"}`,
				link+`, {"id": "e0", "from": "a1", "to": "a2"}, {"id": "e2", "from": "a2", "to": "s1"}`),
			want: "Check: validates one value; connect exactly one block to it",
		},
		"unknown port": {
			body: flow(replySchema, `, {"id": "n1", "type": "agent", "name": "Next", "parentId": "p1", "backend": "claude", "prompt": "x"}`,
				link+`, {"id": "e2", "from": "s1", "to": "n1", "fromPort": "maybe"}`),
			want: `Check has no output "maybe"; use valid or invalid`,
		},
		"workspace input": {
			body: fmt.Sprintf(`{"nodes": [
				{"id": "p1", "type": "project", "name": "api", "path": "/tmp"},
				{"id": "g1", "type": "github", "name": "GitHub 1", "parentId": "p1", "start": true},
				{"id": "w1", "type": "action", "action": "worktree", "name": "Create worktree", "parentId": "g1", "start": true, "branch": "x"},
				{"id": "s1", "type": "jsonschema", "name": "Check", "parentId": "p1", "schema": "%s"}
			], "edges": [{"id": "e1", "from": "w1", "to": "s1"}]}`, replySchema),
			want: "Check: validates one value; connect exactly one block to it",
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

func TestSchemaBlocksLiveInsideProjectsAndAgents(t *testing.T) {
	body := fmt.Sprintf(`{"nodes": [{"id": "s1", "type": "jsonschema", "name": "Check", "schema": "%s"}]}`, replySchema)

	response, _ := postRun(t, body)

	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "jsonschema cannot be placed inside root") {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
}
