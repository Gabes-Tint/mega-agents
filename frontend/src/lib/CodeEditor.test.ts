import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/svelte";
import { afterEach, describe, expect, test, vi } from "vitest";
import CodeEditor from "./CodeEditor.svelte";
import { codeEditor, typeIntoEditor } from "../test-setup.js";
import type { VariableCompletion } from "./completions.js";

afterEach(cleanup);

const WORKSPACE_PATH: VariableCompletion = {
  label: "{{workspace.path}}",
  detail: "the worktree's folder on this machine",
  kind: "workspace",
};

interface EditorProps {
  value?: string;
  language?: "shell" | "json" | "text";
  label?: string;
  placeholder?: string;
  onchange?: (value: string) => void;
  completions?: () => VariableCompletion[];
}

function mount(props: EditorProps) {
  return render(CodeEditor, {
    props: {
      value: props.value ?? "",
      label: props.label ?? "Command",
      language: props.language ?? "shell",
      placeholder: props.placeholder ?? "",
      onchange: props.onchange ?? (() => {}),
      completions: props.completions ?? (() => []),
    },
  });
}

// The editor arrives with its own chunk, so the tests that drive it wait for
// the field to become the editor rather than the plain box it starts as.
async function editor(props: EditorProps) {
  const result = mount(props);
  return { ...result, content: await codeEditor(props.label ?? "Command") };
}

// The popup ignores keys for a moment after it opens so a keystroke already
// on its way cannot pick an entry the user never saw.
const settleCompletion = () =>
  new Promise((resolve) => setTimeout(resolve, 120));

describe("CodeEditor", () => {
  test("names the editing area after the field and shows its text", async () => {
    const { content } = await editor({
      value: "make verify",
      label: "Command",
    });

    expect(content).toHaveTextContent("make verify");
    expect(content).toHaveAttribute("contenteditable", "true");
  });

  test("reports the text as it is edited", async () => {
    const onchange = vi.fn();
    const { content } = await editor({ value: "make", onchange });

    await typeIntoEditor(content, "make verify");

    expect(onchange).toHaveBeenCalledWith("make verify");
  });

  test("takes a command over several lines", async () => {
    const onchange = vi.fn();
    const { content } = await editor({ value: "", onchange });

    await typeIntoEditor(content, "cd repository\nmake verify");

    expect(onchange).toHaveBeenCalledWith("cd repository\nmake verify");
    expect(content.querySelectorAll(".cm-line")).toHaveLength(2);
  });

  test("colours the shell words of a command", async () => {
    const { content } = await editor({
      value: "make verify",
      language: "shell",
    });

    expect(content.querySelector(".tok-builtin")).toHaveTextContent("make");
  });

  test("marks a placeholder apart from the shell around it", async () => {
    const { content } = await editor({
      value: "cd {{workspace.path}} && make verify",
      language: "shell",
    });

    expect(content.querySelector(".tok-template")).toHaveTextContent(
      "{{workspace.path}}",
    );
  });

  test("marks a placeholder in a prompt, which has no syntax of its own", async () => {
    const { content } = await editor({
      value: "Work on {{result}} please",
      language: "text",
      label: "Prompt",
    });

    expect(content.querySelector(".tok-template")).toHaveTextContent(
      "{{result}}",
    );
    expect(content.querySelector(".tok-builtin")).toBeNull();
  });

  test("colours the fields of a JSON schema", async () => {
    const { content } = await editor({
      value: '{"type": "object"}',
      language: "json",
      label: "Schema",
    });

    expect(content.querySelector(".tok-property")).toHaveTextContent('"type"');
  });

  test("shows what the field is for while it is empty", async () => {
    const { content } = await editor({
      value: "",
      placeholder: "e.g. make verify",
    });

    expect(content.querySelector(".cm-placeholder")).toHaveTextContent(
      "e.g. make verify",
    );
  });

  test("follows the field when another block is selected", async () => {
    const { rerender, content } = await editor({ value: "make verify" });

    await rerender({ value: "go test ./..." });

    expect(content).toHaveTextContent("go test ./...");
  });

  test("leaves text the user typed alone", async () => {
    const onchange = vi.fn();
    const { rerender, content } = await editor({ value: "make", onchange });

    await typeIntoEditor(content, "make verify");
    await rerender({ value: "make verify" });

    expect(content).toHaveTextContent("make verify");
    expect(content.querySelectorAll(".cm-line")).toHaveLength(1);
  });

  test("offers the variables that reach the block", async () => {
    const { content } = await editor({
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
    const { content } = await editor({
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
    const { content } = await editor({
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
    const { content } = await editor({ value: "cd ", completions: () => [] });

    content.focus();
    await typeIntoEditor(content, "cd {{");
    await new Promise((resolve) => setTimeout(resolve, 60));

    expect(screen.queryByRole("option")).toBeNull();
  });
});

// The editor's code is fetched only when a field first mounts, so every field
// is a plain text box for as long as that takes.
describe("CodeEditor while the editor's code is on its way", () => {
  test("shows the field's text in a box that already takes typing", () => {
    mount({ value: "make verify", label: "Command" });

    const box = screen.getByLabelText("Command");
    expect(box.tagName).toBe("TEXTAREA");
    expect(box).toHaveValue("make verify");
  });

  test("carries what was typed into the editor that replaces the box", async () => {
    const onchange = vi.fn();
    mount({ value: "make", onchange });

    // Typed while the chunk is still on its way: the keystroke reaches the
    // box before anything awaits, which is the moment the swap must respect.
    const box = screen.getByLabelText("Command") as HTMLTextAreaElement;
    box.value = "make verify";
    box.dispatchEvent(new Event("input", { bubbles: true }));
    const content = await codeEditor("Command");

    expect(onchange).toHaveBeenCalledWith("make verify");
    expect(content).toHaveTextContent("make verify");
  });

  test("moves the focus on when the box was the one being typed in", async () => {
    mount({ value: "make" });

    screen.getByLabelText("Command").focus();
    const content = await codeEditor("Command");

    expect(content).toHaveFocus();
  });

  test("leaves the box behind, so the field is named once", async () => {
    mount({ value: "make" });

    await codeEditor("Command");

    expect(screen.getAllByLabelText("Command")).toHaveLength(1);
  });

  test("leaves a box nobody touched alone", async () => {
    mount({ value: "make" });

    const content = await codeEditor("Command");

    expect(content).not.toHaveFocus();
  });
});
