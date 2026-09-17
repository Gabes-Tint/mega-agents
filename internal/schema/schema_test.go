package schema

import (
	"strings"
	"testing"
)

const verdict = `{
	"type": "object",
	"additionalProperties": false,
	"required": ["verdict", "reason", "findings"],
	"properties": {
		"verdict": {"type": "string", "enum": ["approve", "reject"]},
		"reason": {"type": "string", "minLength": 1},
		"findings": {
			"type": "array",
			"items": {
				"type": "object",
				"additionalProperties": false,
				"required": ["file", "line"],
				"properties": {"file": {"type": "string"}, "line": {"type": ["integer", "null"]}}
			}
		}
	}
}`

func TestCompileAcceptsAStrictObjectSchema(t *testing.T) {
	compiled, err := CompileStrict(verdict)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !strings.HasPrefix(compiled.Text, `{"additionalProperties":false,`) {
		t.Fatalf("text = %s, want the compact, key-sorted document", compiled.Text)
	}
}

func TestValidateReturnsTheValueOrFieldErrors(t *testing.T) {
	compiled, err := Compile(verdict)
	if err != nil {
		t.Fatal(err)
	}

	value, errs := compiled.ValidateJSON(`{"verdict": "approve", "reason": "ok", "findings": [{"file": "a.go", "line": 3}]}`)
	if len(errs) != 0 || value["verdict"] != "approve" {
		t.Fatalf("value = %v, errors = %v", value, errs)
	}

	_, errs = compiled.ValidateJSON(`{"verdict": "maybe", "reason": "", "findings": [{"file": 7, "line": null}]}`)
	got := map[string]string{}
	for _, fieldError := range errs {
		got[fieldError.Path] = fieldError.Message
	}
	for path, fragment := range map[string]string{
		"$.verdict":          "approve",
		"$.reason":           "minLength",
		"$.findings[0].file": "string",
	} {
		if !strings.Contains(strings.ToLower(got[path]), strings.ToLower(fragment)) {
			t.Errorf("errors = %v, want %s mentioning %q", errs, path, fragment)
		}
	}
}

func TestValidateRejectsReplyThatIsNotAnObject(t *testing.T) {
	compiled, _ := Compile(verdict)

	for _, reply := range []string{"Sure! Here it is", `["approve"]`, ""} {
		if _, errs := compiled.ValidateJSON(reply); len(errs) != 1 || errs[0].Path != "$" ||
			!strings.Contains(errs[0].Message, "not a JSON object") {
			t.Errorf("ValidateJSON(%q) errors = %v", reply, errs)
		}
	}
}

func TestExtractObjectFindsTheLastObjectInProse(t *testing.T) {
	reply := "Here is my answer:\n```json\n{\"draft\": true}\n```\nFinal:\n```json\n{\"verdict\": \"approve\", \"nested\": {\"a\": \"}\"}}\n```"

	object, ok := ExtractObject(reply)

	if !ok || object["verdict"] != "approve" {
		t.Fatalf("object = %v, %v", object, ok)
	}
	if _, ok := ExtractObject("no json here {"); ok {
		t.Fatal("prose without an object must not yield one")
	}
}

func TestCompileStrictRejectsSchemasOutsideTheStrictSubset(t *testing.T) {
	cases := map[string]struct {
		schema string
		want   string
	}{
		"not json": {
			schema: `{`,
			want:   "not valid JSON",
		},
		"not a schema": {
			schema: `{"type": "object", "additionalProperties": false, "required": [], "properties": {}, "minProperties": "x"}`,
			want:   "not a valid JSON Schema",
		},
		"open object": {
			schema: `{"type": "object", "required": ["a"], "properties": {"a": {"type": "string"}}}`,
			want:   `$ must set "additionalProperties": false`,
		},
		"optional property": {
			schema: `{"type": "object", "additionalProperties": false, "required": [], "properties": {"a": {"type": "string"}}}`,
			want:   `$ must list every property in "required"; missing 'a'`,
		},
		"nested open object": {
			schema: `{"type": "object", "additionalProperties": false, "required": ["a"], "properties": {"a": {"type": "array", "items": {"properties": {}}}}}`,
			want:   `$.properties.a.items must set "additionalProperties": false`,
		},
		"oneOf": {
			schema: `{"oneOf": [{"type": "object"}]}`,
			want:   `$ uses "oneOf", which is not supported`,
		},
		"not an object root": {
			schema: `{"type": "array"}`,
			want:   "the root schema must describe a JSON object",
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := CompileStrict(testCase.schema)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("err = %v, want it to contain %q", err, testCase.want)
			}
		})
	}
}

func TestCompileAcceptsAnyValidSchemaForValidationBlocks(t *testing.T) {
	compiled, err := Compile(`{"type": "array", "items": {"oneOf": [{"type": "string"}, {"type": "integer"}]}}`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if errs := compiled.Validate([]any{"a", 1.0}); len(errs) != 0 {
		t.Fatalf("errors = %v", errs)
	}
	if errs := compiled.Validate([]any{true}); len(errs) == 0 || errs[0].Path != "$[0]" {
		t.Fatalf("errors = %v, want one at $[0]", errs)
	}
	for _, invalid := range []string{`{`, `{"type": 5}`} {
		if _, err := Compile(invalid); err == nil {
			t.Errorf("Compile(%s) accepted an invalid schema", invalid)
		}
	}
}
