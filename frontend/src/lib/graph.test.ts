import { describe, expect, test } from "vitest";
import { GraphStore, PALETTE, type NodeType } from "./graph.svelte.js";

describe("GraphStore", () => {
  test("adds a node with a default name, position, and selection", () => {
    const graph = new GraphStore();

    const node = graph.addNode("agent", 40, 60);

    expect(graph.nodes).toHaveLength(1);
    expect(node.type).toBe("agent");
    expect(node.name).toBe("Agent 1");
    expect(node.x).toBe(40);
    expect(node.y).toBe(60);
    expect(graph.selectedId).toBe(node.id);
    expect(graph.selected).toEqual(node);
  });

  test("numbers nodes of the same type sequentially", () => {
    const graph = new GraphStore();

    const first = graph.addNode("tool", 0, 0);
    const second = graph.addNode("tool", 10, 10);

    expect(first.name).toBe("Tool 1");
    expect(second.name).toBe("Tool 2");
  });

  test("selection follows the requested node and can be cleared", () => {
    const graph = new GraphStore();

    const first = graph.addNode("agent", 0, 0);
    const second = graph.addNode("tool", 10, 10);

    graph.select(first.id);
    expect(graph.selected?.id).toBe(first.id);

    graph.select(second.id);
    expect(graph.selected?.id).toBe(second.id);

    graph.select(null);
    expect(graph.selected).toBeUndefined();
  });

  test("moves a node to a new position", () => {
    const graph = new GraphStore();
    const node = graph.addNode("agent", 10, 20);

    graph.moveNode(node.id, 50, 60);

    expect(graph.nodes[0]?.x).toBe(50);
    expect(graph.nodes[0]?.y).toBe(60);
  });

  test("moving an unknown id is a no-op", () => {
    const graph = new GraphStore();
    const node = graph.addNode("agent", 10, 20);

    graph.moveNode("not-a-node", 50, 60);

    expect(graph.nodes[0]?.x).toBe(10);
    expect(graph.nodes[0]?.y).toBe(20);
    expect(graph.nodes[0]?.id).toBe(node.id);
  });

  test("renames a node by id", () => {
    const graph = new GraphStore();
    const node = graph.addNode("agent", 0, 0);

    graph.rename(node.id, "Fetcher");

    expect(graph.nodes[0]?.name).toBe("Fetcher");
  });

  test("adds project nodes with a default name", () => {
    const graph = new GraphStore();

    const node = graph.addNode("project", 5, 5);

    expect(node.type).toBe("project");
    expect(node.name).toBe("Project 1");
  });

  test("assigns a project folder to path and derives the name from it", () => {
    const graph = new GraphStore();
    const node = graph.addNode("project", 5, 5);

    graph.assignProjectFolder(node.id, "/home/user/my-agent");

    expect(graph.nodes[0]?.path).toBe("/home/user/my-agent");
    expect(graph.nodes[0]?.name).toBe("my-agent");
  });

  test("keeps the assigned name when the path has no folder segment", () => {
    const graph = new GraphStore();
    const node = graph.addNode("project", 5, 5);
    graph.rename(node.id, "Agent root");

    graph.assignProjectFolder(node.id, "/");

    expect(graph.nodes[0]?.path).toBe("/");
    expect(graph.nodes[0]?.name).toBe("Agent root");
  });

  test("assigning an empty project folder is a no-op", () => {
    const graph = new GraphStore();
    const node = graph.addNode("project", 5, 5);

    graph.assignProjectFolder(node.id, "");

    expect(graph.nodes[0]?.name).toBe("Project 1");
    expect(graph.nodes[0]?.path).toBeUndefined();
  });

  test("sets a path by id", () => {
    const graph = new GraphStore();
    const node = graph.addNode("project", 5, 5);

    graph.setPath(node.id, "~/code/my-agent");

    expect(graph.nodes[0]?.path).toBe("~/code/my-agent");
  });

  test("setting a path on an unknown id is a no-op", () => {
    const graph = new GraphStore();
    const node = graph.addNode("project", 5, 5);

    graph.setPath("not-a-node", "~/code/other");

    expect(graph.nodes[0]?.id).toBe(node.id);
    expect(graph.nodes[0]?.path).toBeUndefined();
  });

  test("renaming an unknown id is a no-op", () => {
    const graph = new GraphStore();
    const node = graph.addNode("agent", 0, 0);

    graph.rename("not-a-node", "Fetcher");

    expect(graph.nodes[0]?.name).toBe("Agent 1");
    expect(graph.nodes[0]?.id).toBe(node.id);
  });

  test("labels unknown component types with the raw type", () => {
    const graph = new GraphStore();

    const node = graph.addNode("mystery" as NodeType, 5, 5);

    expect(node.name).toBe("mystery 1");
    expect(node.type).toBe("mystery");
  });
});

describe("PALETTE", () => {
  test("offers the agent, tool, and project components", () => {
    expect(PALETTE).toEqual([
      { type: "agent", label: "Agent" },
      { type: "tool", label: "Tool" },
      { type: "project", label: "Project" },
    ]);
  });
});
