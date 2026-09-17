package app

import (
	"encoding/json"
	"net/http"
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

	if listed.Code != http.StatusOK || templates[0].Name != "gate-and-fix" {
		t.Fatalf("list = %d %+v", listed.Code, templates)
	}
	if opened.Code != http.StatusOK || !strings.Contains(opened.Body.String(), `"name":"Implementer"`) {
		t.Fatalf("open = %d %s", opened.Code, opened.Body.String())
	}
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing = %d", missing.Code)
	}
}
