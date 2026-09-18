import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/svelte";
import { afterEach, describe, expect, test, vi } from "vitest";
import CodeEditor from "./CodeEditor.svelte";

// The editor's code is a chunk of its own, fetched when a field first mounts.
// This file stands in for a network that never delivers it.
vi.mock("./codeEditorView.js", () => {
  throw new Error("chunk unavailable");
});

afterEach(cleanup);

function field(onchange: (value: string) => void = () => {}) {
  render(CodeEditor, {
    props: {
      value: "make",
      label: "Command",
      language: "shell",
      placeholder: "",
      onchange,
      completions: () => [],
    },
  });
}

describe("CodeEditor without the editor's code", () => {
  test("keeps the field editable as a plain text box", async () => {
    const onchange = vi.fn();
    field(onchange);

    const box = await screen.findByLabelText("Command");
    expect(box.tagName).toBe("TEXTAREA");
    expect(box).toHaveValue("make");

    await fireEvent.input(box, { target: { value: "make verify" } });

    expect(onchange).toHaveBeenCalledWith("make verify");
  });

  test("says that the editor is missing rather than failing silently", async () => {
    field();

    expect(await screen.findByRole("status")).toHaveTextContent(
      /could not load/i,
    );
  });
});
