package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func problemsProject(path string) WorkflowNodeInput {
	return WorkflowNodeInput{ID: "p1", Type: "project", Name: "api", Path: path}
}

func problemsAgent(id string, name string, prompt string, start bool) WorkflowNodeInput {
	return WorkflowNodeInput{
		ID: id, Type: "agent", Name: name, ParentID: "p1", Backend: "claude", Prompt: prompt, Start: start,
	}
}

func TestAnEmptyWorkflowHasNoProblems(t *testing.T) {
	t.Parallel()

	if got := CheckWorkflow(WorkflowRequest{}); len(got) != 0 {
		t.Fatalf("problems = %v, want none", got)
	}
}

func TestARunnableWorkflowHasNoProblems(t *testing.T) {
	t.Parallel()

	got := CheckWorkflow(WorkflowRequest{Nodes: []WorkflowNodeInput{
		problemsProject("/work/api"),
		problemsAgent("a1", "Coder", "fix it", true),
	}})

	if len(got) != 0 {
		t.Fatalf("problems = %v, want none", got)
	}
}

func TestProblemsListEveryBlockToFixNotOnlyTheFirst(t *testing.T) {
	t.Parallel()

	got := CheckWorkflow(WorkflowRequest{Nodes: []WorkflowNodeInput{
		problemsProject("/work/api"),
		problemsAgent("a1", "Coder", "", true),
		problemsAgent("a2", "Reviewer", "", true),
		{ID: "c1", Type: "command", Name: "Tests", ParentID: "p1"},
	}, Edges: []WorkflowEdgeInput{{ID: "e1", From: "a1", To: "c1"}}})

	want := []Problem{
		{NodeID: "a1", Severity: SeverityError, Message: "Coder: write the prompt the agent receives"},
		{NodeID: "a2", Severity: SeverityError, Message: "Reviewer: write the prompt the agent receives"},
		{NodeID: "c1", Severity: SeverityError, Message: "Tests: write the command to run"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("problems = %#v\nwant %#v", got, want)
	}
}

func TestAWorkflowWithoutAStartingPointCannotRun(t *testing.T) {
	t.Parallel()

	got := CheckWorkflow(WorkflowRequest{Nodes: []WorkflowNodeInput{
		problemsProject("/work/api"),
		problemsAgent("a1", "Coder", "fix it", false),
	}})

	want := []Problem{{Severity: SeverityError, Message: errNoStartingBlock.Error()}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("problems = %#v\nwant %#v", got, want)
	}
}

func TestABlockNoStartingPointReachesIsAWarning(t *testing.T) {
	t.Parallel()

	got := CheckWorkflow(WorkflowRequest{Nodes: []WorkflowNodeInput{
		problemsProject("/work/api"),
		problemsAgent("a1", "Coder", "fix it", true),
		problemsAgent("a2", "Stray", "", false),
	}})

	want := []Problem{{
		NodeID: "a2", Severity: SeverityWarning,
		Message: "Stray does not run: no starting point leads to it",
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("problems = %#v\nwant %#v", got, want)
	}
}

func TestAProjectWithoutAPathIsAWarning(t *testing.T) {
	t.Parallel()

	got := CheckWorkflow(WorkflowRequest{Nodes: []WorkflowNodeInput{
		problemsProject(""),
		problemsAgent("a1", "Coder", "fix it", true),
	}})

	want := []Problem{{
		NodeID: "p1", Severity: SeverityWarning,
		Message: "api: set the folder its blocks work in",
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("problems = %#v\nwant %#v", got, want)
	}
}

func TestAGraphTheEditorCouldNotBuildIsOneProblem(t *testing.T) {
	t.Parallel()

	got := CheckWorkflow(WorkflowRequest{Nodes: []WorkflowNodeInput{{ID: "x", Type: "spaceship", Name: "X"}}})

	want := []Problem{{Severity: SeverityError, Message: `unknown component type "spaceship"`}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("problems = %#v\nwant %#v", got, want)
	}
}

func TestProblemsEndpoint(t *testing.T) {
	t.Parallel()
	handler := NewHandler(testAssets())
	post := func(body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/workflows/problems", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}

	response := post(`{"nodes": [{"id": "p1", "type": "project", "name": "api", "path": "/work/api"},
		{"id": "a1", "type": "agent", "name": "Coder", "parentId": "p1", "backend": "claude", "start": true}]}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body)
	}
	var problems []Problem
	if err := json.NewDecoder(response.Body).Decode(&problems); err != nil {
		t.Fatal(err)
	}
	if len(problems) != 1 || problems[0].NodeID != "a1" || problems[0].Severity != "error" {
		t.Fatalf("problems = %#v", problems)
	}
	if empty := post(`{"nodes": []}`); strings.TrimSpace(empty.Body.String()) != "[]" {
		t.Fatalf("empty workflow body = %q, want []", empty.Body)
	}
	if bad := post(`{`); bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid body status = %d", bad.Code)
	}
}
