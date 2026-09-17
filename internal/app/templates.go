package app

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strings"

	"gopkg.in/yaml.v3"
)

// Templates are ready-made workflows the editor and the command line start
// from, embedded in the binary.
//
//go:embed templates/*.yaml
var templateFiles embed.FS

var errTemplateNotFound = errors.New("template not found")

type TemplateSummary struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// ListTemplates returns every template by name.
func ListTemplates() ([]TemplateSummary, error) {
	entries, err := fs.ReadDir(templateFiles, "templates")
	if err != nil {
		return nil, err
	}
	templates := []TemplateSummary{}
	for _, entry := range entries {
		name := strings.TrimSuffix(entry.Name(), ".yaml")
		document, err := templateFiles.ReadFile("templates/" + entry.Name())
		if err != nil {
			return nil, err
		}
		var header struct {
			Metadata struct {
				Title       string `yaml:"title"`
				Description string `yaml:"description"`
			} `yaml:"metadata"`
		}
		if err := yaml.Unmarshal(document, &header); err != nil {
			return nil, fmt.Errorf("template %s: %w", name, err)
		}
		templates = append(templates, TemplateSummary{Name: name, Title: header.Metadata.Title, Description: header.Metadata.Description})
	}
	return templates, nil
}

// LoadTemplate returns the named template as the editor's graph.
func LoadTemplate(name string) (WorkflowRequest, error) {
	if checkWorkflowName(name) != nil {
		return WorkflowRequest{}, errTemplateNotFound
	}
	document, err := templateFiles.ReadFile("templates/" + name + ".yaml")
	if err != nil {
		return WorkflowRequest{}, errTemplateNotFound
	}
	return ParseWorkflowYAML(document)
}

func registerTemplateHandler(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/templates", func(w http.ResponseWriter, _ *http.Request) {
		templates, err := ListTemplates()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, templates)
	})
	mux.HandleFunc("GET /api/templates/{name}", func(w http.ResponseWriter, r *http.Request) {
		request, err := LoadTemplate(r.PathValue("name"))
		if errors.Is(err, errTemplateNotFound) {
			http.Error(w, "template "+r.PathValue("name")+" not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, request)
	})
}
