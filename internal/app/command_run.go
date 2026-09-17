package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/Gabes-Tint/mega-agents/internal/engine"
	"github.com/Gabes-Tint/mega-agents/internal/gitops"
)

const (
	passedPort            = "passed"
	failedPort            = "failed"
	defaultCommandTimeout = 10.0
	maxCommandOutput      = 16 * 1024
)

// commandTask plans a Command block: a shell command, such as a test suite
// or a linter, run in the workspace connected to it or in its project's
// folder. Exit status 0 leaves on the passed output; any other leaves on
// failed, carrying the exit code and the output's tail, when an arrow takes
// that branch, and otherwise fails the block. It is the workflow's gate.
func (planner runPlanner) commandTask(node WorkflowNodeInput, included map[string]bool) (engine.Task, error) {
	fail := func(format string, args ...any) (engine.Task, error) {
		return engine.Task{}, fmt.Errorf("%s: %s", node.Name, fmt.Sprintf(format, args...))
	}
	if strings.TrimSpace(node.Command) == "" {
		return fail("write the command to run")
	}
	timeout := defaultCommandTimeout
	if node.TimeoutMinutes != nil {
		timeout = *node.TimeoutMinutes
	}
	if timeout <= 0 || timeout > maxAgentTimeout {
		return fail("the timeout must be more than 0 and at most %g minutes", maxAgentTimeout)
	}
	var needs []engine.Need
	for _, edge := range planner.request.Edges {
		if edge.To != node.ID || !included[edge.From] {
			continue
		}
		port, err := planner.sourcePort(edge)
		if err != nil {
			return engine.Task{}, err
		}
		needs = append(needs, engine.Need{TaskID: edge.From, Port: port})
	}
	handlesFailure := false
	for _, edge := range planner.request.Edges {
		if edge.From != node.ID {
			continue
		}
		port, err := planner.sourcePort(edge)
		if err != nil {
			return engine.Task{}, err
		}
		handlesFailure = handlesFailure || port == failedPort
	}
	limit := time.Duration(timeout * float64(time.Minute))
	return engine.Task{
		ID: node.ID, Name: node.Name, Kind: "command", Needs: needs,
		Run: func(ctx context.Context, inputs []engine.Input, log io.Writer) (engine.Result, error) {
			environment := gitops.CleanEnvironment(os.Environ())
			var dir string
			for _, input := range inputs {
				if workspace, ok := input.Value.(gitops.Workspace); ok {
					dir = workspace.Path
					environment = append(environment,
						"MEGA_AGENTS_WORKSPACE_PATH="+workspace.Path, "MEGA_AGENTS_WORKSPACE_BRANCH="+workspace.Branch,
						"MEGA_AGENTS_WORKSPACE_BASE="+workspace.Base, "MEGA_AGENTS_WORKSPACE_REPOSITORY="+workspace.Repository,
					)
				}
			}
			if dir == "" {
				project := planner.projectOf(node)
				if project.Path == "" {
					return engine.Result{}, fmt.Errorf("set the path on the project that contains %s", node.Name)
				}
				var err error
				if dir, err = projectDir(project); err != nil {
					return engine.Result{}, err
				}
			}
			ctx, cancel := context.WithTimeout(ctx, limit)
			defer cancel()
			exitCode, output, err := runShell(ctx, dir, node.Command, environment, log)
			details := map[string]any{"exitCode": exitCode, "output": output, "dir": dir}
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return engine.Result{Details: details}, fmt.Errorf("%s stopped after its %s timeout", node.Name, limit)
			}
			if err != nil {
				return engine.Result{Details: details}, err
			}
			value := map[string]any{"exitCode": exitCode, "output": output}
			if exitCode == 0 {
				return engine.Result{Outputs: map[string]any{passedPort: value}, Details: details}, nil
			}
			if !handlesFailure {
				return engine.Result{Details: details}, fmt.Errorf("%s exited %d: %s", node.Name, exitCode, lastLines(output, 20))
			}
			return engine.Result{Outputs: map[string]any{failedPort: value}, Details: details}, nil
		},
	}, nil
}

// runShell runs the command with sh in its own process group, streaming
// its combined output to the log and keeping the tail.
func runShell(ctx context.Context, dir string, command string, environment []string, log io.Writer) (int, string, error) {
	fmt.Fprintf(log, "$ %s\n(in %s)\n", command, dir)
	process := exec.Command("sh", "-c", command)
	process.Dir = dir
	process.Env = environment
	process.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	reader, writer := io.Pipe()
	process.Stdout, process.Stderr = writer, writer
	started := time.Now()
	if err := process.Start(); err != nil {
		return -1, "", fmt.Errorf("cannot start the command: %w", err)
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = syscall.Kill(-process.Process.Pid, syscall.SIGKILL)
		case <-done:
		}
	}()
	var output strings.Builder
	collected := make(chan struct{})
	go func() {
		defer close(collected)
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
		for scanner.Scan() {
			fmt.Fprintln(log, scanner.Text())
			output.WriteString(scanner.Text() + "\n")
		}
		_, _ = io.Copy(io.Discard, reader)
	}()
	waitErr := process.Wait()
	close(done)
	writer.Close()
	<-collected
	exitCode := process.ProcessState.ExitCode()
	fmt.Fprintf(log, "exited %d after %s\n", exitCode, time.Since(started).Round(time.Millisecond))
	text := output.String()
	if len(text) > maxCommandOutput {
		text = "…" + text[len(text)-maxCommandOutput:]
	}
	var exitError *exec.ExitError
	if waitErr != nil && !errors.As(waitErr, &exitError) {
		return exitCode, text, fmt.Errorf("the command failed: %w", waitErr)
	}
	return exitCode, text, nil
}

func lastLines(text string, count int) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) > count {
		lines = lines[len(lines)-count:]
	}
	return strings.Join(lines, "\n")
}
