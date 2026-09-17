import { describe, expect, test } from "vitest";
import {
  CompletionContext,
  type CompletionResult,
} from "@codemirror/autocomplete";
import { EditorState } from "@codemirror/state";
import { variableCompletionSource } from "./editorExtensions.js";
import type { VariableCompletion } from "./completions.js";

const VARIABLES: VariableCompletion[] = [
  {
    label: "{{workspace.path}}",
    detail: "the worktree's folder on this machine",
    kind: "workspace",
  },
  {
    label: "{{result}}",
    detail: "the value from Reviewer",
    kind: "result",
  },
  {
    label: "$MEGA_AGENTS_WORKSPACE_PATH",
    detail: "the worktree's folder on this machine",
    kind: "env",
  },
];

function complete(
  doc: string,
  pos: number,
  explicit = false,
  variables = VARIABLES,
): CompletionResult | null {
  const state = EditorState.create({ doc, selection: { anchor: pos } });
  return variableCompletionSource(() => variables)(
    new CompletionContext(state, pos, explicit),
  );
}

describe("variableCompletionSource", () => {
  test("offers the placeholders as soon as two braces are open", () => {
    const result = complete("cd {{", 5);

    expect(result?.from).toBe(3);
    expect(result?.options.map((option) => option.label)).toEqual([
      "{{workspace.path}}",
      "{{result}}",
    ]);
  });

  test("keeps offering them while the name is being typed", () => {
    const result = complete("cd {{works", 10);

    expect(result?.from).toBe(3);
    expect(result?.options.map((option) => option.label)).toContain(
      "{{workspace.path}}",
    );
  });

  test("swallows braces the editor already closed so none are doubled", () => {
    const result = complete("cd {{}}", 5);

    expect(result?.from).toBe(3);
    expect(result?.to).toBe(7);
  });

  test("stops at the end of the placeholder being replaced", () => {
    const result = complete("cd {{", 5);

    expect(result?.to).toBe(5);
  });

  test("offers the environment variables after a dollar sign", () => {
    const result = complete("echo $", 6);

    expect(result?.from).toBe(5);
    expect(result?.options.map((option) => option.label)).toEqual([
      "$MEGA_AGENTS_WORKSPACE_PATH",
    ]);
  });

  test("spells the variable in braces when the braces are open", () => {
    const result = complete("echo ${", 7);

    expect(result?.from).toBe(5);
    expect(result?.options.map((option) => option.label)).toEqual([
      "${MEGA_AGENTS_WORKSPACE_PATH}",
    ]);
  });

  test("offers everything when the completion is asked for", () => {
    const result = complete("make verify", 11, true);

    expect(result?.from).toBe(11);
    expect(result?.options.map((option) => option.label)).toEqual([
      "{{workspace.path}}",
      "{{result}}",
      "$MEGA_AGENTS_WORKSPACE_PATH",
    ]);
  });

  test("keeps quiet while plain text is typed", () => {
    expect(complete("make verify", 11)).toBeNull();
  });

  test("keeps quiet when nothing reaches the block", () => {
    expect(complete("cd {{", 5, false, [])).toBeNull();
  });

  test("keeps the order the block offers the variables in", () => {
    const result = complete("cd {{", 5);

    // Equally good matches are otherwise listed alphabetically, which would
    // bury {{workspace.path}} under the fields nobody reaches for first.
    const [first, second] = result?.options ?? [];
    expect(first?.label).toBe("{{workspace.path}}");
    expect(first?.boost ?? 0).toBeGreaterThan(second?.boost ?? 0);
  });

  test("explains each variable in the popup", () => {
    const result = complete("cd {{", 5);

    expect(result?.options[0]).toMatchObject({
      detail: "the worktree's folder on this machine",
      type: "variable",
    });
  });
});
