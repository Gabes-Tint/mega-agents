package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func workflowBody() string {
	return `{
		"name": "issue-to-pull-request",
		"nodes": [
			{"id": "n1", "type": "project", "name": "my-agent", "x": 30, "y": 20, "w": 160, "h": 64, "path": "/home/user/my-agent"},
			{"id": "n2", "type": "github", "name": "GitHub 1", "x": 10, "y": 10, "w": 160, "h": 64, "parentId": "n1", "repository": "https://github.com/example/project", "secretKey": "secret://github-bot"},
			{"id": "n3", "type": "githubapp", "name": "GitHub App 1", "x": 10, "y": 10, "w": 160, "h": 64, "parentId": "n2", "appId": "123456", "privateKeyPath": "/home/user/.keys/github-app.pem"}
		],
		"edges": [
			{"id": "e1", "from": "n1", "to": "n2"}
		]
	}`
}

func postWorkflow(t *testing.T, handler http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/workflows/yaml", strings.NewReader(body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestWorkflowYAMLOrdering(t *testing.T) {
	handler := NewHandler(testAssets())

	response := postWorkflow(t, handler, workflowBody())

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	disposition := response.Header().Get("Content-Disposition")
	if !strings.Contains(disposition, "attachment") ||
		!strings.Contains(disposition, "issue-to-pull-request.yaml") {
		t.Fatalf("expected attachment disposition, got %q", disposition)
	}
	body := response.Body.String()
	expectations := []string{
		"apiVersion: megaagents.dev/v1alpha1",
		"kind: Workflow",
		"  name: issue-to-pull-request",
		"  my-agent:",
		"    uses: project@v1",
		`      path: "/home/user/my-agent"`,
		"    children:",
		"      github-1:",
		"        uses: github@v1",
		`        repository: "https://github.com/example/project"`,
		`        secretKey: "secret://github-bot"`,
		"        children:",
		"          github-app-1:",
		"            uses: githubapp@v1",
		`            appId: "123456"`,
		`            privateKeyPath: "/home/user/.keys/github-app.pem"`,
		"        needs:",
		"          - my-agent",
	}
	for _, want := range expectations {
		if !strings.Contains(body, want) {
			t.Errorf("expected YAML to contain %q, got:\n%s", want, body)
		}
	}
}

func TestWorkflowYAMLIsDeterministic(t *testing.T) {
	handler := NewHandler(testAssets())

	first := postWorkflow(t, handler, workflowBody()).Body.String()
	second := postWorkflow(t, handler, workflowBody()).Body.String()

	if first != second {
		t.Fatalf("expected identical YAML for identical graphs, got:\n%s\n---\n%s", first, second)
	}
}

func TestWorkflowYAMLRejectsUnknownComponentTypes(t *testing.T) {
	handler := NewHandler(testAssets())

	response := postWorkflow(t, handler, `{
		"nodes": [{"id": "n1", "type": "mystery", "name": "Mystery 1", "x": 0, "y": 0, "w": 160, "h": 64}]
	}`)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "unknown component type") {
		t.Fatalf("expected unknown component type error, got %q", response.Body.String())
	}
}

func TestWorkflowYAMLDefaultsWorkflowName(t *testing.T) {
	handler := NewHandler(testAssets())

	response := postWorkflow(t, handler, `{
		"nodes": [{"id": "n1", "type": "project", "name": "Project 1", "x": 0, "y": 0, "w": 160, "h": 64}]
	}`)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	if !strings.Contains(response.Body.String(), "  name: workflow") {
		t.Errorf("expected default workflow name, got:\n%s", response.Body.String())
	}
	disposition := response.Header().Get("Content-Disposition")
	if !strings.Contains(disposition, "workflow.yaml") {
		t.Errorf("expected default filename, got %q", disposition)
	}
}

func TestWorkflowYAMLResolvesEdgesDeclaredBeforeTheirTarget(t *testing.T) {
	handler := NewHandler(testAssets())

	// The edge points from the first project (declared first) to the second
	// project (declared last), so identifiers must exist before needs are
	// emitted.
	response := postWorkflow(t, handler, `{
		"nodes": [
			{"id": "n1", "type": "project", "name": "alpha", "x": 0, "y": 0, "w": 160, "h": 64},
			{"id": "n2", "type": "project", "name": "my-agent", "x": 400, "y": 300, "w": 160, "h": 64}
		],
		"edges": [{"id": "e1", "from": "n1", "to": "n2"}]
	}`)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	body := response.Body.String()
	if !strings.Contains(body, "  my-agent:\n    uses: project@v1\n    needs:\n      - alpha\n") {
		t.Errorf("expected needs on the later node, got:\n%s", body)
	}
}

func TestWorkflowYAMLRejectsEdgesWithUnknownEndpoints(t *testing.T) {
	handler := NewHandler(testAssets())

	response := postWorkflow(t, handler, `{
		"nodes": [{"id": "n1", "type": "project", "name": "my-agent", "x": 0, "y": 0, "w": 160, "h": 64}],
		"edges": [{"id": "e1", "from": "n1", "to": "missing"}]
	}`)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", response.Code, response.Body.String())
	}
}

func TestWorkflowYAMLRejectsEmptyGraphs(t *testing.T) {
	handler := NewHandler(testAssets())

	response := postWorkflow(t, handler, `{"nodes": [], "edges": []}`)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", response.Code, response.Body.String())
	}
}

func TestWorkflowYAMLRejectsDuplicateNodeIDs(t *testing.T) {
	handler := NewHandler(testAssets())

	response := postWorkflow(t, handler, `{
		"nodes": [
			{"id": "n1", "type": "project", "name": "Project 1", "x": 0, "y": 0, "w": 160, "h": 64},
			{"id": "n1", "type": "project", "name": "Project 2", "x": 10, "y": 0, "w": 160, "h": 64}
		]
	}`)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", response.Code, response.Body.String())
	}
}

func TestWorkflowYAMLKeepsCollisionSuffixesUnique(t *testing.T) {
	handler := NewHandler(testAssets())

	response := postWorkflow(t, handler, `{
		"nodes": [
			{"id": "n1", "type": "project", "name": "A", "x": 0, "y": 0, "w": 160, "h": 64},
			{"id": "n2", "type": "project", "name": "A", "x": 10, "y": 0, "w": 160, "h": 64},
			{"id": "n3", "type": "project", "name": "A 2", "x": 20, "y": 0, "w": 160, "h": 64}
		]
	}`)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	body := response.Body.String()
	if strings.Count(body, "  a:\n") != 1 ||
		strings.Count(body, "  a-2:\n") != 1 ||
		strings.Count(body, "  a-2-2:\n") != 1 {
		t.Errorf("expected three distinct identifiers, got:\n%s", body)
	}
}

func TestWorkflowYAMLQuotesValuesWithSpecialCharacters(t *testing.T) {
	handler := NewHandler(testAssets())

	response := postWorkflow(t, handler, `{
		"nodes": [{"id": "n1", "type": "project", "name": "Project 1", "x": 0, "y": 0, "w": 160, "h": 64, "path": "C:\\home"}]
	}`)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	if !strings.Contains(response.Body.String(), `      path: "C:\\home"`) {
		t.Errorf("expected single-escaped backslash, got:\n%s", response.Body.String())
	}
}

func TestWorkflowYAMLNestsChildrenInsideParents(t *testing.T) {
	handler := NewHandler(testAssets())

	response := postWorkflow(t, handler, `{
		"nodes": [
			{"id": "n0", "type": "project", "name": "my-agent", "x": 0, "y": 0, "w": 160, "h": 64},
			{"id": "n1", "type": "githubapp", "name": "GitHub App 1", "x": 10, "y": 10, "w": 160, "h": 64, "parentId": "n2"},
			{"id": "n2", "type": "github", "name": "GitHub 1", "x": 10, "y": 10, "w": 160, "h": 64, "parentId": "n0"}
		]
	}`)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	body := response.Body.String()
	if strings.Contains(body, "parent:") {
		t.Errorf("expected no parent tags, got:\n%s", body)
	}
	if !strings.Contains(
		body,
		"      github-1:\n        uses: github@v1\n        layout:\n          x: 10\n          y: 10\n          w: 160\n          h: 64\n        children:\n          github-app-1:\n            uses: githubapp@v1\n",
	) {
		t.Errorf("expected the app nested inside its GitHub object, got:\n%s", body)
	}
}

func TestWorkflowYAMLNestsDeeplyStackedContainers(t *testing.T) {
	handler := NewHandler(testAssets())

	response := postWorkflow(t, handler, `{
		"nodes": [
			{"id": "n1", "type": "project", "name": "my-agent", "x": 0, "y": 0, "w": 160, "h": 64},
			{"id": "n2", "type": "github", "name": "GitHub 1", "x": 10, "y": 10, "w": 160, "h": 64, "parentId": "n1"},
			{"id": "n3", "type": "githubapp", "name": "GitHub App 1", "x": 5, "y": 5, "w": 160, "h": 64, "parentId": "n2"}
		]
	}`)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(
		body,
		"  my-agent:\n    uses: project@v1\n    layout:\n      x: 0\n      y: 0\n      w: 160\n      h: 64\n    children:\n      github-1:\n        uses: github@v1\n        layout:\n          x: 10\n          y: 10\n          w: 160\n          h: 64\n        children:\n          github-app-1:\n            uses: githubapp@v1\n",
	) {
		t.Errorf("expected the app nested inside a GitHub inside the project, got:\n%s", body)
	}
}

func TestWorkflowYAMLRejectsGateBaseAsDisplayableType(t *testing.T) {
	handler := NewHandler(testAssets())

	response := postWorkflow(t, handler, `{
		"nodes": [{"id": "n1", "type": "gatebase", "name": "GateBase 1", "x": 0, "y": 0, "w": 160, "h": 64}]
	}`)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", response.Code, response.Body.String())
	}
}

func TestWorkflowYAMLRejectsTopLevelGitHubApp(t *testing.T) {
	handler := NewHandler(testAssets())

	response := postWorkflow(t, handler, `{
		"nodes": [{"id": "n1", "type": "githubapp", "name": "GitHub App 1", "x": 0, "y": 0, "w": 160, "h": 64}]
	}`)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", response.Code, response.Body.String())
	}
}

func TestWorkflowYAMLRejectsAgentAtTheRoot(t *testing.T) {
	handler := NewHandler(testAssets())

	response := postWorkflow(t, handler, `{
		"nodes": [{"id": "n1", "type": "agent", "name": "Agent 1", "x": 0, "y": 0, "w": 160, "h": 64}]
	}`)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", response.Code, response.Body.String())
	}
}

func TestWorkflowYAMLRejectsGitHubAtTheRoot(t *testing.T) {
	handler := NewHandler(testAssets())

	response := postWorkflow(t, handler, `{
		"nodes": [{"id": "n1", "type": "github", "name": "GitHub 1", "x": 0, "y": 0, "w": 160, "h": 64}]
	}`)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", response.Code, response.Body.String())
	}
}

func TestWorkflowYAMLRejectsAgentInsideGitHub(t *testing.T) {
	handler := NewHandler(testAssets())

	response := postWorkflow(t, handler, `{
		"nodes": [
			{"id": "n1", "type": "project", "name": "my-agent", "x": 0, "y": 0, "w": 160, "h": 64},
			{"id": "n2", "type": "github", "name": "GitHub 1", "x": 10, "y": 10, "w": 160, "h": 64, "parentId": "n1"},
			{"id": "n3", "type": "agent", "name": "Agent 1", "x": 10, "y": 10, "w": 160, "h": 64, "parentId": "n2"}
		]
	}`)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", response.Code, response.Body.String())
	}
}

func TestWorkflowYAMLRejectsGitHubAppOutsideGitHub(t *testing.T) {
	handler := NewHandler(testAssets())

	response := postWorkflow(t, handler, `{
		"nodes": [
			{"id": "n1", "type": "project", "name": "my-agent", "x": 0, "y": 0, "w": 160, "h": 64},
			{"id": "n2", "type": "githubapp", "name": "GitHub App 1", "x": 10, "y": 10, "w": 160, "h": 64, "parentId": "n1"}
		]
	}`)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", response.Code, response.Body.String())
	}
}

func TestWorkflowYAMLNestsForgeInsideProjectWithIndentedConfig(t *testing.T) {
	handler := NewHandler(testAssets())

	response := postWorkflow(t, handler, `{
		"nodes": [
			{"id": "n1", "type": "project", "name": "my-agent", "x": 0, "y": 0, "w": 160, "h": 64},
			{"id": "n2", "type": "gitlab", "name": "GitLab 1", "x": 20, "y": 10, "w": 160, "h": 64, "parentId": "n1", "repository": "https://gitlab.com/example/project", "secretKey": "secret://gitlab-bot"}
		]
	}`)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	body := response.Body.String()
	if !strings.Contains(
		body,
		"    children:\n      gitlab-1:\n        uses: gitlab@v1\n        with:\n          repository: \"https://gitlab.com/example/project\"\n",
	) {
		t.Errorf("expected forge config indented inside the nested object, got:\n%s", body)
	}
}

func TestWorkflowYAMLKeepsNeedsOnNestedNodes(t *testing.T) {
	handler := NewHandler(testAssets())

	// Two GitHub Apps inside one GitHub box are on the same level, so they
	// may be connected; the nested node must keep its needs entry.
	response := postWorkflow(t, handler, `{
		"nodes": [
			{"id": "n0", "type": "project", "name": "my-agent", "x": 0, "y": 0, "w": 160, "h": 64},
			{"id": "n1", "type": "github", "name": "GitHub 1", "x": 10, "y": 10, "w": 160, "h": 64, "parentId": "n0"},
			{"id": "n2", "type": "githubapp", "name": "GitHub App 1", "x": 10, "y": 10, "w": 160, "h": 64, "parentId": "n1"},
			{"id": "n3", "type": "githubapp", "name": "GitHub App 2", "x": 20, "y": 20, "w": 160, "h": 64, "parentId": "n1"}
		],
		"edges": [{"id": "e1", "from": "n2", "to": "n3"}]
	}`)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	body := response.Body.String()
	if !strings.Contains(
		body,
		"          github-app-2:\n            uses: githubapp@v1\n            needs:\n              - github-app-1\n",
	) {
		t.Errorf("expected the nested node to keep its needs, got:\n%s", body)
	}
}

func TestWorkflowYAMLExportsTheAuthenticatedForgeFlag(t *testing.T) {
	handler := NewHandler(testAssets())

	response := postWorkflow(t, handler, `{
		"nodes": [
			{"id": "n1", "type": "project", "name": "api", "x": 0, "y": 0, "w": 160, "h": 64},
			{"id": "n2", "type": "github", "name": "GitHub 1", "x": 20, "y": 10, "w": 160, "h": 64, "parentId": "n1", "repository": "acme/api", "authenticated": true},
			{"id": "n3", "type": "gitlab", "name": "GitLab 1", "x": 20, "y": 90, "w": 160, "h": 64, "parentId": "n1", "repository": "acme/web"}
		]
	}`)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	body := response.Body.String()
	if !strings.Contains(body, "          repository: \"acme/api\"\n          authenticated: true\n") {
		t.Errorf("expected the GitHub block to export authenticated: true, got:\n%s", body)
	}
	if strings.Count(body, "authenticated:") != 1 {
		t.Errorf("expected only the authenticated block to export the flag, got:\n%s", body)
	}
}
