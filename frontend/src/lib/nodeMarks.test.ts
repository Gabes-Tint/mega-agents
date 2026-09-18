import { describe, expect, test } from "vitest";
import { endingNodeIds, MARK_LABELS, statusMark } from "./nodeMarks.js";
import { GraphStore } from "./graph.svelte.js";

// The blocks the canvas would mark with the finish flag, by name.
function ends(graph: GraphStore): string[] {
  const ending = endingNodeIds(graph.nodes, graph.edges);
  return graph.nodes
    .filter((node) => ending.has(node.id))
    .map((node) => node.name)
    .sort();
}

describe("where a run can finish", () => {
  test("a lone starting block is both the start and the end", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    const coder = graph.addNode("agent", 20, 40, project.id);

    graph.setStart(coder.id, true);

    expect(ends(graph)).toEqual(["Agent 1"]);
  });

  test("an arrow out of a block hands the end on, and losing it takes it back", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    const coder = graph.addNode("agent", 20, 40, project.id);
    const reviewer = graph.addNode("agent", 240, 40, project.id);
    graph.setStart(coder.id, true);

    graph.connect(coder.id, reviewer.id);

    expect(ends(graph)).toEqual(["Agent 2"]);
    graph.removeEdge(graph.edges[0]?.id ?? "");
    expect(ends(graph)).toEqual(["Agent 1"]);
  });

  test("blocks no starting point reaches end nothing", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    const coder = graph.addNode("agent", 20, 40, project.id);
    const stray = graph.addNode("agent", 240, 40, project.id);

    expect(ends(graph)).toEqual([]);
    graph.setStart(coder.id, true);
    expect(ends(graph)).toEqual(["Agent 1"]);
    expect(ends(graph)).not.toContain(stray.name);
  });

  test("a GitHub block ends at the last action of its sequence, not itself", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    const github = graph.addNode("github", 20, 40, project.id);
    graph.addAction(github.id, "fetch");
    graph.addAction(github.id, "worktree");
    graph.setStart(github.id, true);

    expect(ends(graph)).toEqual(["Create worktree"]);
  });

  test("an action feeding a block beside its GitHub block is not the end", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    const github = graph.addNode("github", 20, 40, project.id);
    const fetch = graph.addAction(github.id, "fetch");
    const coder = graph.addNode("agent", 320, 40, project.id);
    graph.setStart(github.id, true);

    graph.connect(fetch?.id ?? "", coder.id);

    expect(ends(graph)).toEqual(["Agent 1"]);
  });

  test("a loop ends the flow; the blocks it repeats do not", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    const coder = graph.addNode("agent", 20, 40, project.id);
    const loop = graph.addNode("loop", 20, 200, project.id);
    graph.addNode("command", 20, 40, loop.id);
    graph.addNode("agent", 220, 40, loop.id);
    graph.setStart(coder.id, true);
    graph.connect(coder.id, loop.id);

    expect(ends(graph)).toEqual(["Loop 1"]);
  });

  test("a schema check ends the flow while any of its outputs is free", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    const coder = graph.addNode("agent", 20, 40, project.id);
    const check = graph.addNode("jsonschema", 10, 30, coder.id);
    const fixer = graph.addNode("agent", 240, 40, project.id);
    const shipper = graph.addNode("agent", 460, 40, project.id);
    graph.setStart(coder.id, true);

    // The agent hands its reply to the check inside it, so it is not an end.
    expect(ends(graph)).toEqual([check.name]);
    graph.connect(check.id, fixer.id, "invalid");
    expect(ends(graph)).toEqual(["Agent 2", check.name].sort());
    graph.connect(check.id, shipper.id, "valid");
    expect(ends(graph)).toEqual(["Agent 2", "Agent 3"]);
  });

  test("a router ends the flow while any of its routes is free", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    const coder = graph.addNode("agent", 20, 40, project.id);
    // A new router routes an "approved" case and everything else.
    const router = graph.addNode("router", 240, 40, project.id);
    const fixer = graph.addNode("agent", 460, 40, project.id);
    const shipper = graph.addNode("agent", 680, 40, project.id);
    graph.setStart(coder.id, true);
    graph.connect(coder.id, router.id);

    expect(ends(graph)).toEqual(["Router 1"]);
    graph.connect(router.id, fixer.id, "default");
    expect(ends(graph)).toEqual(["Agent 2", "Router 1"]);
    graph.connect(router.id, shipper.id, "approved");
    expect(ends(graph)).toEqual(["Agent 2", "Agent 3"]);
    graph.addCase(router.id);
    expect(ends(graph)).toEqual(["Agent 2", "Agent 3", "Router 1"]);
  });

  test("a starting block inside a loop leaves the flow without an end", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    const loop = graph.addNode("loop", 20, 40, project.id);
    const fixer = graph.addNode("agent", 20, 40, loop.id);

    graph.setStart(fixer.id, true);

    expect(ends(graph)).toEqual([]);
  });
});

describe("the mark a run status carries", () => {
  test("only a block the run has reached carries one", () => {
    expect(statusMark("running")).toBe("running");
    expect(statusMark("succeeded")).toBe("succeeded");
    expect(statusMark("failed")).toBe("failed");
    expect(statusMark("skipped")).toBe("skipped");
    expect(statusMark("pending")).toBeUndefined();
    expect(statusMark(undefined)).toBeUndefined();
  });

  test("a run waiting at a breakpoint marks the block it waits before", () => {
    expect(statusMark("paused")).toBe("paused");
  });

  test("every mark says what it means", () => {
    expect(Object.values(MARK_LABELS)).toEqual([
      "Starting point",
      "Ending block",
      "Breakpoint",
      "Running",
      "Paused at a breakpoint",
      "Succeeded",
      "Failed",
      "Skipped",
    ]);
  });
});
