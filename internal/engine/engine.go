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
	"slices"
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
	// Loop, when set, makes the task repeat a nested plan instead of Run.
	Loop *Loop
	// WaitForAny runs the task once its needs settle if any of them arrived,
	// with the inputs that did, as where exclusive branches join again.
	WaitForAny bool
}

// Loop repeats Body, a plan of its own, until the body task Until names
// takes its port, at most MaxIterations times. Body tasks may need each other
// and the tasks the loop itself needs, whose outputs every iteration
// receives afresh. Finish turns the outcome into the loop task's result.
type Loop struct {
	Body          []Task
	MaxIterations int
	Until         Need
	Finish        func(inputs []Input, outcome LoopOutcome) (Result, error)
}

// LoopOutcome is how a loop ended: whether the exit port was taken, after
// how many iterations, and the outputs of the Until task in the last one.
type LoopOutcome struct {
	Ended      bool
	Iterations int
	Outputs    map[string]any
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
	// Loop is the loop a body step belongs to, and Iteration the iteration
	// the step shows.
	Loop      string `json:"loop,omitempty"`
	Iteration int    `json:"iteration,omitempty"`
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
	// Given are outputs of tasks outside the plan that its tasks may need,
	// as a loop gives its body.
	Given map[string]map[string]any
}

// DefaultParallelism is how many independent tasks run at once.
const DefaultParallelism = 4

// Execute validates the plan, then runs its tasks one at a time in dependency
// order, keeping declaration order among tasks that are ready together. It
// returns an error only for a plan that cannot run; task failures are
// recorded on their steps.
func Execute(ctx context.Context, tasks []Task, options Options) (Run, error) {
	if _, err := schedule(tasks, options.Given); err != nil {
		return Run{}, err
	}
	limit := options.Parallelism
	if limit <= 0 {
		limit = DefaultParallelism
	}
	run := Run{Status: Running, Steps: PendingSteps(tasks)}
	index := make(map[string]int, len(tasks))
	stepOf := make([]int, len(tasks))
	for i, task := range tasks {
		index[task.ID] = i
		if i > 0 {
			stepOf[i] = stepOf[i-1] + 1
			if previous := tasks[i-1].Loop; previous != nil {
				stepOf[i] += len(previous.Body)
			}
		}
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	execution := &execution{
		ctx: ctx, tasks: tasks, options: options, run: &run, index: index, stepOf: stepOf, now: now,
		outputs: make(map[string]map[string]any, len(tasks)),
		logs:    make([]io.Writer, len(tasks)),
		settled: make([]bool, len(tasks)), started: make([]bool, len(tasks)),
		finished: make(chan int), progress: make(chan func()),
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
		execution.complete(execution.next())
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

// PendingSteps are the steps of a plan before it runs: each task's, followed
// by the steps of a loop's body.
func PendingSteps(tasks []Task) []Step {
	var steps []Step
	for _, task := range tasks {
		steps = append(steps, Step{TaskID: task.ID, Name: task.Name, Kind: task.Kind, Status: Pending})
		if task.Loop != nil {
			for _, body := range task.Loop.Body {
				steps = append(steps, Step{TaskID: body.ID, Name: body.Name, Kind: body.Kind, Status: Pending, Loop: task.ID})
			}
		}
	}
	return steps
}

// execution is one run in progress. Only the goroutine running Execute
// changes the run; tasks report back through finished, and loops send the
// changes to their body's steps through progress.
type execution struct {
	ctx      context.Context
	tasks    []Task
	options  Options
	run      *Run
	index    map[string]int
	stepOf   []int
	now      func() time.Time
	outputs  map[string]map[string]any
	logs     []io.Writer
	settled  []bool
	started  []bool
	results  sync.Map
	finished chan int
	progress chan func()
	begun    []time.Time
}

// next applies loop progress until a task finishes, and returns that task.
func (e *execution) next() int {
	for {
		select {
		case apply := <-e.progress:
			apply()
		case i := <-e.finished:
			return i
		}
	}
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
		if j, inPlan := e.index[need.TaskID]; inPlan && !e.settled[j] {
			return false
		}
	}
	return true
}

func (e *execution) blocked(i int) string {
	_, reason := e.gather(e.tasks[i])
	if reason == "" && e.ctx.Err() != nil {
		reason = "the run was cancelled"
	}
	return reason
}

func (e *execution) skip(i int, reason string) {
	step := &e.run.Steps[e.stepOf[i]]
	e.started[i], e.settled[i] = true, true
	step.Status, step.Error = Skipped, reason
	if loop := e.tasks[i].Loop; loop != nil {
		for offset := range loop.Body {
			body := &e.run.Steps[e.stepOf[i]+1+offset]
			body.Status, body.Error = Skipped, reason
		}
	}
	fmt.Fprintf(e.log(i), "%s %s skipped: %s\n", Skipped.Emoji(), labelOf(e.tasks[i]), reason)
	e.notify()
}

func (e *execution) start(i int) {
	task, step := e.tasks[i], &e.run.Steps[e.stepOf[i]]
	inputs, _ := e.gather(task)
	started, startedAt := e.stamp()
	if e.begun == nil {
		e.begun = make([]time.Time, len(e.tasks))
	}
	e.begun[i] = started
	e.started[i] = true
	step.Status, step.StartedAt = Running, startedAt
	log := e.log(i)
	fmt.Fprintf(log, "%s %s started at %s\n", Running.Emoji(), labelOf(task), startedAt)
	e.notify()
	if task.Loop != nil {
		runner := e.loopRunner(i, inputs, log)
		go func() {
			result, err := runner.run()
			e.results.Store(i, taskResult{result: result, err: err})
			e.finished <- i
		}()
		return
	}
	go func() {
		result, err := task.Run(e.ctx, inputs, log)
		e.results.Store(i, taskResult{result: result, err: err})
		e.finished <- i
	}()
}

func (e *execution) complete(i int) {
	value, _ := e.results.LoadAndDelete(i)
	outcome := value.(taskResult)
	task, step := e.tasks[i], &e.run.Steps[e.stepOf[i]]
	finished, finishedAt := e.stamp()
	e.settled[i] = true
	step.Details, step.FinishedAt = outcome.result.Details, finishedAt
	elapsed := finished.Sub(e.begun[i]).Round(time.Millisecond)
	log := e.log(i)
	if outcome.err != nil {
		step.Status, step.Error = Failed, outcome.err.Error()
		fmt.Fprintf(log, "%s %s failed in %s: %s\n", Failed.Emoji(), labelOf(task), elapsed, outcome.err)
	} else {
		step.Status = Succeeded
		step.Outputs = outcome.result.Outputs
		e.outputs[task.ID] = outcome.result.Outputs
		fmt.Fprintf(log, "%s %s succeeded in %s\n", Succeeded.Emoji(), labelOf(task), elapsed)
	}
	e.notify()
}

// gather collects a task's inputs, or explains why it cannot run.
func (e *execution) gather(task Task) ([]Input, string) {
	var inputs []Input
	var missing []string
	for _, need := range task.Needs {
		given, isGiven := e.options.Given[need.TaskID]
		outputs := given
		name := need.TaskID
		reason := ""
		if !isGiven {
			dependency := e.run.Steps[e.stepOf[e.index[need.TaskID]]]
			outputs = e.outputs[need.TaskID]
			if dependency.Name != "" {
				name = dependency.Name
			}
			switch dependency.Status {
			case Failed:
				reason = name + " failed"
			case Skipped:
				reason = name + " was skipped"
			}
		}
		value, ok := outputs[need.Port]
		if reason == "" && need.Port != "" && !ok {
			reason = fmt.Sprintf("%s did not take %q", name, need.Port)
		}
		switch {
		case reason != "" && !task.WaitForAny:
			return nil, reason
		case reason != "":
			if !slices.Contains(missing, name) {
				missing = append(missing, name)
			}
		case need.Port != "":
			inputs = append(inputs, Input{TaskID: need.TaskID, Port: need.Port, Value: value})
		}
	}
	if task.WaitForAny {
		// An arrow arrives with every output it carries or not at all, so a
		// workspace passed along a branch not taken does not count.
		inputs = slices.DeleteFunc(inputs, func(input Input) bool {
			return slices.Contains(missing, e.nameOf(input.TaskID))
		})
		if len(inputs) == 0 && len(missing) > 0 {
			return nil, "nothing arrived from " + joinNames(missing)
		}
	}
	return inputs, ""
}

// nameOf is how a task is named in explanations.
func (e *execution) nameOf(taskID string) string {
	if _, isGiven := e.options.Given[taskID]; !isGiven {
		if step := e.run.Steps[e.stepOf[e.index[taskID]]]; step.Name != "" {
			return step.Name
		}
	}
	return taskID
}

// joinNames spells a list as "a, b or c".
func joinNames(names []string) string {
	if len(names) == 1 {
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " or " + names[len(names)-1]
}

// Validate reports why a plan cannot run: no tasks, duplicate ids, unknown
// dependencies or a cycle. Execute performs the same check.
func Validate(tasks []Task) error {
	_, err := schedule(tasks, nil)
	return err
}

// schedule orders tasks so every need comes first, choosing the earliest
// declared ready task at each point.
func schedule(tasks []Task, given map[string]map[string]any) ([]int, error) {
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
			if _, ok := index[need.TaskID]; !ok && given[need.TaskID] == nil {
				return nil, fmt.Errorf("%s needs unknown task %q", task.ID, need.TaskID)
			}
		}
		if err := validateLoop(task, index); err != nil {
			return nil, err
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
		if i, ok := index[need.TaskID]; ok && !done[i] {
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
