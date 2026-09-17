package app

import (
	"reflect"
	"strings"
	"testing"
)

func TestImportedYAMLRebuildsTheExportedGraph(t *testing.T) {
	retries, timeout := 0, 12.5
	request := WorkflowRequest{
		Name: "Issue to PR",
		Nodes: []WorkflowNodeInput{
			{ID: "p1", Type: "project", Name: "api", X: 0, Y: 0, W: 400, H: 300, Path: "/home/user/api"},
			{ID: "g1", Type: "github", Name: "GitHub 1", X: 20, Y: 10, W: 200, H: 240, ParentID: "p1", Start: true,
				Repository: "acme/api", SecretKey: "secret://bot", Authenticated: true},
			{ID: "a1", Type: "action", Action: "worktree", Name: "Create worktree", X: 12, Y: 40, W: 160, H: 64,
				ParentID: "g1", Start: true, Branch: "feature/login", Base: "origin/main", WorktreePath: "/tmp/login"},
			{ID: "a2", Type: "action", Action: "rebase", Name: "Rebase", X: 12, Y: 128, W: 160, H: 64, ParentID: "g1", Onto: "origin/next"},
			{ID: "x1", Type: "githubapp", Name: "App", X: 1, Y: 2, W: 3, H: 4, ParentID: "g1", AppID: "42", PrivateKeyPath: "/k.pem"},
			{ID: "g2", Type: "agent", Name: "Agent 1", X: 240, Y: 10, W: 160, H: 64, ParentID: "p1",
				Backend: "opencode", Model: "opencode-go/glm-5.3-flash", Effort: "high",
				Prompt: "Implement {{workspace.branch}}\nwith \"care\"", OutputSchema: `{"type":"object"}`,
				Retries: &retries, TimeoutMinutes: &timeout},
		},
		Edges: []WorkflowEdgeInput{{ID: "e1", From: "a1", To: "a2"}, {ID: "e2", From: "a1", To: "g2"}},
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
	rename := map[string]string{"p1": "api", "g1": "github-1", "a1": "create-worktree", "a2": "rebase", "x1": "app", "g2": "agent-1"}
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
		edges[edge.From+">"+edge.To] = true
	}
	if len(imported.Edges) != 2 || !edges["create-worktree>rebase"] || !edges["create-worktree>agent-1"] {
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
