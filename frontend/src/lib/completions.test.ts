import { describe, expect, test } from "vitest";
import { GraphStore } from "./graph.svelte.js";
import { blockIdentifier, completionsFor } from "./completions.js";

// A project holding a GitHub block with a Create worktree action, the usual
// way a workspace enters a flow.
function projectWithWorktree() {
  const graph = new GraphStore();
  const project = graph.addNode("project", 0, 0);
  const github = graph.addNode("github", 10, 10, project.id);
  const worktree = graph.addAction(github.id, "worktree");
  if (!worktree) throw new Error("the GitHub block took no action");
  return { graph, project, github, worktree };
}

function labels(
  graph: GraphStore,
  nodeId: string,
  language: "shell" | "json" | "text" = "text",
): string[] {
  return completionsFor(graph, nodeId, language).map(
    (completion) => completion.label,
  );
}

describe("completionsFor", () => {
  test("offers nothing to a block nothing reaches", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    const agent = graph.addNode("agent", 10, 10, project.id);

    expect(completionsFor(graph, agent.id, "text")).toEqual([]);
  });

  test("offers the workspace fields once a worktree reaches the block", () => {
    const { graph, project, worktree } = projectWithWorktree();
    const agent = graph.addNode("agent", 240, 10, project.id);
    graph.connect(worktree.id, agent.id);

    expect(labels(graph, agent.id)).toEqual([
      "{{workspace.path}}",
      "{{workspace.branch}}",
      "{{workspace.base}}",
      "{{workspace.repository}}",
    ]);
  });

  test("explains what each workspace field holds", () => {
    const { graph, project, worktree } = projectWithWorktree();
    const agent = graph.addNode("agent", 240, 10, project.id);
    graph.connect(worktree.id, agent.id);

    expect(completionsFor(graph, agent.id, "text")[0]).toEqual({
      label: "{{workspace.path}}",
      detail: "the worktree's folder on this machine",
      kind: "workspace",
    });
  });

  test("carries the workspace along the blocks that pass it on", () => {
    const { graph, project, worktree } = projectWithWorktree();
    const gate = graph.addNode("command", 240, 10, project.id);
    const fixer = graph.addNode("agent", 420, 10, project.id);
    graph.connect(worktree.id, gate.id);
    graph.connect(gate.id, fixer.id);

    expect(labels(graph, fixer.id)).toContain("{{workspace.path}}");
  });

  test("does not offer the workspace when only a plain block feeds it", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    const first = graph.addNode("agent", 10, 10, project.id);
    const second = graph.addNode("agent", 240, 10, project.id);
    graph.connect(first.id, second.id);

    expect(labels(graph, second.id)).not.toContain("{{workspace.path}}");
  });

  test("offers the single connected block's value as the result", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    const reviewer = graph.addNode("agent", 10, 10, project.id);
    graph.rename(reviewer.id, "Reviewer");
    const shipper = graph.addNode("agent", 240, 10, project.id);
    graph.connect(reviewer.id, shipper.id);

    expect(completionsFor(graph, shipper.id, "text")).toEqual([
      {
        label: "{{result}}",
        detail: "the value from Reviewer",
        kind: "result",
      },
      {
        label: "{{results.reviewer}}",
        detail: "the value from Reviewer, an Agent",
        kind: "result",
      },
    ]);
  });

  test("names every connected block when several feed the block", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    const tests = graph.addNode("command", 10, 10, project.id);
    graph.rename(tests.id, "Unit tests");
    const review = graph.addNode("agent", 10, 120, project.id);
    graph.rename(review.id, "Reviewer");
    const fixer = graph.addNode("agent", 240, 10, project.id);
    graph.connect(tests.id, fixer.id);
    graph.connect(review.id, fixer.id);

    expect(labels(graph, fixer.id)).toEqual([
      "{{results.unit-tests}}",
      "{{results.reviewer}}",
    ]);
  });

  test("offers the result again when the block runs on whichever arrives", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    const first = graph.addNode("agent", 10, 10, project.id);
    const second = graph.addNode("agent", 10, 120, project.id);
    const next = graph.addNode("agent", 240, 10, project.id);
    graph.connect(first.id, next.id);
    graph.connect(second.id, next.id);
    graph.setWaitForAny(next.id, true);

    expect(labels(graph, next.id)).toContain("{{result}}");
  });

  test("a worktree passes a workspace but no value of its own", () => {
    const { graph, project, worktree } = projectWithWorktree();
    const agent = graph.addNode("agent", 240, 10, project.id);
    graph.connect(worktree.id, agent.id);

    expect(labels(graph, agent.id)).not.toContain("{{result}}");
  });

  test("a schema check takes the reply of the agent it sits in", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    const agent = graph.addNode("agent", 10, 10, project.id);
    graph.rename(agent.id, "Reviewer");
    const check = graph.addNode("jsonschema", 20, 30, agent.id);

    expect(labels(graph, check.id)).toContain("{{results.reviewer}}");
  });

  test("a block a loop feeds takes the arrows into the loop", () => {
    const { graph, project, worktree } = projectWithWorktree();
    const loop = graph.addNode("loop", 240, 10, project.id);
    const fixer = graph.addNode("agent", 20, 40, loop.id);
    graph.connect(worktree.id, loop.id);

    expect(labels(graph, fixer.id)).toContain("{{workspace.path}}");
  });

  test("a shell field also offers the workspace environment variables", () => {
    const { graph, project, worktree } = projectWithWorktree();
    const gate = graph.addNode("command", 240, 10, project.id);
    graph.connect(worktree.id, gate.id);

    expect(labels(graph, gate.id, "shell")).toEqual([
      "{{workspace.path}}",
      "{{workspace.branch}}",
      "{{workspace.base}}",
      "{{workspace.repository}}",
      "$MEGA_AGENTS_WORKSPACE_PATH",
      "$MEGA_AGENTS_WORKSPACE_BRANCH",
      "$MEGA_AGENTS_WORKSPACE_BASE",
      "$MEGA_AGENTS_WORKSPACE_REPOSITORY",
    ]);
  });

  test("keeps the environment variables out of a field without a shell", () => {
    const { graph, project, worktree } = projectWithWorktree();
    const agent = graph.addNode("agent", 240, 10, project.id);
    graph.connect(worktree.id, agent.id);

    expect(labels(graph, agent.id, "json")).not.toContain(
      "$MEGA_AGENTS_WORKSPACE_PATH",
    );
  });

  test("offers nothing for a block that is not in the graph", () => {
    const graph = new GraphStore();

    expect(completionsFor(graph, "missing", "shell")).toEqual([]);
  });
});

describe("blockIdentifier", () => {
  test("spells a name the way the backend names its result", () => {
    expect(blockIdentifier("Unit tests")).toBe("unit-tests");
    expect(blockIdentifier("Check reply")).toBe("check-reply");
    expect(blockIdentifier("API/v2 gate")).toBe("api-v2-gate");
  });

  test("falls back to node for a name with nothing to spell", () => {
    expect(blockIdentifier("")).toBe("node");
    expect(blockIdentifier("日本語")).toBe("node");
  });
});
