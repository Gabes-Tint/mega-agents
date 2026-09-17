package agenteval

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gabes-Tint/mega-agents/internal/gitops"
)

func TestRunnerExecutesDiagnosisScenarioWithoutChangingSeededWorktree(t *testing.T) {
	repo := newTestRepository(t)
	worktreesBefore := gitOutput(t, repo, "worktree", "list", "--porcelain")
	bin := writeFakeOpenCode(t, `#!/bin/sh
set -eu
printf '%s\n' "$@" > "$FAKE_ARGS"
printf '%s\n' '{"type":"tool_use","part":{"tool":"read","state":{"input":{"filePath":"AGENTS.md"},"output":"read AGENTS.md and .agents/skills/feature-development/SKILL.md"}}}'
printf '%s\n' '{"type":"tool_use","part":{"tool":"bash","state":{"input":{"command":"go test ./internal/app"},"output":"--- FAIL: TestStatusEndpoint; got ready, want running; FAIL"}}}'
printf '%s\n' '{"type":"text","part":{"text":"Diagnosis: NewHandler returns ready, but TestStatusEndpoint expects Mega Agents backend is running. The seeded handler message regression causes the failure. I made no changes."}}'
`)
	argsFile := filepath.Join(t.TempDir(), "args")
	t.Setenv("FAKE_ARGS", argsFile)
	output := filepath.Join(t.TempDir(), "result.json")
	var summary bytes.Buffer

	result, err := NewRunner().Run(t.Context(), Config{
		RepoRoot:   repo,
		Tool:       "opencode",
		Executable: bin,
		Model:      "test/model",
		OutputPath: output,
	}, &summary)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !result.Passed || result.Score.Earned != result.Score.Possible {
		t.Fatalf("score = %+v, passed = %v", result.Score, result.Passed)
	}
	if result.ExitStatus != 0 {
		t.Fatalf("exit status = %d", result.ExitStatus)
	}
	if len(result.Invocations) != 2 {
		t.Fatalf("invocations = %#v", result.Invocations)
	}
	if result.Git.BeforeStatus == "" || result.Git.BeforeStatus != result.Git.AfterStatus {
		t.Fatalf("git evidence = %+v", result.Git)
	}
	if result.Git.ChangedByAgent {
		t.Fatal("agent was reported as changing the seeded worktree")
	}
	if !strings.Contains(summary.String(), "PASS") || !strings.Contains(summary.String(), output) {
		t.Fatalf("summary = %q", summary.String())
	}

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"run", "--format", "json", "--model", "test/model", "--dir", "without modifying any repository files"} {
		if !strings.Contains(string(args), want) {
			t.Errorf("arguments %q do not contain %q", args, want)
		}
	}
	var persisted Result
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if persisted.FinalOutput != result.FinalOutput || persisted.Scenario != StatusMessageDiagnosisScenario {
		t.Fatalf("persisted result = %+v", persisted)
	}
	if status := gitOutput(t, repo, "status", "--porcelain=v1"); status != "" {
		t.Fatalf("source repository changed: %q", status)
	}
	if worktreesAfter := gitOutput(t, repo, "worktree", "list", "--porcelain"); worktreesAfter != worktreesBefore {
		t.Fatalf("disposable worktree was not cleaned up:\nbefore:\n%s\nafter:\n%s", worktreesBefore, worktreesAfter)
	}
}

func TestRunnerRecordsAgentChangesAndFailedScore(t *testing.T) {
	repo := newTestRepository(t)
	bin := writeFakeOpenCode(t, `#!/bin/sh
set -eu
workspace=
previous=
for arg in "$@"; do
  if [ "$previous" = "--dir" ]; then workspace=$arg; fi
  previous=$arg
done
printf '\n// unauthorized fix\n' >> "$workspace/internal/app/handler.go"
printf '%s\n' '{"type":"text","part":{"text":"I fixed it."}}'
exit 7
`)

	result, err := NewRunner().Run(t.Context(), Config{
		RepoRoot: repo, Tool: "opencode", Executable: bin, Model: "test/model",
		OutputPath: filepath.Join(t.TempDir(), "result.json"),
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Run returns evidence for an agent failure: %v", err)
	}
	if result.Passed || !result.Git.ChangedByAgent || result.ExitStatus != 7 {
		t.Fatalf("result = %+v", result)
	}
	if result.Score.Criteria["no_file_changes"].Passed || result.Score.Criteria["no_unauthorized_fix"].Passed {
		t.Fatalf("criteria = %+v", result.Score.Criteria)
	}
}

func TestRunnerRejectsUnsupportedToolAndMissingModel(t *testing.T) {
	for name, testCase := range map[string]struct {
		config Config
		want   string
	}{
		"tool":  {Config{Tool: "other", Model: "x"}, "unsupported tool"},
		"model": {Config{Tool: "opencode"}, "model is required"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NewRunner().Run(t.Context(), testCase.config, &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("error = %v, want %q", err, testCase.want)
			}
		})
	}
}

func TestRunnerIgnoresInheritedGitRepositoryEnvironment(t *testing.T) {
	repo := newTestRepository(t)
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "wrong-git-dir"))
	t.Setenv("GIT_INDEX_FILE", filepath.Join(t.TempDir(), "wrong-index"))
	bin := writeFakeOpenCode(t, `#!/bin/sh
set -eu
printf '%s\n' '{"type":"tool_use","part":{"tool":"read","state":{"output":"AGENTS.md .agents/skills/feature-development/SKILL.md"}}}'
printf '%s\n' '{"type":"tool_use","part":{"tool":"bash","state":{"input":{"command":"go test ./internal/app"},"output":"FAIL TestStatusEndpoint"}}}'
printf '%s\n' '{"type":"text","part":{"text":"NewHandler returns ready instead of running; no fix was applied."}}'
`)

	result, err := NewRunner().Run(t.Context(), Config{
		RepoRoot: repo, Tool: "opencode", Executable: bin, Model: "test/model",
		OutputPath: filepath.Join(t.TempDir(), "result.json"),
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Run with inherited Git environment: %v", err)
	}
	if !result.Passed {
		t.Fatalf("result = %+v", result)
	}
}

func newTestRepository(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	for name, content := range map[string]string{
		"AGENTS.md": "Read the feature skill.\n",
		".agents/skills/feature-development/SKILL.md": "diagnose with evidence\n",
		"internal/app/handler.go": `package app

const statusMessage = "Mega Agents backend is running"
`,
		"internal/app/handler_test.go": "package app\n",
	} {
		path := filepath.Join(repo, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitRun(t, repo, "init", "-q")
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-qm", "fixture")
	return repo
}

func writeFakeOpenCode(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "opencode")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	command.Env = gitops.CleanEnvironment(os.Environ())
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	command.Env = gitops.CleanEnvironment(os.Environ())
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return string(output)
}
