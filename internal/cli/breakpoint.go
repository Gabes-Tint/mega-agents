package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/Gabes-Tint/mega-agents/internal/app"
	"github.com/Gabes-Tint/mega-agents/internal/engine"
)

// breakpointBlocks names the blocks of a workflow that carry a breakpoint,
// in the order the file declares them.
func breakpointBlocks(request app.WorkflowRequest) []string {
	var blocks []string
	for _, node := range request.Nodes {
		if node.Breakpoint {
			name := node.Name
			if name == "" {
				name = node.ID
			}
			blocks = append(blocks, name)
		}
	}
	return blocks
}

// answerBreakpoints returns what answers the breakpoints of a run started
// from the command line. At a terminal it asks; anywhere else - CI, a piped
// run - it warns which blocks it is running straight past, because a run
// nobody can answer must never hang silently.
func answerBreakpoints(request app.WorkflowRequest, env Env) func(context.Context, string, engine.Pause) engine.Resume {
	blocks := breakpointBlocks(request)
	if len(blocks) == 0 {
		return nil
	}
	if !env.Interactive || env.Stdin == nil {
		fmt.Fprintf(env.Stderr,
			"⏸️ Ignoring the breakpoint on %s: this run has no terminal to answer it. Run it from the editor to stop there.\n",
			joinNames(blocks),
		)
		return nil
	}
	prompt := &terminalBreakpoints{env: env, reader: bufio.NewReader(env.Stdin)}
	return prompt.answer
}

// joinNames spells a list as "a, b and c".
func joinNames(names []string) string {
	if len(names) == 1 {
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// terminalBreakpoints asks at the terminal what a paused run does next. One
// question is asked at a time, even when two blocks pause together.
type terminalBreakpoints struct {
	env    Env
	mu     sync.Mutex
	reader *bufio.Reader
}

func (prompt *terminalBreakpoints) answer(_ context.Context, _ string, at engine.Pause) engine.Resume {
	prompt.mu.Lock()
	defer prompt.mu.Unlock()
	out := prompt.env.Stdout
	fmt.Fprintf(out, "\n%s %s is about to run\n", engine.Paused.Emoji(), labelOf(at))
	showResolved(out, at)
	for {
		fmt.Fprint(out, "[c]ontinue, [s]tep, s[k]ip or [i]nspect? ")
		line, err := prompt.reader.ReadString('\n')
		if err != nil && strings.TrimSpace(line) == "" {
			// Nothing is left to answer with, so the run goes on rather than
			// waiting at a terminal that has gone.
			fmt.Fprintln(out, "continue")
			return engine.Resume{Action: engine.ResumeContinue}
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "c", "continue", "":
			return engine.Resume{Action: engine.ResumeContinue}
		case "s", "step":
			return engine.Resume{Action: engine.ResumeStep}
		case "k", "skip":
			return engine.Resume{Action: engine.ResumeSkip}
		case "i", "inspect":
			showInputs(out, at)
		default:
			fmt.Fprintf(out, "%q is not one of continue, step, skip or inspect.\n", strings.TrimSpace(line))
		}
	}
}

// labelOf names a paused block, with the repeat of the loop it is in.
func labelOf(at engine.Pause) string {
	if at.Loop != "" && at.Iteration > 0 {
		return fmt.Sprintf("%s (repeat %d)", at.Name, at.Iteration)
	}
	return at.Name
}

func showResolved(out io.Writer, at engine.Pause) {
	if at.Resolved.Prompt != "" {
		fmt.Fprintf(out, "Prompt:\n%s\n", at.Resolved.Prompt)
	}
	if at.Resolved.Command != "" {
		fmt.Fprintf(out, "Command:\n%s\n", at.Resolved.Command)
	}
}

func showInputs(out io.Writer, at engine.Pause) {
	if len(at.Inputs) == 0 {
		fmt.Fprintln(out, "Nothing arrives at this block.")
		return
	}
	for _, input := range at.Inputs {
		value, _ := json.Marshal(input.Value)
		fmt.Fprintf(out, "%s.%s: %s\n", input.TaskID, input.Port, value)
	}
}
