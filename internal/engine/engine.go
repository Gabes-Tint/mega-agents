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
	// Breakpoint pauses the run before the task runs, every time it runs,
	// so whoever watches decides what happens next.
	Breakpoint bool
	// Resolve is the text the task is about to run, shown at a breakpoint
	// and kept as the original beside an edit made there.
	Resolve func(inputs []Input) Edit
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
	// Edited is set on a step that ran with an edit made at a breakpoint: it
	// keeps both what ran and what the workflow holds.
	Edited *Edited `json:"edited,omitempty"`
}

type Run struct {
	Status Status `json:"status"`
	Steps  []Step `json:"steps"`
	// Paused are the breakpoints the run waits at, one for each block
	// stopped before it runs; the blocks beside them keep running.
	Paused []Pause `json:"paused,omitempty"`
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
	// CancelGrace is how long a cancelled run waits for the tasks still
	// running to report before it records them as cancelled and ends; 0 uses
	// DefaultCancelGrace. A task that ignores cancellation cannot hold a run
	// open forever.
	CancelGrace time.Duration
	// Loop names the loop whose body this plan is, and Iteration the repeat
	// it runs, so a breakpoint inside a loop says where it stopped. A loop
	// sets them on the plan it repeats.
	Loop      string
	Iteration int
	// Pause answers a breakpoint: it is called before a task with one runs,
	// on that task's own goroutine, and blocks for as long as the run stays
	// paused, which is as long as it takes. Nil ignores breakpoints, so a run
	// nobody watches never waits for an answer that cannot come.
	Pause func(ctx context.Context, at Pause) Resume
}

// DefaultParallelism is how many independent tasks run at once.
const DefaultParallelism = 4

// DefaultCancelGrace is how long a cancelled run waits for its running tasks
// to report what they did before it settles them as cancelled.
const DefaultCancelGrace = 2 * time.Second

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
	grace := options.CancelGrace
	if grace <= 0 {
		grace = DefaultCancelGrace
	}
	execution := &execution{
		ctx: ctx, tasks: tasks, options: options, run: &run, index: index, stepOf: stepOf, now: now,
		outputs: make(map[string]map[string]any, len(tasks)),
		logs:    make([]io.Writer, len(tasks)),
		settled: make([]bool, len(tasks)), started: make([]bool, len(tasks)),
		// A settled run abandons the tasks that never reported, so the channel
		// buffers their results rather than leaving their goroutines stuck.
		finished: make(chan int, len(tasks)), progress: make(chan func()), grace: grace,
		resolved: make(map[int]Edit), done: make(chan struct{}),
	}
	// Nothing a task reports after the run ended can change it, and its
	// goroutine must not block trying.
	defer close(execution.done)
	execution.notify()
	running := 0
	for remaining := len(tasks); remaining > 0; {
		progressed := false
		for i := range tasks {
			if execution.started[i] || !execution.ready(i) {
				continue
			}
			if reason := execution.blocked(i); reason != "" {
				execution.skip(i, reason)
				remaining--
				progressed = true
				continue
			}
			if running == limit {
				break
			}
			execution.start(i)
			running++
			progressed = true
		}
		if remaining == 0 {
			break
		}
		if running == 0 {
			// Nothing is running, so no task can report. A pass that settled
			// something may have readied a task declared before it, which the
			// next pass picks up; every such pass settles at least one task, so
			// looking again cannot spin. A pass that settled nothing means the
			// run can never go on, and the rest fails rather than waiting.
			if progressed {
				continue
			}
			execution.settle(Failed, "nothing left in the flow can run")
			break
		}
		if i := execution.next(); i != noTask {
			execution.complete(i)
			running--
			remaining--
			continue
		}
		execution.settle(Skipped, "the run was cancelled")
		break
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
	grace    time.Duration
	// stepping makes the next task the run starts pause, after a resume that
	// asked to step.
	stepping bool
	// resolved is what each paused task would have run, kept as the original
	// beside an edit made at its breakpoint.
	resolved map[int]Edit
	// done is closed when the run has ended.
	done chan struct{}
}

// report hands a change to the scheduler, which applies it while it waits.
// A run that has already ended drops it rather than holding the goroutine.
func (e *execution) report(apply func()) {
	select {
	case e.progress <- apply:
	case <-e.done:
	}
}

// noTask is what next returns when no task will report: a cancelled run
// whose running tasks did not settle within the grace.
const noTask = -1

// next applies loop progress until a task finishes, and returns that task.
// A cancelled run gives the tasks still running the grace to report what
// they did, and then stops waiting, so a task that ignores cancellation
// cannot hold the run open.
func (e *execution) next() int {
	done := e.ctx.Done()
	var expired <-chan time.Time
	for {
		select {
		case apply := <-e.progress:
			apply()
		case i := <-e.finished:
			return i
		case <-done:
			// Done stays ready once it fires, so it is taken only once.
			done = nil
			timer := time.NewTimer(e.grace)
			defer timer.Stop()
			expired = timer.C
		case <-expired:
			return noTask
		}
	}
}

// settle ends a run that cannot wait any longer: tasks that already reported
// are completed, and every task still unsettled is recorded with the reason.
func (e *execution) settle(status Status, reason string) {
	for {
		select {
		case i := <-e.finished:
			e.complete(i)
			continue
		default:
		}
		break
	}
	// A run that ends waits at no breakpoint, including one it never answered.
	e.run.Paused = nil
	for i := range e.tasks {
		if e.settled[i] {
			continue
		}
		if status == Failed {
			e.fail(i, reason)
			continue
		}
		e.skip(i, reason)
	}
}

type taskResult struct {
	result Result
	err    error
	// settled is the status of a task that never ran, such as one skipped at
	// a breakpoint, with the reason it carries.
	settled Status
	reason  string
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
	e.settleStep(i, Skipped, reason)
}

// fail settles a task that never ran, and everything a loop holds, with the
// reason it could not.
func (e *execution) fail(i int, reason string) {
	e.settleStep(i, Failed, reason)
}

// settleStep ends a task without running it, or without waiting for it any
// longer, and ends the body of a loop with it.
func (e *execution) settleStep(i int, status Status, reason string) {
	step := &e.run.Steps[e.stepOf[i]]
	e.started[i], e.settled[i] = true, true
	step.Status, step.Error = status, reason
	if loop := e.tasks[i].Loop; loop != nil {
		for offset := range loop.Body {
			body := &e.run.Steps[e.stepOf[i]+1+offset]
			if body.Status == Succeeded || body.Status == Failed {
				continue
			}
			body.Status, body.Error = status, reason
		}
	}
	fmt.Fprintf(e.log(i), "%s %s %s: %s\n", status.Emoji(), labelOf(e.tasks[i]), status, reason)
	e.notify()
}

func (e *execution) start(i int) {
	inputs, _ := e.gather(e.tasks[i])
	e.started[i] = true
	log := e.log(i)
	if e.pausing(i) {
		e.pause(i, inputs, log)
		return
	}
	e.begin(i, Edit{})
	e.launch(i, inputs, log, Edit{})
}

// begin starts a task's clock, which a breakpoint before it never did.
func (e *execution) begin(i int, edit Edit) {
	task, step := e.tasks[i], &e.run.Steps[e.stepOf[i]]
	started, startedAt := e.stamp()
	if e.begun == nil {
		e.begun = make([]time.Time, len(e.tasks))
	}
	e.begun[i] = started
	step.Status, step.StartedAt = Running, startedAt
	if !edit.empty() {
		step.Edited = &Edited{Ran: edit, Original: e.resolved[i]}
	}
	fmt.Fprintf(e.log(i), "%s %s started at %s\n", Running.Emoji(), labelOf(task), startedAt)
	e.notify()
}

// launch runs the task on a goroutine of its own, with the edit made at its
// breakpoint when there was one.
func (e *execution) launch(i int, inputs []Input, log io.Writer, edit Edit) {
	task := e.tasks[i]
	if task.Loop != nil {
		runner := e.loopRunner(i, inputs, log)
		go func() {
			result, err := runner.run()
			e.finish(i, taskResult{result: result, err: err})
		}()
		return
	}
	ctx := editing(e.ctx, edit)
	go func() {
		result, err := task.Run(ctx, inputs, log)
		e.finish(i, taskResult{result: result, err: err})
	}()
}

func (e *execution) complete(i int) {
	value, _ := e.results.LoadAndDelete(i)
	outcome := value.(taskResult)
	task, step := e.tasks[i], &e.run.Steps[e.stepOf[i]]
	finished, finishedAt := e.stamp()
	e.settled[i] = true
	if outcome.settled != "" {
		// The task never ran: it was settled at the breakpoint before it.
		step.Status, step.Error, step.FinishedAt = outcome.settled, outcome.reason, finishedAt
		fmt.Fprintf(e.log(i), "%s %s %s: %s\n", outcome.settled.Emoji(), labelOf(task), outcome.settled, outcome.reason)
		e.notify()
		return
	}
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
	return Run{Status: run.Status, Steps: steps, Paused: slices.Clone(run.Paused)}
}
