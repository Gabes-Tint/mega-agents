import { describe, expect, test } from "vitest";
import {
  DEFAULT_NODE_HEIGHT,
  DEFAULT_NODE_WIDTH,
  GraphStore,
  MIN_NODE_HEIGHT,
  MIN_NODE_WIDTH,
  PALETTE,
  type NodeType,
} from "./graph.svelte.js";

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

  test("adds a node with the default box size", () => {
    const graph = new GraphStore();

    const node = graph.addNode("agent", 5, 5);

    expect(node.w).toBe(DEFAULT_NODE_WIDTH);
    expect(node.h).toBe(DEFAULT_NODE_HEIGHT);
  });

  test("resizes a node to explicit dimensions", () => {
    const graph = new GraphStore();
    const node = graph.addNode("agent", 5, 5);

    graph.resizeNode(node.id, 320, 200);

    expect(graph.nodes[0]?.w).toBe(320);
    expect(graph.nodes[0]?.h).toBe(200);
  });

  test("resizing clamps dimensions to the box minimums", () => {
    const graph = new GraphStore();
    const node = graph.addNode("agent", 5, 5);

    graph.resizeNode(node.id, MIN_NODE_WIDTH - 60, MIN_NODE_HEIGHT - 40);

    expect(graph.nodes[0]?.w).toBe(MIN_NODE_WIDTH);
    expect(graph.nodes[0]?.h).toBe(MIN_NODE_HEIGHT);
  });

  test("resizing an unknown id is a no-op", () => {
    const graph = new GraphStore();
    const node = graph.addNode("agent", 5, 5);

    graph.resizeNode("not-a-node", 320, 200);

    expect(graph.nodes[0]?.w).toBe(DEFAULT_NODE_WIDTH);
    expect(graph.nodes[0]?.h).toBe(DEFAULT_NODE_HEIGHT);
    expect(graph.nodes[0]?.id).toBe(node.id);
  });

  test("adds a node inside a parent with parent-relative coordinates", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 30, 20);

    const child = graph.addNode("agent", 70, 30, project.id);

    expect(child.parentId).toBe(project.id);
    expect(child.x).toBe(70);
    expect(child.y).toBe(30);
  });

  test("attaches a top-level node to a project and converts its coordinates", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 100, 80);
    const agent = graph.addNode("agent", 130, 130);

    graph.attachToProject(agent.id, project.id, 140, 100);

    expect(graph.nodes.at(-1)?.parentId).toBe(project.id);
    expect(graph.nodes.at(-1)?.x).toBe(40);
    expect(graph.nodes.at(-1)?.y).toBe(20);
  });

  test("attaching with an unknown node or parent is a no-op", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 100, 80);
    graph.addNode("agent", 130, 130);

    graph.attachToProject(graph.nodes.at(-1)?.id ?? "", "not-a-node", 140, 100);
    graph.attachToProject("not-a-node", project.id, 140, 100);

    expect(graph.nodes.at(-1)?.parentId).toBeUndefined();
    expect(graph.nodes.at(-1)?.x).toBe(130);
  });

  test("attaching to a project that is itself nested is a no-op", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 100, 80);
    const nested = graph.addNode("project", 110, 90, project.id);
    graph.addNode("agent", 130, 130);

    graph.attachToProject(graph.nodes.at(-1)?.id ?? "", nested.id, 140, 100);

    expect(graph.nodes.at(-1)?.parentId).toBeUndefined();
    expect(graph.nodes.at(-1)?.x).toBe(130);
  });

  test("attaching a node that already has children is a no-op", () => {
    const graph = new GraphStore();
    const first = graph.addNode("project", 100, 80);
    graph.addNode("agent", 120, 100, first.id);
    graph.addNode("project", 300, 300);

    graph.attachToProject(first.id, graph.nodes.at(-1)?.id ?? "", 320, 320);

    expect(graph.nodes[0]?.parentId).toBeUndefined();
    expect(graph.nodes[0]?.x).toBe(100);
    expect(graph.nodes[0]?.y).toBe(80);
  });

  test("attaching a node to itself is a no-op", () => {
    const graph = new GraphStore();
    graph.addNode("project", 100, 80);

    graph.attachToProject(
      graph.nodes[0]?.id ?? "",
      graph.nodes[0]?.id ?? "",
      120,
      100,
    );

    expect(graph.nodes[0]?.parentId).toBeUndefined();
    expect(graph.nodes[0]?.x).toBe(100);
  });

  test("detaches a child back to the canvas with absolute coordinates", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 100, 80);
    graph.addNode("agent", 30, 20, project.id);

    graph.detachNode(graph.nodes.at(-1)?.id ?? "", 260, 180);

    expect(graph.nodes.at(-1)?.parentId).toBeUndefined();
    expect(graph.nodes.at(-1)?.x).toBe(260);
    expect(graph.nodes.at(-1)?.y).toBe(180);
  });

  test("detaching a top-level node is a no-op", () => {
    const graph = new GraphStore();
    graph.addNode("agent", 130, 130);

    graph.detachNode(graph.nodes.at(-1)?.id ?? "", 260, 180);

    expect(graph.nodes.at(-1)?.x).toBe(130);
  });

  test("marks a node as the start point", () => {
    const graph = new GraphStore();
    graph.addNode("agent", 0, 0);

    graph.setStart(graph.nodes[0]?.id ?? "", true);

    expect(graph.nodes[0]?.start).toBe(true);
  });

  test("marking a sibling as start clears the previous start", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    graph.addNode("agent", 10, 10, project.id);
    graph.addNode("tool", 20, 20, project.id);

    graph.setStart(graph.nodes[1]?.id ?? "", true);
    graph.setStart(graph.nodes[2]?.id ?? "", true);

    expect(graph.nodes[1]?.start).toBe(false);
    expect(graph.nodes[2]?.start).toBe(true);
  });

  test("each project keeps its own start point", () => {
    const graph = new GraphStore();
    const first = graph.addNode("project", 0, 0);
    graph.addNode("agent", 10, 10, first.id);
    const second = graph.addNode("project", 400, 400);
    graph.addNode("tool", 410, 410, second.id);

    graph.setStart(graph.nodes[1]?.id ?? "", true);
    graph.setStart(graph.nodes[3]?.id ?? "", true);

    expect(graph.nodes[1]?.start).toBe(true);
    expect(graph.nodes[3]?.start).toBe(true);
  });

  test("top-level nodes share a single start point", () => {
    const graph = new GraphStore();
    graph.addNode("agent", 0, 0);
    graph.addNode("tool", 400, 400);

    graph.setStart(graph.nodes[0]?.id ?? "", true);
    graph.setStart(graph.nodes[1]?.id ?? "", true);

    expect(graph.nodes[0]?.start).toBe(false);
    expect(graph.nodes[1]?.start).toBe(true);
  });

  test("unmarks a start point", () => {
    const graph = new GraphStore();
    graph.addNode("agent", 0, 0);
    graph.setStart(graph.nodes[0]?.id ?? "", true);

    graph.setStart(graph.nodes[0]?.id ?? "", false);

    expect(graph.nodes[0]?.start).toBe(false);
  });

  test("setting a start point on an unknown id is a no-op", () => {
    const graph = new GraphStore();
    graph.addNode("agent", 0, 0);

    graph.setStart("not-a-node", true);

    expect(graph.nodes[0]?.start).toBeUndefined();
  });

  test("attaching a flagged node clears its start flag", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 100, 80);
    graph.addNode("agent", 0, 0);
    graph.setStart(graph.nodes[1]?.id ?? "", true);

    graph.attachToProject(graph.nodes[1]?.id ?? "", project.id, 120, 100);

    expect(graph.nodes[1]?.start).toBe(false);
  });

  test("detaching a flagged node clears its start flag", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 100, 80);
    graph.addNode("agent", 10, 10, project.id);
    graph.setStart(graph.nodes[1]?.id ?? "", true);

    graph.detachNode(graph.nodes[1]?.id ?? "", 300, 300);

    expect(graph.nodes[1]?.start).toBe(false);
  });

  test("moving a project carries its children along", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 30, 20);
    graph.addNode("agent", 70, 30, project.id);

    graph.moveNode(project.id, 60, 50);

    expect(graph.nodes[0]?.x).toBe(60);
    expect(graph.nodes[0]?.y).toBe(50);
    // The child's relative position is untouched; it follows via its parent.
    expect(graph.nodes.at(-1)?.x).toBe(70);
    expect(graph.nodes.at(-1)?.y).toBe(30);
  });

  test("moving a child updates only its own relative position", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 30, 20);
    graph.addNode("agent", 70, 30, project.id);
    graph.addNode("tool", 300, 300);

    graph.moveNode(graph.nodes[1]?.id ?? "", 40, 10);

    expect(graph.nodes[1]?.x).toBe(40);
    expect(graph.nodes[1]?.y).toBe(10);
    expect(graph.nodes[0]?.x).toBe(30);
    expect(graph.nodes[0]?.y).toBe(20);
    expect(graph.nodes.at(-1)?.x).toBe(300);
    expect(graph.nodes.at(-1)?.y).toBe(300);
  });

  test("reports whether a node has children", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 30, 20);
    const child = graph.addNode("agent", 70, 30, project.id);

    expect(graph.hasChildren(project.id)).toBe(true);
    expect(graph.hasChildren(child.id)).toBe(false);
    expect(graph.hasChildren("not-a-node")).toBe(false);
  });

  test("finds the top-level project containing a content point", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 100, 80);

    expect(graph.projectAt(100, 80)?.id).toBe(project.id);
    expect(graph.projectAt(259, 143)?.id).toBe(project.id);
    expect(graph.projectAt(261, 145)).toBeUndefined();
  });

  test("projectAt ignores non-projects and already-nested projects", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 100, 80);
    graph.addNode("agent", 120, 100, project.id);
    const nested = graph.addNode("project", 150, 90, project.id);

    expect(graph.projectAt(110, 90)?.id).toBe(project.id);
    expect(graph.projectAt(160, 100)?.id).toBe(project.id);
    expect(nested.parentId).toBe(project.id);
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
