package router

import (
	"strings"
	"testing"
)

func review(verdict string, score float64) map[string]any {
	return map[string]any{"verdict": verdict, "score": score, "findings": []any{map[string]any{"severity": "high"}}}
}

func TestTheFirstMatchingCaseWins(t *testing.T) {
	router, err := Compile([]Case{
		{Name: "blocked", Expression: `value.findings.exists(f, f.severity == "high")`},
		{Name: "approved", Expression: `value.verdict == "approve"`},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	for value, want := range map[*map[string]any]string{
		ptr(review("approve", 9)):                                      "blocked",
		ptr(map[string]any{"verdict": "approve", "findings": []any{}}): "approved",
		ptr(map[string]any{"verdict": "reject", "findings": []any{}}):  Default,
	} {
		got, err := router.Route(*value)
		if err != nil || got != want {
			t.Errorf("Route(%v) = %q, %v; want %q", *value, got, err, want)
		}
	}
}

func ptr(value map[string]any) *map[string]any { return &value }

func TestNumbersCompareAcrossIntegerAndDouble(t *testing.T) {
	router, err := Compile([]Case{{Name: "high", Expression: "value.score >= 8"}})
	if err != nil {
		t.Fatal(err)
	}

	if got, err := router.Route(review("approve", 8.5)); err != nil || got != "high" {
		t.Fatalf("Route = %q, %v", got, err)
	}
}

func TestCompileChecksCasesBeforeAnyRun(t *testing.T) {
	cases := map[string]struct {
		cases []Case
		want  string
	}{
		"no cases":        {cases: nil, want: "add at least one case"},
		"unnamed":         {cases: []Case{{Expression: "true"}}, want: "case 1 needs a name"},
		"bad name":        {cases: []Case{{Name: "needs review", Expression: "true"}}, want: `case name "needs review" may only use letters, digits, - and _`},
		"reserved":        {cases: []Case{{Name: "default", Expression: "true"}}, want: `"default" is the route taken when no case matches`},
		"duplicate":       {cases: []Case{{Name: "a", Expression: "true"}, {Name: "a", Expression: "false"}}, want: `case "a" is declared twice`},
		"empty":           {cases: []Case{{Name: "a"}}, want: `case "a" needs an expression`},
		"syntax":          {cases: []Case{{Name: "a", Expression: "value.verdict =="}}, want: `case "a": ERROR: <input>:1:17: Syntax error`},
		"unknown":         {cases: []Case{{Name: "a", Expression: "verdict == 'x'"}}, want: `case "a": ERROR: <input>:1:1: undeclared reference to 'verdict'`},
		"not a condition": {cases: []Case{{Name: "a", Expression: "1 + 2"}}, want: `case "a" must be a condition that is true or false, not int`},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Compile(testCase.cases)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("err = %v, want it to contain %q", err, testCase.want)
			}
		})
	}
}

func TestRouteReportsExpressionsThatFailOnTheValue(t *testing.T) {
	router, err := Compile([]Case{{Name: "a", Expression: "value.missing == 1"}})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := router.Route(map[string]any{}); err == nil || !strings.Contains(err.Error(), `case "a": no such key: missing`) {
		t.Fatalf("err = %v", err)
	}
	dynamic, _ := Compile([]Case{{Name: "a", Expression: "value.label"}})
	if _, err := dynamic.Route(map[string]any{"label": "x"}); err == nil || !strings.Contains(err.Error(), `case "a" evaluated to string, not true or false`) {
		t.Fatalf("err = %v", err)
	}
}

func TestExpensiveExpressionsAreStopped(t *testing.T) {
	router, err := Compile([]Case{{Name: "a", Expression: "value.items.all(x, value.items.all(y, value.items.all(z, true)))"}})
	if err != nil {
		t.Fatal(err)
	}
	items := make([]any, 400)
	for i := range items {
		items[i] = i
	}

	if _, err := router.Route(map[string]any{"items": items}); err == nil || !strings.Contains(err.Error(), "cost limit") {
		t.Fatalf("err = %v, want the cost limit to stop the expression", err)
	}
}
