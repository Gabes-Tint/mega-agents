package app

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
)

func TestEveryTemplateIsAWorkflowThatPlans(t *testing.T) {
	templates, err := ListTemplates()
	if err != nil {
		t.Fatal(err)
	}
	if len(templates) < 3 {
		t.Fatalf("templates = %+v", templates)
	}
	for _, template := range templates {
		t.Run(template.Name, func(t *testing.T) {
			if template.Description == "" || template.Title == "" {
				t.Fatalf("template = %+v, want a title and a description", template)
			}
			request, err := LoadTemplate(template.Name)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if _, err := planRun(request); err != nil {
				t.Fatalf("the template does not plan: %v", err)
			}
		})
	}
}

func TestTemplatesAreServedToTheEditor(t *testing.T) {
	handler, _ := workflowHandler(t)

	listed := workflowRequest(t, handler, http.MethodGet, "/api/templates", "", "")
	var templates []TemplateSummary
	if err := json.NewDecoder(listed.Body).Decode(&templates); err != nil {
		t.Fatal(err)
	}
	opened := workflowRequest(t, handler, http.MethodGet, "/api/templates/issue-to-pull-request", "", "")
	missing := workflowRequest(t, handler, http.MethodGet, "/api/templates/nope", "", "")

	names := []string{}
	for _, template := range templates {
		names = append(names, template.Name)
	}
	if listed.Code != http.StatusOK || !slices.Contains(names, "gate-and-fix") || !slices.Contains(names, "fit-development-flow") {
		t.Fatalf("list = %d %+v", listed.Code, templates)
	}
	if opened.Code != http.StatusOK || !strings.Contains(opened.Body.String(), `"name":"Implementer"`) {
		t.Fatalf("open = %d %s", opened.Code, opened.Body.String())
	}
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing = %d", missing.Code)
	}
}

func TestTheFitDevelopmentFlowTemplateHasNoErrors(t *testing.T) {
	request, err := LoadTemplate("fit-development-flow")
	if err != nil {
		t.Fatal(err)
	}

	for _, problem := range CheckWorkflow(request) {
		if problem.Severity == SeverityError || !strings.Contains(problem.Message, "set the folder") {
			t.Errorf("problem: %+v", problem)
		}
	}
	names := map[string]bool{}
	for _, node := range request.Nodes {
		names[node.Name] = true
	}
	for _, step := range []string{
		"Sync", "Pick story", "Whose call", "Hand to Gabriel", "Hold", "Slice at layer boundary", "Write failing tests",
		"Delegate", "Capability rung", "Mechanic implements", "Builder implements", "Solver implements", "Final barrier",
		"Freeze", "Open pull request", "Review rounds", "CI fix rounds", "Merge", "QA deploy", "Main CI green",
		"Production deploy", "Cleanup", "Mark blocked",
	} {
		if !names[step] {
			t.Errorf("the template lacks %q", step)
		}
	}
}
