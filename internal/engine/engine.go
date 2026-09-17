// Package engine executes a planned workflow: tasks run after the tasks they
// need, each receives the outputs it asked for, and a failure or a branch not
// taken skips everything downstream of it. The engine knows nothing about
// block types; planning turns a graph into tasks.
package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"strings"
	"sync"
	"time"
)

type Status string

const (
	Pending   Status = "pending"
	Running   Status = "running"
	Succeeded Status = "succeeded"
	Failed    Status = "failed"
	Skipped   Status = "skipped"
)

// Need names a task that must finish first. An empty Port only orders the
// two tasks; a named port also requires the task to emit that output, which
// the dependent then receives as an input.
type Need struct {
	TaskID string
	Port   string
}

type Input struct {
	TaskID string
	Port   string
	Value  any
}

// Result is what a task produced: outputs by port, for dependents, and
// details, the evidence shown to whoever watches the run.
type Result struct {
	Outputs map[string]any
	Details map[string]any
}

type Task struct {
	ID    string
	Name  string
	Kind  string
	Needs []Need
	// Run does the work, writing what it does to log: the commands it runs
	// and their output, for whoever inspects the step afterwards.
	Run func(ctx context.Context, inputs []Input, log io.Writer) (Result, error)
}

type Step struct {
	TaskID  string         `json:"nodeId"`
	Name    string         `json:"name"`
	Kind    string         `json:"action"`
	Status  Status         `json:"status"`
	Error   string         `json:"error,omitempty"`
	Details map[string]any `json:"details,omitempty"`
	// StartedAt and FinishedAt are RFC 3339 UTC times; empty until reached.
	StartedAt  string `json:"startedAt,omitempty"`
	FinishedAt string `json:"finishedAt,omitempty"`
	// Outputs are what a succeeded step gave its dependents, kept so a retry
	// can reuse the step instead of running it again.
	Outputs map[string]any `json:"outputs,omitempty"`
}

type Run struct {
	Status Status `json:"status"`
	Steps  []Step `json:"steps"`
}

// Observer receives a snapshot of the run each time a step changes status.
type Observer func(Run)

// Options customize one execution. Every field is optional.
type Options struct {
	Observe Observer
	// Log returns the writer that keeps a task's log; nil discards logs.
	Log func(taskID string) io.Writer
	// Now is the clock steps are timed with; nil uses the system clock.
	Now func() time.Time
	// Parallelism bounds how many tasks run at once; 0 means
	// DefaultParallelism and 1 runs them one at a time.
	Parallelism int
}

// DefaultParallelism is how many independent tasks run at once.
const DefaultParallelism = 4

// Execute validates the plan, then runs its tasks one at a time in dependency
// order, keeping declaration order among tasks that are ready together. It
// returns an error only for a plan that cannot run; task failures are
// recorded on their steps.
func Execute(ctx context.Context, tasks []Task, options Options) (Run, error) {
	if _, err := schedule(tasks); err != nil {
		return Run{}, err
	}
	limit := options.Parallelism
	if limit <= 0 {
		limit = DefaultParallelism
	}
	run := Run{Status: Running, Steps: make([]Step, len(tasks))}
	index := make(map[string]int, len(tasks))
	for i, task := range tasks {
		index[task.ID] = i
		run.Steps[i] = Step{TaskID: task.ID, Name: task.Name, Kind: task.Kind, Status: Pending}
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	execution := &execution{
		ctx: ctx, tasks: tasks, options: options, run: &run, index: index, now: now,
		outputs: make(map[string]map[string]any, len(tasks)),
		logs:    make([]io.Writer, len(tasks)),
		settled: make([]bool, len(tasks)), started: make([]bool, len(tasks)),
		finished: make(chan int),
	}
	execution.notify()
	running := 0
	for remaining := len(tasks); remaining > 0; {
		for i := range tasks {
			if execution.started[i] || !execution.ready(i) {
				continue
			}
			if reason := execution.blocked(i); reason != "" {
				execution.skip(i, reason)
				remaining--
				continue
			}
			if running == limit {
				break
			}
			execution.start(i)
			running++
		}
		if remaining == 0 {
			break
		}
		execution.complete(<-execution.finished)
		running--
		remaining--
	}
	run.Status = Succeeded
	for _, step := range run.Steps {
		cancelled := step.Status == Skipped && step.Error == "the run was cancelled"
		if step.Status == Failed || cancelled {
			run.Status = Failed
		}
	}
	return run, nil
}

// execution is one run in progress. Only the goroutine running Execute
// changes the run; tasks report back through finished.
type execution struct {
	ctx      context.Context
	tasks    []Task
	options  Options
	run      *Run
	index    map[string]int
	now      func() time.Time
	outputs  map[string]map[string]any
	logs     []io.Writer
	settled  []bool
	started  []bool
	results  sync.Map
	finished chan int
	begun    []time.Time
}

type taskResult struct {
	result Result
	err    error
}

func (e *execution) notify() {
	if e.options.Observe != nil {
		e.options.Observe(snapshot(*e.run))
	}
}

func (e *execution) stamp() (time.Time, string) {
	at := e.now().UTC()
	return at, at.Format(time.RFC3339)
}

func (e *execution) log(i int) io.Writer {
	if e.logs[i] == nil {
		e.logs[i] = io.Discard
		if e.options.Log != nil {
			if writer := e.options.Log(e.tasks[i].ID); writer != nil {
				e.logs[i] = writer
			}
		}
	}
	return e.logs[i]
}

// ready reports whether every task i needs has settled.
func (e *execution) ready(i int) bool {
	for _, need := range e.tasks[i].Needs {
		if !e.settled[e.index[need.TaskID]] {
			return false
		}
	}
	return true
}

func (e *execution) blocked(i int) string {
	_, reason := gather(e.tasks[i], e.run.Steps, e.index, e.outputs)
	if reason == "" && e.ctx.Err() != nil {
		reason = "the run was cancelled"
	}
	return reason
}

func (e *execution) skip(i int, reason string) {
	step := &e.run.Steps[i]
	e.started[i], e.settled[i] = true, true
	step.Status, step.Error = Skipped, reason
	fmt.Fprintf(e.log(i), "%s skipped: %s\n", labelOf(e.tasks[i]), reason)
	e.notify()
}

func (e *execution) start(i int) {
	task, step := e.tasks[i], &e.run.Steps[i]
	inputs, _ := gather(task, e.run.Steps, e.index, e.outputs)
	started, startedAt := e.stamp()
	if e.begun == nil {
		e.begun = make([]time.Time, len(e.tasks))
	}
	e.begun[i] = started
	e.started[i] = true
	step.Status, step.StartedAt = Running, startedAt
	log := e.log(i)
	fmt.Fprintf(log, "%s started at %s\n", labelOf(task), startedAt)
	e.notify()
	go func() {
		result, err := task.Run(e.ctx, inputs, log)
		e.results.Store(i, taskResult{result: result, err: err})
		e.finished <- i
	}()
}

func (e *execution) complete(i int) {
	value, _ := e.results.LoadAndDelete(i)
	outcome := value.(taskResult)
	task, step := e.tasks[i], &e.run.Steps[i]
	finished, finishedAt := e.stamp()
	e.settled[i] = true
	step.Details, step.FinishedAt = outcome.result.Details, finishedAt
	elapsed := finished.Sub(e.begun[i]).Round(time.Millisecond)
	log := e.log(i)
	if outcome.err != nil {
		step.Status, step.Error = Failed, outcome.err.Error()
		fmt.Fprintf(log, "%s failed in %s: %s\n", labelOf(task), elapsed, outcome.err)
	} else {
		step.Status = Succeeded
		step.Outputs = outcome.result.Outputs
		e.outputs[task.ID] = outcome.result.Outputs
		fmt.Fprintf(log, "%s succeeded in %s\n", labelOf(task), elapsed)
	}
	e.notify()
}

// gather collects a task's inputs, or explains why it cannot run.
func gather(task Task, steps []Step, index map[string]int, outputs map[string]map[string]any) ([]Input, string) {
	var inputs []Input
	for _, need := range task.Needs {
		dependency := steps[index[need.TaskID]]
		name := dependency.Name
		if name == "" {
			name = dependency.TaskID
		}
		switch dependency.Status {
		case Failed:
			return nil, name + " failed"
		case Skipped:
			return nil, name + " was skipped"
		}
		if need.Port == "" {
			continue
		}
		value, ok := outputs[need.TaskID][need.Port]
		if !ok {
			return nil, fmt.Sprintf("%s did not take %q", name, need.Port)
		}
		inputs = append(inputs, Input{TaskID: need.TaskID, Port: need.Port, Value: value})
	}
	return inputs, ""
}

// Validate reports why a plan cannot run: no tasks, duplicate ids, unknown
// dependencies or a cycle. Execute performs the same check.
func Validate(tasks []Task) error {
	_, err := schedule(tasks)
	return err
}

// schedule orders tasks so every need comes first, choosing the earliest
// declared ready task at each point.
func schedule(tasks []Task) ([]int, error) {
	if len(tasks) == 0 {
		return nil, errors.New("nothing to run")
	}
	index := make(map[string]int, len(tasks))
	for i, task := range tasks {
		if _, seen := index[task.ID]; seen {
			return nil, fmt.Errorf("duplicate task %q", task.ID)
		}
		index[task.ID] = i
	}
	for _, task := range tasks {
		for _, need := range task.Needs {
			if _, ok := index[need.TaskID]; !ok {
				return nil, fmt.Errorf("%s needs unknown task %q", task.ID, need.TaskID)
			}
		}
	}
	done := make([]bool, len(tasks))
	order := make([]int, 0, len(tasks))
	for len(order) < len(tasks) {
		progressed := false
		for i, task := range tasks {
			if done[i] || !ready(task, index, done) {
				continue
			}
			done[i], progressed = true, true
			order = append(order, i)
			break
		}
		if !progressed {
			var waiting []string
			for i, task := range tasks {
				if !done[i] {
					waiting = append(waiting, labelOf(task))
				}
			}
			return nil, fmt.Errorf("the flow has a cycle through %s", strings.Join(waiting, ", "))
		}
	}
	return order, nil
}

func ready(task Task, index map[string]int, done []bool) bool {
	for _, need := range task.Needs {
		if !done[index[need.TaskID]] {
			return false
		}
	}
	return true
}

func labelOf(task Task) string {
	if task.Name != "" {
		return task.Name
	}
	return task.ID
}

func snapshot(run Run) Run {
	steps := make([]Step, len(run.Steps))
	for i, step := range run.Steps {
		step.Details = maps.Clone(step.Details)
		step.Outputs = maps.Clone(step.Outputs)
		steps[i] = step
	}
	return Run{Status: run.Status, Steps: steps}
}
