import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/svelte";
import { afterEach, describe, expect, test, vi } from "vitest";
import CodeEditor from "./CodeEditor.svelte";
import { typeIntoEditor } from "../test-setup.js";
import type { VariableCompletion } from "./completions.js";

afterEach(cleanup);

const WORKSPACE_PATH: VariableCompletion = {
  label: "{{workspace.path}}",
  detail: "the worktree's folder on this machine",
  kind: "workspace",
};

function editor(props: {
  value?: string;
  language?: "shell" | "json" | "text";
  label?: string;
  placeholder?: string;
  onchange?: (value: string) => void;
  completions?: () => VariableCompletion[];
}) {
  const result = render(CodeEditor, {
    props: {
      value: props.value ?? "",
      label: props.label ?? "Command",
      language: props.language ?? "shell",
      placeholder: props.placeholder ?? "",
      onchange: props.onchange ?? (() => {}),
      completions: props.completions ?? (() => []),
    },
  });
  return {
    ...result,
    content: screen.getByLabelText(props.label ?? "Command"),
  };
}

// The popup ignores keys for a moment after it opens so a keystroke already
// on its way cannot pick an entry the user never saw.
const settleCompletion = () =>
  new Promise((resolve) => setTimeout(resolve, 120));

describe("CodeEditor", () => {
  test("names the editing area after the field and shows its text", () => {
    const { content } = editor({ value: "make verify", label: "Command" });

    expect(content).toHaveTextContent("make verify");
    expect(content).toHaveAttribute("contenteditable", "true");
  });

  test("reports the text as it is edited", async () => {
    const onchange = vi.fn();
    const { content } = editor({ value: "make", onchange });

    await typeIntoEditor(content, "make verify");

    expect(onchange).toHaveBeenCalledWith("make verify");
  });

  test("takes a command over several lines", async () => {
    const onchange = vi.fn();
    const { content } = editor({ value: "", onchange });

    await typeIntoEditor(content, "cd repository\nmake verify");

    expect(onchange).toHaveBeenCalledWith("cd repository\nmake verify");
    expect(content.querySelectorAll(".cm-line")).toHaveLength(2);
  });

  test("colours the shell words of a command", () => {
    const { content } = editor({ value: "make verify", language: "shell" });

    expect(content.querySelector(".tok-builtin")).toHaveTextContent("make");
  });

  test("marks a placeholder apart from the shell around it", () => {
    const { content } = editor({
      value: "cd {{workspace.path}} && make verify",
      language: "shell",
    });

    expect(content.querySelector(".tok-template")).toHaveTextContent(
      "{{workspace.path}}",
    );
  });

  test("marks a placeholder in a prompt, which has no syntax of its own", () => {
    const { content } = editor({
      value: "Work on {{result}} please",
      language: "text",
      label: "Prompt",
    });

    expect(content.querySelector(".tok-template")).toHaveTextContent(
      "{{result}}",
    );
    expect(content.querySelector(".tok-builtin")).toBeNull();
  });

  test("colours the fields of a JSON schema", () => {
    const { content } = editor({
      value: '{"type": "object"}',
      language: "json",
      label: "Schema",
    });

    expect(content.querySelector(".tok-property")).toHaveTextContent('"type"');
  });

  test("shows what the field is for while it is empty", () => {
    const { content } = editor({ value: "", placeholder: "e.g. make verify" });

    expect(content.querySelector(".cm-placeholder")).toHaveTextContent(
      "e.g. make verify",
    );
  });

  test("follows the field when another block is selected", async () => {
    const { rerender, content } = editor({ value: "make verify" });

    await rerender({ value: "go test ./..." });

    expect(content).toHaveTextContent("go test ./...");
  });

  test("leaves text the user typed alone", async () => {
    const onchange = vi.fn();
    const { rerender, content } = editor({ value: "make", onchange });

    await typeIntoEditor(content, "make verify");
    await rerender({ value: "make verify" });

    expect(content).toHaveTextContent("make verify");
    expect(content.querySelectorAll(".cm-line")).toHaveLength(1);
  });

  test("offers the variables that reach the block", async () => {
    const { content } = editor({
      value: "cd ",
      completions: () => [WORKSPACE_PATH],
    });

    content.focus();
    await typeIntoEditor(content, "cd {{");

    expect(
      await screen.findByRole("option", { name: /workspace\.path/ }),
    ).toBeInTheDocument();
  });

  test("inserts the highlighted variable on Enter without doubling braces", async () => {
    const onchange = vi.fn();
    const { content } = editor({
      value: "cd ",
      completions: () => [WORKSPACE_PATH],
      onchange,
    });

    content.focus();
    await typeIntoEditor(content, "cd {{");
    await screen.findByRole("option", { name: /workspace\.path/ });
    await settleCompletion();
    await fireEvent.keyDown(content, { key: "Enter" });

    expect(onchange).toHaveBeenLastCalledWith("cd {{workspace.path}}");
  });

  test("accepts the highlighted variable with Tab as well", async () => {
    const onchange = vi.fn();
    const { content } = editor({
      value: "cd ",
      completions: () => [WORKSPACE_PATH],
      onchange,
    });

    content.focus();
    await typeIntoEditor(content, "cd {{");
    await screen.findByRole("option", { name: /workspace\.path/ });
    await settleCompletion();
    await fireEvent.keyDown(content, { key: "Tab" });

    expect(onchange).toHaveBeenLastCalledWith("cd {{workspace.path}}");
  });

  test("offers nothing when no variable reaches the block", async () => {
    const { content } = editor({ value: "cd ", completions: () => [] });

    content.focus();
    await typeIntoEditor(content, "cd {{");
    await new Promise((resolve) => setTimeout(resolve, 60));

    expect(screen.queryByRole("option")).toBeNull();
  });
});
