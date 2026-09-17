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

// fakeGH puts a gh first on PATH that records each call's arguments and
// answers issue views with issue #7 (issue #8 is labeled "Paused") and pull
// request creation with #42.
func fakeGHCLI(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	script := `#!/bin/sh
here="$(dirname "$0")"
printf '%s\n' "$@" >> "$here/calls"
echo "--" >> "$here/calls"
case "$1 $2" in
  "issue view")
    if [ "$3" = 8 ]; then
      echo '{"number":8,"title":"Later","body":"","url":"https://github.com/acme/api/issues/8","labels":[{"name":"Paused"}],"comments":[]}'
    else
      echo '{"number":7,"title":"Add a changelog","body":"Write CHANGELOG.md","url":"https://github.com/acme/api/issues/7","labels":[],"comments":[]}'
    fi ;;
  "pr create") echo "https://github.com/acme/api/pull/42" ;;
  *) echo "unexpected gh $*" >&2; exit 1 ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return bin
}

func TestAnIssueBecomesAPullRequest(t *testing.T) {
	gh := fakeGHCLI(t)
	clone, _ := projectClone(t)
	runGit(t, clone, "config", "user.name", "Test")
	runGit(t, clone, "config", "user.email", "test@example.com")
	worktree := filepath.Join(t.TempDir(), "changelog")
	body := fmt.Sprintf(`{"nodes": [
		{"id": "p1", "type": "project", "name": "api", "path": %q},
		{"id": "g1", "type": "github", "name": "GitHub 1", "parentId": "p1", "start": true, "authenticated": true},
		{"id": "i1", "type": "action", "action": "issue", "name": "Read issue", "parentId": "g1", "start": true, "issue": 7},
		{"id": "w1", "type": "action", "action": "worktree", "name": "Create worktree", "parentId": "g1", "branch": "issue-7", "worktreePath": %q},
		{"id": "c1", "type": "command", "name": "Implement", "parentId": "p1", "command": "echo \"$MEGA_AGENTS_WORKSPACE_BRANCH\" > CHANGELOG.md"},
		{"id": "k1", "type": "action", "action": "commit", "name": "Commit", "parentId": "g1", "message": "Implement {{workspace.branch}}"},
		{"id": "s1", "type": "action", "action": "push", "name": "Push", "parentId": "g1"},
		{"id": "r1", "type": "action", "action": "pullrequest", "name": "Open pull request", "parentId": "g1",
		 "title": "Close issue on {{workspace.branch}}", "body": "Made by Mega Agents"}
	], "edges": [
		{"id": "e1", "from": "i1", "to": "w1"},
		{"id": "e2", "from": "w1", "to": "c1"},
		{"id": "e3", "from": "c1", "to": "k1"},
		{"id": "e4", "from": "k1", "to": "s1"},
		{"id": "e5", "from": "s1", "to": "r1"}
	]}`, clone, worktree)

	record := finishedRunOnly(t, body)

	if record.Status != engine.Succeeded {
		t.Fatalf("record = %+v", record)
	}
	details := map[string]map[string]any{}
	for _, step := range record.Steps {
		details[step.TaskID] = step.Details
	}
	if details["i1"]["issue"].(map[string]any)["title"] != "Add a changelog" {
		t.Fatalf("issue details = %+v", details["i1"])
	}
	commit, _ := details["k1"]["commit"].(string)
	if subject := runGit(t, worktree, "log", "-1", "--format=%s"); subject != "Implement issue-7" || commit == "" {
		t.Fatalf("commit %q subject = %q", commit, subject)
	}
	remote := runGit(t, clone, "ls-remote", "origin", "refs/heads/issue-7")
	if !strings.HasPrefix(remote, commit) {
		t.Fatalf("remote = %q, want the pushed commit %s", remote, commit)
	}
	if details["r1"]["url"] != "https://github.com/acme/api/pull/42" || details["r1"]["number"] != 42.0 {
		t.Fatalf("pull request details = %+v", details["r1"])
	}
	calls, _ := os.ReadFile(filepath.Join(gh, "calls"))
	if !strings.Contains(string(calls), "pr\ncreate\n--repo\nacme/api\n--head\nissue-7\n--base\nmain\n--title\nClose issue on issue-7\n") {
		t.Fatalf("gh calls =\n%s", calls)
	}
}

func TestReadIssueFailsOnAnIssueWithALabelToIgnore(t *testing.T) {
	fakeGHCLI(t)
	clone, _ := projectClone(t)
	flow := func(settings string) string {
		return fmt.Sprintf(`{"nodes": [
			{"id": "p1", "type": "project", "name": "api", "path": %q},
			{"id": "g1", "type": "github", "name": "GitHub 1", "parentId": "p1", "start": true, "authenticated": true},
			{"id": "i1", "type": "action", "action": "issue", "name": "Read issue", "parentId": "g1", "start": true, "issue": 8%s}
		], "edges": []}`, clone, settings)
	}

	ignored := finishedRunOnly(t, flow(`, "ignoreLabels": ["draft", "paused"]`))

	if ignored.Status != engine.Failed ||
		ignored.Steps[0].Error != `issue #8 is labeled "Paused", one of the labels to ignore` {
		t.Fatalf("record = %+v", ignored)
	}
	// A workflow saved before the setting existed ignores no labels.
	for _, settings := range []string{"", `, "ignoreLabels": []`, `, "ignoreLabels": ["draft"]`} {
		if record := finishedRunOnly(t, flow(settings)); record.Status != engine.Succeeded {
			t.Fatalf("with %q: record = %+v", settings, record)
		}
	}
}

func TestAnAgentPassesItsWorkspaceOn(t *testing.T) {
	fakeClaude(t)
	clone, _ := projectClone(t)
	worktree := filepath.Join(t.TempDir(), "pass")
	body := fmt.Sprintf(`{"nodes": [
		{"id": "p1", "type": "project", "name": "api", "path": %q},
		{"id": "g1", "type": "github", "name": "GitHub 1", "parentId": "p1", "start": true},
		{"id": "w1", "type": "action", "action": "worktree", "name": "Create worktree", "parentId": "g1", "start": true, "branch": "pass", "worktreePath": %q},
		{"id": "a1", "type": "agent", "name": "Coder", "parentId": "p1", "backend": "claude", "prompt": "Code"},
		{"id": "a2", "type": "agent", "name": "Reviewer", "parentId": "p1", "backend": "claude", "prompt": "Review {{workspace.branch}} after {{result}}"},
		{"id": "c1", "type": "command", "name": "Where", "parentId": "p1", "command": "pwd"}
	], "edges": [
		{"id": "e1", "from": "w1", "to": "a1"},
		{"id": "e2", "from": "a1", "to": "a2"},
		{"id": "e3", "from": "a2", "to": "c1"}
	]}`, clone, worktree)

	record := finishedRunOnly(t, body)

	if record.Status != engine.Succeeded {
		t.Fatalf("record = %+v", record)
	}
	if output := record.Steps[3].Details["output"]; output != worktree+"\n" {
		t.Fatalf("the command ran in %q, want the worktree %s", output, worktree)
	}
}

func TestDeliveryPlansAreCheckedBeforeRunning(t *testing.T) {
	flow := func(actions string, edges string) string {
		return fmt.Sprintf(`{"nodes": [
			{"id": "p1", "type": "project", "name": "api", "path": "/tmp"},
			{"id": "g1", "type": "github", "name": "GitHub 1", "parentId": "p1", "start": true},
			%s
		], "edges": [%s]}`, actions, edges)
	}
	cases := map[string]struct {
		body string
		want string
	}{
		"commit without a workspace": {
			body: flow(`{"id": "k1", "type": "action", "action": "commit", "name": "Commit", "parentId": "g1", "start": true, "message": "x"}`, ""),
			want: "Commit needs a workspace; connect a Create worktree action before it",
		},
		"issue without a number": {
			body: flow(`{"id": "i1", "type": "action", "action": "issue", "name": "Read issue", "parentId": "g1", "start": true}`, ""),
			want: "Read issue: set the issue number",
		},
		"unknown placeholder in a title": {
			body: flow(`{"id": "w1", "type": "action", "action": "worktree", "name": "Create worktree", "parentId": "g1", "start": true, "branch": "x"},
				{"id": "r1", "type": "action", "action": "pullrequest", "name": "Open pull request", "parentId": "g1", "title": "{{nope}}"}`,
				`{"id": "e1", "from": "w1", "to": "r1"}`),
			want: "Open pull request: unknown placeholder {{nope}}",
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
