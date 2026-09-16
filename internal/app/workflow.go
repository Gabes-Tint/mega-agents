package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"unicode"
)

const workflowAPIVersion = "megaagents.dev/v1alpha1"

// workflowMaxBody bounds the JSON request body to keep the unauthenticated
// localhost endpoint cheap to abuse.
const workflowMaxBodyBytes = 1 << 20

type WorkflowEdgeInput struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
}

type WorkflowNodeInput struct {
	ID             string  `json:"id"`
	Type           string  `json:"type"`
	Name           string  `json:"name"`
	X              float64 `json:"x"`
	Y              float64 `json:"y"`
	W              float64 `json:"w"`
	H              float64 `json:"h"`
	ParentID       string  `json:"parentId,omitempty"`
	Start          bool    `json:"start,omitempty"`
	Path           string  `json:"path,omitempty"`
	Repository     string  `json:"repository,omitempty"`
	SecretKey      string  `json:"secretKey,omitempty"`
	AppID          string  `json:"appId,omitempty"`
	PrivateKeyPath string  `json:"privateKeyPath,omitempty"`
}

type WorkflowRequest struct {
	Name  string              `json:"name"`
	Nodes []WorkflowNodeInput `json:"nodes"`
	Edges []WorkflowEdgeInput `json:"edges"`
}

var knownComponentTypes = map[string]bool{
	"agent":     true,
	"tool":      true,
	"project":   true,
	"gatebase":  true,
	"github":    true,
	"gitlab":    true,
	"githubapp": true,
}

// identifierSet tracks every identifier handed out so collision suffixes can
// never collide with a later slug or a previous suffix.
type identifierSet struct {
	used map[string]int
}

func newIdentifierSet() *identifierSet {
	return &identifierSet{used: map[string]int{}}
}

// slugIdentifier lowercases the name to ASCII [a-z0-9-] so YAML mapping keys
// stay unquoted and stable; collisions get a numeric suffix.
func (set *identifierSet) slugIdentifier(name string) string {
	var builder strings.Builder
	for _, char := range strings.ToLower(name) {
		switch {
		case char >= 'a' && char <= 'z', char >= '0' && char <= '9':
			builder.WriteRune(char)
		case char == ' ' || char == '-' || char == '_' || char == '/' || char == '.' || !unicode.IsLetter(char):
			builder.WriteByte('-')
		}
	}
	slug := builder.String()
	if slug == "" {
		slug = "node"
	}
	count := set.used[slug]
	set.used[slug] = count + 1
	identifier := slug
	if count > 0 {
		identifier = fmt.Sprintf("%s-%d", slug, count+1)
	}
	// Record the returned identifier too, so suffixes cannot be reused by a
	// later name that slugs to the same string.
	set.used[identifier] = 1
	return identifier
}

// yamlString quotes a scalar with Go's %q, whose escaping is compatible with
// YAML double-quoted scalars.
func yamlString(value string) string {
	return fmt.Sprintf("%q", value)
}

// BuildWorkflowYAML serializes the graph deterministically: identifiers are
// assigned in a first pass so needs, parent links, and edge endpoints resolve
// regardless of declaration order, while nodes keep their input order.
// Secrets are exported as credential references only.
func BuildWorkflowYAML(request WorkflowRequest) (string, error) {
	if len(request.Nodes) == 0 {
		return "", fmt.Errorf("no nodes to export")
	}
	for _, node := range request.Nodes {
		if !knownComponentTypes[node.Type] {
			return "", fmt.Errorf("unknown component type %q", node.Type)
		}
	}
	identifiers := newIdentifierSet()
	nodeIDs := make(map[string]bool, len(request.Nodes))
	identifierByNodeID := make(map[string]string, len(request.Nodes))
	for _, node := range request.Nodes {
		if nodeIDs[node.ID] {
			return "", fmt.Errorf("duplicate node id %q", node.ID)
		}
		nodeIDs[node.ID] = true
		identifierByNodeID[node.ID] = identifiers.slugIdentifier(node.Name)
	}
	for _, edge := range request.Edges {
		if !nodeIDs[edge.From] || !nodeIDs[edge.To] {
			return "", fmt.Errorf("edge references an unknown node")
		}
	}
	if request.Name == "" {
		request.Name = "workflow"
	}

	metadataName := newIdentifierSet().slugIdentifier(request.Name)

	var nodesBuilder strings.Builder
	for _, node := range request.Nodes {
		identifier := identifierByNodeID[node.ID]
		nodesBuilder.WriteString(fmt.Sprintf("  %s:\n", identifier))
		nodesBuilder.WriteString(fmt.Sprintf("    uses: %s@v1\n", node.Type))
		var needs []string
		for _, edge := range request.Edges {
			if edge.To == node.ID {
				needs = append(needs, identifierByNodeID[edge.From])
			}
		}
		if len(needs) > 0 {
			nodesBuilder.WriteString("    needs:\n")
			for _, target := range needs {
				nodesBuilder.WriteString(fmt.Sprintf("      - %s\n", target))
			}
		}
		if node.ParentID != "" {
			if parent, ok := identifierByNodeID[node.ParentID]; ok {
				nodesBuilder.WriteString(fmt.Sprintf("    parent: %s\n", parent))
			}
		}
		if node.Start {
			nodesBuilder.WriteString("    start: true\n")
		}
		var with []string
		if node.Path != "" {
			with = append(with, fmt.Sprintf("      path: %s", yamlString(node.Path)))
		}
		if node.Repository != "" {
			with = append(with, fmt.Sprintf("      repository: %s", yamlString(node.Repository)))
		}
		if node.SecretKey != "" {
			with = append(with, fmt.Sprintf("      secretKey: %s", yamlString(node.SecretKey)))
		}
		if node.AppID != "" {
			with = append(with, fmt.Sprintf("      appId: %s", yamlString(node.AppID)))
		}
		if node.PrivateKeyPath != "" {
			with = append(with, fmt.Sprintf("      privateKeyPath: %s", yamlString(node.PrivateKeyPath)))
		}
		if len(with) > 0 {
			nodesBuilder.WriteString("    with:\n")
			for _, line := range with {
				nodesBuilder.WriteString(line)
				nodesBuilder.WriteString("\n")
			}
		}
		nodesBuilder.WriteString("    layout:\n")
		nodesBuilder.WriteString(fmt.Sprintf("      x: %g\n", node.X))
		nodesBuilder.WriteString(fmt.Sprintf("      y: %g\n", node.Y))
		nodesBuilder.WriteString(fmt.Sprintf("      w: %g\n", node.W))
		nodesBuilder.WriteString(fmt.Sprintf("      h: %g\n", node.H))
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("apiVersion: %s\n", workflowAPIVersion))
	builder.WriteString("kind: Workflow\n")
	builder.WriteString("\n")
	builder.WriteString("metadata:\n")
	builder.WriteString(fmt.Sprintf("  name: %s\n", metadataName))
	builder.WriteString("\n")
	builder.WriteString("nodes:\n")
	builder.WriteString(nodesBuilder.String())
	return builder.String(), nil
}

func registerWorkflowYAMLHandler(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/workflows/yaml", func(w http.ResponseWriter, r *http.Request) {
		var request WorkflowRequest
		body := http.MaxBytesReader(w, r.Body, workflowMaxBodyBytes)
		if err := json.NewDecoder(body).Decode(&request); err != nil {
			http.Error(w, "invalid workflow request", http.StatusBadRequest)
			return
		}
		name := request.Name
		if name == "" {
			name = "workflow"
		}
		yaml, err := BuildWorkflowYAML(request)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/x-yaml")
		w.Header().Set(
			"Content-Disposition",
			fmt.Sprintf(
				"attachment; filename=%q",
				newIdentifierSet().slugIdentifier(name)+".yaml",
			),
		)
		_, _ = w.Write([]byte(yaml))
	})
}
