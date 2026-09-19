package app

import (
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/Gabes-Tint/mega-agents/internal/cron"
)

// previewRuns is how many runs ahead a schedule is shown, both while an
// expression is being typed and in the list of saved workflows.
const previewRuns = 3

// SchedulePreview answers when an expression would next run. The times are
// written in the server's own zone, which is the zone the scheduler reads
// the expression in, so what is shown is what will happen.
type SchedulePreview struct {
	Schedule string   `json:"schedule"`
	NextRuns []string `json:"nextRuns"`
}

// ScheduleRequest sets, or with an empty expression clears, the schedule a
// saved workflow runs itself on.
type ScheduleRequest struct {
	Schedule string `json:"schedule"`
}

// nextRuns is when the expression runs next, as RFC 3339 times in the
// server's zone. An expression that cannot run, or names a date that never
// comes round, has no runs coming.
func nextRuns(schedule string, from time.Time) []string {
	parsed, err := cron.Parse(schedule)
	if err != nil {
		return nil
	}
	runs := []string{}
	for _, moment := range parsed.NextRuns(from, previewRuns) {
		runs = append(runs, moment.Format(time.RFC3339))
	}
	return runs
}

func registerScheduleHandler(mux *http.ServeMux, store WorkflowStore) {
	// The editor asks for this on every keystroke, so the expression is
	// read here rather than a second time in the browser: what the page
	// shows is then what the scheduler will do, in the server's own zone.
	mux.HandleFunc("GET /api/schedule/preview", func(w http.ResponseWriter, r *http.Request) {
		schedule := strings.TrimSpace(r.URL.Query().Get("schedule"))
		parsed, err := cron.Parse(schedule)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		runs := []string{}
		for _, moment := range parsed.NextRuns(time.Now(), previewRuns) {
			runs = append(runs, moment.Format(time.RFC3339))
		}
		writeJSON(w, http.StatusOK, SchedulePreview{Schedule: parsed.String(), NextRuns: runs})
	})

	// Setting a schedule writes the workflow file, so like saving it only
	// accepts JSON, which browsers must preflight cross-origin.
	mux.HandleFunc("PUT /api/workflows/{name}/schedule", func(w http.ResponseWriter, r *http.Request) {
		mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if mediaType != "application/json" {
			http.Error(w, "setting a schedule requires a JSON request", http.StatusUnsupportedMediaType)
			return
		}
		var request ScheduleRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, workflowMaxBodyBytes)).Decode(&request); err != nil {
			http.Error(w, "invalid schedule request", http.StatusBadRequest)
			return
		}
		name := r.PathValue("name")
		summary, err := store.SetSchedule(name, request.Schedule)
		if errors.Is(err, errWorkflowNotFound) {
			http.Error(w, "workflow "+name+" not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, summary)
	})
}
