package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"unicode"

	"github.com/Gabes-Tint/mega-agents/internal/router"
)

const workflowAPIVersion = "megaagents.dev/v1alpha1"

// workflowMaxBody bounds the JSON request body to keep the unauthenticated
// localhost endpoint cheap to abuse.
const workflowMaxBodyBytes = 1 << 20

type WorkflowEdgeInput struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
	// FromPort names which output of a block with several the arrow
	// carries, such as the invalid branch of a schema block.
	FromPort string `json:"fromPort,omitempty"`
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
	Authenticated  bool    `json:"authenticated,omitempty"`
	AppID          string  `json:"appId,omitempty"`
	PrivateKeyPath string  `json:"privateKeyPath,omitempty"`
	// Git action blocks nested inside a GitHub block.
	Action       string `json:"action,omitempty"`
	Branch       string `json:"branch,omitempty"`
	Base         string `json:"base,omitempty"`
	WorktreePath string `json:"worktreePath,omitempty"`
	Onto         string `json:"onto,omitempty"`
	// Agent blocks.
	Backend        string   `json:"backend,omitempty"`
	Model          string   `json:"model,omitempty"`
	Effort         string   `json:"effort,omitempty"`
	Prompt         string   `json:"prompt,omitempty"`
	OutputSchema   string   `json:"outputSchema,omitempty"`
	Retries        *int     `json:"retries,omitempty"`
	TimeoutMinutes *float64 `json:"timeoutMinutes,omitempty"`
	MaxCostUSD     *float64 `json:"maxCostUsd,omitempty"`
	// ContinueSession resumes a copy of the connected agent's conversation.
	ContinueSession bool `json:"continueSession,omitempty"`
	// JSON Schema blocks.
	Schema string `json:"schema,omitempty"`
	// Router blocks.
	Cases []router.Case `json:"cases,omitempty"`
	// Command blocks.
	Command string `json:"command,omitempty"`
	// Loop blocks: how many times the blocks inside may run, and the block
	// inside and its output that end the loop.
	MaxIterations *int   `json:"maxIterations,omitempty"`
	UntilNode     string `json:"untilNode,omitempty"`
	UntilPort     string `json:"untilPort,omitempty"`
	// WaitForAny runs an agent, command or loop when any of the arrows into
	// it arrives, where exclusive branches join again.
	WaitForAny bool `json:"waitForAny,omitempty"`
	// Delivery actions: a commit message, a pull request's title and body,
	// and the issue to read with the labels that stop it from being read.
	// Without the setting the default labels apply; an empty list ignores none.
	Message      string    `json:"message,omitempty"`
	Title        string    `json:"title,omitempty"`
	Body         string    `json:"body,omitempty"`
	Issue        int       `json:"issue,omitempty"`
	IgnoreLabels *[]string `json:"ignoreLabels,omitempty"`
}

// defaultIgnoreLabels are the labels Read issue ignores unless the action
// lists its own.
var defaultIgnoreLabels = []string{"paused", "draft", "needs-attention"}

func (node WorkflowNodeInput) ignoreLabels() []string {
	if node.IgnoreLabels == nil {
		return defaultIgnoreLabels
	}
	return *node.IgnoreLabels
}

type WorkflowRequest struct {
	Name  string              `json:"name"`
	Nodes []WorkflowNodeInput `json:"nodes"`
	Edges []WorkflowEdgeInput `json:"edges"`
}

var knownComponentTypes = map[string]bool{
	"agent":      true,
	"project":    true,
	"github":     true,
	"gitlab":     true,
	"githubapp":  true,
	"action":     true,
	"jsonschema": true,
	"router":     true,
	"command":    true,
	"loop":       true,
}

// containmentMatrix mirrors the frontend CONTAINMENT_MATRIX: for each
// component type, the targets (including the canvas root) that accept it.
// Any placement outside these lists is rejected so the exported YAML always
// matches what the builder validates while dragging.
var containmentMatrix = map[string]map[string]bool{
	"agent":      {"project": true, "agent": true, "loop": true},
	"project":    {"root": true, "project": true},
	"github":     {"project": true},
	"gitlab":     {"project": true},
	"githubapp":  {"github": true},
	"action":     {"github": true},
	"jsonschema": {"agent": true},
	"router":     {"project": true, "agent": true, "loop": true},
	"command":    {"project": true, "agent": true, "loop": true},
	"loop":       {"project": true},
}

func containmentAllows(target string, childType string) bool {
	return containmentMatrix[childType][target]
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

// gitBase is the code-level abstraction the GitHub and GitLab components
// derive from. It is not a displayable component type; both forge controllers
// emit their configuration through it to avoid duplicated serialization.
type gitBase struct {
	Repository    string
	SecretKey     string
	Authenticated bool
}

func (base gitBase) withLines(indent string) []string {
	var with []string
	if base.Repository != "" {
		with = append(with, fmt.Sprintf("%srepository: %s", indent, yamlString(base.Repository)))
	}
	if base.SecretKey != "" {
		with = append(with, fmt.Sprintf("%ssecretKey: %s", indent, yamlString(base.SecretKey)))
	}
	if base.Authenticated {
		with = append(with, fmt.Sprintf("%sauthenticated: true", indent))
	}
	return with
}

func gitBaseFor(node WorkflowNodeInput) gitBase {
	return gitBase{Repository: node.Repository, SecretKey: node.SecretKey, Authenticated: node.Authenticated}
}

type workflowEmitter struct {
	request          WorkflowRequest
	identifierByNode map[string]string
	childrenOf       map[string][]WorkflowNodeInput
}

func (emitter *workflowEmitter) emitNode(
	builder *strings.Builder,
	node WorkflowNodeInput,
	identifier string,
	indent string,
) {
	builder.WriteString(fmt.Sprintf("%s%s:\n", indent, identifier))
	uses := node.Type
	if node.Type == "action" {
		// Git actions are local Git capabilities provided through their
		// GitHub block, so they export under the git block family.
		uses = "git/" + node.Action
	}
	builder.WriteString(fmt.Sprintf("%s  uses: %s@v1\n", indent, uses))
	if node.Name != identifier {
		// The identifier is a slug of the name; the name itself is kept so an
		// imported workflow shows the same labels.
		builder.WriteString(fmt.Sprintf("%s  name: %s\n", indent, yamlString(node.Name)))
	}
	var needs []string
	for _, edge := range emitter.request.Edges {
		if edge.To == node.ID {
			need := emitter.identifierByNode[edge.From]
			if edge.FromPort != "" {
				// Identifiers never contain dots, so identifier.port is
				// unambiguous.
				need += "." + edge.FromPort
			}
			needs = append(needs, need)
		}
	}
	if len(needs) > 0 {
		builder.WriteString(fmt.Sprintf("%s  needs:\n", indent))
		for _, target := range needs {
			builder.WriteString(fmt.Sprintf("%s    - %s\n", indent, target))
		}
	}
	if node.Start {
		builder.WriteString(fmt.Sprintf("%s  start: true\n", indent))
	}
	var with []string
	if node.Type == "github" || node.Type == "gitlab" {
		// GitHub and GitLab derive their configuration from gitBase; the
		// with entries indent two spaces past the node's own keys.
		with = append(with, gitBaseFor(node).withLines(indent+"    ")...)
	}
	if node.Path != "" {
		with = append(with, fmt.Sprintf("%s    path: %s", indent, yamlString(node.Path)))
	}
	for _, field := range []struct{ key, value string }{
		{"branch", node.Branch}, {"base", node.Base}, {"worktreePath", node.WorktreePath}, {"onto", node.Onto},
		{"backend", node.Backend}, {"model", node.Model}, {"effort", node.Effort}, {"prompt", node.Prompt},
		{"outputSchema", node.OutputSchema}, {"schema", node.Schema}, {"command", node.Command},
		{"message", node.Message}, {"title", node.Title}, {"body", node.Body},
	} {
		if field.value != "" {
			with = append(with, fmt.Sprintf("%s    %s: %s", indent, field.key, yamlString(field.value)))
		}
	}
	if len(node.Cases) > 0 {
		with = append(with, fmt.Sprintf("%s    cases:", indent))
		for _, routeCase := range node.Cases {
			with = append(with,
				fmt.Sprintf("%s      - name: %s", indent, yamlString(routeCase.Name)),
				fmt.Sprintf("%s        expression: %s", indent, yamlString(routeCase.Expression)),
			)
		}
	}
	if node.MaxIterations != nil {
		with = append(with, fmt.Sprintf("%s    maxIterations: %d", indent, *node.MaxIterations))
	}
	if node.UntilNode != "" {
		// The block that ends the loop is named by its identifier, as needs are.
		with = append(with, fmt.Sprintf("%s    untilNode: %s", indent, yamlString(emitter.identifierByNode[node.UntilNode])))
	}
	if node.UntilPort != "" {
		with = append(with, fmt.Sprintf("%s    untilPort: %s", indent, yamlString(node.UntilPort)))
	}
	if node.Issue != 0 {
		with = append(with, fmt.Sprintf("%s    issue: %d", indent, node.Issue))
	}
	if node.IgnoreLabels != nil {
		// A cleared list is written as [] so it does not fall back to the defaults.
		var labels []string
		for _, label := range *node.IgnoreLabels {
			if label = strings.TrimSpace(label); label != "" {
				labels = append(labels, yamlString(label))
			}
		}
		with = append(with, fmt.Sprintf("%s    ignoreLabels: [%s]", indent, strings.Join(labels, ", ")))
	}
	if node.Retries != nil {
		with = append(with, fmt.Sprintf("%s    retries: %d", indent, *node.Retries))
	}
	if node.TimeoutMinutes != nil {
		with = append(with, fmt.Sprintf("%s    timeoutMinutes: %g", indent, *node.TimeoutMinutes))
	}
	if node.ContinueSession {
		with = append(with, fmt.Sprintf("%s    continueSession: true", indent))
	}
	if node.WaitForAny {
		with = append(with, fmt.Sprintf("%s    waitForAny: true", indent))
	}
	if node.MaxCostUSD != nil {
		with = append(with, fmt.Sprintf("%s    maxCostUsd: %g", indent, *node.MaxCostUSD))
	}
	if node.AppID != "" {
		with = append(with, fmt.Sprintf("%s    appId: %s", indent, yamlString(node.AppID)))
	}
	if node.PrivateKeyPath != "" {
		with = append(with, fmt.Sprintf("%s    privateKeyPath: %s", indent, yamlString(node.PrivateKeyPath)))
	}
	if len(with) > 0 {
		builder.WriteString(fmt.Sprintf("%s  with:\n", indent))
		for _, line := range with {
			builder.WriteString(line)
			builder.WriteString("\n")
		}
	}
	builder.WriteString(fmt.Sprintf("%s  layout:\n", indent))
	builder.WriteString(fmt.Sprintf("%s    x: %g\n", indent, node.X))
	builder.WriteString(fmt.Sprintf("%s    y: %g\n", indent, node.Y))
	builder.WriteString(fmt.Sprintf("%s    w: %g\n", indent, node.W))
	builder.WriteString(fmt.Sprintf("%s    h: %g\n", indent, node.H))
	children := emitter.childrenOf[node.ID]
	if len(children) > 0 {
		// The children mapping is two spaces deeper than its parent node, so
		// child objects indent one nesting level further in.
		builder.WriteString(fmt.Sprintf("%s  children:\n", indent))
		for _, child := range children {
			emitter.emitNode(
				builder,
				child,
				emitter.identifierByNode[child.ID],
				indent+"    ",
			)
		}
	}
}

// validateGraph applies the rules the editor enforces while dragging: known
// types, unique ids, edges and parents that resolve, and the containment
// matrix. Export and run share it so neither accepts a graph the other rejects.
func validateGraph(request WorkflowRequest) (map[string]WorkflowNodeInput, error) {
	if len(request.Nodes) == 0 {
		return nil, fmt.Errorf("no nodes to export")
	}
	for _, node := range request.Nodes {
		if !knownComponentTypes[node.Type] {
			return nil, fmt.Errorf("unknown component type %q", node.Type)
		}
	}
	nodeByID := make(map[string]WorkflowNodeInput, len(request.Nodes))
	for _, node := range request.Nodes {
		if nodeByID[node.ID].Type != "" {
			return nil, fmt.Errorf("duplicate node id %q", node.ID)
		}
		nodeByID[node.ID] = node
	}
	for _, edge := range request.Edges {
		_, fromOK := nodeByID[edge.From]
		_, toOK := nodeByID[edge.To]
		if !fromOK || !toOK {
			return nil, fmt.Errorf("edge references an unknown node")
		}
	}
	for _, node := range request.Nodes {
		target := "root"
		if node.ParentID != "" {
			parent, ok := nodeByID[node.ParentID]
			if !ok {
				return nil, fmt.Errorf("node %q references an unknown parent", node.ID)
			}
			target = parent.Type
		}
		if !containmentAllows(target, node.Type) {
			return nil, fmt.Errorf(
				"%s cannot be placed inside %s",
				node.Type,
				target,
			)
		}
	}
	return nodeByID, nil
}

// BuildWorkflowYAML serializes the graph deterministically: identifiers are
// assigned in a first pass so needs and containment resolve regardless of
// declaration order, while nodes keep their input order. Contained boxes are
// serialized as child objects inside their parent instead of using a parent
// tag. Secrets are exported as credential references only.
func BuildWorkflowYAML(request WorkflowRequest) (string, error) {
	if _, err := validateGraph(request); err != nil {
		return "", err
	}
	identifiers := newIdentifierSet()
	identifierByNode := make(map[string]string, len(request.Nodes))
	childrenOf := make(map[string][]WorkflowNodeInput, len(request.Nodes))
	for _, node := range request.Nodes {
		identifierByNode[node.ID] = identifiers.slugIdentifier(node.Name)
		if node.ParentID != "" {
			childrenOf[node.ParentID] = append(childrenOf[node.ParentID], node)
		}
	}
	if request.Name == "" {
		request.Name = "workflow"
	}

	metadataName := newIdentifierSet().slugIdentifier(request.Name)
	emitter := &workflowEmitter{
		request:          request,
		identifierByNode: identifierByNode,
		childrenOf:       childrenOf,
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("apiVersion: %s\n", workflowAPIVersion))
	builder.WriteString("kind: Workflow\n")
	builder.WriteString("\n")
	builder.WriteString("metadata:\n")
	builder.WriteString(fmt.Sprintf("  name: %s\n", metadataName))
	builder.WriteString("\n")
	builder.WriteString("nodes:\n")
	for _, node := range request.Nodes {
		if node.ParentID != "" {
			continue
		}
		emitter.emitNode(&builder, node, identifierByNode[node.ID], "  ")
	}
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
