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

// fakeClaude puts a claude executable first on PATH. Each turn records its
// working directory and prompt under the returned folder, numbered by turn,
// and answers with the reply files the test wrote there (reply-1.json, ...),
// or with a default reply echoing nothing.
func fakeClaude(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	script := `#!/bin/sh
here="$(dirname "$0")"
count=$(( $(cat "$here/count" 2>/dev/null || echo 0) + 1 ))
echo $count > "$here/count"
pwd > "$here/cwd-$count"
while [ $# -gt 0 ]; do
  case "$1" in
    -p) printf '%s' "$2" > "$here/prompt-$count"; shift ;;
    --model) echo "$2" > "$here/model-$count"; shift ;;
    --resume) echo "$2" > "$here/resume-$count"; shift ;;
    --fork-session) touch "$here/fork-$count" ;;
  esac
  shift
done
if [ -f "$here/reply-$count.json" ]; then cat "$here/reply-$count.json"; else
  echo '{"type":"result","session_id":"session-'$count'","result":"done"}'
fi
`
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return bin
}

func recorded(t *testing.T, bin string, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(bin, name))
	if err != nil {
		t.Fatalf("the fake agent recorded no %s: %v", name, err)
	}
	return strings.TrimSpace(string(data))
}

func writeReply(t *testing.T, bin string, turn int, envelope string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(bin, fmt.Sprintf("reply-%d.json", turn)), []byte(envelope), 0o600); err != nil {
		t.Fatal(err)
	}
}

const verdictSchemaJSON = `{\"type\":\"object\",\"additionalProperties\":false,\"required\":[\"verdict\"],\"properties\":{\"verdict\":{\"type\":\"string\",\"enum\":[\"approve\",\"reject\"]}}}`

func TestAStartingAgentRunsInItsProjectFolder(t *testing.T) {
	bin := fakeClaude(t)
	project := t.TempDir()
	body := fmt.Sprintf(`{"nodes": [
		{"id": "p1", "type": "project", "name": "api", "path": %q},
		{"id": "a1", "type": "agent", "name": "Planner", "parentId": "p1", "start": true,
		 "backend": "claude", "model": "haiku", "prompt": "Summarize the repository"}
	]}`, project)

	record := finishedRunOnly(t, body)

	if record.Status != engine.Succeeded || record.Steps[0].Kind != "agent" {
		t.Fatalf("record = %+v", record)
	}
	details := record.Steps[0].Details
	if details["sessionId"] != "session-1" || details["backend"] != "claude" || details["reply"] != "done" {
		t.Fatalf("details = %+v", details)
	}
	if recorded(t, bin, "cwd-1") != project || recorded(t, bin, "prompt-1") != "Summarize the repository" || recorded(t, bin, "model-1") != "haiku" {
		t.Fatalf("the agent ran in %s with %q", recorded(t, bin, "cwd-1"), recorded(t, bin, "prompt-1"))
	}
}

func TestAnAgentWorksInTheWorkspaceItReceives(t *testing.T) {
	bin := fakeClaude(t)
	clone, _ := projectClone(t)
	worktree := filepath.Join(t.TempDir(), "login")
	body := fmt.Sprintf(`{"nodes": [
		{"id": "p1", "type": "project", "name": "api", "path": %q},
		{"id": "g1", "type": "github", "name": "GitHub 1", "parentId": "p1", "start": true},
		{"id": "w1", "type": "action", "action": "worktree", "name": "Create worktree", "parentId": "g1", "start": true,
		 "branch": "feature/login", "worktreePath": %q},
		{"id": "a1", "type": "agent", "name": "Implementer", "parentId": "p1", "backend": "claude",
		 "prompt": "Implement login on {{workspace.branch}} in {{workspace.path}} ({{workspace.repository}})"}
	], "edges": [{"id": "e1", "from": "w1", "to": "a1"}]}`, clone, worktree)

	record := finishedRunOnly(t, body)

	if record.Status != engine.Succeeded {
		t.Fatalf("record = %+v", record)
	}
	if recorded(t, bin, "cwd-1") != worktree {
		t.Fatalf("cwd = %s, want the worktree %s", recorded(t, bin, "cwd-1"), worktree)
	}
	if want := "Implement login on feature/login in " + worktree + " (acme/api)"; recorded(t, bin, "prompt-1") != want {
		t.Fatalf("prompt = %q, want %q", recorded(t, bin, "prompt-1"), want)
	}
}

func TestAnAgentHoldsItsReplyToTheSchemaAndPassesTheResultOn(t *testing.T) {
	bin := fakeClaude(t)
	writeReply(t, bin, 1, `{"type":"result","session_id":"s1","result":"looks good"}`)
	writeReply(t, bin, 2, `{"type":"result","session_id":"s1","result":"{\"verdict\":\"approve\"}","structured_output":{"verdict":"approve"}}`)
	body := fmt.Sprintf(`{"nodes": [
		{"id": "p1", "type": "project", "name": "api", "path": %q},
		{"id": "r1", "type": "agent", "name": "Reviewer", "parentId": "p1", "start": true, "backend": "claude",
		 "prompt": "Review", "outputSchema": "%s", "retries": 1},
		{"id": "m1", "type": "agent", "name": "Merger", "parentId": "p1", "backend": "claude",
		 "prompt": "The review said {{results.reviewer}}; also {{result}}"}
	], "edges": [{"id": "e1", "from": "r1", "to": "m1"}]}`, t.TempDir(), verdictSchemaJSON)

	record, handler := finishedRun(t, body)

	if record.Status != engine.Succeeded {
		t.Fatalf("record = %+v", record)
	}
	reviewer := record.Steps[0].Details
	if reviewer["attempts"] != 2.0 || reviewer["result"].(map[string]any)["verdict"] != "approve" {
		t.Fatalf("reviewer details = %+v", reviewer)
	}
	if !strings.Contains(recorded(t, bin, "prompt-2"), "That reply was rejected: $: the reply was not a JSON object") {
		t.Fatalf("repair prompt = %q", recorded(t, bin, "prompt-2"))
	}
	if want := `The review said {"verdict":"approve"}; also {"verdict":"approve"}`; recorded(t, bin, "prompt-3") != want {
		t.Fatalf("merger prompt = %q, want %q", recorded(t, bin, "prompt-3"), want)
	}
	log := get(t, handler, "/api/runs/"+record.ID+"/logs/r1").Body.String()
	for _, want := range []string{"$ claude --print", "attempt 1 did not satisfy the schema", "Attempt 2", "claude exited 0"} {
		if !strings.Contains(log, want) {
			t.Errorf("reviewer log lacks %q:\n%s", want, log)
		}
	}
}

func TestAnAgentThatNeverSatisfiesItsSchemaFailsWithEvidence(t *testing.T) {
	fakeClaude(t)
	body := fmt.Sprintf(`{"nodes": [
		{"id": "p1", "type": "project", "name": "api", "path": %q},
		{"id": "r1", "type": "agent", "name": "Reviewer", "parentId": "p1", "start": true, "backend": "claude",
		 "prompt": "Review", "outputSchema": "%s", "retries": 0},
		{"id": "m1", "type": "agent", "name": "Merger", "parentId": "p1", "backend": "claude", "prompt": "Merge"}
	], "edges": [{"id": "e1", "from": "r1", "to": "m1"}]}`, t.TempDir(), verdictSchemaJSON)

	record := finishedRunOnly(t, body)

	if record.Steps[0].Status != engine.Failed || record.Steps[1].Status != engine.Skipped {
		t.Fatalf("record = %+v", record)
	}
	if !strings.Contains(record.Steps[0].Error, "did not satisfy the schema after 1 attempts") || record.Steps[0].Details["reply"] != "done" {
		t.Fatalf("step = %+v", record.Steps[0])
	}
}

func TestAnAgentFailsClearlyWithoutAFolderOrCLI(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	cases := map[string]struct {
		path string
		want string
	}{
		"no project path": {path: "", want: "set the path on the project that contains Planner"},
		"no cli":          {path: t.TempDir(), want: "claude is not installed or not on PATH"},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			body := fmt.Sprintf(`{"nodes": [
				{"id": "p1", "type": "project", "name": "api", "path": %q},
				{"id": "a1", "type": "agent", "name": "Planner", "parentId": "p1", "start": true, "backend": "claude", "prompt": "Plan"}
			]}`, testCase.path)

			record := finishedRunOnly(t, body)

			if record.Status != engine.Failed || !strings.Contains(record.Steps[0].Error, testCase.want) {
				t.Fatalf("record = %+v, want %q", record, testCase.want)
			}
		})
	}
}

func TestAgentPlansAreCheckedBeforeRunning(t *testing.T) {
	agent := func(fields string) string {
		return fmt.Sprintf(`{"nodes": [
			{"id": "p1", "type": "project", "name": "api", "path": "/tmp"},
			{"id": "a1", "type": "agent", "name": "Planner", "parentId": "p1", "start": true %s}
		]}`, fields)
	}
	cases := map[string]struct {
		body string
		want string
	}{
		"unknown backend": {
			body: agent(`, "backend": "gemini", "prompt": "x"`),
			want: `Planner: unknown agent backend "gemini"`,
		},
		"no prompt": {
			body: agent(`, "backend": "claude"`),
			want: "Planner: write the prompt the agent receives",
		},
		"lax schema": {
			body: agent(`, "backend": "claude", "prompt": "x", "outputSchema": "{\"type\":\"object\",\"properties\":{\"a\":{\"type\":\"string\"}}}"`),
			want: `Planner: output schema: $ must set "additionalProperties": false`,
		},
		"workspace placeholder without a workspace": {
			body: agent(`, "backend": "claude", "prompt": "work in {{workspace.path}}"`),
			want: "Planner: {{workspace.path}} needs a workspace; connect a Create worktree action to this block",
		},
		"unknown placeholder": {
			body: agent(`, "backend": "claude", "prompt": "{{secrets.token}}"`),
			want: "Planner: unknown placeholder {{secrets.token}}",
		},
		"result from nowhere": {
			body: agent(`, "backend": "claude", "prompt": "{{results.reviewer}}"`),
			want: "Planner: {{results.reviewer}} names no block connected to this one",
		},
		"negative retries": {
			body: agent(`, "backend": "claude", "prompt": "x", "retries": -1`),
			want: "Planner: retries must be between 0 and 5",
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

func TestIndependentAgentsWorkAtTheSameTime(t *testing.T) {
	bin := t.TempDir()
	script := "#!/bin/sh\nsleep 1\necho '{\"type\":\"result\",\"session_id\":\"s\",\"result\":\"done\"}'\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	body := fmt.Sprintf(`{"nodes": [
		{"id": "p1", "type": "project", "name": "api", "path": %q},
		{"id": "s1", "type": "command", "name": "Start", "parentId": "p1", "command": "true"},
		{"id": "a1", "type": "agent", "name": "Domain", "parentId": "p1", "start": true, "backend": "claude", "prompt": "Domain"},
		{"id": "a2", "type": "agent", "name": "Interface", "parentId": "p1", "start": true, "backend": "claude", "prompt": "UI"}
	]}`, t.TempDir())
	started := time.Now()

	record := finishedRunOnly(t, body)

	if record.Status != engine.Succeeded {
		t.Fatalf("record = %+v", record)
	}
	if elapsed := time.Since(started); elapsed > 1800*time.Millisecond {
		t.Fatalf("two one-second agents took %s; they did not run at the same time", elapsed)
	}
}

func TestAnAgentStepReportsWhatItCost(t *testing.T) {
	bin := fakeClaude(t)
	writeReply(t, bin, 1, `{"type":"result","session_id":"s1","result":"done","total_cost_usd":0.25,"usage":{"input_tokens":100,"cache_read_input_tokens":50,"output_tokens":20}}`)
	body := fmt.Sprintf(`{"nodes": [
		{"id": "p1", "type": "project", "name": "api", "path": %q},
		{"id": "a1", "type": "agent", "name": "Planner", "parentId": "p1", "start": true, "backend": "claude", "prompt": "Plan", "maxCostUsd": 1.5}
	]}`, t.TempDir())

	record := finishedRunOnly(t, body)

	usage, _ := record.Steps[0].Details["usage"].(map[string]any)
	if record.Status != engine.Succeeded || usage["costUsd"] != 0.25 || usage["inputTokens"] != 150.0 || usage["outputTokens"] != 20.0 {
		t.Fatalf("details = %+v", record.Steps[0].Details)
	}
}

func TestAnAgentCostBudgetMustBePositive(t *testing.T) {
	body := `{"nodes": [
		{"id": "p1", "type": "project", "name": "api", "path": "/tmp"},
		{"id": "a1", "type": "agent", "name": "Planner", "parentId": "p1", "start": true, "backend": "claude", "prompt": "Plan", "maxCostUsd": -1}
	]}`

	response, _ := postRun(t, body)

	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "Planner: the cost budget must be more than 0 dollars") {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
}

func TestAFixerContinuesACopyOfTheCodersConversation(t *testing.T) {
	bin := fakeClaude(t)
	body := fmt.Sprintf(`{"nodes": [
		{"id": "p1", "type": "project", "name": "api", "path": %q},
		{"id": "a1", "type": "agent", "name": "Coder", "parentId": "p1", "start": true, "backend": "claude", "prompt": "Code"},
		{"id": "a2", "type": "agent", "name": "Fixer", "parentId": "p1", "backend": "claude", "prompt": "Now fix it", "continueSession": true}
	], "edges": [{"id": "e1", "from": "a1", "to": "a2"}]}`, t.TempDir())

	record := finishedRunOnly(t, body)

	if record.Status != engine.Succeeded {
		t.Fatalf("record = %+v", record)
	}
	if recorded(t, bin, "resume-2") != "session-1" {
		t.Fatalf("the fixer resumed %q, want the coder's session-1", recorded(t, bin, "resume-2"))
	}
	if _, err := os.Stat(filepath.Join(bin, "fork-2")); err != nil {
		t.Fatal("the fixer resumed the coder's session itself instead of a copy")
	}
	if details := record.Steps[1].Details; details["continuedFrom"] != "session-1" || details["sessionId"] != "session-2" {
		t.Fatalf("details = %+v", details)
	}
}

func TestContinuingASessionNeedsOneAgentOfTheSameBackend(t *testing.T) {
	flow := func(coderBackend string, edges string) string {
		return fmt.Sprintf(`{"nodes": [
			{"id": "p1", "type": "project", "name": "api", "path": "/tmp"},
			{"id": "a1", "type": "agent", "name": "Coder", "parentId": "p1", "start": true, "backend": %q, "prompt": "Code"},
			{"id": "a2", "type": "agent", "name": "Fixer", "parentId": "p1", "backend": "claude", "prompt": "Fix", "continueSession": true}
		], "edges": [%s]}`, coderBackend, edges)
	}
	cases := map[string]struct {
		body string
		want string
	}{
		"another backend": {
			body: flow("opencode", `{"id": "e1", "from": "a1", "to": "a2"}`),
			want: "Fixer: continues the session of Coder, which runs on opencode, not claude",
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
	lonely := `{"nodes": [
		{"id": "p1", "type": "project", "name": "api", "path": "/tmp"},
		{"id": "a2", "type": "agent", "name": "Fixer", "parentId": "p1", "start": true, "backend": "claude", "prompt": "Fix", "continueSession": true}
	]}`
	if response, _ := postRun(t, lonely); response.Code != http.StatusBadRequest ||
		!strings.Contains(response.Body.String(), "Fixer: continuing a session needs exactly one agent connected to this one") {
		t.Fatalf("lonely = %d %q", response.Code, response.Body.String())
	}
}
