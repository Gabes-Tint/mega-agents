import "@testing-library/jest-dom/vitest";
import { cleanup, render } from "@testing-library/svelte";
import { afterEach, describe, expect, test } from "vitest";
import NodeMark from "./NodeMark.svelte";
import { MARK_LABELS, type MarkKind } from "./nodeMarks.js";

afterEach(cleanup);

function mark(kind: MarkKind, id?: string): HTMLElement {
  const { container } = render(NodeMark, { kind, id });
  const element = container.querySelector<HTMLElement>(".node-mark");
  if (!element) throw new Error(`${kind} drew nothing`);
  return element;
}

describe("the marks a block header carries", () => {
  test("each mark names itself for the pointer and for readers", () => {
    for (const kind of Object.keys(MARK_LABELS) as MarkKind[]) {
      const element = mark(kind);
      expect(element).toHaveAttribute("data-mark", kind);
      expect(element).toHaveAttribute("title", MARK_LABELS[kind]);
      // The icon is decorative, so the block's name stays its own; the text
      // beside it is what an aria-describedby reference announces.
      expect(element).toHaveAttribute("aria-hidden", "true");
      expect(element).toHaveTextContent(MARK_LABELS[kind]);
      cleanup();
    }
  });

  test("the start flag is plain and the finish flag is chequered", () => {
    expect(mark("start").querySelectorAll(".chequer")).toHaveLength(0);
    cleanup();
    const end = mark("end");
    expect(end.querySelector(".cloth")).toBeInTheDocument();
    expect(end.querySelector(".chequer")).toBeInTheDocument();
  });

  test("a running block spins and the finished ones hold still", () => {
    expect(mark("running").querySelector(".spinner")).toBeInTheDocument();
    cleanup();
    for (const kind of ["succeeded", "failed", "skipped"] as MarkKind[]) {
      const element = mark(kind);
      expect(element.querySelector(".spinner")).toBeNull();
      expect(element.querySelector(".line")).toBeInTheDocument();
      cleanup();
    }
  });

  test("a mark takes an id so a block can point its description at it", () => {
    expect(mark("running", "status-n1")).toHaveAttribute("id", "status-n1");
  });
});
