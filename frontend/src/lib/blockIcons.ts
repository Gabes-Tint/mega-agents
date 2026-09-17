import type { GraphNode, NodeType } from "./graph.svelte.js";

// A two-tone icon on a 24-unit grid: a soft filled shape under a line
// drawing, or a brand mark filled in one color.
export interface BlockIconShape {
  tone?: string;
  line?: string;
  mark?: string;
}

// Sources are the things blocks work on; every other block does work when a
// run reaches it.
export type BlockKind = "object" | "action";

const OBJECTS: readonly NodeType[] = [
  "project",
  "github",
  "gitlab",
  "githubapp",
];

export function blockKind(type: NodeType): BlockKind {
  return OBJECTS.includes(type) ? "object" : "action";
}

const circle = (x: number, y: number, r: number) =>
  `M${x - r} ${y}a${r} ${r} 0 1 0 ${2 * r} 0a${r} ${r} 0 1 0 ${-2 * r} 0`;

export const BLOCK_ICONS: Record<string, BlockIconShape> = {
  project: {
    tone: "M3 8h18v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z",
    line: "M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z",
  },
  github: {
    mark: "M12 .3a12 12 0 0 0-3.8 23.38c.6.12.83-.26.83-.57L9 21.07c-3.34.72-4.04-1.61-4.04-1.61-.55-1.39-1.34-1.76-1.34-1.76-1.08-.74.09-.73.09-.73 1.2.09 1.83 1.24 1.83 1.24 1.07 1.83 2.81 1.3 3.5 1 .1-.78.41-1.31.76-1.61-2.67-.3-5.47-1.33-5.47-5.93 0-1.31.47-2.38 1.24-3.22-.14-.3-.54-1.52.1-3.18 0 0 1-.32 3.3 1.23a11.5 11.5 0 0 1 6 0c2.28-1.55 3.29-1.23 3.29-1.23.64 1.66.24 2.88.12 3.18a4.65 4.65 0 0 1 1.23 3.22c0 4.61-2.8 5.62-5.48 5.92.42.36.81 1.1.81 2.22l-.01 3.29c0 .32.21.7.82.58A12 12 0 0 0 12 .3",
  },
  gitlab: {
    mark: "m23.6 9.59-.03-.09L20.3.98a.85.85 0 0 0-.34-.4.87.87 0 0 0-1 .05.87.87 0 0 0-.29.44l-2.2 6.75H7.54L5.33 1.07a.86.86 0 0 0-.29-.44.87.87 0 0 0-1-.06.86.86 0 0 0-.34.41L.43 9.5l-.03.09a6.07 6.07 0 0 0 2.01 7.01l.01.01.03.02 4.98 3.73 2.46 1.86 1.5 1.13a1 1 0 0 0 1.22 0l1.5-1.13 2.46-1.86 5.01-3.75.01-.01a6.07 6.07 0 0 0 2.01-7z",
  },
  githubapp: {
    tone: circle(8, 15, 4),
    line: `${circle(8, 15, 4)}M10.8 12.2 20 3m-4 4 3 3m-1-5 2 2`,
  },
  agent: {
    tone: "M11 4l1.8 4.9L17.7 10.7l-4.9 1.8L11 17.4l-1.8-4.9L4.3 10.7l4.9-1.8z",
    line: "M11 4l1.8 4.9L17.7 10.7l-4.9 1.8L11 17.4l-1.8-4.9L4.3 10.7l4.9-1.8zM19 15v6m-3-3h6",
  },
  command: {
    tone: "M3 8h18v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z",
    line: "M5 4h14a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2zm2 6 3 3-3 3m5 0h5",
  },
  jsonschema: {
    tone: "M9 9h6v6H9z",
    line: "M8 3H7a2 2 0 0 0-2 2v5a2 2 0 0 1-2 2 2 2 0 0 1 2 2v5a2 2 0 0 0 2 2h1m8-18h1a2 2 0 0 1 2 2v5a2 2 0 0 0 2 2 2 2 0 0 0-2 2v5a2 2 0 0 1-2 2h-1M10 12l1.5 1.5L14 11",
  },
  router: {
    tone: circle(5, 12, 2.5),
    line: `${circle(5, 12, 2.5)}M7.5 12H10l4-6h6m-10 6 4 6h6M17 3l3 3-3 3m0 6 3 3-3 3`,
  },
  loop: {
    tone: circle(12, 12, 3),
    line: "M17 2l4 4-4 4M3 11v-1a4 4 0 0 1 4-4h14M7 22l-4-4 4-4M21 13v1a4 4 0 0 1-4 4H3",
  },
  fetch: {
    tone: "M4 17h16v4H4z",
    line: "M12 3v12m-5-5 5 5 5-5M4 20h16",
  },
  worktree: {
    tone: `${circle(18, 6, 3)}${circle(6, 18, 3)}`,
    line: `M6 3v12${circle(18, 6, 3)}${circle(6, 18, 3)}M18 9a9 9 0 0 1-9 9`,
  },
  rebase: {
    tone: circle(18, 18, 3),
    line: `M6 3v18${circle(18, 18, 3)}M18 15V11a4 4 0 0 0-4-4H9m3-3-3 3 3 3`,
  },
  issue: {
    tone: circle(12, 12, 3),
    line: `${circle(12, 12, 9)}${circle(12, 12, 1)}`,
  },
  commit: {
    tone: circle(12, 12, 3),
    line: `M3 12h6m6 0h6${circle(12, 12, 3)}`,
  },
  push: {
    tone: "M4 3h16v4H4z",
    line: "M12 21V9m-5 5 5-5 5 5M4 4h16",
  },
  pullrequest: {
    tone: `${circle(6, 6, 3)}${circle(18, 18, 3)}`,
    line: `${circle(6, 6, 3)}${circle(18, 18, 3)}M6 9v12m12-6V9a3 3 0 0 0-3-3h-4m2-3-3 3 3 3`,
  },
};

// The icon a block shows: its Git action's for an action, else its type's.
export function iconOf(node: Pick<GraphNode, "type" | "action">): string {
  return node.type === "action" && node.action ? node.action : node.type;
}
