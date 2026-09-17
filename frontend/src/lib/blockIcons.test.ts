import { describe, expect, test } from "vitest";
import { BLOCK_ICONS, blockKind, iconOf } from "./blockIcons.js";
import { GIT_ACTIONS, GraphStore, PALETTE } from "./graph.svelte.js";

describe("block icons", () => {
  test("sources are objects; blocks that do work when run are actions", () => {
    const kinds = Object.fromEntries(
      PALETTE.map((item) => [item.type, blockKind(item.type)]),
    );

    expect(kinds).toEqual({
      project: "object",
      github: "object",
      gitlab: "object",
      githubapp: "object",
      agent: "action",
      command: "action",
      jsonschema: "action",
      router: "action",
      loop: "action",
    });
    expect(blockKind("action")).toBe("action");
  });

  test("each block type has an icon, and each Git action its own", () => {
    const graph = new GraphStore();
    const project = graph.addNode("project", 0, 0);
    const github = graph.addNode("github", 0, 0, project.id);
    const icons = new Set(PALETTE.map((item) => iconOf({ type: item.type })));
    for (const { action } of GIT_ACTIONS)
      icons.add(iconOf(graph.addAction(github.id, action)!));

    expect(icons.size).toBe(PALETTE.length + GIT_ACTIONS.length);
    for (const icon of icons) expect(BLOCK_ICONS[icon]).toBeDefined();
    expect(iconOf({ type: "github" })).toBe("github");
    expect(iconOf({ type: "action", action: "commit" })).toBe("commit");
  });
});
