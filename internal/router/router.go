// Package router chooses a route for a value with CEL (Common Expression
// Language) conditions: named cases checked in order, the first that holds
// wins, and "default" is taken when none does. Expressions are parsed and
// type-checked when the router is compiled, never evaluated as code, and
// each evaluation is bounded by a cost limit.
package router

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"cel.dev/cel-go/cel"
	"cel.dev/cel-go/common/types"
)

// Default is the route taken when no case holds.
const Default = "default"

// costLimit bounds one evaluation, so a comprehension over a large value
// cannot stall a run.
const costLimit = 1_000_000

var caseName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type Case struct {
	Name       string `json:"name"`
	Expression string `json:"expression"`
}

type Router struct {
	cases    []Case
	programs []cel.Program
}

// Compile checks every case: a unique name, and an expression over value
// that parses, type-checks and yields a condition.
func Compile(cases []Case) (*Router, error) {
	if len(cases) == 0 {
		return nil, errors.New("add at least one case")
	}
	environment, err := cel.NewEnv(
		cel.Variable("value", cel.DynType),
		cel.CrossTypeNumericComparisons(true),
	)
	if err != nil {
		return nil, fmt.Errorf("cannot prepare expressions: %w", err)
	}
	router := &Router{cases: cases}
	seen := map[string]bool{}
	for index, routeCase := range cases {
		switch {
		case routeCase.Name == "":
			return nil, fmt.Errorf("case %d needs a name", index+1)
		case !caseName.MatchString(routeCase.Name):
			return nil, fmt.Errorf("case name %q may only use letters, digits, - and _", routeCase.Name)
		case routeCase.Name == Default:
			return nil, fmt.Errorf("%q is the route taken when no case matches; name the case differently", Default)
		case seen[routeCase.Name]:
			return nil, fmt.Errorf("case %q is declared twice", routeCase.Name)
		case strings.TrimSpace(routeCase.Expression) == "":
			return nil, fmt.Errorf("case %q needs an expression", routeCase.Name)
		}
		seen[routeCase.Name] = true
		checked, issues := environment.Compile(routeCase.Expression)
		if issues != nil && issues.Err() != nil {
			return nil, fmt.Errorf("case %q: %s", routeCase.Name, issues.Err())
		}
		output := checked.OutputType()
		if !output.IsExactType(types.BoolType) && !output.IsExactType(types.DynType) {
			return nil, fmt.Errorf("case %q must be a condition that is true or false, not %s", routeCase.Name, output)
		}
		program, err := environment.Program(checked, cel.CostLimit(costLimit))
		if err != nil {
			return nil, fmt.Errorf("case %q: %w", routeCase.Name, err)
		}
		router.programs = append(router.programs, program)
	}
	return router, nil
}

// Cases lists the route names in order, ending with Default.
func (router *Router) Cases() []string {
	names := make([]string, 0, len(router.cases)+1)
	for _, routeCase := range router.cases {
		names = append(names, routeCase.Name)
	}
	return append(names, Default)
}

// Route returns the first case whose condition holds for value, or Default.
func (router *Router) Route(value any) (string, error) {
	for index, program := range router.programs {
		name := router.cases[index].Name
		result, _, err := program.Eval(map[string]any{"value": value})
		if err != nil {
			return "", fmt.Errorf("case %q: %w", name, err)
		}
		holds, ok := result.Value().(bool)
		if !ok {
			return "", fmt.Errorf("case %q evaluated to %s, not true or false", name, result.Type().TypeName())
		}
		if holds {
			return name, nil
		}
	}
	return Default, nil
}
