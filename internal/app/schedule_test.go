package app

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSavedScheduleTravelsWithTheWorkflowFile(t *testing.T) {
	handler, home := workflowHandler(t)
	graph := strings.Replace(savedGraph, `"name": "Issue to PR",`, `"name": "Issue to PR", "schedule": "0 9 * * 1-5",`, 1)

	saved := workflowRequest(t, handler, http.MethodPut, "/api/workflows/issue-to-pr", graph, "application/json")
	if saved.Code != http.StatusOK {
		t.Fatalf("save = %d %s", saved.Code, saved.Body.String())
	}

	file, err := os.ReadFile(filepath.Join(home, "workflows", "issue-to-pr.yaml"))
	if err != nil || !strings.Contains(string(file), `schedule: "0 9 * * 1-5"`) {
		t.Fatalf("file = %q, %v", file, err)
	}
	opened := workflowRequest(t, handler, http.MethodGet, "/api/workflows/issue-to-pr", "", "")
	var request WorkflowRequest
	if err := json.NewDecoder(opened.Body).Decode(&request); err != nil {
		t.Fatal(err)
	}
	if request.Schedule != "0 9 * * 1-5" {
		t.Fatalf("opened schedule = %q", request.Schedule)
	}
}

func TestSavingRejectsAScheduleThatCannotRun(t *testing.T) {
	handler, _ := workflowHandler(t)
	graph := strings.Replace(savedGraph, `"name": "Issue to PR",`, `"name": "Issue to PR", "schedule": "every friday",`, 1)

	saved := workflowRequest(t, handler, http.MethodPut, "/api/workflows/issue-to-pr", graph, "application/json")

	if saved.Code != http.StatusBadRequest {
		t.Fatalf("save = %d %s", saved.Code, saved.Body.String())
	}
}

func TestListedWorkflowsCarryTheirScheduleAndNextRuns(t *testing.T) {
	handler, _ := workflowHandler(t)
	scheduled := strings.Replace(savedGraph, `"name": "Issue to PR",`, `"name": "Issue to PR", "schedule": "@daily",`, 1)
	workflowRequest(t, handler, http.MethodPut, "/api/workflows/nightly", scheduled, "application/json")
	workflowRequest(t, handler, http.MethodPut, "/api/workflows/on-demand", savedGraph, "application/json")

	listed := workflowRequest(t, handler, http.MethodGet, "/api/workflows", "", "")

	var workflows []WorkflowSummary
	if err := json.NewDecoder(listed.Body).Decode(&workflows); err != nil {
		t.Fatal(err)
	}
	if len(workflows) != 2 {
		t.Fatalf("workflows = %+v", workflows)
	}
	nightly, onDemand := workflows[0], workflows[1]
	if nightly.Name != "nightly" || nightly.Schedule != "@daily" || len(nightly.NextRuns) != 3 {
		t.Fatalf("nightly = %+v", nightly)
	}
	if onDemand.Schedule != "" || len(onDemand.NextRuns) != 0 {
		t.Fatalf("on-demand = %+v", onDemand)
	}
	for _, run := range nightly.NextRuns {
		if _, err := time.Parse(time.RFC3339, run); err != nil {
			t.Fatalf("next run %q: %v", run, err)
		}
	}
}

func TestSettingAScheduleKeepsTheWorkflowItIsOn(t *testing.T) {
	handler, _ := workflowHandler(t)
	workflowRequest(t, handler, http.MethodPut, "/api/workflows/nightly", savedGraph, "application/json")

	set := workflowRequest(
		t, handler, http.MethodPut, "/api/workflows/nightly/schedule",
		`{"schedule": "15 3 * * *"}`, "application/json",
	)

	if set.Code != http.StatusOK {
		t.Fatalf("set = %d %s", set.Code, set.Body.String())
	}
	var summary WorkflowSummary
	if err := json.NewDecoder(set.Body).Decode(&summary); err != nil {
		t.Fatal(err)
	}
	if summary.Schedule != "15 3 * * *" || len(summary.NextRuns) != 3 {
		t.Fatalf("summary = %+v", summary)
	}
	opened := workflowRequest(t, handler, http.MethodGet, "/api/workflows/nightly", "", "")
	var request WorkflowRequest
	if err := json.NewDecoder(opened.Body).Decode(&request); err != nil {
		t.Fatal(err)
	}
	if request.Schedule != "15 3 * * *" || len(request.Nodes) != 2 {
		t.Fatalf("opened = %+v", request)
	}
}

func TestClearingAScheduleStopsTheWorkflowRunningOnItsOwn(t *testing.T) {
	handler, _ := workflowHandler(t)
	scheduled := strings.Replace(savedGraph, `"name": "Issue to PR",`, `"name": "Issue to PR", "schedule": "@daily",`, 1)
	workflowRequest(t, handler, http.MethodPut, "/api/workflows/nightly", scheduled, "application/json")

	cleared := workflowRequest(
		t, handler, http.MethodPut, "/api/workflows/nightly/schedule", `{"schedule": ""}`, "application/json",
	)

	if cleared.Code != http.StatusOK {
		t.Fatalf("clear = %d %s", cleared.Code, cleared.Body.String())
	}
	opened := workflowRequest(t, handler, http.MethodGet, "/api/workflows/nightly", "", "")
	var request WorkflowRequest
	if err := json.NewDecoder(opened.Body).Decode(&request); err != nil {
		t.Fatal(err)
	}
	if request.Schedule != "" {
		t.Fatalf("schedule = %q, want it cleared", request.Schedule)
	}
}

func TestSettingAScheduleReportsOneThatCannotRun(t *testing.T) {
	handler, _ := workflowHandler(t)
	workflowRequest(t, handler, http.MethodPut, "/api/workflows/nightly", savedGraph, "application/json")

	set := workflowRequest(
		t, handler, http.MethodPut, "/api/workflows/nightly/schedule", `{"schedule": "99 * * * *"}`, "application/json",
	)

	if set.Code != http.StatusBadRequest || !strings.Contains(set.Body.String(), "minute") {
		t.Fatalf("set = %d %s", set.Code, set.Body.String())
	}
}

func TestSettingAScheduleOnAWorkflowThatIsNotThere(t *testing.T) {
	handler, _ := workflowHandler(t)

	set := workflowRequest(
		t, handler, http.MethodPut, "/api/workflows/missing/schedule", `{"schedule": "@daily"}`, "application/json",
	)

	if set.Code != http.StatusNotFound {
		t.Fatalf("set = %d %s", set.Code, set.Body.String())
	}
}

func TestSchedulePreviewShowsTheNextThreeRuns(t *testing.T) {
	handler, _ := workflowHandler(t)

	preview := workflowRequest(t, handler, http.MethodGet, "/api/schedule/preview?schedule=0+9+*+*+*", "", "")

	if preview.Code != http.StatusOK {
		t.Fatalf("preview = %d %s", preview.Code, preview.Body.String())
	}
	var answer SchedulePreview
	if err := json.NewDecoder(preview.Body).Decode(&answer); err != nil {
		t.Fatal(err)
	}
	if len(answer.NextRuns) != 3 {
		t.Fatalf("preview = %+v", answer)
	}
	previous := time.Now()
	for _, run := range answer.NextRuns {
		moment, err := time.Parse(time.RFC3339, run)
		if err != nil {
			t.Fatalf("next run %q: %v", run, err)
		}
		if !moment.After(previous) {
			t.Fatalf("runs must come in order and in the future: %v", answer.NextRuns)
		}
		if moment.Hour() != 9 || moment.Minute() != 0 {
			t.Fatalf("run %s is not 9am", run)
		}
		previous = moment
	}
}

func TestSchedulePreviewExplainsAnExpressionItCannotRun(t *testing.T) {
	handler, _ := workflowHandler(t)

	preview := workflowRequest(t, handler, http.MethodGet, "/api/schedule/preview?schedule=0+9+*+*", "", "")

	if preview.Code != http.StatusBadRequest || !strings.Contains(preview.Body.String(), "five fields") {
		t.Fatalf("preview = %d %s", preview.Code, preview.Body.String())
	}
}

// An expression that never comes round parses, so the preview answers with
// no runs rather than an error.
func TestSchedulePreviewAnswersNoRunsForADateThatNeverComes(t *testing.T) {
	handler, _ := workflowHandler(t)

	preview := workflowRequest(t, handler, http.MethodGet, "/api/schedule/preview?schedule=0+0+30+2+*", "", "")

	var answer SchedulePreview
	if err := json.NewDecoder(preview.Body).Decode(&answer); err != nil {
		t.Fatal(err)
	}
	if preview.Code != http.StatusOK || len(answer.NextRuns) != 0 {
		t.Fatalf("preview = %d %+v", preview.Code, answer)
	}
}
