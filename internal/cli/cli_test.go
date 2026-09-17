package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Gabes-Tint/mega-agents/internal/app"
	"github.com/Gabes-Tint/mega-agents/internal/gitops"
)

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{
		"-c", "user.name=Test", "-c", "user.email=test@example.com",
		"-c", "init.defaultBranch=main", "-c", "commit.gpgsign=false",
	}, args...)...)
	command.Dir = dir
	command.Env = gitops.CleanEnvironment(os.Environ())
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

// projectClone returns a clone of https://github.com/acme/api, rewritten
// locally to a bare repository holding a commit the clone has not fetched.
func projectClone(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	bare, seed, clone := filepath.Join(root, "remote.git"), filepath.Join(root, "seed"), filepath.Join(root, "clone")
	runGit(t, root, "init", "--bare", "--quiet", bare)
	runGit(t, root, "init", "--quiet", seed)
	runGit(t, seed, "commit", "--quiet", "--allow-empty", "-m", "first")
	runGit(t, seed, "push", "--quiet", bare, "HEAD:refs/heads/main")
	runGit(t, root, "clone", "--quiet", bare, clone)
	runGit(t, clone, "remote", "set-url", "origin", "https://github.com/acme/api")
	runGit(t, clone, "config", "url."+bare+".insteadOf", "https://github.com/acme/api")
	runGit(t, seed, "commit", "--quiet", "--allow-empty", "-m", "second")
	runGit(t, seed, "push", "--quiet", bare, "HEAD:refs/heads/main")
	return clone
}

type result struct {
	code   int
	stdout string
	stderr string
}

func run(t *testing.T, args ...string) result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Main(args, Env{
		Stdout: &stdout, Stderr: &stderr,
		Serve: func() error { return errors.New("serve was not expected") },
	})
	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func writeWorkflow(t *testing.T, name string, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func fetchWorkflowYAML(clone string) string {
	return fmt.Sprintf(`apiVersion: megaagents.dev/v1alpha1
kind: Workflow
metadata:
  name: nightly-sync
nodes:
  api:
    uses: project@v1
    with:
      path: %q
    children:
      github:
        uses: github@v1
        start: true
        with:
          authenticated: true
        children:
          fetch:
            uses: git/fetch@v1
            name: "Fetch"
            start: true
          worktree:
            uses: git/worktree@v1
            name: "Create worktree"
            needs: [fetch]
`, clone)
}

var runID = regexp.MustCompile(`Run (\d{8}T\d{6}Z-[0-9a-f]{6})`)

func TestRunExecutesAWorkflowFileAndReportsEachStep(t *testing.T) {
	t.Setenv("MEGA_AGENTS_HOME", t.TempDir())
	path := writeWorkflow(t, "sync.yaml", fetchWorkflowYAML(projectClone(t)))

	got := run(t, "run", path)

	if got.code != 1 {
		t.Fatalf("code = %d, want 1 for a failed run\nstdout:\n%s\nstderr:\n%s", got.code, got.stdout, got.stderr)
	}
	for _, want := range []string{
		"started nightly-sync",
		"✔ Fetch succeeded",
		"✖ Create worktree failed: set the branch the worktree works on",
		"failed",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, got.stdout)
		}
	}
	if !runID.MatchString(got.stdout) {
		t.Errorf("stdout names no run id:\n%s", got.stdout)
	}
}

func TestRunSucceedsWithExitZero(t *testing.T) {
	t.Setenv("MEGA_AGENTS_HOME", t.TempDir())
	workflow := strings.Replace(fetchWorkflowYAML(projectClone(t)), "            needs: [fetch]\n",
		"            needs: [fetch]\n            with:\n              branch: nightly\n", 1)

	got := run(t, "run", writeWorkflow(t, "sync.yml", workflow))

	if got.code != 0 || !strings.Contains(got.stdout, "✔ Create worktree succeeded") {
		t.Fatalf("code = %d\nstdout:\n%s\nstderr:\n%s", got.code, got.stdout, got.stderr)
	}
}

func TestRunAcceptsTheEditorsJSONGraph(t *testing.T) {
	t.Setenv("MEGA_AGENTS_HOME", t.TempDir())
	graph := fmt.Sprintf(`{"name": "from-editor", "nodes": [
		{"id": "p1", "type": "project", "name": "api", "path": %q},
		{"id": "g1", "type": "github", "name": "GitHub 1", "parentId": "p1", "start": true, "authenticated": true}
	]}`, projectClone(t))

	got := run(t, "run", writeWorkflow(t, "graph.json", graph))

	if got.code != 0 || !strings.Contains(got.stdout, "✔ GitHub 1 succeeded") || !strings.Contains(got.stdout, "started from-editor") {
		t.Fatalf("code = %d\nstdout:\n%s\nstderr:\n%s", got.code, got.stdout, got.stderr)
	}
}

func TestRunRejectsWorkflowsThatCannotRun(t *testing.T) {
	t.Setenv("MEGA_AGENTS_HOME", t.TempDir())
	cases := map[string]struct {
		args []string
		want string
	}{
		"no file":        {args: []string{"run"}, want: "usage: mega-agents run <workflow.yaml|workflow.json>"},
		"missing file":   {args: []string{"run", "/nonexistent/flow.yaml"}, want: "cannot read /nonexistent/flow.yaml"},
		"unknown format": {args: []string{"run", writeWorkflow(t, "flow.txt", "x")}, want: "use a .yaml, .yml or .json workflow"},
		"bad json":       {args: []string{"run", writeWorkflow(t, "flow.json", "{")}, want: "is not a valid workflow graph"},
		"bad yaml":       {args: []string{"run", writeWorkflow(t, "flow.yaml", "kind: Other\n")}, want: `kind "Other" is not Workflow`},
		"no start": {
			args: []string{"run", writeWorkflow(t, "flow.json", `{"nodes": [{"id": "p1", "type": "project", "name": "api"}]}`)},
			want: "flag a GitHub block or an agent as the starting point",
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			got := run(t, testCase.args...)

			if got.code != 2 || !strings.Contains(got.stderr, testCase.want) {
				t.Fatalf("code = %d, stderr = %q, want 2 and %q", got.code, got.stderr, testCase.want)
			}
		})
	}
}

// recordedRun runs the fetch workflow once and returns its run id.
func recordedRun(t *testing.T) string {
	t.Helper()
	t.Setenv("MEGA_AGENTS_HOME", t.TempDir())
	got := run(t, "run", writeWorkflow(t, "sync.yaml", fetchWorkflowYAML(projectClone(t))))
	match := runID.FindStringSubmatch(got.stdout)
	if match == nil {
		t.Fatalf("no run id in:\n%s", got.stdout)
	}
	return match[1]
}

func TestRunsListsRecordedRuns(t *testing.T) {
	id := recordedRun(t)

	got := run(t, "runs")

	if got.code != 0 || !strings.Contains(got.stdout, id) || !strings.Contains(got.stdout, "failed") ||
		!strings.Contains(got.stdout, "nightly-sync") {
		t.Fatalf("code = %d\nstdout:\n%s\nstderr:\n%s", got.code, got.stdout, got.stderr)
	}
}

func TestRunsWithNothingRecorded(t *testing.T) {
	t.Setenv("MEGA_AGENTS_HOME", t.TempDir())

	got := run(t, "runs")

	if got.code != 0 || !strings.Contains(got.stdout, "No runs recorded yet") {
		t.Fatalf("code = %d, stdout = %q", got.code, got.stdout)
	}
}

func TestRunsShowDescribesEveryStep(t *testing.T) {
	id := recordedRun(t)

	got := run(t, "runs", "show", id)

	for _, want := range []string{
		"Run " + id, "nightly-sync", "failed",
		"fetch  Fetch  fetch  succeeded",
		"worktree  Create worktree  worktree  failed",
		"set the branch the worktree works on",
		"remote: origin",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, got.stdout)
		}
	}
}

func TestLogsPrintsOneStepByIDOrName(t *testing.T) {
	id := recordedRun(t)

	for _, step := range []string{"fetch", "Fetch"} {
		got := run(t, "logs", id, step)

		if got.code != 0 || !strings.Contains(got.stdout, "$ git fetch origin") || strings.Contains(got.stdout, "worktree") {
			t.Fatalf("logs %s: code = %d\nstdout:\n%s\nstderr:\n%s", step, got.code, got.stdout, got.stderr)
		}
	}
}

func TestLogsPrintsEveryStepInOrder(t *testing.T) {
	id := recordedRun(t)

	got := run(t, "logs", id)

	fetch := strings.Index(got.stdout, "== Fetch (fetch) succeeded ==")
	worktree := strings.Index(got.stdout, "== Create worktree (worktree) failed ==")
	if got.code != 0 || fetch < 0 || worktree < fetch || !strings.Contains(got.stdout, "Create worktree failed in") {
		t.Fatalf("code = %d\nstdout:\n%s", got.code, got.stdout)
	}
}

func TestInspectionCommandsExplainMistakes(t *testing.T) {
	id := recordedRun(t)
	cases := map[string]struct {
		args []string
		code int
		want string
	}{
		"show without id":  {args: []string{"runs", "show"}, code: 2, want: "usage: mega-agents runs show <run-id>"},
		"unknown subcmd":   {args: []string{"runs", "delete"}, code: 2, want: "usage: mega-agents runs [show <run-id>]"},
		"unknown run":      {args: []string{"runs", "show", "20000101T000000Z-000000"}, code: 1, want: "run 20000101T000000Z-000000 not found"},
		"logs without id":  {args: []string{"logs"}, code: 2, want: "usage: mega-agents logs <run-id> [step]"},
		"logs unknown run": {args: []string{"logs", "nope"}, code: 1, want: "run nope not found"},
		"unknown step":     {args: []string{"logs", id, "deploy"}, code: 1, want: `run ` + id + ` has no step "deploy"`},
		"unknown command":  {args: []string{"deploy"}, code: 2, want: `unknown command "deploy"`},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			got := run(t, testCase.args...)

			if got.code != testCase.code || !strings.Contains(got.stderr, testCase.want) {
				t.Fatalf("code = %d, stderr = %q, want %d and %q", got.code, got.stderr, testCase.code, testCase.want)
			}
		})
	}
}

func TestHelpListsTheCommands(t *testing.T) {
	got := run(t, "help")

	for _, want := range []string{"serve", "run <workflow", "workflows", "runs", "runs show <run-id>", "logs <run-id> [step]"} {
		if got.code != 0 || !strings.Contains(got.stdout, want) {
			t.Fatalf("help lacks %q:\n%s", want, got.stdout)
		}
	}
}

func TestNoArgumentsServesTheEditor(t *testing.T) {
	served := false
	code := Main(nil, Env{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Serve: func() error {
		served = true
		return nil
	}})

	if code != 0 || !served {
		t.Fatalf("code = %d, served = %v", code, served)
	}
	var stderr bytes.Buffer
	if code := Main([]string{"serve"}, Env{Stdout: &bytes.Buffer{}, Stderr: &stderr, Serve: func() error {
		return errors.New("port in use")
	}}); code != 1 || !strings.Contains(stderr.String(), "port in use") {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
}

func TestRunExecutesASavedWorkflowByName(t *testing.T) {
	t.Setenv("MEGA_AGENTS_HOME", t.TempDir())
	request, err := app.ParseWorkflowYAML([]byte(fetchWorkflowYAML(projectClone(t))))
	if err != nil {
		t.Fatal(err)
	}
	store, _ := app.DefaultWorkflowStore()
	if err := store.Save("nightly-sync", request); err != nil {
		t.Fatal(err)
	}

	listed := run(t, "workflows")
	got := run(t, "run", "nightly-sync")

	if listed.code != 0 || !strings.Contains(listed.stdout, "nightly-sync") {
		t.Fatalf("workflows: %d %q", listed.code, listed.stdout)
	}
	if !strings.Contains(got.stdout, "started nightly-sync") || !strings.Contains(got.stdout, "✔ Fetch succeeded") {
		t.Fatalf("run: %d\n%s\n%s", got.code, got.stdout, got.stderr)
	}
	if missing := run(t, "run", "nope"); missing.code != 2 || !strings.Contains(missing.stderr, "no workflow file or saved workflow named nope") {
		t.Fatalf("missing: %d %q", missing.code, missing.stderr)
	}
}

func TestWorkflowsWithNothingSaved(t *testing.T) {
	t.Setenv("MEGA_AGENTS_HOME", t.TempDir())

	if got := run(t, "workflows"); got.code != 0 || !strings.Contains(got.stdout, "No workflows saved yet") {
		t.Fatalf("got %d %q", got.code, got.stdout)
	}
}

func TestRunIsRecordedAsCancelledWhenInterrupted(t *testing.T) {
	t.Setenv("MEGA_AGENTS_HOME", t.TempDir())
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\nsleep 30\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	path := writeWorkflow(t, "slow.json", fmt.Sprintf(`{"nodes": [
		{"id": "p1", "type": "project", "name": "api", "path": %q},
		{"id": "a1", "type": "agent", "name": "Slow", "parentId": "p1", "start": true, "backend": "claude", "prompt": "Think"}
	]}`, t.TempDir()))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(300*time.Millisecond, cancel)
	var stdout, stderr bytes.Buffer

	code := Main([]string{"run", path}, Env{Stdout: &stdout, Stderr: &stderr, Context: ctx})

	if code != 1 || !strings.Contains(stdout.String(), "cancelled") {
		t.Fatalf("code = %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
}

func TestRetryRunsWhatFailedAgain(t *testing.T) {
	t.Setenv("MEGA_AGENTS_HOME", t.TempDir())
	marker := filepath.Join(t.TempDir(), "fixed")
	path := writeWorkflow(t, "gate.json", fmt.Sprintf(`{"name": "gate", "nodes": [
		{"id": "p1", "type": "project", "name": "api", "path": %q},
		{"id": "g1", "type": "github", "name": "GitHub 1", "parentId": "p1", "start": true, "authenticated": true},
		{"id": "f1", "type": "action", "action": "fetch", "name": "Fetch", "parentId": "g1", "start": true},
		{"id": "c1", "type": "command", "name": "Gate", "parentId": "p1", "command": "test -e %s"}
	], "edges": [{"id": "e1", "from": "f1", "to": "c1"}]}`, projectClone(t), marker))
	first := run(t, "run", path)
	id := runID.FindStringSubmatch(first.stdout)[1]
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got := run(t, "retry", id)

	if got.code != 0 || !strings.Contains(got.stdout, "retrying "+id) || !strings.Contains(got.stdout, "✔ Gate succeeded") {
		t.Fatalf("code = %d\n%s\n%s", got.code, got.stdout, got.stderr)
	}
	if again := run(t, "retry", runID.FindStringSubmatch(got.stdout)[1]); again.code != 1 || !strings.Contains(again.stderr, "nothing to retry") {
		t.Fatalf("retrying a success = %d %q", again.code, again.stderr)
	}
	if usage := run(t, "retry"); usage.code != 2 || !strings.Contains(usage.stderr, "usage: mega-agents retry <run-id>") {
		t.Fatalf("usage = %d %q", usage.code, usage.stderr)
	}
}

func TestTemplatesAreListedAndStartSavedWorkflows(t *testing.T) {
	t.Setenv("MEGA_AGENTS_HOME", t.TempDir())

	listed := run(t, "templates")
	created := run(t, "init", "gate-and-fix", "nightly-gate")
	defaulted := run(t, "init", "review-and-route")
	saved := run(t, "workflows")

	if listed.code != 0 || !strings.Contains(listed.stdout, "issue-to-pull-request") || !strings.Contains(listed.stdout, "Gate and fix") {
		t.Fatalf("templates: %d %q", listed.code, listed.stdout)
	}
	if created.code != 0 || !strings.Contains(created.stdout, "Saved nightly-gate from template gate-and-fix") {
		t.Fatalf("init: %d %q %q", created.code, created.stdout, created.stderr)
	}
	if defaulted.code != 0 || !strings.Contains(saved.stdout, "nightly-gate") || !strings.Contains(saved.stdout, "review-and-route") {
		t.Fatalf("workflows: %q", saved.stdout)
	}
	for args, want := range map[string]string{
		"init":                "usage: mega-agents init <template> [name]",
		"init nope":           "template nope not found; see mega-agents templates",
		"init gate-and-fix X": "use lowercase letters, digits and -",
	} {
		got := run(t, strings.Fields(args)...)
		if got.code == 0 || !strings.Contains(got.stderr, want) {
			t.Errorf("%s: %d %q, want %q", args, got.code, got.stderr, want)
		}
	}
}
