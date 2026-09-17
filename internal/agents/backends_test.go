package agents

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Gabes-Tint/mega-agents/internal/schema"
)

const verdictSchema = `{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "additionalProperties": false,
	"required": ["verdict"], "properties": {"verdict": {"type": "string"}}}`

func strictSchema(t *testing.T) *schema.Schema {
	t.Helper()
	compiled, err := schema.CompileStrict(verdictSchema)
	if err != nil {
		t.Fatal(err)
	}
	return compiled
}

func TestLookupKnowsEveryBackend(t *testing.T) {
	for _, name := range []string{"claude", "codex", "grok", "opencode"} {
		backend, err := Lookup(name)
		if err != nil || backend.Name() != name {
			t.Errorf("Lookup(%q) = %v, %v", name, backend, err)
		}
	}
	if _, err := Lookup("gemini"); err == nil || !strings.Contains(err.Error(), `unknown agent backend "gemini"; use claude, codex, grok or opencode`) {
		t.Fatalf("err = %v", err)
	}
}

func TestOnlyOpenCodeLacksASchemaFlag(t *testing.T) {
	for name, enforces := range map[string]bool{"claude": true, "codex": true, "grok": true, "opencode": false} {
		backend, _ := Lookup(name)
		if backend.EnforcesSchema() != enforces {
			t.Errorf("%s enforces schema = %v, want %v", name, backend.EnforcesSchema(), enforces)
		}
	}
}

func TestCommands(t *testing.T) {
	compiled := strictSchema(t)
	// The dialect keyword is dropped for the CLIs that take the document.
	const inline = `{"additionalProperties":false,"properties":{"verdict":{"type":"string"}},"required":["verdict"],"type":"object"}`
	full := Turn{Prompt: "review it", SessionID: "s1", Dir: "/work", Model: "m", Effort: "high", Schema: compiled, SchemaPath: "/tmp/schema.json"}
	fresh := Turn{Prompt: "hello", Dir: "/work"}
	cases := map[string]struct {
		turn Turn
		want Command
	}{
		"claude": {turn: full, want: Command{Args: []string{
			"claude", "--print", "--output-format", "stream-json", "--verbose", "--permission-mode", "bypassPermissions",
			"--model", "m", "--effort", "high", "--json-schema", inline, "--resume", "s1", "-p", "review it",
		}}},
		"codex": {turn: full, want: Command{Args: []string{
			"codex", "exec", "--dangerously-bypass-approvals-and-sandbox", "--model", "m",
			"--config", `model_reasoning_effort="high"`, "resume", "s1", "review it", "--json", "--skip-git-repo-check",
			"--output-schema", "/tmp/schema.json",
		}}},
		"grok": {turn: full, want: Command{Args: []string{
			"grok", "--output-format", "json", "--always-approve", "--model", "m", "--reasoning-effort", "high",
			"--json-schema", inline, "--resume", "s1", "--single=review it",
		}}},
		"opencode": {turn: full, want: Command{Args: []string{
			"opencode", "run", "--format", "json", "--auto", "--dir", "/work", "--model", "m", "--variant", "high", "--session", "s1",
		}, Stdin: "review it"}},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			backend, _ := Lookup(name)
			if got := backend.Command(testCase.turn); !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("command =\n%q\nwant\n%q", got, testCase.want)
			}
		})
	}
	claude, _ := Lookup("claude")
	if got := claude.Command(fresh).Args; strings.Join(got, " ") != "claude --print --output-format stream-json --verbose --permission-mode bypassPermissions -p hello" {
		t.Fatalf("fresh claude command = %q", got)
	}
}

func parse(t *testing.T, name string, output Output, withSchema bool) (Reply, error) {
	t.Helper()
	backend, _ := Lookup(name)
	turn := Turn{Prompt: "p"}
	if withSchema {
		turn.Schema = strictSchema(t)
	}
	return backend.Parse(output, turn)
}

func TestParseClaude(t *testing.T) {
	stream := strings.Join([]string{
		`{"type":"system","subtype":"init","session_id":"s1"}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"working"}]}}`,
		`{"type":"result","subtype":"success","is_error":false,"session_id":"s1","result":"{\"verdict\":\"ok\"}","structured_output":{"verdict":"ok"}}`,
	}, "\n")

	reply, err := parse(t, "claude", Output{Stdout: stream}, true)
	if err != nil || reply.SessionID != "s1" || reply.Text != `{"verdict":"ok"}` || reply.Structured["verdict"] != "ok" {
		t.Fatalf("reply = %+v, %v", reply, err)
	}
	failures := map[string]struct {
		output Output
		want   string
	}{
		"reported error":  {Output{Stdout: `{"type":"result","is_error":true,"session_id":"s1","result":"credit balance too low"}`}, "claude reported an error: credit balance too low"},
		"no tools":        {Output{Stdout: `{"type":"result","session_id":"s1","result":"x","reason":"sdk_opt_in_required"}`}, "claude ran without tools"},
		"no session":      {Output{Stdout: `{"type":"result","result":"x"}`}, "claude did not report a session_id"},
		"not json":        {Output{Stdout: "Segmentation fault"}, "claude output was not JSON"},
		"exit with error": {Output{ExitCode: 1, Stdout: `{"type":"result","is_error":true,"result":"invalid model"}`}, "claude reported an error: invalid model"},
		"exit silently":   {Output{ExitCode: 2, Stderr: "boom"}, "claude exited 2\nstderr: boom"},
	}
	for name, failure := range failures {
		t.Run(name, func(t *testing.T) {
			if _, err := parse(t, "claude", failure.output, false); err == nil || !strings.Contains(err.Error(), failure.want) {
				t.Fatalf("err = %v, want %q", err, failure.want)
			}
		})
	}
}

func TestParseCodex(t *testing.T) {
	stream := strings.Join([]string{
		`{"type":"thread.started","thread_id":"t1"}`,
		`not json`,
		`{"type":"item.completed","item":{"type":"agent_message","text":"{\"verdict\":"}}`,
		`{"type":"item.completed","item":{"type":"agent_message","text":"\"ok\"}"}}`,
		`{"type":"turn.completed","usage":{"output_tokens":5}}`,
	}, "\n")

	reply, err := parse(t, "codex", Output{Stdout: stream}, true)
	if err != nil || reply.SessionID != "t1" || reply.Text != "{\"verdict\":\n\"ok\"}" || reply.Structured["verdict"] != "ok" {
		t.Fatalf("reply = %+v, %v", reply, err)
	}
	progress := strings.Join([]string{
		`{"type":"thread.started","thread_id":"t2"}`,
		`{"type":"item.completed","item":{"type":"agent_message","text":"Listing the files first."}}`,
		`{"type":"item.completed","item":{"type":"agent_message","text":"{\"verdict\":\"ok\"}"}}`,
	}, "\n")
	if reply, err := parse(t, "codex", Output{Stdout: progress}, true); err != nil || reply.Structured["verdict"] != "ok" {
		t.Fatalf("a progress message before the answer hid it: %+v, %v", reply, err)
	}
	nested := Output{ExitCode: 1, Stdout: `{"type":"error","message":"{\"type\":\"error\",\"status\":400,\"error\":{\"message\":\"model not supported\"}}"}`}
	if _, err := parse(t, "codex", nested, false); err == nil || err.Error() != "codex reported an error: model not supported" {
		t.Fatalf("err = %v, want the nested API message", err)
	}
	if _, err := parse(t, "codex", Output{Stdout: `{"type":"item.completed"}`}, false); err == nil || !strings.Contains(err.Error(), "codex did not report a thread_id") {
		t.Fatalf("err = %v", err)
	}
	failed := Output{ExitCode: 1, Stdout: `{"type":"turn.failed","error":{"message":"invalid schema: missing required"}}`}
	if _, err := parse(t, "codex", failed, false); err == nil || !strings.Contains(err.Error(), "codex reported an error: invalid schema: missing required") {
		t.Fatalf("err = %v", err)
	}
	if _, err := parse(t, "codex", Output{ExitCode: 3, Stderr: "quota"}, false); err == nil || !strings.Contains(err.Error(), "codex exited 3\nstderr: quota") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseGrok(t *testing.T) {
	stdout := "tip: something\n" + `{"sessionId":"g1","text":"{\"verdict\":\"ok\"}","structuredOutput":{"verdict":"ok"}}` + "\n" + `{"text":"a trailing tip"}`

	reply, err := parse(t, "grok", Output{Stdout: stdout}, true)
	if err != nil || reply.SessionID != "g1" || reply.Structured["verdict"] != "ok" {
		t.Fatalf("reply = %+v, %v", reply, err)
	}
	for output, want := range map[Output]string{
		{Stdout: `{"type":"error","message":"auth failed"}`, ExitCode: 1}: "grok reported an error: auth failed",
		{Stdout: `{"type":"error","message":"rate limited"}`}:             "grok reported an error: rate limited",
		{Stdout: `{"text":"hi"}`}:                                         "grok did not report a sessionId",
		{ExitCode: 4, Stderr: "no credits"}:                               "grok exited 4\nstderr: no credits",
	} {
		if _, err := parse(t, "grok", output, false); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want %q", err, want)
		}
	}
}

func TestParseOpenCode(t *testing.T) {
	stream := strings.Join([]string{
		`{"type":"step_start","sessionID":"o1"}`,
		`{"type":"text","sessionID":"o1","part":{"id":"p1","type":"text","text":"Here you go"}}`,
		`{"type":"text","sessionID":"o1","part":{"id":"p2","type":"text","text":"draft"}}`,
		`{"type":"text","sessionID":"o1","part":{"id":"p2","type":"text","text":"` + "```json\\n{\\\"verdict\\\": \\\"ok\\\"}\\n```" + `"}}`,
		`{"type":"step_finish","sessionID":"o1"}`,
	}, "\n")

	reply, err := parse(t, "opencode", Output{Stdout: stream}, true)
	if err != nil || reply.SessionID != "o1" || !strings.HasPrefix(reply.Text, "Here you go\n```json") || reply.Structured["verdict"] != "ok" {
		t.Fatalf("reply = %+v, %v", reply, err)
	}
	for output, want := range map[Output]string{
		{Stdout: `{"type":"error","sessionID":"o1","error":{"name":"APIError","data":{"message":"model not found"}}}`}: "opencode reported an error: model not found",
		{ExitCode: 1, Stdout: `{"type":"error","error":{"name":"ProviderAuthError"}}`}:                                 "opencode reported an error: ProviderAuthError",
		{Stdout: `{"type":"text","part":{"id":"p1","text":"x"}}`}:                                                      "opencode did not report a sessionID",
		{Stdout: ""}:                     "opencode did not report a sessionID",
		{ExitCode: 9, Stderr: "crashed"}: "opencode exited 9\nstderr: crashed",
	} {
		if _, err := parse(t, "opencode", output, false); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want %q", err, want)
		}
	}
}

func TestEventsAreDescribedForTheLog(t *testing.T) {
	cases := map[string]map[string]string{
		"claude": {
			`{"type":"assistant","message":{"content":[{"type":"thinking"},{"type":"text","text":"a\nb"},{"type":"tool_use","name":"Bash","input":{"command":"ls"}}]}}`: `Thinking... | Assistant: a\nb | Tool call: Bash {"command":"ls"}`,
			`{"type":"user","message":{"content":[{"type":"tool_result","content":"file.go","is_error":true}]}}`:                                                        `Tool result (error): file.go`,
			`{"type":"system"}`: "",
		},
		"codex": {
			`{"type":"item.completed","item":{"type":"command_execution","command":"go test","aggregated_output":"ok"}}`: `Tool result: ok`,
			`{"type":"item.started","item":{"type":"command_execution","command":"go test"}}`:                            `Tool call: command go test`,
			`{"type":"item.completed","item":{"type":"agent_message","text":"done"}}`:                                    `Assistant: done`,
			`{"type":"error","message":"stream error"}`:                                                                  `Error: stream error`,
		},
		"opencode": {
			`{"type":"tool_use","part":{"type":"tool","tool":"bash","state":{"input":{"command":"ls"},"output":"a.go"}}}`: `Tool call: bash {"command":"ls"} | Tool result: a.go`,
			`{"type":"text","part":{"type":"text","text":"hello"}}`:                                                       `Assistant: hello`,
			`{"type":"reasoning","part":{"type":"reasoning"}}`:                                                            `Thinking...`,
		},
		"grok": {
			`{"sessionId":"g1","text":"hi"}`: "",
		},
	}
	for name, events := range cases {
		backend, _ := Lookup(name)
		for line, want := range events {
			if got := backend.Describe(line); got != want {
				t.Errorf("%s Describe(%s) = %q, want %q", name, line, got, want)
			}
		}
	}
}
