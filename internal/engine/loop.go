package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
)

type previousKey struct{}

// Previous returns what a body task put out in its loop's previous
// iteration, or nil in the first; a fixer continues its own conversation
// with it.
func Previous(ctx context.Context, taskID string) map[string]any {
	previous, _ := ctx.Value(previousKey{}).(map[string]map[string]any)
	return previous[taskID]
}

// validateLoop checks a loop's body against the plan it sits in: at least
// one iteration, body ids of its own, needs on the body or on what the loop
// needs, an exit inside the body, and no cycle.
func validateLoop(task Task, index map[string]int) error {
	loop := task.Loop
	if loop == nil {
		return nil
	}
	if loop.MaxIterations < 1 {
		return fmt.Errorf("%s must run at least one iteration", labelOf(task))
	}
	given := map[string]map[string]any{}
	for _, need := range task.Needs {
		given[need.TaskID] = map[string]any{}
	}
	inBody := false
	for _, body := range loop.Body {
		if _, taken := index[body.ID]; taken {
			return fmt.Errorf("%s: body task %q is also a task of the plan", labelOf(task), body.ID)
		}
		inBody = inBody || body.ID == loop.Until.TaskID
	}
	if len(loop.Body) > 0 && !inBody {
		return fmt.Errorf("%s ends on %q, which is not in its body", labelOf(task), loop.Until.TaskID)
	}
	if _, err := schedule(loop.Body, given); err != nil {
		return fmt.Errorf("%s: %w", labelOf(task), err)
	}
	return nil
}

// loopRunner runs one loop task's iterations on a goroutine of its own. It
// never touches the run: body step changes go to the execution's progress.
type loopRunner struct {
	e      *execution
	i      int
	task   Task
	inputs []Input
	log    io.Writer
	given  map[string]map[string]any
	logs   map[string]io.Writer
}

// loopRunner prepares a loop on the execution's goroutine: the outputs it
// gives its body, and its body's logs.
func (e *execution) loopRunner(i int, inputs []Input, log io.Writer) *loopRunner {
	task := e.tasks[i]
	runner := &loopRunner{
		e: e, i: i, task: task, inputs: inputs, log: log,
		given: map[string]map[string]any{}, logs: map[string]io.Writer{},
	}
	maps.Copy(runner.given, e.options.Given)
	for _, need := range task.Needs {
		if outputs, ok := e.outputs[need.TaskID]; ok {
			runner.given[need.TaskID] = maps.Clone(outputs)
		} else if runner.given[need.TaskID] == nil {
			runner.given[need.TaskID] = map[string]any{}
		}
	}
	for _, body := range task.Loop.Body {
		runner.logs[body.ID] = io.Discard
		if e.options.Log != nil {
			if writer := e.options.Log(body.ID); writer != nil {
				runner.logs[body.ID] = writer
			}
		}
	}
	return runner
}

func (runner *loopRunner) run() (Result, error) {
	loop, e := runner.task.Loop, runner.e
	var previous map[string]map[string]any
	outcome := LoopOutcome{}
	for outcome.Iterations < loop.MaxIterations {
		if e.ctx.Err() != nil {
			return runner.stopped(outcome, errors.New("the run was cancelled"))
		}
		outcome.Iterations++
		iteration := outcome.Iterations
		header := fmt.Sprintf("🔁 Iteration %d of %d\n", iteration, loop.MaxIterations)
		fmt.Fprint(runner.log, header)
		for _, writer := range runner.logs {
			fmt.Fprint(writer, header)
		}
		nested, err := Execute(context.WithValue(e.ctx, previousKey{}, previous), loop.Body, Options{
			Given:       runner.given,
			Now:         e.options.Now,
			Parallelism: e.options.Parallelism,
			CancelGrace: e.options.CancelGrace,
			Loop:        runner.task.ID,
			Iteration:   iteration,
			// A breakpoint inside a loop pauses on every repeat: each one runs
			// the body again, and the block carries its breakpoint with it.
			Pause: e.options.Pause,
			Log:   func(taskID string) io.Writer { return runner.logs[taskID] },
			Observe: func(current Run) {
				e.report(func() { runner.show(current, iteration) })
			},
		})
		if err != nil {
			return runner.stopped(outcome, err)
		}
		previous = map[string]map[string]any{}
		for _, step := range nested.Steps {
			if step.Outputs != nil {
				previous[step.TaskID] = step.Outputs
			}
			if step.Status == Failed {
				return runner.stopped(outcome, fmt.Errorf("%s failed in iteration %d: %s", labelOf(Task{ID: step.TaskID, Name: step.Name}), iteration, step.Error))
			}
			if step.TaskID == loop.Until.TaskID {
				outcome.Outputs = step.Outputs
			}
		}
		if _, ended := outcome.Outputs[loop.Until.Port]; ended {
			outcome.Ended = true
			fmt.Fprintf(runner.log, "%s %s ended after %d iterations\n", Succeeded.Emoji(), labelOf(runner.task), iteration)
			break
		}
	}
	if !outcome.Ended {
		fmt.Fprintf(runner.log, "⚠️ %s did not end after %d iterations\n", labelOf(runner.task), outcome.Iterations)
	}
	result, err := loop.Finish(runner.inputs, outcome)
	return withIterations(result, outcome), err
}

func (runner *loopRunner) stopped(outcome LoopOutcome, err error) (Result, error) {
	return withIterations(Result{}, outcome), err
}

func withIterations(result Result, outcome LoopOutcome) Result {
	details := maps.Clone(result.Details)
	if details == nil {
		details = map[string]any{}
	}
	details["iterations"] = outcome.Iterations
	result.Details = details
	return result
}

// show copies an iteration's body steps into the run; it runs on the
// execution's goroutine.
func (runner *loopRunner) show(current Run, iteration int) {
	e := runner.e
	first := e.stepOf[runner.i] + 1
	for offset, step := range current.Steps {
		step.Loop, step.Iteration = runner.task.ID, iteration
		e.run.Steps[first+offset] = step
	}
	// A body block pauses inside the loop's own execution, which already says
	// which loop and repeat it stopped in; the run shows those breakpoints
	// with its own, so they can be resumed like any other.
	e.run.Paused = slices.DeleteFunc(e.run.Paused, func(at Pause) bool { return at.Loop == runner.task.ID })
	e.run.Paused = append(e.run.Paused, current.Paused...)
	e.notify()
}
