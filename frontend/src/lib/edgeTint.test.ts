import { describe, expect, test } from "vitest";
import { edgeHue, linkSourceId } from "./edgeTint.js";
import { BLOCK_HUES, type NodeType } from "./graph.svelte.js";

describe("the hue an arrow takes", () => {
  test("an arrow takes the hue of the block it leaves", () => {
    expect(edgeHue("command")).toBe(BLOCK_HUES.command);
    expect(edgeHue("agent")).toBe(BLOCK_HUES.agent);
  });

  test("two block types give an arrow two different hues", () => {
    expect(edgeHue("command")).not.toBe(edgeHue("agent"));
  });

  test("an arrow whose source has not resolved keeps the neutral colour", () => {
    expect(edgeHue(undefined)).toBeUndefined();
  });

  test("a block type with no hue keeps the neutral colour", () => {
    expect(edgeHue("stencil" as NodeType)).toBeUndefined();
  });
});

describe("which block an arrow being drawn leaves", () => {
  test("a new arrow leaves the block its handle was dragged from", () => {
    expect(linkSourceId({ anchorId: "n1", end: "to", targetId: "n2" })).toBe(
      "n1",
    );
  });

  test("a head dragged over nothing still leaves its own block", () => {
    expect(linkSourceId({ anchorId: "n1", end: "to" })).toBe("n1");
  });

  test("a tail dragged onto a block leaves that block", () => {
    expect(linkSourceId({ anchorId: "n1", end: "from", targetId: "n2" })).toBe(
      "n2",
    );
  });

  test("a tail dragged over nothing has no source yet", () => {
    expect(linkSourceId({ anchorId: "n1", end: "from" })).toBeUndefined();
  });
});
