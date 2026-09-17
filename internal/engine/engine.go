// Package engine executes a planned workflow: tasks run after the tasks they
// need, each receives the outputs it asked for, and a failure or a branch not
// taken skips everything downstream of it. The engine knows nothing about
// block types; planning turns a graph into tasks.
package engine

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
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
	Run   func(ctx context.Context, inputs []Input) (Result, error)
}

type Step struct {
	TaskID  string         `json:"nodeId"`
	Name    string         `json:"name"`
	Kind    string         `json:"action"`
	Status  Status         `json:"status"`
	Error   string         `json:"error,omitempty"`
	Details map[string]any `json:"details,omitempty"`
}

type Run struct {
	Status Status `json:"status"`
	Steps  []Step `json:"steps"`
}

// Observer receives a snapshot of the run each time a step changes status.
type Observer func(Run)

// Execute validates the plan, then runs its tasks one at a time in dependency
// order, keeping declaration order among tasks that are ready together. It
// returns an error only for a plan that cannot run; task failures are
// recorded on their steps.
func Execute(ctx context.Context, tasks []Task, observe Observer) (Run, error) {
	order, err := schedule(tasks)
	if err != nil {
		return Run{}, err
	}
	run := Run{Status: Running, Steps: make([]Step, len(tasks))}
	index := make(map[string]int, len(tasks))
	for i, task := range tasks {
		index[task.ID] = i
		run.Steps[i] = Step{TaskID: task.ID, Name: task.Name, Kind: task.Kind, Status: Pending}
	}
	outputs := make(map[string]map[string]any, len(tasks))
	notify := func() {
		if observe != nil {
			observe(snapshot(run))
		}
	}
	notify()
	for _, i := range order {
		task, step := tasks[i], &run.Steps[i]
		inputs, reason := gather(task, run.Steps, index, outputs)
		if reason == "" && ctx.Err() != nil {
			reason = "the run was cancelled"
		}
		if reason != "" {
			step.Status, step.Error = Skipped, reason
			notify()
			continue
		}
		step.Status = Running
		notify()
		result, err := task.Run(ctx, inputs)
		step.Details = result.Details
		if err != nil {
			step.Status, step.Error = Failed, err.Error()
		} else {
			step.Status = Succeeded
			outputs[task.ID] = result.Outputs
		}
		notify()
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
		steps[i] = step
	}
	return Run{Status: run.Status, Steps: steps}
}
