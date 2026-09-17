package app

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gabes-Tint/mega-agents/internal/engine"
)

// gateFlow: a worktree feeding a command gate, with whatever follows it.
func gateFlow(t *testing.T, command string, more string, edges string) string {
	t.Helper()
	clone, _ := projectClone(t)
	worktree := filepath.Join(t.TempDir(), "gate")
	return fmt.Sprintf(`{"nodes": [
		{"id": "p1", "type": "project", "name": "api", "path": %q},
		{"id": "g1", "type": "github", "name": "GitHub 1", "parentId": "p1", "start": true},
		{"id": "w1", "type": "action", "action": "worktree", "name": "Create worktree", "parentId": "g1", "start": true,
		 "branch": "gate", "worktreePath": %q},
		{"id": "c1", "type": "command", "name": "Tests", "parentId": "p1", "command": %q}
		%s
	], "edges": [{"id": "e1", "from": "w1", "to": "c1"} %s]}`, clone, worktree, command, more, edges)
}

func TestACommandRunsInTheWorkspaceAndPassesItsOutputOn(t *testing.T) {
	bin := fakeClaude(t)
	body := gateFlow(t, `pwd; git branch --show-current; echo "$MEGA_AGENTS_WORKSPACE_BRANCH"`,
		`, {"id": "a1", "type": "agent", "name": "Report", "parentId": "p1", "backend": "claude", "prompt": "Gate said {{result}}"}`,
		`, {"id": "e2", "from": "c1", "to": "a1"}`)

	record, handler := finishedRun(t, body)

	if record.Status != engine.Succeeded {
		t.Fatalf("record = %+v", record)
	}
	gate := record.Steps[1]
	if gate.Kind != "command" || gate.Details["exitCode"] != 0.0 || !strings.Contains(gate.Details["output"].(string), "gate\ngate") {
		t.Fatalf("gate = %+v", gate)
	}
	if prompt := recorded(t, bin, "prompt-1"); !strings.HasPrefix(prompt, `Gate said {"exitCode":0,"output":"`) {
		t.Fatalf("prompt = %q", prompt)
	}
	log := get(t, handler, "/api/runs/"+record.ID+"/logs/c1").Body.String()
	if !strings.Contains(log, "$ pwd; git branch --show-current;") || !strings.Contains(log, "exited 0") {
		t.Fatalf("log = %s", log)
	}
}

func TestAFailingCommandFailsTheRunWhenNothingHandlesIt(t *testing.T) {
	body := gateFlow(t, `echo "3 tests failed"; exit 3`, "", "")

	record := finishedRunOnly(t, body)

	gate := record.Steps[1]
	if record.Status != engine.Failed || gate.Status != engine.Failed || gate.Details["exitCode"] != 3.0 ||
		!strings.Contains(gate.Error, "Tests exited 3: 3 tests failed") {
		t.Fatalf("gate = %+v", gate)
	}
}

func TestAFailingCommandTakesTheFailedBranchWhenOneIsConnected(t *testing.T) {
	bin := fakeClaude(t)
	body := gateFlow(t, `echo "lint: unused import"; exit 1`,
		`, {"id": "fix", "type": "agent", "name": "Fix", "parentId": "p1", "backend": "claude", "prompt": "Fix in {{workspace.path}}: {{results.tests}}"},
		   {"id": "ship", "type": "agent", "name": "Ship", "parentId": "p1", "backend": "claude", "prompt": "Ship"}`,
		`, {"id": "e2", "from": "c1", "to": "fix", "fromPort": "failed"}, {"id": "e3", "from": "w1", "to": "fix"},
		   {"id": "e4", "from": "c1", "to": "ship", "fromPort": "passed"}`)

	record := finishedRunOnly(t, body)

	statuses := map[string]engine.Status{}
	for _, step := range record.Steps {
		statuses[step.TaskID] = step.Status
	}
	if record.Status != engine.Succeeded || statuses["c1"] != engine.Succeeded || statuses["fix"] != engine.Succeeded || statuses["ship"] != engine.Skipped {
		t.Fatalf("statuses = %v", statuses)
	}
	if prompt := recorded(t, bin, "prompt-1"); !strings.Contains(prompt, `{"exitCode":1,"output":"lint: unused import\n"}`) {
		t.Fatalf("prompt = %q", prompt)
	}
}

func TestACommandStopsAtItsTimeout(t *testing.T) {
	body := gateFlow(t, `sleep 30`, "", "")
	body = strings.Replace(body, `"command": "sleep 30"`, `"command": "sleep 30", "timeoutMinutes": 0.005`, 1)

	record := finishedRunOnly(t, body)

	if gate := record.Steps[1]; gate.Status != engine.Failed || !strings.Contains(gate.Error, "Tests stopped after its 300ms timeout") {
		t.Fatalf("gate = %+v", gate)
	}
}

func TestCommandPlansAreCheckedBeforeRunning(t *testing.T) {
	cases := map[string]struct {
		body string
		want string
	}{
		"no command": {
			body: gateFlow(t, ` `, "", ""),
			want: "Tests: write the command to run",
		},
		"unknown port": {
			body: gateFlow(t, `true`, `, {"id": "a1", "type": "agent", "name": "Next", "parentId": "p1", "backend": "claude", "prompt": "x"}`,
				`, {"id": "e2", "from": "c1", "to": "a1", "fromPort": "green"}`),
			want: `Tests has no output "green"; use passed or failed`,
		},
		"bad timeout": {
			body: strings.Replace(gateFlow(t, `true`, "", ""), `"command": "true"`, `"command": "true", "timeoutMinutes": 0`, 1),
			want: "Tests: the timeout must be more than 0 and at most 240 minutes",
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
