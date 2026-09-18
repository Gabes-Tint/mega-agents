package engine

import (
	"context"
	"fmt"
	"io"
	"slices"
)

// Paused marks the run, and the step it waits before, that stopped at a
// breakpoint. The step has not started: its clock starts when it runs.
const Paused Status = "paused"

// Edit is the text a block runs with: an agent's prompt or a command
// block's command, with the placeholders already filled in. A breakpoint
// shows it as the block is about to run, and an edit made there replaces it
// for that run only; the saved workflow never changes.
type Edit struct {
	Prompt  string `json:"prompt,omitempty"`
	Command string `json:"command,omitempty"`
}

func (edit Edit) empty() bool {
	return edit == Edit{}
}

// Edited is how a step that ran with an edit made at a breakpoint is
// recorded: what ran, and what the workflow holds, so a recorded run never
// misrepresents what was executed.
type Edited struct {
	Ran      Edit `json:"ran"`
	Original Edit `json:"original"`
}

// PausedInput is one value a paused block is about to receive.
type PausedInput struct {
	TaskID string `json:"nodeId"`
	Port   string `json:"port"`
	Value  any    `json:"value,omitempty"`
}

// Pause is a run stopped before a block runs, with everything needed to
// decide what to do with it: which block, what it receives, and the text it
// would run.
type Pause struct {
	TaskID string `json:"nodeId"`
	Name   string `json:"name"`
	Kind   string `json:"action"`
	// At is when the run paused, as an RFC 3339 UTC time.
	At string `json:"at,omitempty"`
	// Loop is the loop a paused body block repeats in, and Iteration the
	// repeat it paused on.
	Loop      string `json:"loop,omitempty"`
	Iteration int    `json:"iteration,omitempty"`
	// Inputs are the resolved values the block is about to receive.
	Inputs []PausedInput `json:"inputs,omitempty"`
	// Resolved is the prompt or command the block would run.
	Resolved Edit `json:"resolved"`
}

// Action is what someone paused at a breakpoint chose to do next.
type Action string

const (
	// ResumeContinue runs the block and then runs on to the next breakpoint
	// or the end of the flow.
	ResumeContinue Action = "continue"
	// ResumeStep runs the block and pauses again before the next one.
	ResumeStep Action = "step"
	// ResumeSkip settles the block as skipped without running it.
	ResumeSkip Action = "skip"
)

// Resume is the answer to a pause: what to do, and the edit to run the
// block with.
type Resume struct {
	Action Action `json:"action"`
	Edit   Edit   `json:"edit,omitempty"`
}

// skippedAtBreakpoint is why a block someone skipped while paused did not
// run; the blocks after it are skipped as they are after any skip.
const skippedAtBreakpoint = "skipped at a breakpoint"

type editKey struct{}

// EditIn returns the edit a block runs with, made at the breakpoint before
// it. Tasks read it instead of their own text when it is there.
func EditIn(ctx context.Context) (Edit, bool) {
	edit, edited := ctx.Value(editKey{}).(Edit)
	return edit, edited && !edit.empty()
}

func editing(ctx context.Context, edit Edit) context.Context {
	if edit.empty() {
		return ctx
	}
	return context.WithValue(ctx, editKey{}, edit)
}

// pausing reports whether the run stops before task i: the block carries a
// breakpoint, or the last resume asked to step. Without a Pause option
// nobody can answer, so breakpoints are ignored rather than hanging the run.
func (e *execution) pausing(i int) bool {
	return e.options.Pause != nil && (e.tasks[i].Breakpoint || e.stepping)
}

// pause records the run as waiting before task i and asks the Pause option
// what to do, on a goroutine of its own: the scheduler keeps running every
// other block, and only this one waits.
func (e *execution) pause(i int, inputs []Input, log io.Writer) {
	task := e.tasks[i]
	e.stepping = false
	at := Pause{
		TaskID: task.ID, Name: task.Name, Kind: task.Kind, Inputs: pausedInputs(inputs),
		Loop: e.options.Loop, Iteration: e.options.Iteration,
	}
	_, at.At = e.stamp()
	if task.Resolve != nil {
		at.Resolved = task.Resolve(inputs)
	}
	e.resolved[i] = at.Resolved
	e.run.Steps[e.stepOf[i]].Status = Paused
	e.run.Paused = append(e.run.Paused, at)
	fmt.Fprintf(log, "%s %s paused at a breakpoint\n", Paused.Emoji(), labelOf(task))
	e.notify()
	go func() {
		resume := e.options.Pause(e.ctx, at)
		e.report(func() { e.unpause(i, inputs, log, resume) })
	}()
}

// unpause takes the run on from a breakpoint; it runs on the scheduler's
// goroutine, as everything that changes the run does.
func (e *execution) unpause(i int, inputs []Input, log io.Writer, resume Resume) {
	id := e.tasks[i].ID
	e.run.Paused = slices.DeleteFunc(e.run.Paused, func(at Pause) bool { return at.TaskID == id })
	switch {
	case e.ctx.Err() != nil:
		// A cancelled run settles the block it waited before as every other
		// step a cancelled run never reached.
		e.finish(i, taskResult{settled: Skipped, reason: "the run was cancelled"})
		return
	case resume.Action == ResumeSkip:
		e.finish(i, taskResult{settled: Skipped, reason: skippedAtBreakpoint})
		return
	case resume.Action == ResumeStep:
		e.stepping = true
	}
	e.begin(i, resume.Edit)
	e.launch(i, inputs, log, resume.Edit)
}

// finish reports a task the scheduler must still account for, such as one
// settled at a breakpoint instead of run.
func (e *execution) finish(i int, outcome taskResult) {
	e.results.Store(i, outcome)
	e.finished <- i
}

func pausedInputs(inputs []Input) []PausedInput {
	var paused []PausedInput
	for _, input := range inputs {
		paused = append(paused, PausedInput(input))
	}
	return paused
}
