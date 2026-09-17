import { describe, expect, test } from "vitest";
import {
  DEFAULT_NODE_HEIGHT,
  DEFAULT_NODE_WIDTH,
  GIT_BASE_TYPES,
  GraphStore,
  MIN_NODE_HEIGHT,
  MIN_NODE_WIDTH,
  PALETTE,
  canExistTopLevel,
  canHostChild,
  isContainerType,
  isForgeType,
  rejectedDropHint,
  shortNodeId,
  GIT_ACTIONS,
  DEFAULT_IGNORE_LABELS,
  BLOCK_HUES,
  workflowSlug,
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

    const first = graph.addNode("project", 0, 0);
    const second = graph.addNode("project", 10, 10);

    expect(first.name).toBe("Project 1");
    expect(second.name).toBe("Project 2");
  });

  test("selection follows the requested node and can be cleared", () => {
    const graph = new GraphStore();

    const first = graph.addNode("agent", 0, 0);
    const second = graph.addNode("project", 10, 10);

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

    graph.attachToContainer(agent.id, project.id, 140, 100);

    expect(graph.nodes.at(-1)?.parentId).toBe(project.id);
    expect(graph.nodes.at(-1)?.x).toBe(40);
    expect(graph.nodes.at(-1)?.y).toBe(20);
  });

  test("attaching with an unknown node or parent is a no-op", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 100, 80);
    graph.addNode("agent", 130, 130);

    graph.attachToContainer(
      graph.nodes.at(-1)?.id ?? "",
      "not-a-node",
      140,
      100,
    );
    graph.attachToContainer("not-a-node", project.id, 140, 100);

    expect(graph.nodes.at(-1)?.parentId).toBeUndefined();
    expect(graph.nodes.at(-1)?.x).toBe(130);
  });

  test("attaches to a project that is itself nested", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 100, 80);
    const nested = graph.addNode("project", 110, 90, project.id);
    graph.addNode("agent", 130, 130);

    graph.attachToContainer(graph.nodes.at(-1)?.id ?? "", nested.id, 220, 180);

    expect(graph.nodes.at(-1)?.parentId).toBe(nested.id);
    // The drop position converts to the nested project's own coordinate space.
    expect(graph.nodes.at(-1)?.x).toBe(10);
    expect(graph.nodes.at(-1)?.y).toBe(10);
  });

  test("attaching a node that already has children is a no-op", () => {
    const graph = new GraphStore();
    const first = graph.addNode("project", 100, 80);
    graph.addNode("agent", 120, 100, first.id);
    graph.addNode("project", 300, 300);

    graph.attachToContainer(first.id, graph.nodes.at(-1)?.id ?? "", 320, 320);

    expect(graph.nodes[0]?.parentId).toBeUndefined();
    expect(graph.nodes[0]?.x).toBe(100);
    expect(graph.nodes[0]?.y).toBe(80);
  });

  test("attaching a node to itself is a no-op", () => {
    const graph = new GraphStore();
    graph.addNode("project", 100, 80);

    graph.attachToContainer(
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
    graph.addNode("project", 20, 20, project.id);

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
    graph.addNode("project", 410, 410, second.id);

    graph.setStart(graph.nodes[1]?.id ?? "", true);
    graph.setStart(graph.nodes[3]?.id ?? "", true);

    expect(graph.nodes[1]?.start).toBe(true);
    expect(graph.nodes[3]?.start).toBe(true);
  });

  test("top-level nodes share a single start point", () => {
    const graph = new GraphStore();
    graph.addNode("agent", 0, 0);
    graph.addNode("project", 400, 400);

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

    graph.attachToContainer(graph.nodes[1]?.id ?? "", project.id, 120, 100);

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

  test("connects two same-level nodes with an edge", () => {
    const graph = new GraphStore();
    const first = graph.addNode("agent", 0, 0);
    const second = graph.addNode("project", 400, 400);

    expect(graph.connect(first.id, second.id)).toBe(true);
    expect(graph.edges).toHaveLength(1);
    expect(graph.edges[0]?.from).toBe(first.id);
    expect(graph.edges[0]?.to).toBe(second.id);
  });

  test("rejects connecting a node to itself", () => {
    const graph = new GraphStore();
    const node = graph.addNode("agent", 0, 0);

    expect(graph.connect(node.id, node.id)).toBe(false);
    expect(graph.edges).toHaveLength(0);
  });

  test("rejects connecting nodes on different levels", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    const child = graph.addNode("agent", 10, 10, project.id);
    const top = graph.addNode("project", 400, 400);

    expect(graph.connect(project.id, child.id)).toBe(false);
    expect(graph.connect(top.id, child.id)).toBe(false);
    expect(graph.edges).toHaveLength(0);
  });

  test("rejects duplicate edges between the same pair", () => {
    const graph = new GraphStore();
    const first = graph.addNode("agent", 0, 0);
    const second = graph.addNode("project", 400, 400);
    graph.connect(first.id, second.id);

    expect(graph.connect(first.id, second.id)).toBe(false);
    expect(graph.edges).toHaveLength(1);
  });

  test("rejects the reverse of an existing edge", () => {
    const graph = new GraphStore();
    const first = graph.addNode("agent", 0, 0);
    const second = graph.addNode("project", 400, 400);
    graph.connect(first.id, second.id);

    expect(graph.connect(second.id, first.id)).toBe(false);
    expect(graph.edges).toHaveLength(1);
  });

  test("an edge persists when one of its boxes is nested later", () => {
    const graph = new GraphStore();
    const first = graph.addNode("agent", 0, 0);
    const second = graph.addNode("project", 400, 400);
    graph.connect(first.id, second.id);
    const project = graph.addNode("project", 800, 800);

    graph.attachToContainer(second.id, project.id, 820, 820);

    expect(graph.edges).toHaveLength(1);
    expect(graph.edges[0]?.from).toBe(first.id);
    expect(graph.edges[0]?.to).toBe(second.id);
  });

  test("allows many outgoing edges from one box", () => {
    const graph = new GraphStore();
    const source = graph.addNode("agent", 0, 0);
    const first = graph.addNode("project", 400, 0);
    const second = graph.addNode("project", 400, 300);

    graph.connect(source.id, first.id);
    graph.connect(source.id, second.id);

    expect(graph.edges).toHaveLength(2);
    expect(graph.edges.every((edge) => edge.from === source.id)).toBe(true);
  });

  test("allows many incoming edges into one box", () => {
    const graph = new GraphStore();
    const first = graph.addNode("agent", 0, 0);
    const second = graph.addNode("agent", 0, 300);
    const target = graph.addNode("project", 400, 0);

    graph.connect(first.id, target.id);
    graph.connect(second.id, target.id);

    expect(graph.edges).toHaveLength(2);
    expect(graph.edges.every((edge) => edge.to === target.id)).toBe(true);
  });

  test("rejects connecting nodes in different projects", () => {
    const graph = new GraphStore();
    const first = graph.addNode("project", 0, 0);
    graph.addNode("agent", 10, 10, first.id);
    const second = graph.addNode("project", 400, 400);
    graph.addNode("project", 410, 410, second.id);

    expect(
      graph.connect(graph.nodes[1]?.id ?? "", graph.nodes[3]?.id ?? ""),
    ).toBe(false);
    expect(graph.edges).toHaveLength(0);
  });

  test("connecting with unknown ids is a no-op", () => {
    const graph = new GraphStore();
    const node = graph.addNode("agent", 0, 0);

    expect(graph.connect(node.id, "not-a-node")).toBe(false);
    expect(graph.connect("not-a-node", node.id)).toBe(false);
    expect(graph.edges).toHaveLength(0);
  });

  test("adds GitHub and GitLab nodes with default names", () => {
    const graph = new GraphStore();

    const github = graph.addNode("github", 0, 0);
    const gitlab = graph.addNode("gitlab", 400, 400);

    expect(github.name).toBe("GitHub 1");
    expect(gitlab.name).toBe("GitLab 1");
  });

  test("sets a repository link on a forge box", () => {
    const graph = new GraphStore();
    const node = graph.addNode("github", 0, 0);

    graph.setRepository(node.id, "https://github.com/example/project");

    expect(graph.nodes[0]?.repository).toBe(
      "https://github.com/example/project",
    );
  });

  test("setting a repository on an unknown id is a no-op", () => {
    const graph = new GraphStore();
    graph.addNode("github", 0, 0);

    graph.setRepository("not-a-node", "https://github.com/example/project");

    expect(graph.nodes[0]?.repository).toBeUndefined();
  });

  test("sets a secret key reference on a forge box", () => {
    const graph = new GraphStore();
    const node = graph.addNode("gitlab", 0, 0);

    graph.setSecretKey(node.id, "secret://gitlab-bot");

    expect(graph.nodes[0]?.secretKey).toBe("secret://gitlab-bot");
  });

  test("setting a secret key on an unknown id is a no-op", () => {
    const graph = new GraphStore();
    graph.addNode("gitlab", 0, 0);

    graph.setSecretKey("not-a-node", "secret://gitlab-bot");

    expect(graph.nodes[0]?.secretKey).toBeUndefined();
  });

  test("a GitHub hosts nested GitHub Apps as a container", () => {
    const graph = new GraphStore();
    const github = graph.addNode("github", 100, 80);

    const child = graph.addNode("githubapp", 120, 100, github.id);

    expect(child.parentId).toBe(github.id);
    expect(graph.containerAt(110, 90)?.id).toBe(github.id);
  });

  test("finds a nested GitHub box by its absolute position", () => {
    const graph = new GraphStore();
    graph.addNode("project", 100, 80);
    const github = graph.addNode("github", 20, 10, graph.nodes[0]?.id ?? "");

    // GitHub 1 sits at absolute (120, 90) inside the project.
    expect(graph.containerAt(130, 100)?.id).toBe(github.id);
    expect(graph.containerAt(105, 85)?.id).toBe(graph.nodes[0]?.id);
  });

  test("attaches a GitHub App to a GitHub box nested inside a project", () => {
    const graph = new GraphStore();
    graph.addNode("project", 100, 80);
    const github = graph.addNode("github", 20, 10, graph.nodes[0]?.id ?? "");
    graph.addNode("githubapp", 0, 0);

    graph.attachToContainer(graph.nodes.at(-1)?.id ?? "", github.id, 140, 100);

    expect(graph.nodes.at(-1)?.parentId).toBe(github.id);
    expect(graph.nodes.at(-1)?.x).toBe(20);
    expect(graph.nodes.at(-1)?.y).toBe(10);
  });

  test("a GitHub App nests regardless of how many parents the GitHub has", () => {
    const graph = new GraphStore();
    const outer = graph.addNode("project", 0, 0);
    const middle = graph.addNode("project", 10, 10, outer.id);
    const github = graph.addNode("github", 5, 5, middle.id);
    graph.addNode("githubapp", 0, 0);

    graph.attachToContainer(graph.nodes.at(-1)?.id ?? "", github.id, 30, 30);

    expect(graph.nodes.at(-1)?.parentId).toBe(github.id);
  });

  test("adds a GitHub App node with a default name", () => {
    const graph = new GraphStore();

    const app = graph.addNode("githubapp", 0, 0);

    expect(app.name).toBe("GitHub App 1");
    expect(app.type).toBe("githubapp");
  });

  test("adding a GitHub App with a non-GitHub parent ignores the parent", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 100, 80);

    const app = graph.addNode("githubapp", 120, 100, project.id);

    expect(app.parentId).toBeUndefined();
  });

  test("adding an incompatible child with a GitHub parent ignores the parent", () => {
    const graph = new GraphStore();
    const github = graph.addNode("github", 100, 80);

    const agent = graph.addNode("agent", 120, 100, github.id);

    expect(agent.parentId).toBeUndefined();
  });

  test("attaching a GitHub App to a non-GitHub container is a no-op", () => {
    const graph = new GraphStore();
    graph.addNode("project", 100, 80);
    graph.addNode("githubapp", 0, 0);

    graph.attachToContainer(
      graph.nodes[1]?.id ?? "",
      graph.nodes[0]?.id ?? "",
      120,
      100,
    );

    expect(graph.nodes[1]?.parentId).toBeUndefined();
  });

  test("attaching an agent to a GitHub container is a no-op", () => {
    const graph = new GraphStore();
    graph.addNode("github", 100, 80);
    graph.addNode("agent", 0, 0);

    graph.attachToContainer(
      graph.nodes[1]?.id ?? "",
      graph.nodes[0]?.id ?? "",
      120,
      100,
    );

    expect(graph.nodes[1]?.parentId).toBeUndefined();
  });

  test("sets an app id on a GitHub App box", () => {
    const graph = new GraphStore();
    const node = graph.addNode("githubapp", 0, 0);

    graph.setAppId(node.id, "123456");

    expect(graph.nodes[0]?.appId).toBe("123456");
  });

  test("setting an app id on an unknown id is a no-op", () => {
    const graph = new GraphStore();
    graph.addNode("githubapp", 0, 0);

    graph.setAppId("not-a-node", "123456");

    expect(graph.nodes[0]?.appId).toBeUndefined();
  });

  test("sets a private key path on a GitHub App box", () => {
    const graph = new GraphStore();
    const node = graph.addNode("githubapp", 0, 0);

    graph.setPrivateKeyPath(node.id, "/home/user/.keys/github-app.pem");

    expect(graph.nodes[0]?.privateKeyPath).toBe(
      "/home/user/.keys/github-app.pem",
    );
  });

  test("setting a private key path on an unknown id is a no-op", () => {
    const graph = new GraphStore();
    graph.addNode("githubapp", 0, 0);

    graph.setPrivateKeyPath("not-a-node", "/home/user/.keys/github-app.pem");

    expect(graph.nodes[0]?.privateKeyPath).toBeUndefined();
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
    graph.addNode("project", 300, 300);

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

    expect(graph.containerAt(100, 80)?.id).toBe(project.id);
    expect(graph.containerAt(259, 143)?.id).toBe(project.id);
    expect(graph.containerAt(261, 145)).toBeUndefined();
  });

  test("containerAt ignores non-containers", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 100, 80);
    graph.addNode("agent", 120, 100, project.id);

    expect(graph.containerAt(110, 90)?.id).toBe(project.id);
    expect(project.parentId).toBeUndefined();
  });

  test("containerAt finds the innermost container at any depth", () => {
    const graph = new GraphStore();
    const outer = graph.addNode("project", 100, 80);
    const inner = graph.addNode("project", 20, 20, outer.id);

    // The inner project sits at absolute (120, 100).
    expect(graph.containerAt(125, 105)?.id).toBe(inner.id);
    expect(graph.containerAt(105, 85)?.id).toBe(outer.id);
  });

  test("labels unknown component types with the raw type", () => {
    const graph = new GraphStore();

    const node = graph.addNode("mystery" as NodeType, 5, 5);

    expect(node.name).toBe("mystery 1");
    expect(node.type).toBe("mystery");
  });
});

describe("shortNodeId", () => {
  test("derives a stable three-character id from a node id", () => {
    const id = "0b9e6c1e-5d4a-4f8b-9c3e-2a1b7d6f5e4c";

    expect(shortNodeId(id)).toMatch(/^[0-9a-z]{3}$/);
    expect(shortNodeId(id)).toBe(shortNodeId(id));
  });

  test("derives distinct short ids for different node ids", () => {
    const codes = new Set(
      Array.from({ length: 200 }, () => shortNodeId(crypto.randomUUID())),
    );

    expect(codes.size).toBeGreaterThan(150);
  });

  test("derives distinct short ids for the live store nodes", () => {
    const graph = new GraphStore();
    for (const type of PALETTE) graph.addNode(type.type, 0, 0);

    const codes = graph.nodes.map((node) => shortNodeId(node.id));

    expect(new Set(codes).size).toBe(codes.length);
  });
});

describe("PALETTE", () => {
  test("offers the agent, project, github, gitlab, and github app components", () => {
    expect(PALETTE.map(({ type, label }) => ({ type, label }))).toEqual([
      { type: "agent", label: "Agent" },
      { type: "project", label: "Project" },
      { type: "github", label: "GitHub" },
      { type: "gitlab", label: "GitLab" },
      { type: "githubapp", label: "GitHub App" },
      { type: "jsonschema", label: "JSON Schema" },
      { type: "router", label: "Router" },
      { type: "command", label: "Command" },
      { type: "loop", label: "Loop" },
    ]);
  });

  test("explains every component in a sentence or two", () => {
    for (const item of PALETTE) {
      expect(item.description.length, item.label).toBeGreaterThan(20);
      expect(item.description.length, item.label).toBeLessThanOrEqual(160);
    }
  });
});

describe("GitBase abstraction", () => {
  test("the GitHub and GitLab controllers derive from GitBase", () => {
    expect(GIT_BASE_TYPES).toEqual(["github", "gitlab"]);
    expect(isForgeType("github")).toBe(true);
    expect(isForgeType("gitlab")).toBe(true);
    expect(isForgeType("agent")).toBe(false);
    expect(isForgeType("project")).toBe(false);
    expect(isForgeType("githubapp")).toBe(false);
  });
});

describe("canHostChild", () => {
  // The containment matrix agreed with the product owner: rows are dropped
  // blocks, columns are the targets that accept them (root = empty canvas).
  const matrix = [
    ["agent", { root: false, project: true, github: false, agent: true }],
    ["loop", { root: false, project: true, github: false, agent: false }],
    ["project", { root: true, project: true, github: false, agent: false }],
    ["github", { root: false, project: true, github: false, agent: false }],
    ["gitlab", { root: false, project: true, github: false, agent: false }],
    ["githubapp", { root: false, project: false, github: true, agent: false }],
    ["action", { root: false, project: false, github: true, agent: false }],
    ["jsonschema", { root: false, project: false, github: false, agent: true }],
    ["router", { root: false, project: true, github: false, agent: true }],
    ["command", { root: false, project: true, github: false, agent: true }],
  ] as const;

  for (const [childType, targets] of matrix) {
    test(`${childType} drops follow the containment matrix`, () => {
      expect(canHostChild("root", childType)).toBe(targets.root);
      expect(canHostChild("project", childType)).toBe(targets.project);
      expect(canHostChild("github", childType)).toBe(targets.github);
      expect(canHostChild("agent", childType)).toBe(targets.agent);
      expect(canHostChild("gitlab", childType)).toBe(false);
      expect(canHostChild("githubapp", childType)).toBe(false);
    });
  }
});

describe("root-level and hint rules", () => {
  test("only a project may exist at the top level of the canvas", () => {
    expect(canExistTopLevel("project")).toBe(true);
    expect(canExistTopLevel("agent")).toBe(false);
    expect(canExistTopLevel("github")).toBe(false);
    expect(canExistTopLevel("gitlab")).toBe(false);
    expect(canExistTopLevel("githubapp")).toBe(false);
  });

  test("an agent hosts other agents as a container", () => {
    expect(isContainerType("agent")).toBe(true);
    expect(isContainerType("gitlab")).toBe(false);
    expect(isContainerType("githubapp")).toBe(false);
  });

  test("explains a root-level rejection with the allowed targets", () => {
    expect(rejectedDropHint("githubapp")).toBe(
      "A GitHub App can only be dropped inside a GitHub box.",
    );
    expect(rejectedDropHint("agent")).toBe(
      "An Agent can only be dropped inside a project box, another agent or a loop box.",
    );
    expect(rejectedDropHint("github")).toBe(
      "A GitHub can only be dropped inside a project box.",
    );
    expect(rejectedDropHint("jsonschema")).toBe(
      "A JSON Schema can only be dropped inside an agent box.",
    );
  });

  test("explains an incompatible container rejection", () => {
    expect(rejectedDropHint("agent", "github")).toBe(
      "An Agent cannot be placed inside a GitHub box.",
    );
    expect(rejectedDropHint("githubapp", "project")).toBe(
      "A GitHub App cannot be placed inside a project box.",
    );
  });
});

describe("Git actions inside a GitHub block", () => {
  function githubInProject(): { graph: GraphStore; githubId: string } {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    graph.resizeNode(project.id, 600, 500);
    const github = graph.addNode("github", 20, 20, project.id);
    return { graph, githubId: github.id };
  }

  test("offers fetch, worktree, rebase, and the delivery actions", () => {
    expect(GIT_ACTIONS).toEqual([
      { action: "fetch", label: "Fetch" },
      { action: "worktree", label: "Create worktree" },
      { action: "rebase", label: "Rebase" },
      { action: "issue", label: "Read issue" },
      { action: "commit", label: "Commit" },
      { action: "push", label: "Push" },
      { action: "pullrequest", label: "Open pull request" },
    ]);
  });

  test("a block beside a GitHub block may feed one of its actions", () => {
    const { graph, githubId } = githubInProject();
    const github = graph.nodes.find((node) => node.id === githubId)!;
    const commit = graph.addAction(githubId, "commit")!;
    const agent = graph.addNode("agent", 300, 20, github.parentId);
    const other = graph.addNode("project", 900, 0);
    const stranger = graph.addNode("agent", 10, 10, other.id);

    expect(graph.connect(agent.id, commit.id)).toBe(true);
    expect(graph.connect(stranger.id, commit.id)).toBe(false);
  });

  test("sets the issue an action reads", () => {
    const { graph, githubId } = githubInProject();
    const issue = graph.addAction(githubId, "issue")!;

    // 0 reads the next available issue.
    expect(issue.issue).toBe(0);
    graph.setActionIssue(issue.id, "7");
    expect(issue.issue).toBe(7);
    graph.setActionIssue(issue.id, "");
    expect(issue.issue).toBe(0);
    expect(graph.addAction(githubId, "commit")!.issue).toBeUndefined();
    graph.setActionIssue("missing", "1");
  });

  test("Read issue ignores paused, draft and needs-attention unless its labels are set", () => {
    const { graph, githubId } = githubInProject();
    const issue = graph.addAction(githubId, "issue")!;

    expect(DEFAULT_IGNORE_LABELS).toEqual([
      "paused",
      "draft",
      "needs-attention",
    ]);
    // Without the setting the backend applies the defaults too.
    expect(issue.ignoreLabels).toBeUndefined();
    graph.setActionIgnoreLabels(issue.id, " paused,wip , ");
    expect(issue.ignoreLabels).toEqual(["paused", "wip", ""]);
    graph.setActionIgnoreLabels(issue.id, "");
    expect(issue.ignoreLabels).toEqual([]);
    expect(JSON.stringify(graph.toRequest())).toContain('"ignoreLabels":[]');
    graph.setActionIgnoreLabels("missing", "draft");
  });

  test("the first action nests inside the GitHub block as its starting point", () => {
    const { graph, githubId } = githubInProject();

    const action = graph.addAction(githubId, "fetch");

    expect(action).toMatchObject({
      type: "action",
      action: "fetch",
      name: "Fetch",
      parentId: githubId,
      start: true,
    });
    expect(graph.selectedId).toBe(action?.id);
  });

  test("each later action follows the previous one with an arrow", () => {
    const { graph, githubId } = githubInProject();

    const fetch = graph.addAction(githubId, "fetch");
    const worktree = graph.addAction(githubId, "worktree");
    const rebase = graph.addAction(githubId, "rebase");

    expect(worktree?.start).toBeFalsy();
    expect(graph.edges.map((edge) => [edge.from, edge.to])).toEqual([
      [fetch?.id, worktree?.id],
      [worktree?.id, rebase?.id],
    ]);
    expect(worktree!.y).toBeGreaterThan(fetch!.y + fetch!.h);
    expect(rebase!.y).toBeGreaterThan(worktree!.y + worktree!.h);
  });

  test("the GitHub block grows to show its whole sequence", () => {
    const { graph, githubId } = githubInProject();
    const github = graph.nodes.find((node) => node.id === githubId)!;

    graph.addAction(githubId, "fetch");
    const last = graph.addAction(githubId, "worktree")!;

    expect(github.h).toBeGreaterThanOrEqual(last.y + last.h);
    expect(github.w).toBeGreaterThanOrEqual(last.x + last.w);
  });

  test("the containing project grows along with its GitHub block", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    const github = graph.addNode("github", 20, 20, project.id);

    graph.addAction(github.id, "fetch");
    graph.addAction(github.id, "worktree");

    expect(project.h).toBeGreaterThanOrEqual(github.y + github.h);
    expect(project.w).toBeGreaterThanOrEqual(github.x + github.w);
  });

  test("only a GitHub block holds actions", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);

    expect(graph.addAction(project.id, "fetch")).toBeUndefined();
    expect(graph.addAction("missing", "fetch")).toBeUndefined();
    expect(graph.nodes).toHaveLength(1);
  });

  test("lists a block's actions in run order, loose ones last", () => {
    const { graph, githubId } = githubInProject();
    const fetch = graph.addAction(githubId, "fetch")!;
    const worktree = graph.addAction(githubId, "worktree")!;
    const loose = graph.addNode("action", 300, 300, githubId);
    graph.edges.reverse();

    expect(graph.actionSequence(githubId).map((node) => node.id)).toEqual([
      fetch.id,
      worktree.id,
      loose.id,
    ]);
  });

  test("an action's output may leave its GitHub block toward a block beside it", () => {
    const { graph, githubId } = githubInProject();
    const github = graph.nodes.find((node) => node.id === githubId)!;
    const worktree = graph.addAction(githubId, "worktree")!;
    const agent = graph.addNode("agent", 300, 20, github.parentId);

    expect(graph.connect(worktree.id, agent.id)).toBe(true);
    expect(graph.connect(agent.id, worktree.id)).toBe(false);
  });

  test("an action's output cannot reach into another project", () => {
    const { graph, githubId } = githubInProject();
    const worktree = graph.addAction(githubId, "worktree")!;
    const other = graph.addNode("project", 800, 0);
    const agent = graph.addNode("agent", 10, 10, other.id);

    expect(graph.connect(worktree.id, agent.id)).toBe(false);
  });

  test("updates an action's configuration", () => {
    const { graph, githubId } = githubInProject();
    const worktree = graph.addAction(githubId, "worktree")!;

    graph.setActionField(worktree.id, "branch", "feature/login");
    graph.setActionField(worktree.id, "base", "origin/main");
    graph.setActionField(worktree.id, "worktreePath", "/tmp/login");
    graph.setActionField(worktree.id, "onto", "origin/release");
    graph.setActionField("missing", "branch", "ignored");

    expect(worktree).toMatchObject({
      branch: "feature/login",
      base: "origin/main",
      worktreePath: "/tmp/login",
      onto: "origin/release",
    });
  });
});

describe("run status on the canvas", () => {
  test("shows each step's status on its block", () => {
    const graph = new GraphStore();

    graph.showRun([
      { nodeId: "a", status: "succeeded" },
      { nodeId: "b", status: "failed" },
    ]);

    expect(graph.statusOf("a")).toBe("succeeded");
    expect(graph.statusOf("b")).toBe("failed");
    expect(graph.statusOf("c")).toBeUndefined();
  });

  test("a GitHub block rolls up the status of its actions", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    const github = graph.addNode("github", 10, 10, project.id);
    const fetch = graph.addAction(github.id, "fetch")!;
    const worktree = graph.addAction(github.id, "worktree")!;
    const rollUp = (statuses: string[]) => {
      graph.showRun([
        { nodeId: fetch.id, status: statuses[0]! },
        { nodeId: worktree.id, status: statuses[1]! },
      ]);
      return graph.statusOf(github.id);
    };

    expect(rollUp(["succeeded", "succeeded"])).toBe("succeeded");
    expect(rollUp(["succeeded", "running"])).toBe("running");
    expect(rollUp(["failed", "skipped"])).toBe("failed");
    expect(rollUp(["succeeded", "pending"])).toBe("running");
    expect(rollUp(["pending", "pending"])).toBe("pending");
    graph.showRun([]);
    expect(graph.statusOf(github.id)).toBeUndefined();
  });
});

describe("agent configuration", () => {
  test("a new agent starts on the Claude backend", () => {
    const graph = new GraphStore();

    expect(graph.addNode("agent", 0, 0).backend).toBe("claude");
    expect(graph.addNode("project", 0, 0).backend).toBeUndefined();
  });

  test("updates an agent's text and number settings", () => {
    const graph = new GraphStore();
    const agent = graph.addNode("agent", 0, 0);

    graph.setAgentField(agent.id, "prompt", "Review");
    graph.setAgentNumber(agent.id, "retries", "3");
    graph.setAgentNumber(agent.id, "timeoutMinutes", "");
    graph.setAgentField("missing", "prompt", "ignored");
    graph.setAgentNumber("missing", "retries", "1");

    graph.setContinueSession(agent.id, true);
    graph.setContinueSession("missing", true);
    expect(agent.continueSession).toBe(true);
    expect(agent.prompt).toBe("Review");
    expect(agent.retries).toBe(3);
    expect(agent.timeoutMinutes).toBeUndefined();
  });
});

describe("output ports", () => {
  test("a schema block has valid and invalid outputs; other blocks have one", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    const agent = graph.addNode("agent", 10, 10, project.id);
    const check = graph.addNode("jsonschema", 10, 40, agent.id);

    expect(graph.outputPorts(check.id)).toEqual(["valid", "invalid"]);
    expect(graph.outputPorts(agent.id)).toEqual([]);
    expect(graph.outputPorts("missing")).toEqual([]);
  });

  test("an arrow from a block with ports can take any of its outputs", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    const writer = graph.addNode("agent", 10, 10, project.id);
    const check = graph.addNode("jsonschema", 10, 40, writer.id);
    const fixer = graph.addNode("agent", 200, 10, project.id);
    graph.connect(check.id, fixer.id);
    const edge = graph.edges[0]!;

    expect(graph.portOf(edge)).toBe("valid");
    graph.setEdgePort(edge.id, "invalid");
    expect(graph.portOf(graph.edges[0]!)).toBe("invalid");
    graph.setEdgePort("missing", "valid");
    expect(graph.outgoingEdges(check.id)).toHaveLength(1);
  });

  test("sets a schema block's schema text", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    const agent = graph.addNode("agent", 10, 10, project.id);
    const check = graph.addNode("jsonschema", 10, 40, agent.id);

    graph.setSchema(check.id, '{"type":"object"}');
    graph.setSchema("missing", "ignored");

    expect(check.schema).toBe('{"type":"object"}');
  });
});

describe("router cases", () => {
  function routerInProject() {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    const route = graph.addNode("router", 10, 10, project.id);
    const ship = graph.addNode("agent", 200, 10, project.id);
    return { graph, route, ship };
  }

  test("a new router starts with one example case and a default route", () => {
    const { graph, route } = routerInProject();

    expect(route.cases).toEqual([
      { name: "approved", expression: 'value.verdict == "approve"' },
    ]);
    expect(graph.outputPorts(route.id)).toEqual(["approved", "default"]);
  });

  test("an arrow from a router takes its first route explicitly", () => {
    const { graph, route, ship } = routerInProject();

    graph.connect(route.id, ship.id);

    expect(graph.edges[0]?.fromPort).toBe("approved");
  });

  test("adds, edits, renames, and removes cases", () => {
    const { graph, route, ship } = routerInProject();
    graph.connect(route.id, ship.id);

    graph.addCase(route.id);
    graph.setCase(route.id, 1, "expression", "size(value.findings) > 0");
    graph.setCase(route.id, 0, "name", "ship");

    expect(route.cases).toEqual([
      { name: "ship", expression: 'value.verdict == "approve"' },
      { name: "case-2", expression: "size(value.findings) > 0" },
    ]);
    expect(graph.edges[0]?.fromPort).toBe("ship");
    graph.removeCase(route.id, 0);
    expect(route.cases).toHaveLength(1);
    expect(graph.edges[0]?.fromPort).toBe("default");
    graph.addCase("missing");
    graph.setCase("missing", 0, "name", "x");
    graph.removeCase("missing", 0);
  });
});

describe("whole workflows", () => {
  test("loads a workflow in place of the current graph", () => {
    const graph = new GraphStore();
    const old = graph.addNode("project", 0, 0);
    graph.showRun([{ nodeId: old.id, status: "failed" }], "run-1");

    graph.load({
      name: "issue-to-pr",
      nodes: [
        { id: "api", type: "project", name: "api", x: 0, y: 0, w: 400, h: 300 },
      ],
      edges: [{ id: "e1", from: "api", to: "api" }],
    });

    expect(graph.workflowName).toBe("issue-to-pr");
    expect(graph.nodes.map((node) => node.id)).toEqual(["api"]);
    expect(graph.edges).toHaveLength(1);
    expect(graph.selectedId).toBeNull();
    expect(graph.runId).toBeNull();
    expect(graph.statusOf(old.id)).toBeUndefined();
  });

  test("loads a workflow without edges or a name", () => {
    const graph = new GraphStore();

    graph.load({ nodes: [] });

    expect(graph.workflowName).toBe("workflow");
    expect(graph.edges).toEqual([]);
  });

  test("describes the graph as the request the backend takes", () => {
    const graph = new GraphStore();
    graph.workflowName = "Nightly Sync";
    graph.addNode("project", 1, 2);

    const request = graph.toRequest();

    expect(request.name).toBe("Nightly Sync");
    expect(request.nodes).toHaveLength(1);
    expect(request.edges).toEqual([]);
  });

  test("names the saved file after the workflow", () => {
    expect(workflowSlug("Issue to PR!")).toBe("issue-to-pr");
    expect(workflowSlug("  ")).toBe("workflow");
  });
});

describe("command blocks", () => {
  test("have passed and failed outputs and a command to run", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    const gate = graph.addNode("command", 10, 10, project.id);

    graph.setCommand(gate.id, "make verify");
    graph.setCommand("missing", "ignored");

    expect(graph.outputPorts(gate.id)).toEqual(["passed", "failed"]);
    expect(gate.command).toBe("make verify");
  });
});

describe("deleting blocks", () => {
  test("removes the block, its arrows and the selection", () => {
    const graph = new GraphStore();
    const first = graph.addNode("agent", 0, 0);
    const second = graph.addNode("agent", 300, 0);
    const third = graph.addNode("agent", 600, 0);
    graph.connect(first.id, second.id);
    graph.connect(second.id, third.id);
    graph.connect(first.id, third.id);
    graph.select(second.id);

    graph.removeNode(second.id);

    expect(graph.nodes.map((node) => node.name)).toEqual([
      "Agent 1",
      "Agent 3",
    ]);
    expect(graph.edges).toHaveLength(1);
    expect(graph.edges[0]).toMatchObject({ from: first.id, to: third.id });
    expect(graph.selectedId).toBeNull();
  });

  test("removes everything nested inside a container", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    const outside = graph.addNode("project", 900, 0);
    const github = graph.addNode("github", 10, 40, project.id);
    graph.addAction(github.id, "fetch");
    const agent = graph.addNode("agent", 300, 40, project.id);
    graph.connect(github.id, agent.id);

    graph.removeNode(project.id);

    expect(graph.nodes.map((node) => node.id)).toEqual([outside.id]);
    expect(graph.edges).toEqual([]);
  });

  test("keeps the selection when another block is deleted", () => {
    const graph = new GraphStore();
    const kept = graph.addNode("agent", 0, 0);
    const removed = graph.addNode("agent", 300, 0);
    graph.select(kept.id);

    graph.removeNode(removed.id);

    expect(graph.selectedId).toBe(kept.id);
  });

  test("stops showing the log of a deleted block", () => {
    const graph = new GraphStore();
    const node = graph.addNode("agent", 0, 0);
    graph.showRun([{ nodeId: node.id, status: "failed" }], "run-1");
    graph.openLog(node.id);

    graph.removeNode(node.id);

    expect(graph.logNodeId).toBeNull();
  });

  test("a deleted action's neighbours in the sequence are joined", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    const github = graph.addNode("github", 10, 40, project.id);
    graph.addAction(github.id, "fetch");
    const worktree = graph.addAction(github.id, "worktree")!;
    graph.addAction(github.id, "rebase");

    graph.removeNode(worktree.id);

    expect(graph.actionSequence(github.id).map((node) => node.name)).toEqual([
      "Fetch",
      "Rebase",
    ]);
    expect(graph.edges).toHaveLength(1);
  });

  test("the next action becomes the start when the first is deleted", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    const github = graph.addNode("github", 10, 40, project.id);
    const fetch = graph.addAction(github.id, "fetch")!;
    const worktree = graph.addAction(github.id, "worktree")!;

    graph.removeNode(fetch.id);

    expect(graph.nodes.find((node) => node.id === worktree.id)?.start).toBe(
      true,
    );
    expect(graph.edges).toEqual([]);
  });

  test("ignores an unknown block", () => {
    const graph = new GraphStore();
    graph.addNode("agent", 0, 0);

    graph.removeNode("missing");

    expect(graph.nodes).toHaveLength(1);
  });
});

describe("block colors", () => {
  const TYPES: NodeType[] = [
    "loop",
    "agent",
    "project",
    "github",
    "gitlab",
    "githubapp",
    "action",
    "jsonschema",
    "router",
    "command",
  ];

  test("every block type has its own hue, far from the others", () => {
    for (const type of TYPES) {
      for (const other of TYPES) {
        if (type === other) continue;
        const gap = Math.abs(BLOCK_HUES[type] - BLOCK_HUES[other]) % 360;
        expect(
          Math.min(gap, 360 - gap),
          `${type} vs ${other}`,
        ).toBeGreaterThanOrEqual(35);
      }
    }
  });
});

describe("problems", () => {
  test("a block shows its worst problem", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    const agent = graph.addNode("agent", 10, 40, project.id);

    graph.problems = [
      {
        nodeId: agent.id,
        severity: "warning",
        message: "Agent 1 does not run",
      },
      {
        nodeId: agent.id,
        severity: "error",
        message: "Agent 1: write the prompt",
      },
      { severity: "error", message: "flag a starting point" },
    ];

    expect(graph.severityOf(agent.id)).toBe("error");
    expect(graph.severityOf(project.id)).toBeUndefined();
    expect(graph.problemCounts).toEqual({ errors: 2, warnings: 1 });
  });

  test("a block with only warnings shows a warning", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);

    graph.problems = [
      { nodeId: project.id, severity: "warning", message: "set the folder" },
    ];

    expect(graph.severityOf(project.id)).toBe("warning");
    expect(graph.problemCounts).toEqual({ errors: 0, warnings: 1 });
  });
});

describe("loop blocks", () => {
  function loopWithGate() {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    const loop = graph.addNode("loop", 10, 40, project.id);
    const gate = graph.addNode("command", 10, 40, loop.id);
    const fixer = graph.addNode("agent", 200, 40, loop.id);
    graph.connect(gate.id, fixer.id);
    return { graph, project, loop, gate, fixer };
  }

  test("a loop holds steps, repeats three times by default and is roomy", () => {
    const { loop } = loopWithGate();

    expect(canHostChild("loop", "command")).toBe(true);
    expect(canHostChild("loop", "router")).toBe(true);
    expect(canHostChild("loop", "jsonschema")).toBe(false);
    expect(canHostChild("loop", "loop")).toBe(false);
    expect(canHostChild("loop", "github")).toBe(false);
    expect(loop.maxIterations).toBe(3);
    expect(loop.w).toBeGreaterThan(DEFAULT_NODE_WIDTH * 2);
  });

  test("a loop sends done or exhausted", () => {
    const { graph, loop } = loopWithGate();

    expect(graph.outputPorts(loop.id)).toEqual(["done", "exhausted"]);
  });

  test("the ways a loop can end are its blocks' outputs", () => {
    const { graph, loop, gate } = loopWithGate();
    const route = graph.addNode("router", 10, 150, loop.id);

    expect(graph.loopExits(loop.id)).toEqual([
      { nodeId: gate.id, port: "passed", label: "Command 1 takes passed" },
      { nodeId: gate.id, port: "failed", label: "Command 1 takes failed" },
      { nodeId: route.id, port: "approved", label: "Router 1 takes approved" },
      { nodeId: route.id, port: "default", label: "Router 1 takes default" },
    ]);
  });

  test("a schema check inside an agent of the loop can end it", () => {
    const { graph, loop, fixer } = loopWithGate();
    const check = graph.addNode("jsonschema", 10, 40, fixer.id);

    expect(graph.loopExits(loop.id)).toContainEqual({
      nodeId: check.id,
      port: "valid",
      label: "JSON Schema 1 takes valid",
    });
  });

  test("sets how the loop ends and how often it may repeat", () => {
    const { graph, loop, gate } = loopWithGate();

    graph.setLoopExit(loop.id, `${gate.id}:passed`);
    graph.setMaxIterations(loop.id, "5");

    expect(loop).toMatchObject({
      untilNode: gate.id,
      untilPort: "passed",
      maxIterations: 5,
    });

    graph.setLoopExit(loop.id, "");
    graph.setMaxIterations(loop.id, "");

    expect(loop.untilNode).toBeUndefined();
    expect(loop.untilPort).toBeUndefined();
    expect(loop.maxIterations).toBeUndefined();
  });

  test("deleting the block that ends a loop clears the ending", () => {
    const { graph, loop, gate } = loopWithGate();
    graph.setLoopExit(loop.id, `${gate.id}:failed`);

    graph.removeNode(gate.id);

    expect(loop.untilNode).toBeUndefined();
    expect(loop.untilPort).toBeUndefined();
  });

  test("renaming the case that ends a loop keeps the ending", () => {
    const { graph, loop } = loopWithGate();
    const route = graph.addNode("router", 10, 150, loop.id);
    graph.setLoopExit(loop.id, `${route.id}:approved`);

    graph.setCase(route.id, 0, "name", "lgtm");

    expect(loop.untilPort).toBe("lgtm");
  });

  test("a run shows the iteration a loop is in", () => {
    const { graph, loop, gate, fixer } = loopWithGate();

    graph.showRun(
      [
        { nodeId: loop.id, status: "running" },
        { nodeId: gate.id, status: "failed", loop: loop.id, iteration: 2 },
        { nodeId: fixer.id, status: "running", loop: loop.id, iteration: 2 },
      ],
      "run-1",
    );

    expect(graph.iterationOf(loop.id)).toBe(2);
    expect(graph.iterationOf(gate.id)).toBeUndefined();
    graph.showRun([]);
    expect(graph.iterationOf(loop.id)).toBeUndefined();
  });
});

describe("waiting for any arrow", () => {
  test("agents, commands and loops can run on whichever arrow arrives", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    const agent = graph.addNode("agent", 10, 40, project.id);
    const router = graph.addNode("router", 10, 140, project.id);

    expect(graph.canWaitForAny(agent.id)).toBe(true);
    expect(graph.canWaitForAny(router.id)).toBe(false);
    graph.setWaitForAny(agent.id, true);
    expect(agent.waitForAny).toBe(true);
    graph.setWaitForAny(agent.id, false);
    expect(agent.waitForAny).toBeUndefined();
  });
});

describe("where a dragged block lands", () => {
  // Project 1 spans (100..500, 100..400); GitHub 1 sits inside it at
  // (120..280, 140..204).
  function projectWithGitHub() {
    const graph = new GraphStore();
    const project = graph.addNode("project", 100, 100);
    graph.resizeNode(project.id, 400, 300);
    const github = graph.addNode("github", 20, 40, project.id);
    return { graph, project, github };
  }

  test("a palette block lands inside a container that takes it", () => {
    const { graph, project } = projectWithGitHub();

    expect(graph.landingOf("agent", 400, 300)).toEqual({
      valid: true,
      parentId: project.id,
    });
  });

  test("a palette block the container refuses lands on the canvas when it may", () => {
    const { graph } = projectWithGitHub();

    expect(graph.landingOf("project", 150, 150)).toEqual({ valid: true });
  });

  test("a palette block with nowhere to go is refused", () => {
    const { graph, github } = projectWithGitHub();

    expect(graph.landingOf("agent", 150, 150)).toEqual({
      valid: false,
      refusedBy: github.id,
    });
    expect(graph.landingOf("agent", 900, 900)).toEqual({ valid: false });
  });

  test("a moved block stays in its parent over a container that refuses it", () => {
    const { graph, project, github } = projectWithGitHub();
    const agent = graph.addNode("agent", 200, 200, project.id);

    expect(graph.landingOf("agent", 150, 150, agent.id)).toEqual({
      valid: true,
      parentId: project.id,
    });
    expect(graph.landingOf("agent", 900, 900, agent.id)).toEqual({
      valid: true,
      parentId: project.id,
    });
    expect(graph.landingOf("github", 900, 900, github.id)).toEqual({
      valid: true,
      parentId: project.id,
    });
  });

  test("a moved top-level block nests only when it holds no blocks", () => {
    const { graph, project } = projectWithGitHub();
    const other = graph.addNode("project", 700, 700);

    expect(graph.landingOf("project", 400, 300, other.id)).toEqual({
      valid: true,
      parentId: project.id,
    });
    expect(graph.landingOf("project", 750, 750, project.id)).toEqual({
      valid: true,
    });
    graph.addNode("agent", 10, 40, other.id);
    expect(graph.landingOf("project", 400, 300, other.id)).toEqual({
      valid: true,
    });
  });

  test("a container grows to fit a block placed past its edges", () => {
    const { graph, project } = projectWithGitHub();
    const agent = graph.addNode("agent", 330, 280, project.id);

    graph.fitInParent(agent.id);

    // 330 + 160 + 12 and 280 + 64 + 12.
    expect(project.w).toBe(502);
    expect(project.h).toBe(356);
  });

  test("a block placed above or left of its container moves inside it", () => {
    const { graph, project } = projectWithGitHub();
    const agent = graph.addNode("agent", -30, -10, project.id);

    graph.fitInParent(agent.id);

    expect([agent.x, agent.y]).toEqual([0, 0]);
    expect([project.w, project.h]).toEqual([400, 300]);
  });

  test("growing a nested container grows the containers around it", () => {
    const { graph, project, github } = projectWithGitHub();
    const action = graph.addNode("action", 300, 250, github.id);

    graph.fitInParent(action.id);

    // GitHub 1 grows to 472 × 326 from (20, 40), so Project 1 must reach
    // 20 + 472 + 12 wide and 40 + 326 + 12 tall.
    expect([github.w, github.h]).toEqual([472, 326]);
    expect([project.w, project.h]).toEqual([504, 378]);
  });
});

describe("schema checks inside an agent", () => {
  function writerWithCheck() {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    const writer = graph.addNode("agent", 10, 40, project.id);
    const check = graph.addNode("jsonschema", 10, 40, writer.id);
    const fixer = graph.addNode("agent", 300, 40, project.id);
    return { graph, project, writer, check, fixer };
  }

  test("a check sends its outputs to the blocks beside its agent", () => {
    const { graph, check, fixer } = writerWithCheck();

    expect(graph.connect(check.id, fixer.id)).toBe(true);
    expect(graph.edges[0]).toMatchObject({ fromPort: "valid" });
  });

  test("nothing draws an arrow into a check: it takes its agent's reply", () => {
    const { graph, writer, check, fixer } = writerWithCheck();
    const other = graph.addNode("jsonschema", 10, 120, writer.id);

    expect(graph.connect(writer.id, check.id)).toBe(false);
    expect(graph.connect(fixer.id, check.id)).toBe(false);
    expect(graph.connect(other.id, check.id)).toBe(false);
    expect(graph.edges).toEqual([]);
  });
});

describe("resizing from an edge", () => {
  function projectWithAgent() {
    const graph = new GraphStore();
    const project = graph.addNode("project", 100, 100);
    graph.resizeNode(project.id, 400, 300);
    const agent = graph.addNode("agent", 60, 80, project.id);
    const start = { x: 100, y: 100, w: 400, h: 300 };
    return { graph, project, agent, start };
  }

  test("the right and bottom edges change only the size", () => {
    const { graph, project, start } = projectWithAgent();

    graph.resizeFrom(project.id, start, { right: true }, 40, 99);
    expect(project).toMatchObject({ x: 100, y: 100, w: 440, h: 300 });

    graph.resizeFrom(project.id, start, { bottom: true }, 99, -20);
    expect(project).toMatchObject({ w: 400, h: 280 });
  });

  test("the left and top edges move the block and keep its blocks in place", () => {
    const { graph, project, agent, start } = projectWithAgent();

    graph.resizeFrom(project.id, start, { left: true, top: true }, -30, 20);

    expect(project).toMatchObject({ x: 70, y: 120, w: 430, h: 280 });
    // The agent stays at (160, 180) on the canvas.
    expect([agent.x, agent.y]).toEqual([90, 60]);
  });

  test("a block keeps its minimum size from any edge", () => {
    const graph = new GraphStore();
    const agent = graph.addNode("agent", 100, 100);
    const start = { x: 100, y: 100, w: 160, h: 64 };

    graph.resizeFrom(agent.id, start, { left: true, top: true }, 500, 500);

    expect(agent).toMatchObject({
      x: 100 + 160 - MIN_NODE_WIDTH,
      y: 100 + 64 - MIN_NODE_HEIGHT,
      w: MIN_NODE_WIDTH,
      h: MIN_NODE_HEIGHT,
    });
  });

  test("a container never shrinks past the blocks inside it", () => {
    const { graph, project, agent, start } = projectWithAgent();

    graph.resizeFrom(
      project.id,
      start,
      { right: true, bottom: true },
      -300,
      -250,
    );
    // The agent spans (60..220, 80..144) inside the project.
    expect(project).toMatchObject({ w: 220, h: 144 });

    graph.resizeFrom(project.id, start, { left: true, top: true }, 200, 200);
    expect(project).toMatchObject({ x: 160, y: 180, w: 340, h: 220 });
    expect([agent.x, agent.y]).toEqual([0, 0]);
  });
});

describe("drawing and editing arrows", () => {
  // A project holding a command gate, two agents and a writer with a schema
  // check, beside a second project with an agent of its own.
  function flow() {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    graph.resizeNode(project.id, 900, 600);
    const gate = graph.addNode("command", 20, 40, project.id);
    const coder = graph.addNode("agent", 300, 40, project.id);
    const reviewer = graph.addNode("agent", 600, 40, project.id);
    const writer = graph.addNode("agent", 20, 300, project.id);
    const check = graph.addNode("jsonschema", 10, 40, writer.id);
    const other = graph.addNode("project", 1000, 0);
    const stranger = graph.addNode("agent", 10, 40, other.id);
    return { graph, project, gate, coder, reviewer, writer, check, stranger };
  }

  test("tells whether an arrow may be drawn without drawing it", () => {
    const { graph, project, gate, coder, writer, check, stranger } = flow();

    expect(graph.canConnect(gate.id, coder.id)).toBe(true);
    expect(graph.canConnect(check.id, coder.id)).toBe(true);
    expect(graph.canConnect(coder.id, coder.id)).toBe(false);
    expect(graph.canConnect(coder.id, stranger.id)).toBe(false);
    expect(graph.canConnect(project.id, coder.id)).toBe(false);
    expect(graph.canConnect(writer.id, check.id)).toBe(false);
    expect(graph.canConnect(coder.id, "missing")).toBe(false);
    expect(graph.edges).toEqual([]);

    graph.connect(gate.id, coder.id);
    expect(graph.canConnect(gate.id, coder.id)).toBe(false);
    expect(graph.canConnect(coder.id, gate.id)).toBe(false);
  });

  test("follows the Git action rules connect follows", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    const github = graph.addNode("github", 20, 20, project.id);
    const commit = graph.addAction(github.id, "commit")!;
    const agent = graph.addNode("agent", 300, 20, project.id);
    const other = graph.addNode("project", 900, 0);
    const stranger = graph.addNode("agent", 10, 10, other.id);

    expect(graph.canConnect(agent.id, commit.id)).toBe(true);
    expect(graph.canConnect(commit.id, agent.id)).toBe(true);
    expect(graph.canConnect(stranger.id, commit.id)).toBe(false);
  });

  test("an arrow moving its own end may land where it already is", () => {
    const { graph, gate, coder } = flow();
    graph.connect(gate.id, coder.id);
    const edge = graph.edges[0]!;

    expect(graph.canConnect(gate.id, coder.id, edge.id)).toBe(true);
    expect(graph.canConnect(coder.id, gate.id, edge.id)).toBe(true);
  });

  test("an arrow takes the output it is drawn from", () => {
    const { graph, gate, coder, reviewer } = flow();

    expect(graph.connect(gate.id, coder.id, "failed")).toBe(true);
    graph.connect(gate.id, reviewer.id, "no-such-output");

    expect(graph.edges.map((edge) => edge.fromPort)).toEqual([
      "failed",
      "passed",
    ]);
  });

  test("selecting an arrow and selecting a block exclude each other", () => {
    const { graph, gate, coder } = flow();
    graph.connect(gate.id, coder.id);
    const edge = graph.edges[0]!;

    graph.selectEdge(edge.id);
    expect(graph.selectedEdge).toMatchObject({ id: edge.id });
    expect(graph.selectedId).toBeNull();

    graph.select(coder.id);
    expect(graph.selectedEdgeId).toBeNull();
    expect(graph.selectedEdge).toBeUndefined();

    graph.selectEdge(edge.id);
    graph.addNode("agent", 50, 500);
    expect(graph.selectedEdgeId).toBeNull();

    graph.selectEdge(edge.id);
    graph.load({ nodes: [] });
    expect(graph.selectedEdgeId).toBeNull();
  });

  test("removes one arrow and its selection", () => {
    const { graph, gate, coder, reviewer } = flow();
    graph.connect(gate.id, coder.id);
    graph.connect(coder.id, reviewer.id);
    const [first, second] = graph.edges;
    graph.selectEdge(first!.id);

    graph.removeEdge(first!.id);
    graph.removeEdge("missing");

    expect(graph.edges.map((edge) => edge.id)).toEqual([second!.id]);
    expect(graph.selectedEdgeId).toBeNull();
    expect(graph.canConnect(gate.id, coder.id)).toBe(true);
  });

  test("deleting a block drops the selection of its arrows", () => {
    const { graph, gate, coder } = flow();
    graph.connect(gate.id, coder.id);
    graph.selectEdge(graph.edges[0]!.id);

    graph.removeNode(coder.id);

    expect(graph.selectedEdgeId).toBeNull();
  });

  // Actions run along their arrows from the starting action, so removing an
  // arrow in the sequence leaves the actions after it out of the run until
  // a new arrow joins them; nothing is rejoined behind the user's back.
  test("removing an arrow between Git actions does not rejoin the sequence", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    const github = graph.addNode("github", 20, 20, project.id);
    const fetch = graph.addAction(github.id, "fetch")!;
    const worktree = graph.addAction(github.id, "worktree")!;
    const rebase = graph.addAction(github.id, "rebase")!;
    const link = graph.edges.find((edge) => edge.to === worktree.id)!;

    graph.removeEdge(link.id);

    expect(graph.edges).toHaveLength(1);
    expect(graph.edges[0]).toMatchObject({ from: worktree.id, to: rebase.id });
    expect(graph.actionSequence(github.id).map((node) => node.name)).toEqual([
      "Fetch",
      "Create worktree",
      "Rebase",
    ]);
    const tail = graph.edges[0]!;
    expect(graph.reconnectEdge(tail.id, "from", fetch.id)).toBe(true);
    expect(graph.connect(rebase.id, worktree.id)).toBe(true);
    expect(graph.actionSequence(github.id).map((node) => node.name)).toEqual([
      "Fetch",
      "Rebase",
      "Create worktree",
    ]);
  });

  test("moves an arrow's head to another block and keeps its output", () => {
    const { graph, gate, coder, reviewer, stranger } = flow();
    graph.connect(gate.id, coder.id, "failed");
    const edge = graph.edges[0]!;

    expect(graph.reconnectEdge(edge.id, "to", stranger.id)).toBe(false);
    expect(graph.reconnectEdge(edge.id, "to", gate.id)).toBe(false);
    expect(graph.reconnectEdge(edge.id, "to", reviewer.id)).toBe(true);

    expect(graph.edges).toEqual([
      { id: edge.id, from: gate.id, to: reviewer.id, fromPort: "failed" },
    ]);
  });

  test("moves an arrow's tail and picks up the new source's output", () => {
    const { graph, gate, coder, reviewer, check } = flow();
    graph.connect(gate.id, reviewer.id, "failed");
    const edge = graph.edges[0]!;

    expect(graph.reconnectEdge(edge.id, "from", gate.id)).toBe(true);
    expect(graph.edges[0]?.fromPort).toBe("failed");

    expect(graph.reconnectEdge(edge.id, "from", check.id, "invalid")).toBe(
      true,
    );
    expect(graph.edges[0]).toMatchObject({
      from: check.id,
      to: reviewer.id,
      fromPort: "invalid",
    });

    expect(graph.reconnectEdge(edge.id, "from", coder.id)).toBe(true);
    expect(graph.edges[0]).toEqual({
      id: edge.id,
      from: coder.id,
      to: reviewer.id,
    });
    expect(graph.reconnectEdge("missing", "from", gate.id)).toBe(false);
  });

  test("finds the innermost block under a point", () => {
    const { graph, project, writer, check } = flow();

    expect(graph.nodeAt(35, 345)?.id).toBe(check.id);
    expect(graph.nodeAt(25, 305)?.id).toBe(writer.id);
    expect(graph.nodeAt(850, 550)?.id).toBe(project.id);
    expect(graph.nodeAt(950, 10)).toBeUndefined();
  });
});
