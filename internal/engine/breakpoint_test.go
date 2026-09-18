package engine

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// pauses records every breakpoint a run reached and answers each with the
// resume the test prepared, so a test sees what a paused run offers.
type pauses struct {
	mu     sync.Mutex
	seen   []Pause
	answer func(at Pause) Resume
}

func (p *pauses) option() func(context.Context, Pause) Resume {
	return func(_ context.Context, at Pause) Resume {
		p.mu.Lock()
		p.seen = append(p.seen, at)
		p.mu.Unlock()
		return p.answer(at)
	}
}

func (p *pauses) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.seen)
}

func always(action Action) func(Pause) Resume {
	return func(Pause) Resume { return Resume{Action: action} }
}

func TestABreakpointPausesBeforeTheBlockRunsAndContinuesOnResume(t *testing.T) {
	r := &recorder{}
	paused := &pauses{answer: always(ResumeContinue)}
	tasks := []Task{
		r.task("worktree", nil, emits("workspace", "/work")),
		{ID: "coder", Name: "Coder", Kind: "agent", Breakpoint: true,
			Needs: []Need{{TaskID: "worktree", Port: "workspace"}},
			Resolve: func(inputs []Input) Edit {
				return Edit{Prompt: "work in " + inputs[0].Value.(string)}
			},
			Run: func(context.Context, []Input, io.Writer) (Result, error) {
				if paused.count() == 0 {
					t.Error("the block ran before the run paused")
				}
				return Result{}, nil
			}},
	}
	var states []Status
	observe := func(run Run) { states = append(states, run.Steps[1].Status) }

	run := finishes(t, context.Background(), tasks, Options{Pause: paused.option(), Observe: observe}, 5*time.Second)

	if run.Status != Succeeded || len(paused.seen) != 1 {
		t.Fatalf("run = %+v, pauses = %d", run, len(paused.seen))
	}
	at := paused.seen[0]
	if at.TaskID != "coder" || at.Name != "Coder" || at.Kind != "agent" || at.Resolved.Prompt != "work in /work" {
		t.Fatalf("pause = %+v, want the block and the prompt it is about to run", at)
	}
	want := []PausedInput{{TaskID: "worktree", Port: "workspace", Value: "/work"}}
	if !reflect.DeepEqual(at.Inputs, want) {
		t.Fatalf("inputs = %+v, want %+v", at.Inputs, want)
	}
	if !containsStatus(states, Paused) {
		t.Fatalf("statuses = %v, want the step to show as paused while it waited", states)
	}
	if step := run.Steps[1]; step.Status != Succeeded || step.StartedAt == "" || step.Edited != nil {
		t.Fatalf("step = %+v", step)
	}
}

func containsStatus(states []Status, want Status) bool {
	for _, status := range states {
		if status == want {
			return true
		}
	}
	return false
}

func TestARunWithNobodyWatchingIgnoresBreakpoints(t *testing.T) {
	r := &recorder{}
	tasks := []Task{r.task("coder", nil, emits("result", "done"))}
	tasks[0].Breakpoint = true

	run := finishes(t, context.Background(), tasks, Options{}, 5*time.Second)

	if run.Status != Succeeded || len(r.order) != 1 {
		t.Fatalf("run = %+v, order = %v", run, r.order)
	}
}

func TestSteppingPausesAgainBeforeTheNextBlock(t *testing.T) {
	r := &recorder{}
	paused := &pauses{}
	paused.answer = func(at Pause) Resume {
		if at.TaskID == "gate" {
			return Resume{Action: ResumeContinue}
		}
		return Resume{Action: ResumeStep}
	}
	tasks := []Task{
		{ID: "coder", Breakpoint: true, Run: func(context.Context, []Input, io.Writer) (Result, error) {
			return Result{Outputs: map[string]any{"result": "code"}}, nil
		}},
		r.task("gate", []Need{{TaskID: "coder", Port: "result"}}, emits("passed", "ok")),
	}

	run := finishes(t, context.Background(), tasks, Options{Pause: paused.option()}, 5*time.Second)

	if run.Status != Succeeded {
		t.Fatalf("run = %+v", run)
	}
	var stopped []string
	for _, at := range paused.seen {
		stopped = append(stopped, at.TaskID)
	}
	if !reflect.DeepEqual(stopped, []string{"coder", "gate"}) {
		t.Fatalf("paused at %v, want a step to stop before the block after it too", stopped)
	}
}

func TestSkippingAtABreakpointSkipsTheBlockAndWhatFollows(t *testing.T) {
	r := &recorder{}
	paused := &pauses{answer: always(ResumeSkip)}
	tasks := []Task{
		{ID: "coder", Name: "Coder", Breakpoint: true, Run: func(context.Context, []Input, io.Writer) (Result, error) {
			t.Error("a skipped block ran")
			return Result{}, nil
		}},
		r.task("gate", []Need{{TaskID: "coder", Port: "result"}}, emits("passed", "ok")),
	}

	run := finishes(t, context.Background(), tasks, Options{Pause: paused.option()}, 5*time.Second)

	want := map[string]Status{"coder": Skipped, "gate": Skipped}
	if got := statuses(run); !reflect.DeepEqual(got, want) {
		t.Fatalf("statuses = %v, want %v", got, want)
	}
	if run.Status != Succeeded {
		t.Fatalf("status = %s; skipping a block at a breakpoint is not a failure", run.Status)
	}
	if run.Steps[0].Error != "skipped at a breakpoint" || run.Steps[0].FinishedAt == "" {
		t.Fatalf("coder = %+v", run.Steps[0])
	}
	if !strings.Contains(run.Steps[1].Error, "Coder was skipped") {
		t.Fatalf("gate = %+v", run.Steps[1])
	}
}

func TestAnEditAtABreakpointRunsWithTheEditAndRecordsWhatTheWorkflowHolds(t *testing.T) {
	paused := &pauses{answer: func(Pause) Resume {
		return Resume{Action: ResumeContinue, Edit: Edit{Prompt: "fix the typo only"}}
	}}
	var ran string
	tasks := []Task{{
		ID: "coder", Name: "Coder", Breakpoint: true,
		Resolve: func([]Input) Edit { return Edit{Prompt: "rewrite everything"} },
		Run: func(ctx context.Context, _ []Input, _ io.Writer) (Result, error) {
			ran = "rewrite everything"
			if edit, edited := EditIn(ctx); edited {
				ran = edit.Prompt
			}
			return Result{}, nil
		},
	}}

	run := finishes(t, context.Background(), tasks, Options{Pause: paused.option()}, 5*time.Second)

	if ran != "fix the typo only" {
		t.Fatalf("the block ran with %q, want the edit made at the breakpoint", ran)
	}
	step := run.Steps[0]
	if step.Status != Succeeded || step.Edited == nil {
		t.Fatalf("step = %+v, want the edit recorded", step)
	}
	if step.Edited.Ran.Prompt != "fix the typo only" || step.Edited.Original.Prompt != "rewrite everything" {
		t.Fatalf("edited = %+v, want both what ran and what the workflow holds", step.Edited)
	}
}

func TestABreakpointPausesOnlyItsOwnBlockWhileTheOthersRun(t *testing.T) {
	ran := make(chan string, 3)
	release := make(chan struct{})
	paused := &pauses{answer: func(Pause) Resume {
		// The paused block waits until the branch beside it has finished, so
		// the run can only end if a breakpoint holds one block, not the run.
		<-release
		return Resume{Action: ResumeContinue}
	}}
	tasks := []Task{
		{ID: "reviewer", Breakpoint: true, Run: func(context.Context, []Input, io.Writer) (Result, error) {
			ran <- "reviewer"
			return Result{}, nil
		}},
		{ID: "tester", Run: func(context.Context, []Input, io.Writer) (Result, error) {
			ran <- "tester"
			return Result{}, nil
		}},
		{ID: "after-tester", Needs: []Need{{TaskID: "tester"}}, Run: func(context.Context, []Input, io.Writer) (Result, error) {
			ran <- "after tester"
			close(release)
			return Result{}, nil
		}},
	}

	run := finishes(t, context.Background(), tasks, Options{Pause: paused.option()}, 5*time.Second)

	if run.Status != Succeeded {
		t.Fatalf("run = %+v", run)
	}
	// The tester and the block after it both ran while the reviewer waited.
	if first, second := <-ran, <-ran; first != "tester" || second != "after tester" {
		t.Fatalf("ran %q then %q, want the branch beside the breakpoint to keep running", first, second)
	}
	if last := <-ran; last != "reviewer" {
		t.Fatalf("last = %q", last)
	}
}

func TestCancellingARunWhilePausedSettlesIt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reached := make(chan struct{})
	// A paused run waits indefinitely, so only the cancellation releases it.
	pause := func(pauseCtx context.Context, _ Pause) Resume {
		close(reached)
		<-pauseCtx.Done()
		return Resume{}
	}
	tasks := []Task{
		{ID: "coder", Name: "Coder", Breakpoint: true, Run: func(context.Context, []Input, io.Writer) (Result, error) {
			t.Error("a cancelled run ran the block it was paused before")
			return Result{}, nil
		}},
		{ID: "gate", Needs: []Need{{TaskID: "coder"}}, Run: func(context.Context, []Input, io.Writer) (Result, error) {
			t.Error("a cancelled run started the block after the breakpoint")
			return Result{}, nil
		}},
	}
	go func() {
		<-reached
		cancel()
	}()

	run := finishes(t, ctx, tasks, Options{Pause: pause, CancelGrace: time.Second}, 5*time.Second)

	if run.Status != Failed {
		t.Fatalf("status = %s, want a cancelled run to end failed", run.Status)
	}
	if step := run.Steps[0]; step.Status != Skipped || step.Error != "the run was cancelled" {
		t.Fatalf("coder = %+v", step)
	}
}

func TestABreakpointInsideALoopPausesOnEveryRepeat(t *testing.T) {
	paused := &pauses{answer: always(ResumeContinue)}
	attempts := 0
	tasks := []Task{{
		ID: "loop", Name: "Until it passes", Kind: "loop",
		Loop: &Loop{
			MaxIterations: 3,
			Until:         Need{TaskID: "gate", Port: "passed"},
			Body: []Task{{
				ID: "gate", Name: "Tests", Breakpoint: true,
				Run: func(context.Context, []Input, io.Writer) (Result, error) {
					attempts++
					if attempts < 2 {
						return Result{Outputs: map[string]any{"failed": "red"}}, nil
					}
					return Result{Outputs: map[string]any{"passed": "green"}}, nil
				},
			}},
			Finish: func([]Input, LoopOutcome) (Result, error) { return Result{}, nil },
		},
	}}

	run := finishes(t, context.Background(), tasks, Options{Pause: paused.option()}, 5*time.Second)

	if run.Status != Succeeded || attempts != 2 {
		t.Fatalf("run = %+v, attempts = %d", run, attempts)
	}
	if len(paused.seen) != 2 {
		t.Fatalf("paused %d times, want one pause per repeat of the block", len(paused.seen))
	}
	if at := paused.seen[1]; at.Loop != "loop" || at.Iteration != 2 {
		t.Fatalf("second pause = %+v, want the loop and repeat it belongs to", at)
	}
}

func TestAPausedRunShowsWhereItWaits(t *testing.T) {
	waiting := make(chan struct{})
	release := make(chan struct{})
	paused := &pauses{answer: func(Pause) Resume {
		close(waiting)
		<-release
		return Resume{Action: ResumeContinue}
	}}
	var whilePaused Run
	observe := func(run Run) {
		if len(run.Paused) > 0 {
			whilePaused = run
		}
	}
	tasks := []Task{{ID: "coder", Name: "Coder", Breakpoint: true,
		Run: func(context.Context, []Input, io.Writer) (Result, error) { return Result{}, errors.New("late") }}}
	go func() {
		<-waiting
		close(release)
	}()

	run := finishes(t, context.Background(), tasks, Options{Pause: paused.option(), Observe: observe}, 5*time.Second)

	if len(whilePaused.Paused) != 1 || whilePaused.Paused[0].TaskID != "coder" {
		t.Fatalf("paused = %+v, want the block the run waits before", whilePaused.Paused)
	}
	if len(run.Paused) != 0 {
		t.Fatalf("a finished run still shows %+v as paused", run.Paused)
	}
}
