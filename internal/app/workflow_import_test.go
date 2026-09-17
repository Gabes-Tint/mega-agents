package app

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Gabes-Tint/mega-agents/internal/router"
)

func TestImportedYAMLRebuildsTheExportedGraph(t *testing.T) {
	retries, timeout, budget := 0, 12.5, 0.75
	request := WorkflowRequest{
		Name: "Issue to PR",
		Nodes: []WorkflowNodeInput{
			{ID: "p1", Type: "project", Name: "api", X: 0, Y: 0, W: 400, H: 300, Path: "/home/user/api"},
			{ID: "g1", Type: "github", Name: "GitHub 1", X: 20, Y: 10, W: 200, H: 240, ParentID: "p1", Start: true,
				Repository: "acme/api", SecretKey: "secret://bot", Authenticated: true},
			{ID: "a1", Type: "action", Action: "worktree", Name: "Create worktree", X: 12, Y: 40, W: 160, H: 64,
				ParentID: "g1", Start: true, Branch: "feature/login", Base: "origin/main", WorktreePath: "/tmp/login"},
			{ID: "a2", Type: "action", Action: "rebase", Name: "Rebase", X: 12, Y: 128, W: 160, H: 64, ParentID: "g1", Onto: "origin/next"},
			{ID: "i1", Type: "action", Action: "issue", Name: "Read issue", X: 12, Y: 216, W: 160, H: 64, ParentID: "g1",
				Issue: 7, IgnoreLabels: &[]string{"paused", "needs attention"}},
			{ID: "i2", Type: "action", Action: "issue", Name: "Cleared", X: 12, Y: 304, W: 160, H: 64, ParentID: "g1",
				Issue: 8, IgnoreLabels: &[]string{}},
			{ID: "i3", Type: "action", Action: "issue", Name: "Old", X: 12, Y: 392, W: 160, H: 64, ParentID: "g1", Issue: 9},
			{ID: "x1", Type: "githubapp", Name: "App", X: 1, Y: 2, W: 3, H: 4, ParentID: "g1", AppID: "42", PrivateKeyPath: "/k.pem"},
			{ID: "g2", Type: "agent", Name: "Agent 1", X: 240, Y: 10, W: 160, H: 64, ParentID: "p1",
				Backend: "opencode", Model: "opencode-go/glm-5.3-flash", Effort: "high",
				Prompt: "Implement {{workspace.branch}}\nwith \"care\"", OutputSchema: `{"type":"object"}`,
				Retries: &retries, TimeoutMinutes: &timeout, MaxCostUSD: &budget, ContinueSession: true},
			{ID: "s1", Type: "jsonschema", Name: "Check", X: 5, Y: 6, W: 7, H: 8, ParentID: "g2", Schema: `{"type":"object"}`},
			{ID: "r1", Type: "router", Name: "Route", X: 1, Y: 1, W: 1, H: 1, ParentID: "p1", Cases: []router.Case{
				{Name: "approved", Expression: `value.verdict == "approve"`}, {Name: "blocked", Expression: "size(value.findings) > 0"},
			}},
			{ID: "f1", Type: "agent", Name: "Fixer", X: 9, Y: 10, W: 11, H: 12, ParentID: "p1", Backend: "claude", Prompt: "Fix"},
		},
		Edges: []WorkflowEdgeInput{
			{ID: "e1", From: "a1", To: "a2"}, {ID: "e2", From: "a1", To: "g2"},
			{ID: "e4", From: "s1", To: "f1", FromPort: "invalid"},
			{ID: "e5", From: "s1", To: "r1"},
		},
	}
	yaml, err := BuildWorkflowYAML(request)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	imported, err := ParseWorkflowYAML([]byte(yaml))
	if err != nil {
		t.Fatalf("import: %v\n%s", err, yaml)
	}

	if imported.Name != "issue-to-pr" {
		t.Errorf("name = %q", imported.Name)
	}
	// Identifiers replace editor ids, so compare with the ids exported.
	rename := map[string]string{"p1": "api", "g1": "github-1", "a1": "create-worktree", "a2": "rebase", "i1": "read-issue", "i2": "cleared", "i3": "old", "x1": "app", "g2": "agent-1", "s1": "check", "f1": "fixer", "r1": "route"}
	var want []WorkflowNodeInput
	for _, node := range request.Nodes {
		node.ID = rename[node.ID]
		node.ParentID = rename[node.ParentID]
		want = append(want, node)
	}
	if !reflect.DeepEqual(imported.Nodes, want) {
		t.Errorf("nodes =\n%+v\nwant\n%+v", imported.Nodes, want)
	}
	edges := map[string]bool{}
	for _, edge := range imported.Edges {
		edges[edge.From+">"+edge.To+":"+edge.FromPort] = true
	}
	if len(imported.Edges) != 4 || !edges["create-worktree>rebase:"] || !edges["create-worktree>agent-1:"] ||
		!edges["check>route:"] || !edges["check>fixer:invalid"] {
		t.Errorf("edges = %+v", imported.Edges)
	}
}

func TestImportRejectsDocumentsThatAreNotWorkflows(t *testing.T) {
	cases := map[string]struct {
		yaml string
		want string
	}{
		"not yaml": {
			yaml: "nodes: [",
			want: "is not valid YAML",
		},
		"wrong kind": {
			yaml: "apiVersion: megaagents.dev/v1alpha1\nkind: Pipeline\nnodes: {}\n",
			want: `kind "Pipeline" is not Workflow`,
		},
		"wrong version": {
			yaml: "apiVersion: megaagents.dev/v2\nkind: Workflow\nnodes: {}\n",
			want: `apiVersion "megaagents.dev/v2" is not supported`,
		},
		"unknown block": {
			yaml: "apiVersion: megaagents.dev/v1alpha1\nkind: Workflow\nnodes:\n  x:\n    uses: robot@v1\n",
			want: `x uses unknown block "robot@v1"`,
		},
		"unknown need": {
			yaml: "apiVersion: megaagents.dev/v1alpha1\nkind: Workflow\nnodes:\n  x:\n    uses: project@v1\n    needs: [ghost]\n",
			want: `x needs unknown node "ghost"`,
		},
		"bad retries": {
			yaml: "apiVersion: megaagents.dev/v1alpha1\nkind: Workflow\nnodes:\n  x:\n    uses: agent@v1\n    with:\n      retries: many\n",
			want: `x setting "retries" must be a number`,
		},
		"bad cases": {
			yaml: "apiVersion: megaagents.dev/v1alpha1\nkind: Workflow\nnodes:\n  x:\n    uses: router@v1\n    with:\n      cases: yes\n",
			want: `x setting "cases" must be a list of name and expression`,
		},
		"labels that are not a list": {
			yaml: "apiVersion: megaagents.dev/v1alpha1\nkind: Workflow\nnodes:\n  x:\n    uses: git/issue@v1\n    with:\n      ignoreLabels: paused\n",
			want: `x setting "ignoreLabels" must be a list of labels`,
		},
		"a label that is not text": {
			yaml: "apiVersion: megaagents.dev/v1alpha1\nkind: Workflow\nnodes:\n  x:\n    uses: git/issue@v1\n    with:\n      ignoreLabels: [[paused]]\n",
			want: `x setting "ignoreLabels" must be a list of labels`,
		},
		"unknown setting": {
			yaml: "apiVersion: megaagents.dev/v1alpha1\nkind: Workflow\nnodes:\n  x:\n    uses: project@v1\n    with:\n      colour: red\n",
			want: `x has unknown setting "colour"`,
		},
		"duplicate identifier": {
			yaml: "apiVersion: megaagents.dev/v1alpha1\nkind: Workflow\nnodes:\n  x:\n    uses: project@v1\n    children:\n      x:\n        uses: agent@v1\n",
			want: `node "x" is declared twice`,
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseWorkflowYAML([]byte(testCase.yaml))
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("err = %v, want it to contain %q", err, testCase.want)
			}
		})
	}
}

func TestImportKeepsDeclarationOrderAndOptionalSections(t *testing.T) {
	yaml := `apiVersion: megaagents.dev/v1alpha1
kind: Workflow
nodes:
  zeta:
    uses: project@v1
  alpha:
    uses: project@v1
    needs:
      - zeta
`
	imported, err := ParseWorkflowYAML([]byte(yaml))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if imported.Name != "workflow" || len(imported.Nodes) != 2 || imported.Nodes[0].ID != "zeta" || imported.Nodes[1].Name != "alpha" {
		t.Fatalf("imported = %+v", imported)
	}
}
