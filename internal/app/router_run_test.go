package app

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/Gabes-Tint/mega-agents/internal/engine"
)

// routedFlow: a reviewer whose fake reply is the given envelope, a router
// with the given cases, a Ship agent on the approved route and a Fix agent
// on the default route.
func routedFlow(t *testing.T, cases string, extraEdges string) string {
	t.Helper()
	return fmt.Sprintf(`{"nodes": [
		{"id": "p1", "type": "project", "name": "api", "path": %q},
		{"id": "r1", "type": "agent", "name": "Reviewer", "parentId": "p1", "start": true, "backend": "claude", "prompt": "Review"},
		{"id": "x1", "type": "router", "name": "Route", "parentId": "p1", "cases": %s},
		{"id": "ship", "type": "agent", "name": "Ship", "parentId": "p1", "backend": "claude", "prompt": "Ship {{result}}"},
		{"id": "fix", "type": "agent", "name": "Fix", "parentId": "p1", "backend": "claude", "prompt": "Fix {{results.route}}"}
	], "edges": [
		{"id": "e1", "from": "r1", "to": "x1"},
		{"id": "e2", "from": "x1", "to": "ship", "fromPort": "approved"},
		{"id": "e3", "from": "x1", "to": "fix", "fromPort": "default"}
		%s
	]}`, t.TempDir(), cases, extraEdges)
}

func TestARouterSendsTheValueDownTheFirstMatchingCase(t *testing.T) {
	bin := fakeClaude(t)
	writeReply(t, bin, 1, `{"type":"result","session_id":"s1","result":"LGTM, approve"}`)
	body := routedFlow(t, `[
		{"name": "blocked", "expression": "value.text.contains('block')"},
		{"name": "approved", "expression": "value.text.contains('approve')"}
	]`, "")

	record := finishedRunOnly(t, body)

	statuses := map[string]engine.Status{}
	for _, step := range record.Steps {
		statuses[step.TaskID] = step.Status
	}
	if record.Status != engine.Succeeded || statuses["x1"] != engine.Succeeded || statuses["ship"] != engine.Succeeded || statuses["fix"] != engine.Skipped {
		t.Fatalf("statuses = %v, record = %+v", statuses, record)
	}
	route := record.Steps[1]
	if route.Kind != "router" || route.Details["case"] != "approved" {
		t.Fatalf("route = %+v", route)
	}
	if got := recorded(t, bin, "prompt-2"); got != `Ship {"case":"approved","value":{"text":"LGTM, approve"}}` {
		t.Fatalf("ship prompt = %q", got)
	}
}

func TestARouterTakesTheDefaultRouteWhenNoCaseHolds(t *testing.T) {
	bin := fakeClaude(t)
	body := routedFlow(t, `[{"name": "approved", "expression": "value.text == 'approve'"}]`, "")

	record := finishedRunOnly(t, body)

	if record.Status != engine.Succeeded || record.Steps[1].Details["case"] != "default" ||
		record.Steps[2].Status != engine.Skipped || record.Steps[3].Status != engine.Succeeded {
		t.Fatalf("record = %+v", record)
	}
	if got := recorded(t, bin, "prompt-2"); got != `Fix {"case":"default","value":{"text":"done"}}` {
		t.Fatalf("fix prompt = %q", got)
	}
}

func TestARouterWhoseExpressionFailsOnTheValueFails(t *testing.T) {
	fakeClaude(t)
	body := routedFlow(t, `[{"name": "approved", "expression": "value.verdict == 'approve'"}]`, "")

	record := finishedRunOnly(t, body)

	if record.Steps[1].Status != engine.Failed || !strings.Contains(record.Steps[1].Error, `case "approved": no such key: verdict`) ||
		record.Steps[2].Status != engine.Skipped || record.Steps[3].Status != engine.Skipped {
		t.Fatalf("record = %+v", record)
	}
}

func TestRouterPlansAreCheckedBeforeRunning(t *testing.T) {
	approved := `[{"name": "approved", "expression": "true"}]`
	cases := map[string]struct {
		body string
		want string
	}{
		"bad expression": {
			body: routedFlow(t, `[{"name": "approved", "expression": "value.x =="}]`, ""),
			want: `Route: case "approved": ERROR`,
		},
		"no cases": {
			body: routedFlow(t, `[]`, ""),
			want: "Route: add at least one case",
		},
		"unknown route": {
			body: strings.Replace(routedFlow(t, approved, ""), `"fromPort": "approved"`, `"fromPort": "rejected"`, 1),
			want: `Route has no route "rejected"; use approved or default`,
		},
		"unnamed route": {
			body: strings.Replace(routedFlow(t, approved, ""), `, "fromPort": "approved"`, ``, 1),
			want: "choose which route the arrow from Route to Ship takes",
		},
		"two inputs": {
			body: routedFlow(t, approved, `, {"id": "e4", "from": "ship", "to": "x1", "fromPort": ""}`),
			want: "Route: routes one value; connect exactly one block to it",
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			response, _ := postRun(t, testCase.body)

			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), testCase.want) {
				t.Fatalf("response = %d %q, want %q", response.Code, response.Body.String(), testCase.want)
			}
		})
	}
}
