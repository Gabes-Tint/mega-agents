package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gabes-Tint/mega-agents/internal/engine"
	"github.com/Gabes-Tint/mega-agents/internal/runs"
)

// fakeClaude puts a claude that answers at once first on PATH.
func fakeClaude(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	script := "#!/bin/sh\necho '{\"type\":\"result\",\"session_id\":\"session-1\",\"result\":\"done\"}'\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// breakpointWorkflow is an agent that carries a breakpoint, followed by a
// command that records that it ran.
func breakpointWorkflow(t *testing.T, project string) string {
	t.Helper()
	return fmt.Sprintf(`apiVersion: megaagents.dev/v1alpha1
kind: Workflow
metadata:
  name: gated
nodes:
  api:
    uses: project@v1
    with:
      path: %q
    children:
      coder:
        uses: agent@v1
        name: "Coder"
        start: true
        breakpoint: true
        with:
          backend: "claude"
          prompt: "Write the fix"
      gate:
        uses: command@v1
        name: "Tests"
        needs: [coder]
        with:
          command: "echo ran > ran.txt"
`, project)
}

// runAnswering runs the command line with a terminal that answers the
// questions a breakpoint asks.
func runAnswering(t *testing.T, answers string, args ...string) result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Main(args, Env{
		Stdout: &stdout, Stderr: &stderr, Stdin: strings.NewReader(answers), Interactive: answers != "",
		Serve: func() error { return errors.New("serve was not expected") },
	})
	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func TestARunWithNoTerminalRunsPastBreakpointsWithAWarning(t *testing.T) {
	t.Setenv("MEGA_AGENTS_HOME", t.TempDir())
	fakeClaude(t)
	project := t.TempDir()
	path := writeWorkflow(t, "gated.yaml", breakpointWorkflow(t, project))

	got := run(t, "run", path)

	if got.code != 0 {
		t.Fatalf("code = %d, want a run that carried on\nstdout:\n%s\nstderr:\n%s", got.code, got.stdout, got.stderr)
	}
	if !strings.Contains(got.stderr, "Ignoring the breakpoint on Coder") {
		t.Fatalf("stderr = %q, want a warning naming the block it ran past", got.stderr)
	}
	if _, err := os.Stat(filepath.Join(project, "ran.txt")); err != nil {
		t.Fatalf("the flow did not run to its end: %v", err)
	}
}

func TestARunAtATerminalAsksWhatToDoAtABreakpoint(t *testing.T) {
	t.Setenv("MEGA_AGENTS_HOME", t.TempDir())
	fakeClaude(t)
	project := t.TempDir()
	path := writeWorkflow(t, "gated.yaml", breakpointWorkflow(t, project))

	got := runAnswering(t, "i\nc\n", "run", path)

	if got.code != 0 {
		t.Fatalf("code = %d\nstdout:\n%s\nstderr:\n%s", got.code, got.stdout, got.stderr)
	}
	for _, want := range []string{
		"⏸️ Coder is about to run",
		"Prompt:\nWrite the fix",
		"Nothing arrives at this block.",
		"[c]ontinue, [s]tep, s[k]ip or [i]nspect?",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Fatalf("stdout does not contain %q:\n%s", want, got.stdout)
		}
	}
	if strings.Contains(got.stderr, "Ignoring the breakpoint") {
		t.Fatalf("stderr = %q, want a run that stopped instead of running past it", got.stderr)
	}
}

func TestSteppingAtATerminalStopsBeforeTheNextBlock(t *testing.T) {
	t.Setenv("MEGA_AGENTS_HOME", t.TempDir())
	fakeClaude(t)
	project := t.TempDir()
	path := writeWorkflow(t, "gated.yaml", breakpointWorkflow(t, project))

	got := runAnswering(t, "s\nc\n", "run", path)

	if got.code != 0 {
		t.Fatalf("code = %d\nstdout:\n%s\nstderr:\n%s", got.code, got.stdout, got.stderr)
	}
	if !strings.Contains(got.stdout, "⏸️ Tests is about to run") {
		t.Fatalf("stepping did not stop before the block after it:\n%s", got.stdout)
	}
	if !strings.Contains(got.stdout, "Command:\necho ran > ran.txt") {
		t.Fatalf("stdout does not show the command it is about to run:\n%s", got.stdout)
	}
	if _, err := os.Stat(filepath.Join(project, "ran.txt")); err != nil {
		t.Fatalf("the flow did not run to its end: %v", err)
	}
}

func TestSkippingAtABreakpointAtATerminalSkipsTheRestOfTheFlow(t *testing.T) {
	t.Setenv("MEGA_AGENTS_HOME", t.TempDir())
	fakeClaude(t)
	project := t.TempDir()
	path := writeWorkflow(t, "gated.yaml", breakpointWorkflow(t, project))

	got := runAnswering(t, "wat\nk\n", "run", path)

	if !strings.Contains(got.stdout, `"wat" is not one of continue, step, skip or inspect.`) {
		t.Fatalf("stdout does not explain the answers it takes:\n%s", got.stdout)
	}
	if !strings.Contains(got.stdout, "⏭️ Coder skipped: skipped at a breakpoint") {
		t.Fatalf("stdout does not report the skipped block:\n%s", got.stdout)
	}
	if _, err := os.Stat(filepath.Join(project, "ran.txt")); err == nil {
		t.Fatal("the block after the skipped one ran")
	}
	if got.code != 0 {
		t.Fatalf("code = %d, want skipping a block not to fail the run", got.code)
	}
}

func TestRunsShowSaysWhereAPausedRunWaits(t *testing.T) {
	home := t.TempDir()
	t.Setenv("MEGA_AGENTS_HOME", home)
	store := &runs.Store{Root: filepath.Join(home, "runs")}
	record, err := store.Create("gated", []engine.Step{{TaskID: "a1", Name: "Coder", Kind: "agent", Status: engine.Paused}})
	if err != nil {
		t.Fatal(err)
	}
	record.Status = runs.Paused
	record.Paused = []engine.Pause{{
		TaskID: "a1", Name: "Coder", Kind: "agent", At: "2026-09-17T23:20:00Z",
		Resolved: engine.Edit{Prompt: "Write the fix"},
	}}
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}

	got := run(t, "runs", "show", record.ID)

	if got.code != 0 {
		t.Fatalf("code = %d: %s", got.code, got.stderr)
	}
	for _, want := range []string{
		"⏸️ paused",
		"⏸️ waiting before Coder since 2026-09-17T23:20:00Z",
		"Prompt:\nWrite the fix",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Fatalf("stdout does not contain %q:\n%s", want, got.stdout)
		}
	}
}
