package engine

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// gateAndFix is a loop body: a gate that passes on the given attempt and a
// fixer that runs on each failure, both counting their runs.
func gateAndFix(passOn int, runs map[string]int, mu *sync.Mutex) []Task {
	count := func(id string) int {
		mu.Lock()
		defer mu.Unlock()
		runs[id]++
		return runs[id]
	}
	return []Task{
		{ID: "gate", Name: "Gate", Needs: []Need{{TaskID: "coder", Port: "workspace"}},
			Run: func(_ context.Context, inputs []Input, _ io.Writer) (Result, error) {
				if attempt := count("gate"); attempt >= passOn {
					return Result{Outputs: map[string]any{"passed": "green", "workspace": inputs[0].Value}}, nil
				}
				return Result{Outputs: map[string]any{"failed": "red", "workspace": inputs[0].Value}}, nil
			}},
		{ID: "fixer", Name: "Fixer", Needs: []Need{{TaskID: "gate", Port: "failed"}},
			Run: func(_ context.Context, _ []Input, _ io.Writer) (Result, error) {
				count("fixer")
				return Result{Outputs: map[string]any{"result": "fixed"}}, nil
			}},
	}
}

// loopPlan: a coder hands a workspace to a loop around the body, and a
// shipper takes what the loop sends on done.
func loopPlan(body []Task, maxIterations int, finish func([]Input, LoopOutcome) (Result, error)) []Task {
	return []Task{
		{ID: "coder", Name: "Coder", Run: func(context.Context, []Input, io.Writer) (Result, error) {
			return Result{Outputs: map[string]any{"workspace": "/work"}}, nil
		}},
		{ID: "loop", Name: "Until green", Needs: []Need{{TaskID: "coder", Port: "workspace"}}, Loop: &Loop{
			Body: body, MaxIterations: maxIterations, Until: Need{TaskID: "gate", Port: "passed"}, Finish: finish,
		}},
		{ID: "ship", Name: "Ship", Needs: []Need{{TaskID: "loop", Port: "done"}},
			Run: func(_ context.Context, inputs []Input, _ io.Writer) (Result, error) {
				return Result{Details: map[string]any{"got": inputs[0].Value}}, nil
			}},
	}
}

func finishOnDone(_ []Input, outcome LoopOutcome) (Result, error) {
	if !outcome.Ended {
		return Result{Outputs: map[string]any{"exhausted": outcome.Outputs["failed"]}}, nil
	}
	return Result{Outputs: map[string]any{"done": outcome.Outputs["passed"]}}, nil
}

func stepOf(run Run, id string) Step {
	for _, step := range run.Steps {
		if step.TaskID == id {
			return step
		}
	}
	return Step{}
}

func TestALoopRepeatsItsBodyUntilTheExitPortIsTaken(t *testing.T) {
	runs, mu := map[string]int{}, &sync.Mutex{}

	run, err := Execute(context.Background(), loopPlan(gateAndFix(3, runs, mu), 5, finishOnDone), Options{})

	if err != nil || run.Status != Succeeded {
		t.Fatalf("run = %+v, err = %v", run, err)
	}
	if runs["gate"] != 3 || runs["fixer"] != 2 {
		t.Fatalf("runs = %v, want the gate 3 times and the fixer twice", runs)
	}
	loop := stepOf(run, "loop")
	if loop.Status != Succeeded || loop.Details["iterations"] != 3 || loop.Outputs["done"] != "green" {
		t.Fatalf("loop step = %+v", loop)
	}
	if got := stepOf(run, "ship").Details["got"]; got != "green" {
		t.Fatalf("ship got %v", got)
	}
	gate, fixer := stepOf(run, "gate"), stepOf(run, "fixer")
	if gate.Loop != "loop" || gate.Iteration != 3 || gate.Status != Succeeded || fixer.Status != Skipped {
		t.Fatalf("body steps show the last iteration: gate = %+v, fixer = %+v", gate, fixer)
	}
	var order []string
	for _, step := range run.Steps {
		order = append(order, step.TaskID)
	}
	if !reflect.DeepEqual(order, []string{"coder", "loop", "gate", "fixer", "ship"}) {
		t.Fatalf("steps = %v, want each body right after its loop", order)
	}
}

func TestALoopThatNeverEndsReportsItIsExhausted(t *testing.T) {
	runs, mu := map[string]int{}, &sync.Mutex{}
	var seen LoopOutcome
	finish := func(inputs []Input, outcome LoopOutcome) (Result, error) {
		seen = outcome
		if inputs[0].Value != "/work" {
			t.Errorf("finish inputs = %+v", inputs)
		}
		return finishOnDone(inputs, outcome)
	}

	run, _ := Execute(context.Background(), loopPlan(gateAndFix(99, runs, mu), 2, finish), Options{})

	if seen.Ended || seen.Iterations != 2 || seen.Outputs["failed"] != "red" {
		t.Fatalf("outcome = %+v", seen)
	}
	if runs["gate"] != 2 || stepOf(run, "loop").Outputs["exhausted"] != "red" {
		t.Fatalf("runs = %v, loop = %+v", runs, stepOf(run, "loop"))
	}
	if ship := stepOf(run, "ship"); ship.Status != Skipped {
		t.Fatalf("ship = %+v, want skipped: the loop did not take done", ship)
	}
}

func TestALoopFailsWhenFinishRefusesTheOutcome(t *testing.T) {
	runs, mu := map[string]int{}, &sync.Mutex{}
	finish := func(_ []Input, outcome LoopOutcome) (Result, error) {
		return Result{}, errors.New("did not pass after 2 iterations")
	}

	run, _ := Execute(context.Background(), loopPlan(gateAndFix(99, runs, mu), 2, finish), Options{})

	loop := stepOf(run, "loop")
	if run.Status != Failed || loop.Status != Failed || loop.Error != "did not pass after 2 iterations" {
		t.Fatalf("run = %s, loop = %+v", run.Status, loop)
	}
}

func TestAFailingBodyStepFailsTheLoop(t *testing.T) {
	body := []Task{{ID: "gate", Name: "Gate", Needs: []Need{{TaskID: "coder", Port: "workspace"}},
		Run: func(context.Context, []Input, io.Writer) (Result, error) { return Result{}, errors.New("no shell") }}}

	run, _ := Execute(context.Background(), loopPlan(body, 3, finishOnDone), Options{})

	loop := stepOf(run, "loop")
	if run.Status != Failed || loop.Status != Failed || !strings.Contains(loop.Error, "Gate failed in iteration 1: no shell") {
		t.Fatalf("loop = %+v", loop)
	}
	if stepOf(run, "gate").Status != Failed || stepOf(run, "ship").Status != Skipped {
		t.Fatalf("steps = %+v", run.Steps)
	}
}

func TestABodyStepSeesItsOutputsFromThePreviousIteration(t *testing.T) {
	var sessions []any
	body := []Task{
		{ID: "gate", Name: "Gate", Run: func(context.Context, []Input, io.Writer) (Result, error) {
			return Result{Outputs: map[string]any{"failed": "red"}}, nil
		}},
		{ID: "fixer", Name: "Fixer", Needs: []Need{{TaskID: "gate", Port: "failed"}},
			Run: func(ctx context.Context, _ []Input, _ io.Writer) (Result, error) {
				previous := Previous(ctx, "fixer")
				sessions = append(sessions, previous["session"])
				return Result{Outputs: map[string]any{"session": len(sessions)}}, nil
			}},
	}

	Execute(context.Background(), loopPlan(body, 3, finishOnDone), Options{})

	if !reflect.DeepEqual(sessions, []any{nil, 1, 2}) {
		t.Fatalf("previous sessions = %v", sessions)
	}
}

func TestALoopReportsEachIterationAsItRunsAndLogsIt(t *testing.T) {
	runs, mu := map[string]int{}, &sync.Mutex{}
	var observed []int
	logs := map[string]*bytes.Buffer{}
	options := Options{
		Observe: func(run Run) {
			if gate := stepOf(run, "gate"); gate.Status == Running {
				observed = append(observed, gate.Iteration)
			}
		},
		Log: func(taskID string) io.Writer {
			logs[taskID] = &bytes.Buffer{}
			return logs[taskID]
		},
	}

	Execute(context.Background(), loopPlan(gateAndFix(2, runs, mu), 3, finishOnDone), options)

	if !reflect.DeepEqual(observed, []int{1, 2}) {
		t.Fatalf("observed the gate running in iterations %v", observed)
	}
	loopLog, gateLog := logs["loop"].String(), logs["gate"].String()
	for _, want := range []string{"🔁 Iteration 1 of 3", "🔁 Iteration 2 of 3", "✅ Until green ended after 2 iterations"} {
		if !strings.Contains(loopLog, want) {
			t.Errorf("loop log lacks %q:\n%s", want, loopLog)
		}
	}
	if !strings.Contains(gateLog, "🔁 Iteration 2 of 3") || strings.Count(gateLog, "✅ Gate succeeded") != 2 {
		t.Errorf("gate log:\n%s", gateLog)
	}
}

func TestASkippedLoopSkipsItsBody(t *testing.T) {
	runs, mu := map[string]int{}, &sync.Mutex{}
	tasks := loopPlan(gateAndFix(1, runs, mu), 3, finishOnDone)
	tasks[0].Run = func(context.Context, []Input, io.Writer) (Result, error) { return Result{}, errors.New("broken") }

	run, _ := Execute(context.Background(), tasks, Options{})

	if stepOf(run, "loop").Status != Skipped || stepOf(run, "gate").Status != Skipped || stepOf(run, "fixer").Status != Skipped {
		t.Fatalf("steps = %+v", run.Steps)
	}
	if len(runs) != 0 {
		t.Fatalf("the body ran: %v", runs)
	}
}

func TestACancelledLoopStopsBetweenIterations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	body := []Task{{ID: "gate", Name: "Gate", Run: func(context.Context, []Input, io.Writer) (Result, error) {
		cancel()
		return Result{Outputs: map[string]any{"failed": "red"}}, nil
	}}}

	run, _ := Execute(ctx, loopPlan(body, 5, finishOnDone), Options{})

	if loop := stepOf(run, "loop"); run.Status != Failed || loop.Status != Failed || loop.Details["iterations"] != 1 {
		t.Fatalf("run = %s, loop = %+v", run.Status, loop)
	}
}

func TestLoopPlansAreValidated(t *testing.T) {
	gate := Task{ID: "gate", Run: func(context.Context, []Input, io.Writer) (Result, error) { return Result{}, nil }}
	for name, loop := range map[string]Loop{
		"no iterations":        {Body: []Task{gate}, Until: Need{TaskID: "gate", Port: "passed"}},
		"empty body":           {MaxIterations: 2, Until: Need{TaskID: "gate", Port: "passed"}},
		"until outside body":   {Body: []Task{gate}, MaxIterations: 2, Until: Need{TaskID: "coder", Port: "passed"}},
		"body id taken":        {Body: []Task{{ID: "coder", Run: gate.Run}}, MaxIterations: 2, Until: Need{TaskID: "coder", Port: "x"}},
		"body needs elsewhere": {Body: []Task{{ID: "gate", Run: gate.Run, Needs: []Need{{TaskID: "ship"}}}}, MaxIterations: 2, Until: Need{TaskID: "gate", Port: "x"}},
	} {
		tasks := loopPlan(nil, 0, finishOnDone)
		tasks[1].Loop = &loop
		if err := Validate(tasks); err == nil {
			t.Errorf("%s: the plan was accepted", name)
		}
	}
}
