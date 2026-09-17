package agents

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Gabes-Tint/mega-agents/internal/gitops"
)

// Runner executes agent CLI processes.
type Runner struct{}

// Run executes one turn and parses its reply. The log receives the command
// (with the prompt replaced by its size), a description of each event the
// CLI prints as it prints it, its stderr, and its exit status. The context
// bounds the turn: when it ends, the CLI and its children are killed.
func (Runner) Run(ctx context.Context, backend Backend, turn Turn, log io.Writer) (Reply, error) {
	if turn.Schema != nil {
		path, cleanup, err := writeSchema(turn)
		if err != nil {
			return Reply{}, err
		}
		defer cleanup()
		turn.SchemaPath = path
	}
	command := backend.Command(turn)
	executable, err := exec.LookPath(command.Args[0])
	if err != nil {
		return Reply{}, fmt.Errorf("%s is not installed or not on PATH: %w", command.Args[0], err)
	}
	fmt.Fprintf(log, "$ %s\n", describeCommand(command, turn.Prompt))
	process := exec.Command(executable, command.Args[1:]...)
	process.Dir = turn.Dir
	process.Env = gitops.CleanEnvironment(os.Environ())
	process.Stdin = strings.NewReader(command.Stdin)
	// Its own process group, so a stopped turn also stops the tools the
	// agent started.
	process.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var stdout, stderr bytes.Buffer
	var logged sync.Mutex
	stdoutPipe, err := process.StdoutPipe()
	if err != nil {
		return Reply{}, err
	}
	process.Stderr = &stderr
	started := time.Now()
	if err := process.Start(); err != nil {
		return Reply{}, fmt.Errorf("cannot start %s: %w", command.Args[0], err)
	}
	stopped := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = syscall.Kill(-process.Process.Pid, syscall.SIGKILL)
		case <-stopped:
		}
	}()
	scanner := bufio.NewScanner(stdoutPipe)
	scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		stdout.WriteString(line + "\n")
		if description := backend.Describe(line); description != "" {
			logged.Lock()
			fmt.Fprintln(log, description)
			logged.Unlock()
		}
	}
	waitErr := process.Wait()
	close(stopped)
	exitCode := process.ProcessState.ExitCode()
	if text := strings.TrimSpace(stderr.String()); text != "" {
		fmt.Fprintf(log, "stderr: %s\n", tail(text, 4000))
	}
	fmt.Fprintf(log, "%s exited %d after %s\n", command.Args[0], exitCode, time.Since(started).Round(time.Millisecond))
	if ctx.Err() != nil {
		return Reply{}, fmt.Errorf("%s turn stopped after %s: %w", command.Args[0], time.Since(started).Round(time.Second), ctx.Err())
	}
	var exitError *exec.ExitError
	if waitErr != nil && !errors.As(waitErr, &exitError) {
		return Reply{}, fmt.Errorf("%s turn failed: %w", command.Args[0], waitErr)
	}
	return backend.Parse(Output{ExitCode: exitCode, Stdout: stdout.String(), Stderr: stderr.String()}, turn)
}

func writeSchema(turn Turn) (string, func(), error) {
	dir, err := os.MkdirTemp("", "mega-agents-schema-")
	if err != nil {
		return "", nil, fmt.Errorf("cannot write the reply schema: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	path := filepath.Join(dir, "schema.json")
	if err := os.WriteFile(path, []byte(schemaArgument(turn)), 0o600); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("cannot write the reply schema: %w", err)
	}
	return path, cleanup, nil
}

// describeCommand renders the command for a log with the prompt, the one
// unbounded argument, replaced by its size.
func describeCommand(command Command, prompt string) string {
	placeholder := fmt.Sprintf("<prompt: %d chars>", len(prompt))
	rendered := make([]string, len(command.Args))
	for i, arg := range command.Args {
		switch {
		case prompt != "" && arg == prompt:
			rendered[i] = placeholder
		case prompt != "" && strings.HasSuffix(arg, "="+prompt):
			rendered[i] = strings.TrimSuffix(arg, prompt) + placeholder
		case strings.HasPrefix(arg, "{") && len(arg) > 80:
			rendered[i] = fmt.Sprintf("<json: %d chars>", len(arg))
		default:
			rendered[i] = arg
		}
	}
	if command.Stdin != "" {
		rendered = append(rendered, "< "+placeholder)
	}
	return strings.Join(rendered, " ")
}
