package app

import (
	"context"
	"log"
	"time"

	"github.com/Gabes-Tint/mega-agents/internal/cron"
	"github.com/Gabes-Tint/mega-agents/internal/runs"
)

// Scheduler starts the runs the saved workflows are scheduled for. It looks
// at the clock once a minute and starts every workflow whose expression is
// due in that minute, reading expressions in the server's own time zone.
//
// Nothing is caught up: a schedule that came round while the server was not
// running is simply missed, so a restart never lets loose a burst of runs.
// Runs already going do not hold the next one back either; a schedule that
// comes round again starts another run beside them.
type Scheduler struct {
	Workflows WorkflowStore
	Runs      Runs
	// Every is how often the clock is looked at, a minute by default. A
	// schedule is due at most once in each of these windows.
	Every time.Duration
}

func (scheduler Scheduler) every() time.Duration {
	if scheduler.Every > 0 {
		return scheduler.Every
	}
	return time.Minute
}

// Run keeps the schedule until the context is done. It wakes on each window
// boundary, so a schedule due at 9:30 starts then rather than a minute into
// whenever the server happened to start.
func (scheduler Scheduler) Run(ctx context.Context) {
	window := scheduler.every()
	var last time.Time
	for {
		now := time.Now()
		next := now.Truncate(window).Add(window)
		timer := time.NewTimer(next.Sub(now))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case moment := <-timer.C:
			// A late wake-up must not start the same window twice.
			if window := moment.Truncate(window); !window.Equal(last) {
				last = window
				scheduler.Tick(ctx, moment)
			}
		}
	}
}

// Tick starts a run of every saved workflow whose schedule is due in the
// minute the moment falls in, and returns the runs it started. Each one
// executes in the background, as a run started from the editor does.
func (scheduler Scheduler) Tick(ctx context.Context, moment time.Time) []runs.Record {
	workflows, err := scheduler.Workflows.List()
	if err != nil {
		log.Printf("reading the schedule: %v", err)
		return nil
	}
	started := []runs.Record{}
	for _, workflow := range workflows {
		if workflow.Schedule == "" {
			continue
		}
		schedule, err := cron.Parse(workflow.Schedule)
		if err != nil {
			log.Printf("workflow %s is not scheduled: %v", workflow.Name, err)
			continue
		}
		if !schedule.Matches(moment) {
			continue
		}
		record, ok := scheduler.start(ctx, workflow.Name)
		if ok {
			started = append(started, record)
		}
	}
	return started
}

// start records and launches one scheduled run. A workflow that cannot run
// is reported and leaves the rest of the schedule alone.
func (scheduler Scheduler) start(ctx context.Context, name string) (runs.Record, bool) {
	request, err := scheduler.Workflows.Load(name)
	if err != nil {
		log.Printf("scheduled workflow %s: %v", name, err)
		return runs.Record{}, false
	}
	request.Name = name
	record, execute, err := scheduler.Runs.StartScheduled(request)
	if err != nil {
		log.Printf("scheduled workflow %s: %v", name, err)
		return runs.Record{}, false
	}
	// The run is bound to the scheduler's own context, so stopping the
	// server stops the runs it started.
	go execute(ctx, nil)
	return record, true
}
