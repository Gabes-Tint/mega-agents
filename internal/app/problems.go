package app

import (
	"encoding/json"
	"net/http"
	"strings"
)

// Severities of a problem: an error stops the flow from running; a warning
// points at something that probably does not do what was meant.
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
)

// Problem is one thing to fix in a workflow before running it, with the
// block it concerns when there is one.
type Problem struct {
	NodeID   string `json:"nodeId,omitempty"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// CheckWorkflow lists every problem a run of the workflow would meet, in the
// order it would meet them, followed by warnings. An empty workflow has none.
func CheckWorkflow(request WorkflowRequest) []Problem {
	problems := []Problem{}
	if len(request.Nodes) == 0 {
		return problems
	}
	seen := map[string]bool{}
	add := func(problem Problem) {
		// An arrow's problem is met by both blocks it joins; list it once.
		if seen[problem.Message] {
			return
		}
		seen[problem.Message] = true
		problems = append(problems, problem)
	}
	_, failures, included := plan(request)
	for _, failure := range failures {
		add(Problem{NodeID: failure.nodeID, Severity: SeverityError, Message: failure.err.Error()})
	}
	if len(failures) == 1 && failures[0].nodeID == "" {
		// The graph itself cannot be planned, or nothing starts it; block
		// warnings would only repeat that.
		return problems
	}
	for _, node := range request.Nodes {
		switch {
		case node.Type == "project" && strings.TrimSpace(node.Path) == "" && hasChildren(request, node.ID):
			add(Problem{NodeID: node.ID, Severity: SeverityWarning, Message: node.Name + ": set the folder its blocks work in"})
		case executableTypes[node.Type] && !included[node.ID]:
			add(Problem{
				NodeID: node.ID, Severity: SeverityWarning,
				Message: node.Name + " does not run: no starting point leads to it",
			})
		}
	}
	return problems
}

func hasChildren(request WorkflowRequest, id string) bool {
	for _, node := range request.Nodes {
		if node.ParentID == id {
			return true
		}
	}
	return false
}

func registerProblemsHandler(mux *http.ServeMux) {
	// The editor asks for the workflow's problems as it changes, so they are
	// listed before anyone presses Run.
	mux.HandleFunc("POST /api/workflows/problems", func(w http.ResponseWriter, r *http.Request) {
		var request WorkflowRequest
		body := http.MaxBytesReader(w, r.Body, workflowMaxBodyBytes)
		if err := json.NewDecoder(body).Decode(&request); err != nil {
			http.Error(w, "invalid workflow request", http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, CheckWorkflow(request))
	})
}
