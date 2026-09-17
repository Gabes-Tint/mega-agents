package app

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// yamlNode mirrors one entry under nodes in an exported workflow.
type yamlNode struct {
	Uses     string         `yaml:"uses"`
	Name     string         `yaml:"name"`
	Needs    []string       `yaml:"needs"`
	Start    bool           `yaml:"start"`
	With     map[string]any `yaml:"with"`
	Layout   yamlLayout     `yaml:"layout"`
	Children yaml.Node      `yaml:"children"`
}

type yamlLayout struct {
	X float64 `yaml:"x"`
	Y float64 `yaml:"y"`
	W float64 `yaml:"w"`
	H float64 `yaml:"h"`
}

type yamlWorkflow struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Nodes yaml.Node `yaml:"nodes"`
}

// ParseWorkflowYAML rebuilds the editor graph from a workflow document as
// BuildWorkflowYAML writes it, so a workflow drawn in the editor can run
// headless. Node identifiers become node ids, nesting becomes parents, and
// needs become arrows.
func ParseWorkflowYAML(document []byte) (WorkflowRequest, error) {
	var workflow yamlWorkflow
	if err := yaml.Unmarshal(document, &workflow); err != nil {
		return WorkflowRequest{}, fmt.Errorf("the workflow is not valid YAML: %w", err)
	}
	if workflow.Kind != "Workflow" {
		return WorkflowRequest{}, fmt.Errorf("kind %q is not Workflow", workflow.Kind)
	}
	if workflow.APIVersion != workflowAPIVersion {
		return WorkflowRequest{}, fmt.Errorf("apiVersion %q is not supported; use %s", workflow.APIVersion, workflowAPIVersion)
	}
	request := WorkflowRequest{Name: workflow.Metadata.Name}
	if request.Name == "" {
		request.Name = "workflow"
	}
	importer := &yamlImporter{request: &request, declared: map[string]bool{}}
	if err := importer.nodes(&workflow.Nodes, ""); err != nil {
		return WorkflowRequest{}, err
	}
	for _, edge := range importer.needs {
		if !importer.declared[edge.From] {
			return WorkflowRequest{}, fmt.Errorf("%s needs unknown node %q", edge.To, edge.From)
		}
	}
	for i, edge := range importer.needs {
		edge.ID = fmt.Sprintf("e%d", i+1)
		request.Edges = append(request.Edges, edge)
	}
	return request, nil
}

type yamlImporter struct {
	request  *WorkflowRequest
	declared map[string]bool
	needs    []WorkflowEdgeInput
}

func (importer *yamlImporter) nodes(mapping *yaml.Node, parentID string) error {
	if mapping.Kind == 0 {
		return nil
	}
	if mapping.Kind != yaml.MappingNode {
		return fmt.Errorf("nodes must be a mapping of identifiers to blocks")
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		identifier := mapping.Content[i].Value
		if importer.declared[identifier] {
			return fmt.Errorf("node %q is declared twice", identifier)
		}
		importer.declared[identifier] = true
		var entry yamlNode
		if err := mapping.Content[i+1].Decode(&entry); err != nil {
			return fmt.Errorf("node %s: %w", identifier, err)
		}
		node, err := importedNode(identifier, parentID, entry)
		if err != nil {
			return err
		}
		importer.request.Nodes = append(importer.request.Nodes, node)
		for _, need := range entry.Needs {
			importer.needs = append(importer.needs, WorkflowEdgeInput{From: need, To: identifier})
		}
		if err := importer.nodes(&entry.Children, identifier); err != nil {
			return err
		}
	}
	return nil
}

func importedNode(identifier string, parentID string, entry yamlNode) (WorkflowNodeInput, error) {
	node := WorkflowNodeInput{
		ID: identifier, Name: entry.Name, ParentID: parentID, Start: entry.Start,
		X: entry.Layout.X, Y: entry.Layout.Y, W: entry.Layout.W, H: entry.Layout.H,
	}
	if node.Name == "" {
		node.Name = identifier
	}
	block, version, _ := strings.Cut(entry.Uses, "@")
	if action, isGit := strings.CutPrefix(block, "git/"); isGit {
		node.Type, node.Action = "action", action
	} else {
		node.Type = block
	}
	if version != "v1" || !knownComponentTypes[node.Type] {
		return WorkflowNodeInput{}, fmt.Errorf("%s uses unknown block %q", identifier, entry.Uses)
	}
	strings := map[string]*string{
		"repository": &node.Repository, "secretKey": &node.SecretKey, "path": &node.Path,
		"appId": &node.AppID, "privateKeyPath": &node.PrivateKeyPath, "branch": &node.Branch,
		"base": &node.Base, "worktreePath": &node.WorktreePath, "onto": &node.Onto,
	}
	for key, value := range entry.With {
		if key == "authenticated" {
			node.Authenticated = value == true
			continue
		}
		target, known := strings[key]
		if !known {
			return WorkflowNodeInput{}, fmt.Errorf("%s has unknown setting %q", identifier, key)
		}
		*target = fmt.Sprint(value)
	}
	return node, nil
}
