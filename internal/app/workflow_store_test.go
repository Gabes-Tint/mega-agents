package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const savedGraph = `{
	"name": "Issue to PR",
	"nodes": [
		{"id": "n1", "type": "project", "name": "api", "x": 10, "y": 20, "w": 400, "h": 300, "path": "/work/api"},
		{"id": "n2", "type": "agent", "name": "Planner", "x": 20, "y": 40, "w": 160, "h": 64, "parentId": "n1",
		 "start": true, "backend": "claude", "prompt": "Plan"}
	],
	"edges": []
}`

func workflowRequest(t *testing.T, handler http.Handler, method string, path string, body string, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func workflowHandler(t *testing.T) (http.Handler, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("MEGA_AGENTS_HOME", home)
	return NewHandler(testAssets()), home
}

func TestSavedWorkflowsAreYAMLFilesThatOpenAgain(t *testing.T) {
	handler, home := workflowHandler(t)

	saved := workflowRequest(t, handler, http.MethodPut, "/api/workflows/issue-to-pr", savedGraph, "application/json")

	if saved.Code != http.StatusOK {
		t.Fatalf("save = %d %s", saved.Code, saved.Body.String())
	}
	file, err := os.ReadFile(filepath.Join(home, "workflows", "issue-to-pr.yaml"))
	if err != nil || !strings.Contains(string(file), "kind: Workflow") || !strings.Contains(string(file), `prompt: "Plan"`) {
		t.Fatalf("file = %q, %v", file, err)
	}
	opened := workflowRequest(t, handler, http.MethodGet, "/api/workflows/issue-to-pr", "", "")
	var request WorkflowRequest
	if err := json.NewDecoder(opened.Body).Decode(&request); err != nil {
		t.Fatal(err)
	}
	if opened.Code != http.StatusOK || request.Name != "issue-to-pr" || len(request.Nodes) != 2 ||
		request.Nodes[1].Name != "Planner" || request.Nodes[1].ParentID != "api" || request.Nodes[0].W != 400 {
		t.Fatalf("opened = %d %+v", opened.Code, request)
	}
}

func TestASavedWorkflowKeepsAClearedListOfLabelsToIgnore(t *testing.T) {
	handler, _ := workflowHandler(t)
	graph := `{"name": "labels", "nodes": [
		{"id": "p1", "type": "project", "name": "api", "w": 400, "h": 300},
		{"id": "g1", "type": "github", "name": "GitHub 1", "parentId": "p1", "w": 200, "h": 240},
		{"id": "i1", "type": "action", "action": "issue", "name": "Cleared", "parentId": "g1", "issue": 8, "ignoreLabels": []},
		{"id": "i2", "type": "action", "action": "issue", "name": "Old", "parentId": "g1", "issue": 9}
	], "edges": []}`

	if saved := workflowRequest(t, handler, http.MethodPut, "/api/workflows/labels", graph, "application/json"); saved.Code != http.StatusOK {
		t.Fatalf("save = %d %s", saved.Code, saved.Body.String())
	}
	opened := workflowRequest(t, handler, http.MethodGet, "/api/workflows/labels", "", "").Body.String()

	// The editor tells a cleared list from a missing one, which means the defaults.
	if strings.Count(opened, `"ignoreLabels"`) != 1 || !strings.Contains(opened, `"ignoreLabels":[]`) {
		t.Fatalf("opened = %s", opened)
	}
}

func TestSavedWorkflowsAreListedByName(t *testing.T) {
	handler, _ := workflowHandler(t)
	for _, name := range []string{"nightly", "issue-to-pr"} {
		if response := workflowRequest(t, handler, http.MethodPut, "/api/workflows/"+name, savedGraph, "application/json"); response.Code != http.StatusOK {
			t.Fatalf("save %s = %d", name, response.Code)
		}
	}

	listed := workflowRequest(t, handler, http.MethodGet, "/api/workflows", "", "")

	var workflows []WorkflowSummary
	if err := json.NewDecoder(listed.Body).Decode(&workflows); err != nil {
		t.Fatal(err)
	}
	if len(workflows) != 2 || workflows[0].Name != "issue-to-pr" || workflows[1].Name != "nightly" || workflows[0].UpdatedAt == "" {
		t.Fatalf("workflows = %+v", workflows)
	}
}

func TestWorkflowStoreRefusesBadRequests(t *testing.T) {
	handler, _ := workflowHandler(t)
	cases := map[string]struct {
		method, path, body, contentType string
		code                            int
		want                            string
	}{
		"unknown":          {http.MethodGet, "/api/workflows/missing", "", "", http.StatusNotFound, "workflow missing not found"},
		"bad name":         {http.MethodPut, "/api/workflows/Bad%20Name", savedGraph, "application/json", http.StatusBadRequest, "use lowercase letters, digits and -"},
		"not json":         {http.MethodPut, "/api/workflows/x", savedGraph, "text/plain", http.StatusUnsupportedMediaType, "JSON"},
		"invalid graph":    {http.MethodPut, "/api/workflows/x", `{"nodes": [{"id": "a", "type": "agent", "name": "A"}]}`, "application/json", http.StatusBadRequest, "agent cannot be placed inside root"},
		"malformed":        {http.MethodPut, "/api/workflows/x", `{`, "application/json", http.StatusBadRequest, "invalid workflow request"},
		"import not yaml":  {http.MethodPost, "/api/workflows/import", "kind: Other", "application/yaml", http.StatusBadRequest, `kind "Other" is not Workflow`},
		"import bad graph": {http.MethodPost, "/api/workflows/import", "apiVersion: megaagents.dev/v1alpha1\nkind: Workflow\nnodes:\n  a:\n    uses: agent@v1\n", "application/yaml", http.StatusBadRequest, "agent cannot be placed inside root"},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			response := workflowRequest(t, handler, testCase.method, testCase.path, testCase.body, testCase.contentType)

			if response.Code != testCase.code || !strings.Contains(response.Body.String(), testCase.want) {
				t.Fatalf("response = %d %q, want %d %q", response.Code, response.Body.String(), testCase.code, testCase.want)
			}
		})
	}
}

func TestImportTurnsWorkflowYAMLIntoTheEditorsGraph(t *testing.T) {
	handler, _ := workflowHandler(t)
	var source WorkflowRequest
	if err := json.Unmarshal([]byte(savedGraph), &source); err != nil {
		t.Fatal(err)
	}
	yaml, err := BuildWorkflowYAML(source)
	if err != nil {
		t.Fatal(err)
	}

	response := workflowRequest(t, handler, http.MethodPost, "/api/workflows/import", yaml, "application/yaml")

	var request WorkflowRequest
	if err := json.NewDecoder(response.Body).Decode(&request); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || request.Name != "issue-to-pr" || request.Nodes[1].Prompt != "Plan" {
		t.Fatalf("import = %d %+v", response.Code, request)
	}
}
