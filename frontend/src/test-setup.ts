// jsdom lays nothing out, so the ranges CodeMirror measures report no
// rectangles instead of throwing while the editor takes its own size.
if (!Range.prototype.getClientRects)
  Object.assign(Range.prototype, {
    getClientRects: () => [],
    getBoundingClientRect: () => new DOMRect(),
  });

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
