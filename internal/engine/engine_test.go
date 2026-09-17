package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"
)

// recorder builds tasks that log their execution order and hand their
// inputs back as evidence.
type recorder struct {
	order  []string
	inputs map[string][]Input
}

func (r *recorder) task(id string, needs []Need, run func([]Input) (Result, error)) Task {
	return Task{
		ID: id, Needs: needs,
		Run: func(_ context.Context, inputs []Input, _ io.Writer) (Result, error) {
			r.order = append(r.order, id)
			if r.inputs == nil {
				r.inputs = map[string][]Input{}
			}
			r.inputs[id] = inputs
			return run(inputs)
		},
	}
}

func emits(port string, value any) func([]Input) (Result, error) {
	return func([]Input) (Result, error) {
		return Result{Outputs: map[string]any{port: value}}, nil
	}
}

func fails(message string) func([]Input) (Result, error) {
	return func([]Input) (Result, error) { return Result{}, errors.New(message) }
}

func statuses(run Run) map[string]Status {
	got := map[string]Status{}
	for _, step := range run.Steps {
		got[step.TaskID] = step.Status
	}
	return got
}

func TestRunExecutesDependenciesBeforeDependents(t *testing.T) {
	r := &recorder{}
	tasks := []Task{
		r.task("rebase", []Need{{TaskID: "worktree", Port: "workspace"}}, emits("workspace", "rebased")),
		r.task("fetch", nil, emits("", nil)),
		r.task("worktree", []Need{{TaskID: "fetch"}}, emits("workspace", "created")),
	}

	run, err := Execute(context.Background(), tasks, Options{})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !reflect.DeepEqual(r.order, []string{"fetch", "worktree", "rebase"}) {
		t.Fatalf("order = %v", r.order)
	}
	if run.Status != Succeeded {
		t.Fatalf("status = %s", run.Status)
	}
	want := []Input{{TaskID: "worktree", Port: "workspace", Value: "created"}}
	if !reflect.DeepEqual(r.inputs["rebase"], want) {
		t.Fatalf("rebase inputs = %+v, want %+v", r.inputs["rebase"], want)
	}
}

func TestRunKeepsDeclarationOrderAmongIndependentTasks(t *testing.T) {
	r := &recorder{}
	tasks := []Task{
		r.task("b", nil, emits("", nil)),
		r.task("a", nil, emits("", nil)),
		r.task("c", []Need{{TaskID: "a"}}, emits("", nil)),
	}

	if _, err := Execute(context.Background(), tasks, Options{}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !reflect.DeepEqual(r.order, []string{"b", "a", "c"}) {
		t.Fatalf("order = %v", r.order)
	}
}

func TestRunSkipsEverythingDownstreamOfAFailure(t *testing.T) {
	r := &recorder{}
	tasks := []Task{
		r.task("fetch", nil, fails("network down")),
		r.task("worktree", []Need{{TaskID: "fetch"}}, emits("workspace", "w")),
		r.task("agent", []Need{{TaskID: "worktree", Port: "workspace"}}, emits("result", "r")),
		r.task("independent", nil, emits("", nil)),
	}

	run, err := Execute(context.Background(), tasks, Options{})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	want := map[string]Status{"fetch": Failed, "worktree": Skipped, "agent": Skipped, "independent": Succeeded}
	if got := statuses(run); !reflect.DeepEqual(got, want) {
		t.Fatalf("statuses = %v, want %v", got, want)
	}
	if run.Status != Failed || run.Steps[0].Error != "network down" {
		t.Fatalf("run = %+v", run)
	}
	if !strings.Contains(run.Steps[1].Error, "fetch failed") {
		t.Fatalf("skip reason = %q, want it to name the failed dependency", run.Steps[1].Error)
	}
}

func TestRunSkipsADependentWhoseNeededPortWasNotEmitted(t *testing.T) {
	r := &recorder{}
	tasks := []Task{
		r.task("router", nil, emits("approved", "value")),
		r.task("merge", []Need{{TaskID: "router", Port: "approved"}}, emits("", nil)),
		r.task("fix", []Need{{TaskID: "router", Port: "rejected"}}, emits("", nil)),
	}

	run, err := Execute(context.Background(), tasks, Options{})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	want := map[string]Status{"router": Succeeded, "merge": Succeeded, "fix": Skipped}
	if got := statuses(run); !reflect.DeepEqual(got, want) {
		t.Fatalf("statuses = %v, want %v", got, want)
	}
	if run.Status != Succeeded {
		t.Fatalf("a branch not taken must not fail the run: %s", run.Status)
	}
	if !strings.Contains(run.Steps[2].Error, `router did not take "rejected"`) {
		t.Fatalf("skip reason = %q", run.Steps[2].Error)
	}
}

func TestRunReportsProgressAsStepsChange(t *testing.T) {
	r := &recorder{}
	tasks := []Task{
		r.task("fetch", nil, emits("", nil)),
		r.task("worktree", []Need{{TaskID: "fetch"}}, fails("occupied")),
	}
	var seen []string
	observe := func(run Run) {
		var line []string
		for _, step := range run.Steps {
			line = append(line, step.TaskID+"="+string(step.Status))
		}
		seen = append(seen, strings.Join(line, ","))
	}

	if _, err := Execute(context.Background(), tasks, Options{Observe: observe}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	want := []string{
		"fetch=pending,worktree=pending",
		"fetch=running,worktree=pending",
		"fetch=succeeded,worktree=pending",
		"fetch=succeeded,worktree=running",
		"fetch=succeeded,worktree=failed",
	}
	if !reflect.DeepEqual(seen, want) {
		t.Fatalf("progress =\n%s\nwant\n%s", strings.Join(seen, "\n"), strings.Join(want, "\n"))
	}
}

func TestRunKeepsAStepsDetailsEvenWhenItFails(t *testing.T) {
	tasks := []Task{{
		ID: "agent",
		Run: func(context.Context, []Input, io.Writer) (Result, error) {
			return Result{Details: map[string]any{"reply": "not json"}}, errors.New("reply does not match the schema")
		},
	}}

	run, err := Execute(context.Background(), tasks, Options{})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if run.Steps[0].Details["reply"] != "not json" {
		t.Fatalf("details = %+v, want the evidence kept", run.Steps[0].Details)
	}
}

func TestRunRejectsInvalidPlans(t *testing.T) {
	ok := func(context.Context, []Input, io.Writer) (Result, error) { return Result{}, nil }
	cases := map[string]struct {
		tasks []Task
		want  string
	}{
		"cycle": {
			tasks: []Task{
				{ID: "a", Needs: []Need{{TaskID: "b"}}, Run: ok},
				{ID: "b", Needs: []Need{{TaskID: "a"}}, Run: ok},
			},
			want: "cycle",
		},
		"unknown dependency": {
			tasks: []Task{{ID: "a", Needs: []Need{{TaskID: "ghost"}}, Run: ok}},
			want:  `a needs unknown task "ghost"`,
		},
		"duplicate": {
			tasks: []Task{{ID: "a", Run: ok}, {ID: "a", Run: ok}},
			want:  `duplicate task "a"`,
		},
		"empty": {
			tasks: nil,
			want:  "nothing to run",
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Execute(context.Background(), testCase.tasks, Options{})
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("err = %v, want it to contain %q", err, testCase.want)
			}
			if err := Validate(testCase.tasks); err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("validate err = %v, want it to contain %q", err, testCase.want)
			}
		})
	}
}

func TestRunStopsSchedulingWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	tasks := []Task{
		{ID: "first", Run: func(context.Context, []Input, io.Writer) (Result, error) {
			cancel()
			return Result{}, nil
		}},
		{ID: "second", Needs: []Need{{TaskID: "first"}}, Run: func(context.Context, []Input, io.Writer) (Result, error) {
			t.Fatal("a cancelled run started another task")
			return Result{}, nil
		}},
	}

	run, err := Execute(ctx, tasks, Options{})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := statuses(run); got["second"] != Skipped || run.Status != Failed {
		t.Fatalf("run = %+v", run)
	}
}

func TestRunGivesEachTaskItsOwnLog(t *testing.T) {
	logs := map[string]*bytes.Buffer{}
	tasks := []Task{
		{ID: "fetch", Name: "Fetch", Run: func(_ context.Context, _ []Input, log io.Writer) (Result, error) {
			fmt.Fprintln(log, "$ git fetch origin")
			return Result{}, nil
		}},
		{ID: "worktree", Name: "Create worktree", Needs: []Need{{TaskID: "fetch"}}, Run: func(_ context.Context, _ []Input, log io.Writer) (Result, error) {
			fmt.Fprintln(log, "$ git worktree add")
			return Result{}, errors.New("path occupied")
		}},
		{ID: "rebase", Name: "Rebase", Needs: []Need{{TaskID: "worktree"}}, Run: func(context.Context, []Input, io.Writer) (Result, error) {
			t.Fatal("a skipped task ran")
			return Result{}, nil
		}},
	}
	options := Options{Log: func(taskID string) io.Writer {
		logs[taskID] = &bytes.Buffer{}
		return logs[taskID]
	}}

	if _, err := Execute(context.Background(), tasks, options); err != nil {
		t.Fatalf("execute: %v", err)
	}
	fetch := logs["fetch"].String()
	if !strings.Contains(fetch, "Fetch started") || !strings.Contains(fetch, "$ git fetch origin\n") ||
		!strings.Contains(fetch, "Fetch succeeded in ") {
		t.Fatalf("fetch log = %q", fetch)
	}
	if worktree := logs["worktree"].String(); !strings.Contains(worktree, "$ git worktree add\n") ||
		!strings.Contains(worktree, "Create worktree failed in ") || !strings.Contains(worktree, "path occupied") {
		t.Fatalf("worktree log = %q", worktree)
	}
	if rebase := logs["rebase"].String(); !strings.Contains(rebase, "Rebase skipped: Create worktree failed") {
		t.Fatalf("rebase log = %q", rebase)
	}
}

func TestRunTimesEachStep(t *testing.T) {
	now := time.Date(2026, 9, 16, 23, 0, 0, 0, time.UTC)
	clock := func() time.Time {
		now = now.Add(time.Second)
		return now
	}
	tasks := []Task{{ID: "fetch", Run: func(context.Context, []Input, io.Writer) (Result, error) { return Result{}, nil }}}

	run, err := Execute(context.Background(), tasks, Options{Now: clock})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	step := run.Steps[0]
	if step.StartedAt != "2026-09-16T23:00:01Z" || step.FinishedAt != "2026-09-16T23:00:02Z" {
		t.Fatalf("step = %+v", step)
	}
}

func TestASucceededStepKeepsItsOutputs(t *testing.T) {
	tasks := []Task{{ID: "worktree", Run: func(context.Context, []Input, io.Writer) (Result, error) {
		return Result{Outputs: map[string]any{"workspace": "w"}}, nil
	}}}

	run, err := Execute(context.Background(), tasks, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if run.Steps[0].Outputs["workspace"] != "w" {
		t.Fatalf("step = %+v", run.Steps[0])
	}
}
