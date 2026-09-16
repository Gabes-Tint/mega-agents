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
			{"id": "n1", "type": "gatebase", "name": "GateBase 1", "x": 30, "y": 20, "w": 160, "h": 64, "path": "/home/user/my-agent"},
			{"id": "n2", "type": "github", "name": "GitHub 1", "x": 400, "y": 300, "w": 160, "h": 64, "repository": "https://github.com/example/project", "secretKey": "secret://github-bot"},
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
		"  gatebase-1:",
		"    uses: gatebase@v1",
		`      path: "/home/user/my-agent"`,
		"  github-1:",
		"    uses: github@v1",
		`      repository: "https://github.com/example/project"`,
		`      secretKey: "secret://github-bot"`,
		"  github-app-1:",
		"    uses: githubapp@v1",
		"    parent: github-1",
		`      appId: "123456"`,
		`      privateKeyPath: "/home/user/.keys/github-app.pem"`,
		"    needs:",
		"      - gatebase-1",
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
		"nodes": [{"id": "n1", "type": "agent", "name": "Agent 1", "x": 0, "y": 0, "w": 160, "h": 64}]
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

	// The edge points from the GitHub box (declared first) to the GateBase
	// box (declared last), so identifiers must exist before needs are emitted.
	response := postWorkflow(t, handler, `{
		"nodes": [
			{"id": "n1", "type": "github", "name": "GitHub 1", "x": 0, "y": 0, "w": 160, "h": 64},
			{"id": "n2", "type": "gatebase", "name": "GateBase 1", "x": 400, "y": 300, "w": 160, "h": 64}
		],
		"edges": [{"id": "e1", "from": "n1", "to": "n2"}]
	}`)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	body := response.Body.String()
	if !strings.Contains(body, "  gatebase-1:\n    uses: gatebase@v1\n    needs:\n      - github-1\n") {
		t.Errorf("expected needs on the later node, got:\n%s", body)
	}
}

func TestWorkflowYAMLRejectsEdgesWithUnknownEndpoints(t *testing.T) {
	handler := NewHandler(testAssets())

	response := postWorkflow(t, handler, `{
		"nodes": [{"id": "n1", "type": "agent", "name": "Agent 1", "x": 0, "y": 0, "w": 160, "h": 64}],
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
			{"id": "n1", "type": "agent", "name": "Agent 1", "x": 0, "y": 0, "w": 160, "h": 64},
			{"id": "n1", "type": "tool", "name": "Tool 1", "x": 10, "y": 0, "w": 160, "h": 64}
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
			{"id": "n1", "type": "agent", "name": "A", "x": 0, "y": 0, "w": 160, "h": 64},
			{"id": "n2", "type": "agent", "name": "A", "x": 10, "y": 0, "w": 160, "h": 64},
			{"id": "n3", "type": "agent", "name": "A 2", "x": 20, "y": 0, "w": 160, "h": 64}
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

func TestWorkflowYAMLKeepsParentForChildDeclaredFirst(t *testing.T) {
	handler := NewHandler(testAssets())

	response := postWorkflow(t, handler, `{
		"nodes": [
			{"id": "n1", "type": "githubapp", "name": "GitHub App 1", "x": 10, "y": 10, "w": 160, "h": 64, "parentId": "n2"},
			{"id": "n2", "type": "github", "name": "GitHub 1", "x": 0, "y": 0, "w": 160, "h": 64}
		]
	}`)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	if !strings.Contains(response.Body.String(), "    parent: github-1\n") {
		t.Errorf("expected parent link for a child declared first, got:\n%s", response.Body.String())
	}
}
