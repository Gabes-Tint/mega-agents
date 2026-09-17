// Package agents runs turns of coding-agent CLIs (Claude Code, Codex, Grok
// and OpenCode) as bounded workers: one prompt in a working directory, a
// session to resume, and a reply that can be held to a JSON Schema.
//
// Each backend is a pure translation between a Turn and that CLI's argv and
// output; Runner executes the process, and Talk adds schema validation with
// bounded repair turns on the same session. The CLI dialects are ported from
// agents-army-2, where each quirk was measured against the real tools.
package agents

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Gabes-Tint/mega-agents/internal/schema"
)

// Turn is one prompt sent to an agent.
type Turn struct {
	Prompt string
	// SessionID resumes that conversation; empty starts a new one.
	SessionID string
	Dir       string
	Model     string
	Effort    string
	Schema    *schema.Schema
	// SchemaPath is Schema written to a file, for CLIs that read one.
	SchemaPath string
	// Fork resumes a copy of SessionID, leaving the original untouched.
	Fork bool
	// MaxCostUSD stops repair turns once the conversation has cost more;
	// zero means no budget.
	MaxCostUSD float64
}

// Command is how to start the CLI: Args[0] is the executable name.
type Command struct {
	Args  []string
	Stdin string
}

// Output is what the CLI process left behind.
type Output struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// Reply is a turn's result. Structured is the reply as a JSON object when a
// schema was asked for and the reply is one; whether it satisfies the schema
// is decided by Talk.
type Reply struct {
	SessionID  string
	Text       string
	Structured map[string]any
	Usage      Usage
}

type Backend interface {
	Name() string
	// EnforcesSchema reports whether the CLI takes the schema itself; when it
	// does not, the schema travels in the prompt and only validation holds
	// the reply to it.
	EnforcesSchema() bool
	Command(turn Turn) Command
	Parse(output Output, turn Turn) (Reply, error)
	// Describe renders one line of the CLI's output for the turn's log, or
	// returns "" for lines not worth showing.
	Describe(line string) string
}

var backends = map[string]Backend{
	"claude":   claude{},
	"codex":    codex{},
	"grok":     grok{},
	"opencode": opencode{},
}

// Names lists the supported backends.
var Names = []string{"claude", "codex", "grok", "opencode"}

func Lookup(name string) (Backend, error) {
	backend, ok := backends[name]
	if !ok {
		return nil, fmt.Errorf("unknown agent backend %q; use claude, codex, grok or opencode", name)
	}
	return backend, nil
}

// jsonObjects scans text for every top-level JSON object in order, because
// CLIs that promise one envelope still sometimes print text before it.
func jsonObjects(text string) []map[string]any {
	var found []map[string]any
	for index := 0; index < len(text); {
		start := strings.IndexByte(text[index:], '{')
		if start < 0 {
			break
		}
		start += index
		decoder := json.NewDecoder(strings.NewReader(text[start:]))
		var object map[string]any
		if err := decoder.Decode(&object); err != nil {
			index = start + 1
			continue
		}
		found = append(found, object)
		index = start + int(decoder.InputOffset())
	}
	return found
}

// jsonLines parses newline-delimited JSON events, ignoring noise lines.
func jsonLines(text string) []map[string]any {
	var events []map[string]any
	for _, line := range strings.Split(text, "\n") {
		if event, ok := jsonLine(line); ok {
			events = append(events, event)
		}
	}
	return events
}

func jsonLine(line string) (map[string]any, bool) {
	var event map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &event); err != nil {
		return nil, false
	}
	return event, true
}

func stringField(object map[string]any, key string) string {
	value, _ := object[key].(string)
	return value
}

func objectField(object map[string]any, key string) map[string]any {
	value, _ := object[key].(map[string]any)
	return value
}

// tail keeps the end of a long stream, where CLIs put their verdict.
func tail(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return "…" + text[len(text)-limit:]
}

// excerpt keeps both ends of a long dump: a parse error points at the
// start, and the envelope usually sits at the end.
func excerpt(text string) string {
	if len(text) <= 2000 {
		return text
	}
	return text[:400] + "\n…\n" + text[len(text)-1600:]
}

// oneLine keeps a value readable on one log line.
func oneLine(value any) string {
	if text, ok := value.(string); ok {
		return strings.NewReplacer("\r", `\r`, "\n", `\n`).Replace(text)
	}
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

// structured is the reply as the object a schema asked for, preferring the
// CLI's own parse where it publishes one.
func structured(turn Turn, text string, preParsed any) map[string]any {
	if turn.Schema == nil {
		return nil
	}
	if object, ok := preParsed.(map[string]any); ok {
		return object
	}
	var object map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(text)), &object); err != nil {
		return nil
	}
	return object
}

func schemaArgument(turn Turn) string {
	var document map[string]any
	if err := json.Unmarshal([]byte(turn.Schema.Text), &document); err != nil {
		return turn.Schema.Text
	}
	// The CLIs' validators only know draft-07 and refuse a document that
	// declares a newer dialect; the keyword constrains nothing, and the
	// schema was already checked against the dialect it named.
	delete(document, "$schema")
	encoded, _ := json.Marshal(document)
	return string(encoded)
}

type claude struct{}

func (claude) Name() string         { return "claude" }
func (claude) EnforcesSchema() bool { return true }

func (claude) Command(turn Turn) Command {
	// Print mode denies tools unless a permission mode is set, and still
	// exits 0 without them; bypassPermissions is the non-interactive opt-in.
	args := []string{"claude", "--print", "--output-format", "stream-json", "--verbose", "--permission-mode", "bypassPermissions"}
	if turn.Model != "" {
		args = append(args, "--model", turn.Model)
	}
	if turn.Effort != "" {
		args = append(args, "--effort", turn.Effort)
	}
	if turn.Schema != nil {
		args = append(args, "--json-schema", schemaArgument(turn))
	}
	if turn.SessionID != "" {
		args = append(args, "--resume", turn.SessionID)
		if turn.Fork {
			args = append(args, "--fork-session")
		}
	}
	return Command{Args: append(args, "-p", turn.Prompt)}
}

func claudeEnvelope(stdout string) (map[string]any, bool) {
	objects := jsonObjects(stdout)
	for i := len(objects) - 1; i >= 0; i-- {
		if stringField(objects[i], "type") == "result" {
			return objects[i], true
		}
	}
	if len(objects) == 0 {
		return nil, false
	}
	return objects[len(objects)-1], true
}

func (claude) Parse(output Output, turn Turn) (Reply, error) {
	envelope, found := claudeEnvelope(output.Stdout)
	if output.ExitCode != 0 {
		if found && envelope["is_error"] == true {
			return Reply{}, fmt.Errorf("claude reported an error: %v", envelope["result"])
		}
		return Reply{}, fmt.Errorf("claude exited %d\nstderr: %s\nstdout: %s", output.ExitCode, tail(output.Stderr, 2000), excerpt(output.Stdout))
	}
	if !found {
		return Reply{}, fmt.Errorf("claude output was not JSON\nstdout: %s", excerpt(output.Stdout))
	}
	if envelope["is_error"] == true {
		return Reply{}, fmt.Errorf("claude reported an error: %v", envelope["result"])
	}
	if stringField(envelope, "reason") == "sdk_opt_in_required" {
		return Reply{}, fmt.Errorf("claude ran without tools: reason=sdk_opt_in_required; --permission-mode bypassPermissions did not take effect")
	}
	session := stringField(envelope, "session_id")
	if session == "" {
		return Reply{}, fmt.Errorf("claude did not report a session_id\nstdout: %s", excerpt(output.Stdout))
	}
	text := stringField(envelope, "result")
	return Reply{SessionID: session, Text: text, Structured: structured(turn, text, envelope["structured_output"]), Usage: claudeUsage(envelope)}, nil
}

func (claude) Describe(line string) string {
	event, ok := jsonLine(line)
	if !ok {
		return ""
	}
	switch stringField(event, "type") {
	case "assistant", "user":
		content, _ := objectField(event, "message")["content"].([]any)
		var parts []string
		for _, entry := range content {
			block, _ := entry.(map[string]any)
			switch stringField(block, "type") {
			case "thinking":
				parts = append(parts, "Thinking...")
			case "text":
				parts = append(parts, "Assistant: "+oneLine(stringField(block, "text")))
			case "tool_use":
				parts = append(parts, fmt.Sprintf("Tool call: %s %s", stringField(block, "name"), oneLine(block["input"])))
			case "tool_result":
				label := "Tool result"
				if block["is_error"] == true {
					label = "Tool result (error)"
				}
				parts = append(parts, label+": "+oneLine(block["content"]))
			}
		}
		return strings.Join(parts, " | ")
	case "result":
		if event["is_error"] == true {
			return "Error: " + oneLine(event["result"])
		}
	}
	return ""
}

type codex struct{}

func (codex) Name() string         { return "codex" }
func (codex) EnforcesSchema() bool { return true }

func (codex) Command(turn Turn) Command {
	args := []string{"codex", "exec", "--dangerously-bypass-approvals-and-sandbox"}
	if turn.Model != "" {
		args = append(args, "--model", turn.Model)
	}
	if turn.Effort != "" {
		args = append(args, "--config", fmt.Sprintf("model_reasoning_effort=%q", turn.Effort))
	}
	if turn.SessionID != "" {
		subcommand := "resume"
		if turn.Fork {
			subcommand = "fork"
		}
		args = append(args, subcommand, turn.SessionID)
	}
	args = append(args, turn.Prompt, "--json", "--skip-git-repo-check")
	if turn.Schema != nil {
		args = append(args, "--output-schema", turn.SchemaPath)
	}
	return Command{Args: args}
}

func (codex) Parse(output Output, turn Turn) (Reply, error) {
	events := jsonLines(output.Stdout)
	if output.ExitCode != 0 {
		// The last error event is the proximate failure; stderr at that
		// point only holds a notice about reading stdin.
		for i := len(events) - 1; i >= 0; i-- {
			message := stringField(events[i], "message")
			if stringField(events[i], "type") == "turn.failed" {
				message = stringField(objectField(events[i], "error"), "message")
			}
			if message != "" && (stringField(events[i], "type") == "error" || stringField(events[i], "type") == "turn.failed") {
				return Reply{}, fmt.Errorf("codex reported an error: %s", apiMessage(message))
			}
		}
		return Reply{}, fmt.Errorf("codex exited %d\nstderr: %s", output.ExitCode, tail(output.Stderr, 2000))
	}
	var session string
	var parts []string
	for _, event := range events {
		switch stringField(event, "type") {
		case "thread.started":
			session = stringField(event, "thread_id")
		case "item.completed":
			item := objectField(event, "item")
			if stringField(item, "type") == "agent_message" && stringField(item, "text") != "" {
				parts = append(parts, stringField(item, "text"))
			}
		}
	}
	if session == "" {
		return Reply{}, fmt.Errorf("codex did not report a thread_id\nstdout: %s\nstderr: %s", tail(output.Stdout, 2000), tail(output.Stderr, 2000))
	}
	text := strings.Join(parts, "\n")
	var answer any
	if len(parts) > 0 {
		// Progress messages come before the answer, which is the last one.
		if last := structured(turn, parts[len(parts)-1], nil); last != nil {
			answer = last
		}
	}
	return Reply{SessionID: session, Text: text, Structured: structured(turn, text, answer), Usage: codexUsage(events)}, nil
}

// apiMessage unwraps an API error Codex relays as a JSON string.
func apiMessage(message string) string {
	var relayed map[string]any
	if err := json.Unmarshal([]byte(message), &relayed); err == nil {
		if inner := stringField(objectField(relayed, "error"), "message"); inner != "" {
			return inner
		}
	}
	return message
}

func (codex) Describe(line string) string {
	event, ok := jsonLine(line)
	if !ok {
		return ""
	}
	kind := stringField(event, "type")
	if kind == "error" || kind == "turn.failed" {
		message := stringField(event, "message")
		if message == "" {
			message = stringField(objectField(event, "error"), "message")
		}
		return "Error: " + oneLine(message)
	}
	item := objectField(event, "item")
	switch stringField(item, "type") {
	case "reasoning":
		return "Thinking..."
	case "agent_message":
		return "Assistant: " + oneLine(stringField(item, "text"))
	case "command_execution", "mcp_tool_call":
		if kind == "item.completed" {
			for _, key := range []string{"aggregated_output", "output", "result", "error"} {
				if value, ok := item[key]; ok {
					return "Tool result: " + oneLine(value)
				}
			}
			return ""
		}
		if stringField(item, "type") == "mcp_tool_call" {
			return fmt.Sprintf("MCP call: %s %s", stringField(item, "tool"), oneLine(item["arguments"]))
		}
		return "Tool call: command " + oneLine(item["command"])
	}
	return ""
}

type grok struct{}

func (grok) Name() string         { return "grok" }
func (grok) EnforcesSchema() bool { return true }

func (grok) Command(turn Turn) Command {
	args := []string{"grok", "--output-format", "json", "--always-approve"}
	if turn.Model != "" {
		args = append(args, "--model", turn.Model)
	}
	if turn.Effort != "" {
		args = append(args, "--reasoning-effort", turn.Effort)
	}
	if turn.Schema != nil {
		args = append(args, "--json-schema", schemaArgument(turn))
	}
	if turn.SessionID != "" {
		args = append(args, "--resume", turn.SessionID)
		if turn.Fork {
			args = append(args, "--fork-session")
		}
	}
	// Glued to its flag so a prompt starting with "-" is never read as one.
	return Command{Args: append(args, "--single="+turn.Prompt)}
}

// grokEnvelope prefers a success or error envelope; a bare text object is a
// weak marker a trailing tip also carries.
func grokEnvelope(stdout string) (map[string]any, bool) {
	objects := jsonObjects(stdout)
	for i := len(objects) - 1; i >= 0; i-- {
		if _, ok := objects[i]["sessionId"]; ok || stringField(objects[i], "type") == "error" {
			return objects[i], true
		}
	}
	for i := len(objects) - 1; i >= 0; i-- {
		if _, ok := objects[i]["text"]; ok {
			return objects[i], true
		}
	}
	return nil, false
}

func (grok) Parse(output Output, turn Turn) (Reply, error) {
	envelope, found := grokEnvelope(output.Stdout)
	if found && stringField(envelope, "type") == "error" {
		return Reply{}, fmt.Errorf("grok reported an error: %s", stringField(envelope, "message"))
	}
	if output.ExitCode != 0 {
		return Reply{}, fmt.Errorf("grok exited %d\nstderr: %s", output.ExitCode, tail(output.Stderr, 2000))
	}
	session := stringField(envelope, "sessionId")
	if session == "" {
		return Reply{}, fmt.Errorf("grok did not report a sessionId\nstdout: %s", excerpt(output.Stdout))
	}
	text := stringField(envelope, "text")
	return Reply{SessionID: session, Text: text, Structured: structured(turn, text, envelope["structuredOutput"])}, nil
}

// Describe shows nothing: grok prints one envelope when the turn ends.
func (grok) Describe(string) string { return "" }

type opencode struct{}

func (opencode) Name() string         { return "opencode" }
func (opencode) EnforcesSchema() bool { return false }

func (opencode) Command(turn Turn) Command {
	args := []string{"opencode", "run", "--format", "json", "--auto", "--dir", turn.Dir}
	if turn.Model != "" {
		args = append(args, "--model", turn.Model)
	}
	if turn.Effort != "" {
		args = append(args, "--variant", turn.Effort)
	}
	if turn.SessionID != "" {
		args = append(args, "--session", turn.SessionID)
		if turn.Fork {
			args = append(args, "--fork")
		}
	}
	// OpenCode joins positional arguments; stdin keeps the prompt verbatim.
	return Command{Args: args, Stdin: turn.Prompt}
}

func opencodeErrorDetail(event map[string]any) string {
	failure := objectField(event, "error")
	if message := stringField(objectField(failure, "data"), "message"); message != "" {
		return message
	}
	return stringField(failure, "name")
}

func (opencode) Parse(output Output, turn Turn) (Reply, error) {
	events := jsonLines(output.Stdout)
	for i := len(events) - 1; i >= 0; i-- {
		if stringField(events[i], "type") == "error" {
			if detail := opencodeErrorDetail(events[i]); detail != "" {
				return Reply{}, fmt.Errorf("opencode reported an error: %s", detail)
			}
			return Reply{}, fmt.Errorf("opencode reported an error event")
		}
	}
	if output.ExitCode != 0 {
		return Reply{}, fmt.Errorf("opencode exited %d\nstderr: %s", output.ExitCode, tail(output.Stderr, 2000))
	}
	var session string
	var order []string
	texts := map[string]string{}
	for _, event := range events {
		session = stringField(event, "sessionID")
		if strings.TrimSpace(session) == "" {
			break
		}
		part := objectField(event, "part")
		id, text := stringField(part, "id"), stringField(part, "text")
		if stringField(event, "type") != "text" || id == "" {
			continue
		}
		if _, seen := texts[id]; !seen {
			order = append(order, id)
		}
		texts[id] = text
	}
	if strings.TrimSpace(session) == "" {
		return Reply{}, fmt.Errorf("opencode did not report a sessionID\nstdout: %s\nstderr: %s", tail(output.Stdout, 2000), tail(output.Stderr, 2000))
	}
	parts := make([]string, len(order))
	for i, id := range order {
		parts[i] = texts[id]
	}
	text := strings.Join(parts, "\n")
	var embedded any
	// The schema only reaches the model as prompt text, and a model asked
	// for JSON still wraps it in a fence or a sentence.
	if object, ok := schema.ExtractObject(text); ok {
		embedded = object
	}
	return Reply{SessionID: session, Text: text, Structured: structured(turn, text, embedded), Usage: opencodeUsage(events)}, nil
}

func (opencode) Describe(line string) string {
	event, ok := jsonLine(line)
	if !ok {
		return ""
	}
	kind := stringField(event, "type")
	part := objectField(event, "part")
	switch {
	case kind == "error":
		return "Error: " + oneLine(opencodeErrorDetail(event))
	case kind == "reasoning" || stringField(part, "type") == "reasoning":
		return "Thinking..."
	case kind == "text":
		return "Assistant: " + oneLine(stringField(part, "text"))
	case kind == "tool_use":
		state := objectField(part, "state")
		var parts []string
		if input, ok := state["input"]; ok {
			parts = append(parts, fmt.Sprintf("Tool call: %s %s", stringField(part, "tool"), oneLine(input)))
		}
		if failure, ok := state["error"]; ok {
			parts = append(parts, "Tool result (error): "+oneLine(failure))
		} else if result, ok := state["output"]; ok {
			parts = append(parts, "Tool result: "+oneLine(result))
		}
		return strings.Join(parts, " | ")
	}
	return ""
}
