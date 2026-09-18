import { screen, waitFor } from "@testing-library/svelte";

// jsdom lays nothing out, so the ranges CodeMirror measures report no
// rectangles instead of throwing while the editor takes its own size.
if (!Range.prototype.getClientRects)
  Object.assign(Range.prototype, {
    getClientRects: () => [],
    getBoundingClientRect: () => new DOMRect(),
  });

// A code field is a plain text box until the editor's chunk arrives, so a
// test that drives the editor waits for the swap the way a user sees it: the
// field named after the property becomes CodeMirror's own content element.
export async function codeEditor(label: string): Promise<HTMLElement> {
  return await waitFor(() => {
    const field = screen.getByLabelText(label);
    if (!field.classList.contains("cm-content"))
      throw new Error(`the ${label} editor has not arrived yet`);
    return field;
  });
}

// Types the text into a CodeMirror field the way a browser does: the content
// element's lines are replaced and the editor reads the change back through
// its DOM observer, which settles on the next task.
export async function typeIntoEditor(
  content: HTMLElement,
  text: string,
): Promise<void> {
  const lines = text.split("\n").map((text) => {
    const line = document.createElement("div");
    line.className = "cm-line";
    if (text === "") line.append(document.createElement("br"));
    else line.textContent = text;
    return line;
  });
  content.replaceChildren(...lines);
  // Typing leaves the caret after the text, which is where the editor looks
  // to decide whether a variable can be completed.
  const last = lines.at(-1);
  const caret = last?.firstChild;
  if (caret)
    document.getSelection()?.collapse(caret, caret.textContent?.length ?? 0);
  await new Promise((resolve) => setTimeout(resolve, 0));
}
