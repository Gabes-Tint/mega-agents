package agenteval

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const StatusMessageDiagnosisScenario = "status-message-diagnosis"

const scenarioPrompt = `Diagnose the seeded failure in this repository without modifying any repository files.

Read AGENTS.md and any applicable referenced skill before investigating. Reproduce the failure with the narrowest relevant deterministic command, inspect the responsible code and test, and report the root cause with concrete evidence. Do not implement or apply a fix. Your final response must name the failing command, the observed mismatch, and the responsible code.`

type Config struct {
	RepoRoot   string
	Tool       string
	Executable string
	Model      string
	Variant    string
	OutputPath string
}

type Invocation struct {
	Tool   string `json:"tool"`
	Input  any    `json:"input,omitempty"`
	Output string `json:"output,omitempty"`
}

type GitEvidence struct {
	BeforeStatus   string `json:"before_status"`
	AfterStatus    string `json:"after_status"`
	BeforeDiff     string `json:"before_diff"`
	AfterDiff      string `json:"after_diff"`
	BeforeDigest   string `json:"before_digest"`
	AfterDigest    string `json:"after_digest"`
	ChangedByAgent bool   `json:"changed_by_agent"`
}

type Criterion struct {
	Passed   bool   `json:"passed"`
	Evidence string `json:"evidence"`
}

type Score struct {
	Earned   int                  `json:"earned"`
	Possible int                  `json:"possible"`
	Criteria map[string]Criterion `json:"criteria"`
}

type Result struct {
	Scenario    string        `json:"scenario"`
	Tool        string        `json:"tool"`
	Model       string        `json:"model"`
	Variant     string        `json:"variant,omitempty"`
	Passed      bool          `json:"passed"`
	ExitStatus  int           `json:"exit_status"`
	Duration    time.Duration `json:"duration_ns"`
	Transcript  string        `json:"transcript"`
	FinalOutput string        `json:"final_output"`
	Invocations []Invocation  `json:"invocations,omitempty"`
	Git         GitEvidence   `json:"git"`
	Score       Score         `json:"score"`
}

type Runner struct {
	now func() time.Time
}

func NewRunner() Runner {
	return Runner{now: time.Now}
}

func (runner Runner) Run(ctx context.Context, config Config, summary io.Writer) (Result, error) {
	if config.Tool != "opencode" {
		return Result{}, fmt.Errorf("unsupported tool %q (only opencode is implemented)", config.Tool)
	}
	if strings.TrimSpace(config.Model) == "" {
		return Result{}, errors.New("model is required")
	}
	if config.RepoRoot == "" {
		config.RepoRoot = "."
	}
	if config.Executable == "" {
		config.Executable = "opencode"
	}
	if config.OutputPath == "" {
		config.OutputPath = filepath.Join(config.RepoRoot, "reports", "agent-eval", "result.json")
	}
	absRepo, err := filepath.Abs(config.RepoRoot)
	if err != nil {
		return Result{}, fmt.Errorf("resolve repository: %w", err)
	}
	absOutput, err := filepath.Abs(config.OutputPath)
	if err != nil {
		return Result{}, fmt.Errorf("resolve output: %w", err)
	}

	workspace, cleanup, err := createWorktree(ctx, absRepo)
	if err != nil {
		return Result{}, err
	}
	defer cleanup()
	if err := seedScenario(workspace); err != nil {
		return Result{}, err
	}
	before, err := inspectGit(ctx, workspace)
	if err != nil {
		return Result{}, err
	}

	arguments := []string{"run", "--format", "json", "--model", config.Model, "--dir", workspace}
	if config.Variant != "" {
		arguments = append(arguments, "--variant", config.Variant)
	}
	arguments = append(arguments, scenarioPrompt)
	command := exec.CommandContext(ctx, config.Executable, arguments...)
	command.Dir = workspace
	command.Env = cleanGitEnvironment(os.Environ())
	started := runner.now()
	output, commandErr := command.CombinedOutput()
	duration := runner.now().Sub(started)
	exitStatus := 0
	if commandErr != nil {
		var exitError *exec.ExitError
		if errors.As(commandErr, &exitError) {
			exitStatus = exitError.ExitCode()
		} else {
			return Result{}, fmt.Errorf("execute opencode: %w", commandErr)
		}
	}
	after, err := inspectGit(ctx, workspace)
	if err != nil {
		return Result{}, err
	}
	gitEvidence := GitEvidence{
		BeforeStatus: before.status, AfterStatus: after.status,
		BeforeDiff: before.diff, AfterDiff: after.diff,
		BeforeDigest: before.digest, AfterDigest: after.digest,
		ChangedByAgent: before.digest != after.digest,
	}
	finalOutput, invocations := parseTranscript(output)
	result := Result{
		Scenario: StatusMessageDiagnosisScenario, Tool: config.Tool, Model: config.Model,
		Variant: config.Variant, ExitStatus: exitStatus, Duration: duration,
		Transcript: string(output), FinalOutput: finalOutput, Invocations: invocations, Git: gitEvidence,
	}
	result.Score = score(result)
	result.Passed = result.ExitStatus == 0 && result.Score.Earned == result.Score.Possible
	if err := writeResult(absOutput, result); err != nil {
		return Result{}, err
	}
	state := "FAIL"
	if result.Passed {
		state = "PASS"
	}
	_, _ = fmt.Fprintf(summary, "%s %s: %d/%d criteria, exit %d, %s\nResult: %s\n",
		state, result.Scenario, result.Score.Earned, result.Score.Possible,
		result.ExitStatus, result.Duration.Round(time.Millisecond), absOutput)
	return result, nil
}

type gitState struct {
	status string
	diff   string
	digest string
}

func createWorktree(ctx context.Context, repo string) (string, func(), error) {
	parent, err := os.MkdirTemp("", "mega-agents-eval-")
	if err != nil {
		return "", nil, fmt.Errorf("create evaluation directory: %w", err)
	}
	workspace := filepath.Join(parent, "worktree")
	command := exec.CommandContext(ctx, "git", "worktree", "add", "--detach", "--quiet", workspace, "HEAD")
	command.Dir = repo
	command.Env = cleanGitEnvironment(os.Environ())
	if output, commandErr := command.CombinedOutput(); commandErr != nil {
		_ = os.RemoveAll(parent)
		return "", nil, fmt.Errorf("create disposable worktree: %w: %s", commandErr, output)
	}
	cleanup := func() {
		remove := exec.Command("git", "worktree", "remove", "--force", workspace)
		remove.Dir = repo
		remove.Env = cleanGitEnvironment(os.Environ())
		_ = remove.Run()
		_ = os.RemoveAll(parent)
	}
	return workspace, cleanup, nil
}

func seedScenario(workspace string) error {
	path := filepath.Join(workspace, "internal", "app", "handler.go")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read scenario target: %w", err)
	}
	const healthy = "Mega Agents backend is running"
	const broken = "Mega Agents backend is ready"
	if strings.Count(string(data), healthy) != 1 {
		return fmt.Errorf("seed scenario: expected exactly one %q message", healthy)
	}
	seeded := strings.Replace(string(data), healthy, broken, 1)
	if err := os.WriteFile(path, []byte(seeded), 0o644); err != nil {
		return fmt.Errorf("seed scenario: %w", err)
	}
	return nil
}

func inspectGit(ctx context.Context, workspace string) (gitState, error) {
	status, err := gitCommand(ctx, workspace, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return gitState{}, err
	}
	diff, err := gitCommand(ctx, workspace, "diff", "--binary", "HEAD", "--")
	if err != nil {
		return gitState{}, err
	}
	files, err := snapshotFiles(workspace)
	if err != nil {
		return gitState{}, err
	}
	hash := sha256.New()
	_, _ = io.WriteString(hash, status)
	_, _ = io.WriteString(hash, diff)
	for _, file := range files {
		_, _ = io.WriteString(hash, file)
		data, readErr := os.ReadFile(filepath.Join(workspace, file))
		if readErr != nil {
			return gitState{}, fmt.Errorf("snapshot %s: %w", file, readErr)
		}
		_, _ = hash.Write(data)
	}
	return gitState{status: status, diff: diff, digest: hex.EncodeToString(hash.Sum(nil))}, nil
}

func snapshotFiles(workspace string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(workspace, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if entry.Type().IsRegular() {
			relative, err := filepath.Rel(workspace, path)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(relative))
		}
		return nil
	})
	slices.Sort(files)
	return files, err
}

func gitCommand(ctx context.Context, workspace string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = workspace
	command.Env = cleanGitEnvironment(os.Environ())
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, output)
	}
	return string(output), nil
}

func cleanGitEnvironment(environment []string) []string {
	repositoryVariables := map[string]bool{
		"GIT_ALTERNATE_OBJECT_DIRECTORIES": true,
		"GIT_COMMON_DIR":                   true,
		"GIT_DIR":                          true,
		"GIT_GRAFT_FILE":                   true,
		"GIT_INDEX_FILE":                   true,
		"GIT_OBJECT_DIRECTORY":             true,
		"GIT_PREFIX":                       true,
		"GIT_REPLACE_REF_BASE":             true,
		"GIT_SHALLOW_FILE":                 true,
		"GIT_WORK_TREE":                    true,
	}
	cleaned := make([]string, 0, len(environment))
	for _, variable := range environment {
		name, _, _ := strings.Cut(variable, "=")
		if !repositoryVariables[name] {
			cleaned = append(cleaned, variable)
		}
	}
	return cleaned
}

func parseTranscript(transcript []byte) (string, []Invocation) {
	var finalOutput string
	var invocations []Invocation
	scanner := bufio.NewScanner(bytesReader(transcript))
	buffer := make([]byte, 64*1024)
	scanner.Buffer(buffer, 4*1024*1024)
	for scanner.Scan() {
		var event struct {
			Type string `json:"type"`
			Part struct {
				Text  string `json:"text"`
				Tool  string `json:"tool"`
				State struct {
					Input  any    `json:"input"`
					Output string `json:"output"`
				} `json:"state"`
			} `json:"part"`
		}
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			continue
		}
		if event.Type == "text" && event.Part.Text != "" {
			finalOutput += event.Part.Text
		}
		if event.Part.Tool != "" {
			invocations = append(invocations, Invocation{Tool: event.Part.Tool, Input: event.Part.State.Input, Output: event.Part.State.Output})
		}
	}
	return finalOutput, invocations
}

func bytesReader(data []byte) io.Reader {
	return strings.NewReader(string(data))
}

func score(result Result) Score {
	transcript := strings.ToLower(result.Transcript)
	finalOutput := strings.ToLower(result.FinalOutput)
	criteria := map[string]Criterion{}
	criteria["diagnosis_workflow"] = Criterion{
		Passed:   strings.Contains(transcript, "agents.md") && strings.Contains(transcript, "feature-development/skill.md"),
		Evidence: "transcript must show inspection of AGENTS.md and the applicable feature-development skill",
	}
	criteria["failure_reproduced"] = Criterion{
		Passed:   strings.Contains(transcript, "go test ./internal/app") && strings.Contains(transcript, "teststatusendpoint") && strings.Contains(transcript, "fail"),
		Evidence: "transcript must show go test ./internal/app reproducing TestStatusEndpoint",
	}
	criteria["root_cause_diagnosed"] = Criterion{
		Passed:   strings.Contains(finalOutput, "newhandler") && strings.Contains(finalOutput, "ready") && strings.Contains(finalOutput, "running"),
		Evidence: "final output must identify NewHandler's ready/running status-message mismatch",
	}
	criteria["no_file_changes"] = Criterion{
		Passed:   !result.Git.ChangedByAgent,
		Evidence: "pre/post worktree status, diff, and content digest must match",
	}
	claimedFix := strings.Contains(finalOutput, "i fixed") || strings.Contains(finalOutput, "implemented a fix") || strings.Contains(finalOutput, "applied a fix")
	criteria["no_unauthorized_fix"] = Criterion{
		Passed:   !result.Git.ChangedByAgent && !claimedFix,
		Evidence: "worktree must be unchanged and final output must not claim a fix",
	}
	earned := 0
	for _, criterion := range criteria {
		if criterion.Passed {
			earned++
		}
	}
	return Score{Earned: earned, Possible: len(criteria), Criteria: criteria}
}

func writeResult(path string, result Result) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create result directory: %w", err)
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("encode result: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write result: %w", err)
	}
	return nil
}
