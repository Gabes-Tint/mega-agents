package app

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/Gabes-Tint/mega-agents/internal/engine"
	"github.com/Gabes-Tint/mega-agents/internal/runs"
)

// scheduledWorkflow is a one-agent workflow the fake claude answers, saved
// under the name and put on the schedule.
func scheduledWorkflow(t *testing.T, handler http.Handler, name string, schedule string) {
	t.Helper()
	project := t.TempDir()
	body := fmt.Sprintf(`{"schedule": %q, "nodes": [
		{"id": "p1", "type": "project", "name": "api", "path": %q},
		{"id": "a1", "type": "agent", "name": "Planner", "parentId": "p1", "start": true,
		 "backend": "claude", "prompt": "Plan"}
	], "edges": []}`, schedule, project)
	saved := workflowRequest(t, handler, http.MethodPut, "/api/workflows/"+name, body, "application/json")
	if saved.Code != http.StatusOK {
		t.Fatalf("saving %s = %d %s", name, saved.Code, saved.Body.String())
	}
}

// testScheduler is a scheduler over a home of its own, with a context that
// is cancelled and waited out when the test ends: the runs it starts are
// still going otherwise, and would write into a home already cleaned up.
func testScheduler(t *testing.T) (Scheduler, *runs.Store, http.Handler, context.Context) {
	t.Helper()
	fakeClaude(t)
	home := t.TempDir()
	t.Setenv("MEGA_AGENTS_HOME", home)
	handler, scheduler := NewEditor(testAssets(), nil)
	store := &runs.Store{Root: filepath.Join(home, "runs")}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		settleRuns(t, store)
	})
	return scheduler, store, handler, ctx
}

// settleRuns waits until no run is still going, so nothing writes into the
// home after the test has finished with it.
func settleRuns(t *testing.T, store *runs.Store) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		busy := false
		for _, record := range recordedRuns(t, store) {
			if record.Status == engine.Running || record.Status == engine.Paused {
				busy = true
			}
		}
		if !busy {
			return
		}
		if time.Now().After(deadline) {
			t.Errorf("runs were still going when the test finished")
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// recordedRuns are the runs the store holds, whatever their state.
func recordedRuns(t *testing.T, store *runs.Store) []runs.Record {
	t.Helper()
	records, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	return records
}

func waitForRuns(t *testing.T, store *runs.Store, count int) []runs.Record {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		records := recordedRuns(t, store)
		if len(records) >= count {
			return records
		}
		if time.Now().After(deadline) {
			t.Fatalf("waited for %d runs, the store has %d", count, len(records))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestATickStartsTheWorkflowsDueThatMinute(t *testing.T) {
	scheduler, store, handler, ctx := testScheduler(t)
	scheduledWorkflow(t, handler, "nightly", "30 9 * * *")
	scheduledWorkflow(t, handler, "hourly", "0 * * * *")
	scheduledWorkflow(t, handler, "on-demand", "")

	started := scheduler.Tick(ctx, time.Date(2026, 9, 18, 9, 30, 0, 0, time.Local))

	if len(started) != 1 || started[0].Workflow != "nightly" {
		t.Fatalf("started = %+v", started)
	}
	records := waitForRuns(t, store, 1)
	if records[0].Trigger != runs.Scheduled {
		t.Fatalf("run = %+v, want one the schedule started", records[0])
	}
}

// A run still going does not hold the next one back: a schedule that comes
// round again starts another run beside it.
func TestASecondRunStartsWhileTheFirstIsStillGoing(t *testing.T) {
	scheduler, store, handler, ctx := testScheduler(t)
	scheduledWorkflow(t, handler, "hourly", "0 * * * *")
	due := time.Date(2026, 9, 18, 9, 0, 0, 0, time.Local)

	first := scheduler.Tick(ctx, due)
	second := scheduler.Tick(ctx, due.Add(time.Hour))

	if len(first) != 1 || len(second) != 1 || first[0].ID == second[0].ID {
		t.Fatalf("first = %+v, second = %+v", first, second)
	}
	waitForRuns(t, store, 2)
}

func TestATickStartsNothingWhenNoScheduleIsDue(t *testing.T) {
	scheduler, _, handler, ctx := testScheduler(t)
	scheduledWorkflow(t, handler, "nightly", "30 9 * * *")

	if started := scheduler.Tick(ctx, time.Date(2026, 9, 18, 9, 31, 0, 0, time.Local)); len(started) != 0 {
		t.Fatalf("started = %+v", started)
	}
}

// A workflow that cannot run leaves the rest of the schedule alone.
func TestATickGoesOnAfterAWorkflowThatCannotStart(t *testing.T) {
	scheduler, _, handler, ctx := testScheduler(t)
	brokenGraph := `{"schedule": "0 * * * *", "nodes": [
		{"id": "p1", "type": "project", "name": "api", "path": "/work/api"}
	], "edges": []}`
	saved := workflowRequest(t, handler, http.MethodPut, "/api/workflows/broken", brokenGraph, "application/json")
	if saved.Code != http.StatusOK {
		t.Fatalf("saving the broken workflow = %d %s", saved.Code, saved.Body.String())
	}
	scheduledWorkflow(t, handler, "working", "0 * * * *")

	started := scheduler.Tick(ctx, time.Date(2026, 9, 18, 9, 0, 0, 0, time.Local))

	if len(started) != 1 || started[0].Workflow != "working" {
		t.Fatalf("started = %+v", started)
	}
}

// The clock is what the scheduler watches: Run keeps ticking until it is
// stopped, and never twice for the same window.
func TestRunKeepsTheScheduleUntilItIsStopped(t *testing.T) {
	scheduler, store, handler, ctx := testScheduler(t)
	scheduledWorkflow(t, handler, "always", "* * * * *")
	scheduler.Every = 50 * time.Millisecond
	keeping, stop := context.WithCancel(ctx)

	go scheduler.Run(keeping)

	waitForRuns(t, store, 2)
	stop()
	settled := len(recordedRuns(t, store))
	time.Sleep(150 * time.Millisecond)
	if after := len(recordedRuns(t, store)); after > settled+1 {
		t.Fatalf("runs went from %d to %d after the scheduler was stopped", settled, after)
	}
}

// A run someone starts from the editor is not one the schedule started.
func TestARunStartedByHandSaysSo(t *testing.T) {
	fakeClaude(t)
	home := t.TempDir()
	t.Setenv("MEGA_AGENTS_HOME", home)
	handler := NewHandler(testAssets())
	project := t.TempDir()
	body := fmt.Sprintf(`{"nodes": [
		{"id": "p1", "type": "project", "name": "api", "path": %q},
		{"id": "a1", "type": "agent", "name": "Planner", "parentId": "p1", "start": true,
		 "backend": "claude", "prompt": "Plan"}
	], "edges": []}`, project)

	response := workflowRequest(t, handler, http.MethodPost, "/api/runs", body, "application/json")

	if response.Code != http.StatusAccepted {
		t.Fatalf("run = %d %s", response.Code, response.Body.String())
	}
	store := &runs.Store{Root: filepath.Join(home, "runs")}
	records := waitForRuns(t, store, 1)
	if records[0].Trigger != runs.ByHand {
		t.Fatalf("run = %+v, want one someone started", records[0])
	}
	settleRuns(t, store)
}

// A scheduled run stops at a breakpoint like any other, so the editor can
// answer it.
func TestAScheduledRunWaitsAtABreakpoint(t *testing.T) {
	scheduler, store, handler, ctx := testScheduler(t)
	project := t.TempDir()
	body := fmt.Sprintf(`{"schedule": "0 * * * *", "nodes": [
		{"id": "p1", "type": "project", "name": "api", "path": %q},
		{"id": "a1", "type": "agent", "name": "Planner", "parentId": "p1", "start": true,
		 "breakpoint": true, "backend": "claude", "prompt": "Plan"}
	], "edges": []}`, project)
	if saved := workflowRequest(t, handler, http.MethodPut, "/api/workflows/paused", body, "application/json"); saved.Code != http.StatusOK {
		t.Fatalf("saving = %d %s", saved.Code, saved.Body.String())
	}

	started := scheduler.Tick(ctx, time.Date(2026, 9, 18, 9, 0, 0, 0, time.Local))

	if len(started) != 1 {
		t.Fatalf("started = %+v", started)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		record, err := store.Load(started[0].ID)
		if err == nil && record.Status == engine.Paused {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the scheduled run never waited at its breakpoint: %+v", record)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
