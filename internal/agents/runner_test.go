package agents

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeCLI installs an executable named name first on PATH whose body is the
// given shell script, and returns the folder it records into.
func fakeCLI(t *testing.T, name string, script string) string {
	t.Helper()
	bin := t.TempDir()
	path := filepath.Join(bin, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return bin
}

func TestRunnerExecutesTheCLIInTheWorkingDirectory(t *testing.T) {
	bin := fakeCLI(t, "claude", `
pwd > "$(dirname "$0")/cwd"
printf '%s\n' "$@" > "$(dirname "$0")/args"
echo '{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"go test"}}]}}'
echo '{"type":"result","session_id":"s1","result":"all green"}'
echo 'a warning' >&2
`)
	dir := t.TempDir()
	var log strings.Builder

	reply, err := Runner{}.Run(context.Background(), claude{}, Turn{Prompt: "run the tests", Dir: dir, Model: "haiku"}, &log)

	if err != nil || reply.SessionID != "s1" || reply.Text != "all green" {
		t.Fatalf("reply = %+v, %v", reply, err)
	}
	if cwd, _ := os.ReadFile(filepath.Join(bin, "cwd")); strings.TrimSpace(string(cwd)) != dir {
		t.Fatalf("cwd = %q, want %s", cwd, dir)
	}
	for _, want := range []string{
		"$ claude --print --output-format stream-json --verbose --permission-mode bypassPermissions --model haiku -p <prompt: 13 chars>\n",
		`Tool call: Bash {"command":"go test"}`,
		"stderr: a warning",
		"exited 0",
	} {
		if !strings.Contains(log.String(), want) {
			t.Errorf("log lacks %q:\n%s", want, log.String())
		}
	}
}

func TestRunnerPassesThePromptOnStdinAndTheSchemaAsAFile(t *testing.T) {
	fakeCLI(t, "opencode", `cat > "$(dirname "$0")/stdin"; echo '{"type":"text","sessionID":"o1","part":{"id":"p1","text":"ok"}}'`)
	codexBin := fakeCLI(t, "codex", `
while [ $# -gt 0 ]; do
  if [ "$1" = "--output-schema" ]; then cp "$2" "$(dirname "$0")/schema"; fi
  shift
done
echo '{"type":"thread.started","thread_id":"t1"}'
echo '{"type":"item.completed","item":{"type":"agent_message","text":"{\"verdict\":\"ok\"}"}}'
`)
	opencodeBin := filepath.Dir(firstOnPath(t, "opencode"))

	if _, err := (Runner{}).Run(context.Background(), opencode{}, Turn{Prompt: "line one\n  'quoted' line", Dir: t.TempDir()}, &strings.Builder{}); err != nil {
		t.Fatalf("opencode: %v", err)
	}
	if stdin, _ := os.ReadFile(filepath.Join(opencodeBin, "stdin")); string(stdin) != "line one\n  'quoted' line" {
		t.Fatalf("stdin = %q", stdin)
	}
	reply, err := Runner{}.Run(context.Background(), codex{}, Turn{Prompt: "p", Dir: t.TempDir(), Schema: strictSchema(t)}, &strings.Builder{})
	if err != nil || reply.Structured["verdict"] != "ok" {
		t.Fatalf("codex reply = %+v, %v", reply, err)
	}
	if written, _ := os.ReadFile(filepath.Join(codexBin, "schema")); !strings.Contains(string(written), `"additionalProperties":false`) {
		t.Fatalf("schema file = %q", written)
	}
}

func firstOnPath(t *testing.T, name string) string {
	t.Helper()
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return filepath.Join(dir, name)
		}
	}
	t.Fatalf("%s not on PATH", name)
	return ""
}

func TestRunnerExplainsAMissingCLI(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, err := Runner{}.Run(context.Background(), grok{}, Turn{Prompt: "p", Dir: t.TempDir()}, &strings.Builder{})

	if err == nil || !strings.Contains(err.Error(), "grok is not installed or not on PATH") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunnerStopsATurnThatOverrunsItsContext(t *testing.T) {
	fakeCLI(t, "claude", "sleep 5\n")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()

	_, err := Runner{}.Run(ctx, claude{}, Turn{Prompt: "p", Dir: t.TempDir()}, &strings.Builder{})

	if err == nil || !strings.Contains(err.Error(), "claude turn stopped") || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
	if time.Since(started) > 3*time.Second {
		t.Fatalf("the turn was not stopped promptly")
	}
}

func TestRunnerReportsTheCLIsOwnFailure(t *testing.T) {
	fakeCLI(t, "grok", `echo '{"type":"error","message":"insufficient credits"}'; exit 1`)
	var log strings.Builder

	_, err := Runner{}.Run(context.Background(), grok{}, Turn{Prompt: "p", Dir: t.TempDir()}, &log)

	if err == nil || err.Error() != "grok reported an error: insufficient credits" {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(log.String(), "❌ grok exited 1") {
		t.Fatalf("log = %s", log.String())
	}
}
