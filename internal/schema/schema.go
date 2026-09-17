// Package schema compiles JSON Schemas and validates values against them,
// reporting failures as field-level errors a person or an agent can act on.
package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// Schema is a compiled JSON Schema. Text is the document in compact,
// key-sorted form, the spelling passed to agent CLIs and kept in records.
type Schema struct {
	Text     string
	document map[string]any
	compiled *jsonschema.Schema
}

// FieldError is one reason a value failed its schema. Path is a JSONPath
// such as $.findings[0].file; $ is the value itself.
type FieldError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (fieldError FieldError) String() string {
	return fieldError.Path + ": " + fieldError.Message
}

// Compile accepts any valid JSON Schema document.
func Compile(text string) (*Schema, error) {
	decoded, err := jsonschema.UnmarshalJSON(strings.NewReader(text))
	if err != nil {
		return nil, fmt.Errorf("the schema is not valid JSON: %w", err)
	}
	document, ok := decoded.(map[string]any)
	if !ok {
		return nil, errors.New("the schema is not valid JSON Schema: a schema must be a JSON object")
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("mega-agents://schema.json", document); err != nil {
		return nil, fmt.Errorf("the schema is not a valid JSON Schema: %w", err)
	}
	compiled, err := compiler.Compile("mega-agents://schema.json")
	if err != nil {
		return nil, fmt.Errorf("the schema is not a valid JSON Schema: %w", err)
	}
	compact, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("the schema cannot be encoded: %w", err)
	}
	return &Schema{Text: string(compact), document: document, compiled: compiled}, nil
}

// CompileStrict accepts only the subset every agent CLI enforces the same
// way: an object root, every object closed with "additionalProperties": false
// and listing every property as required, and no oneOf, allOf or not. Codex
// rejects anything else before a turn runs, so one schema means the same on
// every backend.
func CompileStrict(text string) (*Schema, error) {
	compiled, err := Compile(text)
	if err != nil {
		return nil, err
	}
	if broken := firstViolation(compiled.document, "$"); broken != "" {
		return nil, fmt.Errorf("%s (codex rejects it; one schema has to mean the same thing on every backend)", broken)
	}
	if !isObjectNode(compiled.document) {
		return nil, errors.New("the root schema must describe a JSON object; a reply is validated as one object")
	}
	return compiled, nil
}

// Validate reports every way the value breaks the schema; none means valid.
func (compiled *Schema) Validate(value any) []FieldError {
	err := compiled.compiled.Validate(value)
	if err == nil {
		return nil
	}
	var failure *jsonschema.ValidationError
	if !errors.As(err, &failure) {
		return []FieldError{{Path: "$", Message: err.Error()}}
	}
	printer := message.NewPrinter(language.English)
	var errs []FieldError
	collectLeaves(failure, printer, &errs)
	return errs
}

// ValidateJSON parses a reply as one JSON object and validates it.
func (compiled *Schema) ValidateJSON(reply string) (map[string]any, []FieldError) {
	object, ok := parseObject(reply)
	if !ok {
		return nil, []FieldError{{Path: "$", Message: "the reply was not a JSON object"}}
	}
	return object, compiled.Validate(object)
}

// Normalize re-reads a value decoded by encoding/json so its numbers take the
// representation the validator expects.
func Normalize(value any) (any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return jsonschema.UnmarshalJSON(bytes.NewReader(encoded))
}

// ExtractObject finds the last JSON object in text, such as a reply that
// wraps its answer in prose or a code fence.
func ExtractObject(text string) (map[string]any, bool) {
	var last map[string]any
	found := false
	for start := strings.IndexByte(text, '{'); start >= 0; {
		decoder := json.NewDecoder(strings.NewReader(text[start:]))
		decoder.UseNumber()
		var candidate map[string]any
		if err := decoder.Decode(&candidate); err == nil {
			last, found = normalizeNumbers(candidate).(map[string]any), true
			start += int(decoder.InputOffset())
		} else {
			start++
		}
		next := strings.IndexByte(text[start:], '{')
		if next < 0 {
			break
		}
		start += next
	}
	return last, found
}

func parseObject(text string) (map[string]any, bool) {
	decoded, err := jsonschema.UnmarshalJSON(bytes.NewReader([]byte(strings.TrimSpace(text))))
	if err != nil {
		return nil, false
	}
	object, ok := decoded.(map[string]any)
	return object, ok
}

// normalizeNumbers turns json.Number into what jsonschema.UnmarshalJSON
// yields, so extracted objects validate exactly like parsed ones.
func normalizeNumbers(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			typed[key] = normalizeNumbers(item)
		}
	case []any:
		for i, item := range typed {
			typed[i] = normalizeNumbers(item)
		}
	case json.Number:
		if decoded, err := jsonschema.UnmarshalJSON(strings.NewReader(typed.String())); err == nil {
			return decoded
		}
	}
	return value
}

func collectLeaves(failure *jsonschema.ValidationError, printer *message.Printer, errs *[]FieldError) {
	if len(failure.Causes) == 0 {
		*errs = append(*errs, FieldError{
			Path:    jsonPath(failure.InstanceLocation),
			Message: failure.ErrorKind.LocalizedString(printer),
		})
		return
	}
	for _, cause := range failure.Causes {
		collectLeaves(cause, printer, errs)
	}
}

func jsonPath(location []string) string {
	var path strings.Builder
	path.WriteString("$")
	for _, segment := range location {
		if _, err := strconv.Atoi(segment); err == nil {
			path.WriteString("[" + segment + "]")
		} else {
			path.WriteString("." + segment)
		}
	}
	return path.String()
}

var forbiddenKeywords = []string{"oneOf", "allOf", "not"}

func declaredTypes(node map[string]any) []any {
	if types, ok := node["type"].([]any); ok {
		return types
	}
	return []any{node["type"]}
}

func isObjectNode(node map[string]any) bool {
	_, hasProperties := node["properties"]
	return slices.Contains(declaredTypes(node), any("object")) || hasProperties
}

func firstViolation(node map[string]any, where string) string {
	for _, keyword := range forbiddenKeywords {
		if _, ok := node[keyword]; ok {
			return fmt.Sprintf(`%s uses %q, which is not supported`, where, keyword)
		}
	}
	if isObjectNode(node) {
		if node["additionalProperties"] != false {
			return fmt.Sprintf(`%s must set "additionalProperties": false`, where)
		}
		required := map[string]bool{}
		if names, ok := node["required"].([]any); ok {
			for _, name := range names {
				if text, ok := name.(string); ok {
					required[text] = true
				}
			}
		}
		properties, _ := node["properties"].(map[string]any)
		var missing []string
		for name := range properties {
			if !required[name] {
				missing = append(missing, "'"+name+"'")
			}
		}
		slices.Sort(missing)
		if len(missing) > 0 {
			return fmt.Sprintf(`%s must list every property in "required"; missing %s`, where, strings.Join(missing, ", "))
		}
	}
	for _, child := range subschemas(node, where) {
		if broken := firstViolation(child.node, child.where); broken != "" {
			return broken
		}
	}
	return ""
}

type located struct {
	node  map[string]any
	where string
}

func subschemas(node map[string]any, where string) []located {
	var children []located
	properties, _ := node["properties"].(map[string]any)
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if child, ok := properties[name].(map[string]any); ok {
			children = append(children, located{child, where + ".properties." + name})
		}
	}
	if items, ok := node["items"].(map[string]any); ok {
		children = append(children, located{items, where + ".items"})
	}
	if branches, ok := node["anyOf"].([]any); ok {
		for index, branch := range branches {
			if child, ok := branch.(map[string]any); ok {
				children = append(children, located{child, fmt.Sprintf("%s.anyOf[%d]", where, index)})
			}
		}
	}
	definitions, _ := node["$defs"].(map[string]any)
	definitionNames := make([]string, 0, len(definitions))
	for name := range definitions {
		definitionNames = append(definitionNames, name)
	}
	slices.Sort(definitionNames)
	for _, name := range definitionNames {
		if child, ok := definitions[name].(map[string]any); ok {
			children = append(children, located{child, where + ".$defs." + name})
		}
	}
	return children
}
