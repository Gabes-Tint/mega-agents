export type NodeType =
  | "agent"
  | "project"
  | "github"
  | "gitlab"
  | "githubapp"
  | "action"
  | "jsonschema"
  | "router"
  | "command"
  | "loop";

export type GitAction =
  "fetch" | "worktree" | "rebase" | "issue" | "commit" | "push" | "pullrequest";

export type ActionField =
  "branch" | "base" | "worktreePath" | "onto" | "message" | "title" | "body";

// Git actions a GitHub block runs as its own internal sequence. They are
// added from the block's properties rather than dragged from the palette.
// The labels Read issue ignores unless the action lists its own.
export const DEFAULT_IGNORE_LABELS = ["paused", "draft", "needs-attention"];

export const GIT_ACTIONS: readonly { action: GitAction; label: string }[] = [
  { action: "fetch", label: "Fetch" },
  { action: "worktree", label: "Create worktree" },
  { action: "rebase", label: "Rebase" },
  { action: "issue", label: "Read issue" },
  { action: "commit", label: "Commit" },
  { action: "push", label: "Push" },
  { action: "pullrequest", label: "Open pull request" },
];

export type AgentField =
  "backend" | "model" | "effort" | "prompt" | "outputSchema";

export type AgentNumber = "retries" | "timeoutMinutes" | "maxCostUsd";

// Coding-agent CLIs an Agent block can run, as the backend names them.
export const AGENT_BACKENDS: readonly { backend: string; label: string }[] = [
  { backend: "claude", label: "Claude Code" },
  { backend: "codex", label: "Codex" },
  { backend: "grok", label: "Grok" },
  { backend: "opencode", label: "OpenCode" },
];

export interface RouteCase {
  name: string;
  expression: string;
}

// The graph as the backend takes it: runs, exports and saved workflows.
export interface WorkflowGraph {
  name?: string;
  nodes: GraphNode[];
  edges?: GraphEdge[];
}

// File name a workflow is saved under: its name in lowercase words joined
// with dashes.
export function workflowSlug(name: string): string {
  const slug = name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  return slug || "workflow";
}

// Something to fix before running, as the backend's check reports it.
export interface Problem {
  nodeId?: string;
  severity: string;
  message: string;
}

export interface RunStepStatus {
  nodeId: string;
  status: string;
  // The loop a step repeats in, and the iteration it shows.
  loop?: string;
  iteration?: number;
}

// One way a loop can end: a block inside it taking one of its outputs.
export interface LoopExit {
  nodeId: string;
  port: string;
  label: string;
}

// Where a dragged block would end up: inside a container, on the canvas
// itself, or nowhere because the container under it refuses it.
export interface Landing {
  valid: boolean;
  parentId?: string;
  refusedBy?: string;
}

// The edges a resize drags.
export interface ResizeEdges {
  left?: boolean;
  right?: boolean;
  top?: boolean;
  bottom?: boolean;
}

// The canvas root as a pseudo target so the containment matrix covers
// top-level drops in the same table as nested drops.
export type DropTarget = NodeType | "root";

export interface GraphEdge {
  id: string;
  from: string;
  to: string;
  // Which output of a block with several the arrow takes.
  fromPort?: string;
}

export interface PaletteItem {
  type: NodeType;
  label: string;
  // What the component does, shown when the palette entry is hovered.
  description: string;
}

export interface GraphNode {
  id: string;
  type: NodeType;
  name: string;
  x: number;
  y: number;
  w: number;
  h: number;
  parentId?: string;
  start?: boolean;
  // Pauses the run before this block runs, every time it runs, until
  // someone continues, steps, skips or edits it.
  breakpoint?: boolean;
  path?: string;
  repository?: string;
  secretKey?: string;
  // The machine already holds forge credentials (SSH keys or an OAuth
  // credential helper), so runs use them instead of a token.
  authenticated?: boolean;
  appId?: string;
  privateKeyPath?: string;
  action?: GitAction;
  branch?: string;
  base?: string;
  worktreePath?: string;
  onto?: string;
  backend?: string;
  model?: string;
  effort?: string;
  prompt?: string;
  outputSchema?: string;
  retries?: number;
  timeoutMinutes?: number;
  maxCostUsd?: number;
  continueSession?: boolean;
  schema?: string;
  cases?: RouteCase[];
  command?: string;
  message?: string;
  title?: string;
  body?: string;
  // Read issue reads this issue, or the next available one when it is 0.
  issue?: number;
  // Labels that stop Read issue from reading an issue; without them the
  // defaults apply, and an empty list ignores none.
  ignoreLabels?: string[];
  maxIterations?: number;
  untilNode?: string;
  untilPort?: string;
  // Runs when any arrow into the block arrives rather than all of them.
  waitForAny?: boolean;
}

export const DEFAULT_NODE_WIDTH = 160;
export const DEFAULT_NODE_HEIGHT = 64;
export const MIN_NODE_WIDTH = 120;
export const MIN_NODE_HEIGHT = 48;

// A loop starts large enough to hold a gate and a fixer.
const LOOP_WIDTH = 400;
const LOOP_HEIGHT = 220;

// The size a block of the type starts with.
export function defaultSize(type: NodeType): { w: number; h: number } {
  return type === "loop"
    ? { w: LOOP_WIDTH, h: LOOP_HEIGHT }
    : { w: DEFAULT_NODE_WIDTH, h: DEFAULT_NODE_HEIGHT };
}

// Layout of a GitHub block's action sequence: below the block header, one
// action under the other with room for the arrow between them.
const ACTION_INSET = 12;
const ACTION_TOP = 40;
const ACTION_GAP = 24;

export const PALETTE: readonly PaletteItem[] = [
  {
    type: "agent",
    label: "Agent",
    description:
      "One conversation with a coding agent (Claude Code, Codex, Grok or OpenCode) in the connected workspace or its project's folder.",
  },
  {
    type: "project",
    label: "Project",
    description:
      "A repository folder on this machine. The blocks inside it work on that repository.",
  },
  {
    type: "github",
    label: "GitHub",
    description:
      "A GitHub repository. Its actions fetch, create a worktree, read an issue, commit, push and open a pull request.",
  },
  {
    type: "gitlab",
    label: "GitLab",
    description: "A GitLab repository's settings. Runs don't use GitLab yet.",
  },
  {
    type: "githubapp",
    label: "GitHub App",
    description:
      "A GitHub App's ID and private key, for the GitHub block it sits in.",
  },
  {
    type: "jsonschema",
    label: "JSON Schema",
    description:
      "Sits inside an agent and checks its reply against a JSON Schema, sending it down valid or invalid to the blocks beside the agent.",
  },
  {
    type: "router",
    label: "Router",
    description:
      "Sends the connected value down the first case whose CEL condition holds, or down default.",
  },
  {
    type: "command",
    label: "Command",
    description:
      "Runs a shell command, such as tests or a linter, in the workspace. Exit 0 goes to passed; anything else to failed.",
  },
  {
    type: "loop",
    label: "Loop",
    description:
      "Repeats the blocks inside it, such as a gate and a fixer, until one takes a chosen output or the repeats run out.",
  },
];

// Hue (OKLCH degrees) that colors each block type on the canvas and in the
// palette, spread around the wheel so neighbours never look alike. The
// stylesheet derives each theme's tints from it.
export const BLOCK_HUES: Record<NodeType, number> = {
  agent: 288,
  project: 252,
  loop: 216,
  action: 180,
  jsonschema: 144,
  command: 108,
  router: 72,
  gitlab: 36,
  githubapp: 0,
  github: 324,
};

// Single source of truth for where a block may go, agreed with the product
// owner. Keys are the block being dropped; the arrays are every target that
// accepts it, including the canvas root as a pseudo target. Anything absent
// from a target's list is rejected.
export const CONTAINMENT_MATRIX: Record<NodeType, readonly DropTarget[]> = {
  agent: ["project", "agent", "loop"],
  project: ["root", "project"],
  github: ["project"],
  gitlab: ["project"],
  githubapp: ["github"],
  action: ["github"],
  jsonschema: ["agent"],
  router: ["project", "agent", "loop"],
  command: ["project", "agent", "loop"],
  loop: ["project"],
};

// Boxes that can host children: every target that appears as a parent in
// the containment matrix.
export const CONTAINER_TYPES: readonly NodeType[] = (
  [
    "agent",
    "project",
    "github",
    "gitlab",
    "githubapp",
    "action",
    "jsonschema",
    "router",
    "command",
    "loop",
  ] as NodeType[]
).filter((type) =>
  Object.values(CONTAINMENT_MATRIX).some((targets) => targets.includes(type)),
);

export function isContainerType(type: NodeType): boolean {
  return CONTAINER_TYPES.includes(type);
}

export function canHostChild(
  parentType: DropTarget,
  childType: NodeType,
): boolean {
  return CONTAINMENT_MATRIX[childType].includes(parentType);
}

// Whether the block may exist at the top level of the canvas, straight from
// the matrix's root column.
export function canExistTopLevel(type: NodeType): boolean {
  return canHostChild("root", type);
}

function article(label: string): string {
  return /^[aeiou]/i.test(label) ? "an" : "a";
}

// Explanation for a rejected palette drop: either the container is not
// allowed to host the block, or the empty canvas does not accept it.
export function rejectedDropHint(
  type: NodeType,
  containerType?: NodeType,
): string {
  const label = labelFor(type);
  const articleLabel = `${article(label)} ${label}`;
  const sentence = containerType
    ? `${articleLabel} cannot be placed inside ${article(labelFor(containerType))} ${TARGET_LABELS[containerType]} box.`
    : `${articleLabel} can only be dropped inside ${allowedTargetsLabel(type)}.`;
  return sentence.charAt(0).toUpperCase() + sentence.slice(1);
}

// How a target type is spelled inside explanations for rejected drops:
// lowercase for common nouns, brand casing for the forge products.
const TARGET_LABELS: Record<NodeType, string> = {
  agent: "agent",
  project: "project",
  github: "GitHub",
  gitlab: "GitLab",
  githubapp: "GitHub App",
  action: "Git action",
  jsonschema: "JSON Schema",
  router: "Router",
  command: "Command",
  loop: "loop",
};

// Human-readable list of the targets a block may be dropped into, used to
// explain rejected drops.
export function allowedTargetsLabel(type: NodeType): string {
  const labels = CONTAINMENT_MATRIX[type]
    .filter((target): target is NodeType => target !== "root")
    .map((target) => {
      if (target === type) return `another ${TARGET_LABELS[type]}`;
      return `${article(TARGET_LABELS[target])} ${TARGET_LABELS[target]} box`;
    });
  if (labels.length === 0) return "nothing";
  if (labels.length === 1) return labels[0] ?? "nothing";
  const last = labels.at(-1) ?? "nothing";
  return `${labels.slice(0, -1).join(", ")} or ${last}`;
}

export function labelFor(type: NodeType): string {
  return PALETTE.find((item) => item.type === type)?.label ?? type;
}

// GitBase is the abstract base the GitHub and GitLab controllers derive
// from; it is a code-level abstraction, not a displayable palette component.
// The Go exporter derives both controllers' YAML configuration from it, and
// the frontend shares the forge property fields through this list.
export const GIT_BASE_TYPES: readonly NodeType[] = ["github", "gitlab"];

export function isForgeType(type: NodeType): boolean {
  return GIT_BASE_TYPES.includes(type);
}

// Tiny three-character identifier shown in the block header, derived
// deterministically from the node id so it stays stable across sessions.
export function shortNodeId(id: string): string {
  let hash = 0x811c9dc5;
  for (let index = 0; index < id.length; index++) {
    hash ^= id.charCodeAt(index);
    hash = Math.imul(hash, 0x01000193);
  }
  return (hash >>> 0).toString(36).padStart(3, "0").slice(-3);
}

function paletteLabel(type: NodeType): string {
  return labelFor(type);
}

// Named outputs a block chooses between; empty for blocks with one output.
export function outputPortsOf(node: GraphNode | undefined): string[] {
  if (node?.type === "jsonschema") return ["valid", "invalid"];
  if (node?.type === "command") return ["passed", "failed"];
  if (node?.type === "loop") return ["done", "exhausted"];
  if (node?.type === "router")
    return [
      ...(node.cases ?? []).map((routeCase) => routeCase.name),
      "default",
    ];
  return [];
}

export class GraphStore {
  nodes = $state<GraphNode[]>([]);
  edges = $state<GraphEdge[]>([]);
  selectedId = $state<string | null>(null);
  // The selected arrow; selecting a block or an arrow clears the other.
  selectedEdgeId = $state<string | null>(null);
  // Component type currently dragged from the palette; dataTransfer.getData
  // is protected while a drag is in flight, so the type travels through the
  // store to power live dragover validation on the canvas.
  draggingType = $state<NodeType | null>(null);
  runStatuses = $state<Record<string, string>>({});
  runIterations = $state<Record<string, number>>({});
  // The run whose statuses the canvas shows, and the block whose log of that
  // run is open.
  runId = $state<string | null>(null);
  workflowName = $state("workflow");
  logNodeId = $state<string | null>(null);
  // What the backend's check found to fix in the graph as it is now.
  problems = $state<Problem[]>([]);

  // Replaces the whole graph, as when a saved workflow is opened.
  load(workflow: WorkflowGraph): void {
    this.nodes = workflow.nodes;
    this.edges = workflow.edges ?? [];
    this.workflowName = workflow.name ?? "workflow";
    this.select(null);
    this.showRun([]);
    this.logNodeId = null;
  }

  toRequest(): Required<WorkflowGraph> {
    return { name: this.workflowName, nodes: this.nodes, edges: this.edges };
  }

  get selected(): GraphNode | undefined {
    return this.nodes.find((node) => node.id === this.selectedId);
  }

  get selectedEdge(): GraphEdge | undefined {
    return this.edges.find((edge) => edge.id === this.selectedEdgeId);
  }

  // Content-space position of a box, accumulated over its full parent chain
  // so arbitrarily deep nesting resolves correctly.
  absolutePosition(node: GraphNode): { x: number; y: number } {
    let x = node.x;
    let y = node.y;
    let current = node;
    while (current.parentId) {
      const parent = this.nodes.find(
        (candidate) => candidate.id === current.parentId,
      );
      if (!parent) break;
      x += parent.x;
      y += parent.y;
      current = parent;
    }
    return { x, y };
  }

  addNode(type: NodeType, x: number, y: number, parentId?: string): GraphNode {
    const count = this.nodes.filter((node) => node.type === type).length + 1;
    let parent: GraphNode | undefined;
    if (parentId)
      parent = this.nodes.find((candidate) => candidate.id === parentId);
    // An incompatible parent is ignored rather than rejected so the palette
    // drop falls through to a top-level box.
    const effectiveParent =
      parent && canHostChild(parent.type, type) ? parentId : undefined;
    const node: GraphNode = {
      id: crypto.randomUUID(),
      type,
      name: `${paletteLabel(type)} ${count}`,
      x,
      y,
      ...defaultSize(type),
      parentId: effectiveParent,
    };
    if (type === "agent") node.backend = "claude";
    if (type === "loop") node.maxIterations = 3;
    if (type === "router")
      node.cases = [
        { name: "approved", expression: 'value.verdict == "approve"' },
      ];
    this.nodes.push(node);
    this.select(node.id);
    // The store holds a reactive proxy of the node; hand that back so later
    // changes through the store are visible to the caller.
    return this.nodes.at(-1) ?? node;
  }

  select(id: string | null): void {
    this.selectedId = id;
    this.selectedEdgeId = null;
  }

  selectEdge(id: string | null): void {
    this.selectedEdgeId = id;
    this.selectedId = null;
  }

  moveNode(id: string, x: number, y: number): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (node) {
      // Children are stored in their parent's coordinate space, so moving a
      // parent carries them along without touching their relative positions.
      node.x = x;
      node.y = y;
    }
  }

  attachToContainer(
    id: string,
    parentId: string,
    absoluteX: number,
    absoluteY: number,
  ): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    const parent = this.nodes.find((candidate) => candidate.id === parentId);
    if (
      !node ||
      !parent ||
      parentId === id ||
      // Containers may sit at any depth, so only the child-type rule and the
      // has-children guard apply: a box that itself holds children stays put.
      !canHostChild(parent.type, node.type) ||
      this.hasChildren(id)
    )
      return;
    // Children live in their parent's coordinate space, so the content-space
    // drop position converts to parent-relative coordinates here. A flagged
    // start point cannot cross containers without breaking the single-start
    // rule, so it arrives unflagged.
    const parentOrigin = this.absolutePosition(parent);
    node.parentId = parentId;
    node.x = absoluteX - parentOrigin.x;
    node.y = absoluteY - parentOrigin.y;
    node.start = false;
  }

  detachNode(id: string, absoluteX: number, absoluteY: number): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (!node || node.parentId === undefined) return;
    node.parentId = undefined;
    node.x = absoluteX;
    node.y = absoluteY;
    node.start = false;
  }

  // Siblings are nodes sharing the same parent (top-level nodes share the
  // implicit canvas root), so at most one sibling carries the start flag.
  setStart(id: string, isStart: boolean): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (!node) return;
    if (isStart) {
      for (const candidate of this.nodes) {
        if (candidate.id !== id && candidate.start) {
          if (candidate.parentId === node.parentId) candidate.start = false;
        }
      }
    }
    node.start = isStart;
  }

  // A breakpoint belongs to the block, so it is saved with the workflow and
  // any number of blocks may carry one.
  setBreakpoint(id: string, on: boolean): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (!node) return;
    node.breakpoint = on || undefined;
  }

  // Boxes connect through arrows only within their own level: two top-level
  // boxes or two children of the same project. Level membership is validated
  // at creation only; edges follow their endpoints afterward. Any number of
  // arrows may leave or enter a box, but a pair is connected at most once in
  // either direction. The arrow named by exceptEdgeId, one whose end is being
  // moved, does not count as connecting its pair.
  canConnect(fromId: string, toId: string, exceptEdgeId?: string): boolean {
    const from = this.nodes.find((candidate) => candidate.id === fromId);
    const to = this.nodes.find((candidate) => candidate.id === toId);
    // A schema check takes the reply of the agent it sits in, never an arrow.
    if (!from || !to || fromId === toId || to.type === "jsonschema")
      return false;
    // A Git action's output also leaves its GitHub block toward the blocks
    // beside that GitHub block, such as an agent that takes the workspace,
    // and a schema check's output leaves its agent the same way.
    const provider =
      from.type === "action" || from.type === "jsonschema"
        ? this.nodes.find((candidate) => candidate.id === from.parentId)
        : undefined;
    const sameLevel = (from.parentId ?? null) === (to.parentId ?? null);
    const besideProvider =
      provider !== undefined && provider.parentId === to.parentId;
    // A block beside a GitHub block may also feed one of its actions, such
    // as an agent whose changes a Commit action records.
    const target =
      to.type === "action"
        ? this.nodes.find((candidate) => candidate.id === to.parentId)
        : undefined;
    const intoProvider =
      target !== undefined && target.parentId === from.parentId;
    if (!sameLevel && !besideProvider && !intoProvider) return false;
    return !this.edges.some(
      (edge) =>
        edge.id !== exceptEdgeId &&
        ((edge.from === fromId && edge.to === toId) ||
          (edge.from === toId && edge.to === fromId)),
    );
  }

  connect(fromId: string, toId: string, fromPort?: string): boolean {
    if (!this.canConnect(fromId, toId)) return false;
    this.edges.push({
      id: crypto.randomUUID(),
      from: fromId,
      to: toId,
      ...this.portFor(fromId, fromPort),
    });
    return true;
  }

  // An arrow from a block with several outputs names the one it takes: the
  // chosen one, or the block's first.
  private portFor(fromId: string, chosen?: string): { fromPort?: string } {
    const ports = this.outputPorts(fromId);
    const fromPort = chosen && ports.includes(chosen) ? chosen : ports[0];
    return fromPort ? { fromPort } : {};
  }

  // Removes one arrow. An arrow between Git actions goes like any other:
  // nothing rejoins the sequence, so the actions after it leave the run, and
  // the problems panel says so, until an arrow joins them again.
  removeEdge(id: string): void {
    this.edges = this.edges.filter((edge) => edge.id !== id);
    if (this.selectedEdgeId === id) this.selectedEdgeId = null;
  }

  // Moves one end of an arrow onto another block when the new pair may be
  // connected. The arrow keeps its output while its source stays, and takes
  // the chosen or first output of a new source.
  reconnectEdge(
    id: string,
    end: "from" | "to",
    nodeId: string,
    fromPort?: string,
  ): boolean {
    const edge = this.edges.find((candidate) => candidate.id === id);
    if (!edge) return false;
    const from = end === "from" ? nodeId : edge.from;
    const to = end === "to" ? nodeId : edge.to;
    if (!this.canConnect(from, to, id)) return false;
    if (from !== edge.from) {
      delete edge.fromPort;
      Object.assign(edge, { from }, this.portFor(from, fromPort));
    }
    edge.to = to;
    return true;
  }

  // Deletes a block with everything nested inside it and every arrow that
  // touches them. A Git action leaves its sequence joined: the action before
  // it leads to the one after it, which starts the sequence if it was first.
  removeNode(id: string): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (!node) return;
    const removed = [id];
    for (let grew = true; grew;) {
      grew = false;
      for (const candidate of this.nodes)
        if (
          candidate.parentId &&
          removed.includes(candidate.parentId) &&
          !removed.includes(candidate.id)
        ) {
          removed.push(candidate.id);
          grew = true;
        }
    }
    const sibling = (nodeId: string) =>
      this.nodes.find(
        (candidate) =>
          candidate.id === nodeId &&
          candidate.type === "action" &&
          candidate.parentId === node.parentId,
      );
    const previous =
      node.type === "action"
        ? this.edges
            .map((edge) => edge.to === id && sibling(edge.from))
            .find(Boolean)
        : undefined;
    const next =
      node.type === "action"
        ? this.edges
            .map((edge) => edge.from === id && sibling(edge.to))
            .find(Boolean)
        : undefined;
    this.nodes = this.nodes.filter(
      (candidate) => !removed.includes(candidate.id),
    );
    this.edges = this.edges.filter(
      (edge) => !removed.includes(edge.from) && !removed.includes(edge.to),
    );
    if (next && node.start) next.start = true;
    if (previous && next) this.connect(previous.id, next.id);
    for (const loop of this.nodes)
      if (loop.untilNode && removed.includes(loop.untilNode)) {
        loop.untilNode = undefined;
        loop.untilPort = undefined;
      }
    if (this.selectedId && removed.includes(this.selectedId))
      this.selectedId = null;
    if (!this.selectedEdge) this.selectedEdgeId = null;
    if (this.logNodeId && removed.includes(this.logNodeId))
      this.logNodeId = null;
  }

  hasChildren(id: string): boolean {
    return this.nodes.some((candidate) => candidate.parentId === id);
  }

  // Innermost (deepest, topmost last-added) container whose absolute box
  // contains the content point. Containers may sit at any depth, so a nested
  // GitHub inside a project resolves for drops on its own area. A dragged
  // node never resolves to its own box, or moving within a box would freeze.
  containerAt(x: number, y: number, ignoreId?: string): GraphNode | undefined {
    let best: GraphNode | undefined;
    let bestDepth = -1;
    for (const node of this.nodes) {
      if (!isContainerType(node.type)) continue;
      if (node.id === ignoreId) continue;
      const origin = this.absolutePosition(node);
      if (
        x >= origin.x &&
        x < origin.x + node.w &&
        y >= origin.y &&
        y < origin.y + node.h
      ) {
        const depth = this.nodeDepth(node);
        if (depth >= bestDepth) {
          best = node;
          bestDepth = depth;
        }
      }
    }
    return best;
  }

  // Innermost block of any type whose box contains the content point, the
  // later one where blocks at the same depth overlap.
  nodeAt(x: number, y: number): GraphNode | undefined {
    let best: GraphNode | undefined;
    for (const node of this.nodes) {
      const origin = this.absolutePosition(node);
      if (
        x >= origin.x &&
        x < origin.x + node.w &&
        y >= origin.y &&
        y < origin.y + node.h &&
        (!best || this.nodeDepth(node) >= this.nodeDepth(best))
      )
        best = node;
    }
    return best;
  }

  // Where a block of the type dropped at the content point lands: from the
  // palette, or moved when movingId names it. A moved block always lands
  // somewhere: a container that refuses it keeps it in its own parent.
  landingOf(type: NodeType, x: number, y: number, movingId?: string): Landing {
    const container = this.containerAt(x, y, movingId);
    const moving = this.nodes.find((candidate) => candidate.id === movingId);
    const hosts = container !== undefined && canHostChild(container.type, type);
    if (!moving) {
      if (hosts) return { valid: true, parentId: container.id };
      if (canExistTopLevel(type)) return { valid: true };
      return container
        ? { valid: false, refusedBy: container.id }
        : { valid: false };
    }
    if (moving.parentId === undefined)
      return hosts && !this.hasChildren(moving.id)
        ? { valid: true, parentId: container.id }
        : { valid: true };
    if (hosts || container?.id === moving.parentId)
      return { valid: true, parentId: container?.id };
    if (!container && canExistTopLevel(type)) return { valid: true };
    return { valid: true, parentId: moving.parentId };
  }

  // Keeps a block inside its container: a block past the top or left edge
  // moves in, and the container, with every container around it, grows to
  // fit a block past its right or bottom edge.
  fitInParent(id: string): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (!node?.parentId) return;
    node.x = Math.max(0, node.x);
    node.y = Math.max(0, node.y);
    this.growToFit(node);
  }

  private nodeDepth(node: GraphNode): number {
    let depth = 0;
    let current = node;
    while (current.parentId) {
      const parent = this.nodes.find(
        (candidate) => candidate.id === current.parentId,
      );
      if (!parent) break;
      depth++;
      current = parent;
    }
    return depth;
  }

  resizeNode(id: string, w: number, h: number): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (node) {
      node.w = Math.max(MIN_NODE_WIDTH, w);
      node.h = Math.max(MIN_NODE_HEIGHT, h);
    }
  }

  // Resizes a block from the edges being dragged, by the pointer's travel
  // since the gesture started at the start box. Moving the left or top edge
  // moves the block, and the blocks inside it keep their place on the
  // canvas. A block keeps its minimum size and still holds its blocks.
  resizeFrom(
    id: string,
    start: { x: number; y: number; w: number; h: number },
    edges: ResizeEdges,
    dx: number,
    dy: number,
  ): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (!node) return;
    const children = this.nodes.filter((child) => child.parentId === id);
    const span = (
      from: number,
      size: number,
      delta: number,
      low: boolean | undefined,
      high: boolean | undefined,
      min: number,
      offset: number,
      extents: [number, number][],
    ): [number, number] => {
      let first = from + (low ? delta : 0);
      let last = from + size + (high ? delta : 0);
      if (low) first = Math.min(first, last - min);
      if (high) last = Math.max(last, first + min);
      for (const [begin, end] of extents) {
        if (low) first = Math.min(first, offset + begin);
        if (high) last = Math.max(last, offset + end);
      }
      return [first, last];
    };
    const [left, right] = span(
      start.x,
      start.w,
      dx,
      edges.left,
      edges.right,
      MIN_NODE_WIDTH,
      node.x,
      children.map((child) => [child.x, child.x + child.w]),
    );
    const [top, bottom] = span(
      start.y,
      start.h,
      dy,
      edges.top,
      edges.bottom,
      MIN_NODE_HEIGHT,
      node.y,
      children.map((child) => [child.y, child.y + child.h]),
    );
    for (const child of children) {
      child.x -= left - node.x;
      child.y -= top - node.y;
    }
    node.x = left;
    node.y = top;
    node.w = right - left;
    node.h = bottom - top;
  }

  rename(id: string, name: string): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (node) node.name = name;
  }

  assignProjectFolder(id: string, folder: string): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (!node || !folder) return;
    node.path = folder;
    const segments = folder.split("/").filter(Boolean);
    if (segments.length > 0)
      node.name = segments[segments.length - 1] ?? node.name;
  }

  setPath(id: string, path: string): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (node) node.path = path;
  }

  setRepository(id: string, repository: string): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (node) node.repository = repository;
  }

  setSecretKey(id: string, secretKey: string): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (node) node.secretKey = secretKey;
  }

  setAuthenticated(id: string, authenticated: boolean): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (node) node.authenticated = authenticated;
  }

  // Appends an action to the GitHub block's sequence: the first becomes the
  // starting point and each later one follows the previous with an arrow.
  // The block and its ancestors grow so the sequence stays inside them.
  addAction(githubId: string, action: GitAction): GraphNode | undefined {
    const github = this.nodes.find((node) => node.id === githubId);
    if (github?.type !== "github") return undefined;
    const sequence = this.actionSequence(githubId);
    const previous = sequence.at(-1);
    const label =
      GIT_ACTIONS.find((candidate) => candidate.action === action)?.label ??
      action;
    const node: GraphNode = {
      id: crypto.randomUUID(),
      type: "action",
      action,
      name: label,
      x: ACTION_INSET,
      y: previous ? previous.y + previous.h + ACTION_GAP : ACTION_TOP,
      w: DEFAULT_NODE_WIDTH,
      h: DEFAULT_NODE_HEIGHT,
      parentId: githubId,
      start: previous === undefined,
      // Read issue starts on 0, the next available issue.
      ...(action === "issue" ? { issue: 0 } : {}),
    };
    this.nodes.push(node);
    const added = this.nodes.at(-1) ?? node;
    if (previous) this.connect(previous.id, added.id);
    this.growToFit(added);
    this.select(added.id);
    return added;
  }

  // The GitHub block's actions from its starting action along their arrows,
  // followed by actions not reachable that way in their creation order.
  actionSequence(githubId: string): GraphNode[] {
    const actions = this.nodes.filter(
      (node) => node.parentId === githubId && node.type === "action",
    );
    const ordered: GraphNode[] = [];
    let current = actions.find((node) => node.start);
    while (current && !ordered.includes(current)) {
      ordered.push(current);
      const from = current.id;
      const next = this.edges.find(
        (edge) =>
          edge.from === from &&
          actions.some((candidate) => candidate.id === edge.to),
      );
      current = actions.find((candidate) => candidate.id === next?.to);
    }
    return [...ordered, ...actions.filter((node) => !ordered.includes(node))];
  }

  private growToFit(child: GraphNode): void {
    let node = child;
    let parent = this.nodes.find((candidate) => candidate.id === node.parentId);
    while (parent) {
      parent.w = Math.max(parent.w, node.x + node.w + ACTION_INSET);
      parent.h = Math.max(parent.h, node.y + node.h + ACTION_INSET);
      node = parent;
      parent = this.nodes.find((candidate) => candidate.id === node.parentId);
    }
  }

  // Named outputs a block chooses between; empty for blocks with one output.
  outputPorts(id: string): string[] {
    return outputPortsOf(this.nodes.find((candidate) => candidate.id === id));
  }

  // The output an arrow takes: its own choice, or its source's first port.
  portOf(edge: GraphEdge): string | undefined {
    return edge.fromPort ?? this.outputPorts(edge.from)[0];
  }

  outgoingEdges(id: string): GraphEdge[] {
    return this.edges.filter((edge) => edge.from === id);
  }

  setEdgePort(edgeId: string, port: string): void {
    const edge = this.edges.find((candidate) => candidate.id === edgeId);
    if (edge) edge.fromPort = port;
  }

  addCase(id: string): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (!node?.cases) return;
    node.cases.push({ name: `case-${node.cases.length + 1}`, expression: "" });
  }

  // Renaming a case keeps the arrows that took it on the renamed route.
  setCase(
    id: string,
    index: number,
    field: keyof RouteCase,
    value: string,
  ): void {
    const routeCase = this.nodes.find((candidate) => candidate.id === id)
      ?.cases?.[index];
    if (!routeCase) return;
    if (field === "name") {
      for (const edge of this.outgoingEdges(id))
        if (edge.fromPort === routeCase.name) edge.fromPort = value;
      for (const loop of this.nodes)
        if (loop.untilNode === id && loop.untilPort === routeCase.name)
          loop.untilPort = value;
    }
    routeCase[field] = value;
  }

  // Arrows that took a removed case fall back to the default route.
  removeCase(id: string, index: number): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    const removed = node?.cases?.[index];
    if (!node?.cases || !removed) return;
    node.cases.splice(index, 1);
    for (const edge of this.outgoingEdges(id))
      if (edge.fromPort === removed.name) edge.fromPort = "default";
  }

  setCommand(id: string, command: string): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (node) node.command = command;
  }

  setSchema(id: string, schema: string): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (node) node.schema = schema;
  }

  setAgentField(id: string, field: AgentField, value: string): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (node) node[field] = value;
  }

  setContinueSession(id: string, continueSession: boolean): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (node) node.continueSession = continueSession;
  }

  // An empty field clears the setting so the backend default applies.
  setAgentNumber(id: string, field: AgentNumber, value: string): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (node) node[field] = value === "" ? undefined : Number(value);
  }

  // An empty field means 0, the next available issue.
  setActionIssue(id: string, value: string): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (node) node.issue = value === "" ? 0 : Number(value);
  }

  // Keeps blank entries while typing; the backend skips them.
  setActionIgnoreLabels(id: string, value: string): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (node)
      node.ignoreLabels =
        value === "" ? [] : value.split(",").map((label) => label.trim());
  }

  setActionField(id: string, field: ActionField, value: string): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (node) node[field] = value;
  }

  // Replaces the statuses shown on the canvas with a run's steps.
  showRun(steps: readonly RunStepStatus[], runId: string | null = null): void {
    this.runId = runId;
    this.runStatuses = Object.fromEntries(
      steps.map((step) => [step.nodeId, step.status]),
    );
    const iterations: Record<string, number> = {};
    for (const step of steps)
      if (step.loop && step.iteration)
        iterations[step.loop] = Math.max(
          iterations[step.loop] ?? 0,
          step.iteration,
        );
    this.runIterations = iterations;
  }

  // The iteration a loop of the shown run has reached.
  iterationOf(id: string): number | undefined {
    return this.runIterations[id];
  }

  // Every block inside the loop with outputs to choose from, and each output,
  // including the schema checks inside its agents.
  loopExits(id: string): LoopExit[] {
    const inside = (node: GraphNode) =>
      node.parentId === id ||
      (node.type === "jsonschema" &&
        this.nodes.some(
          (agent) => agent.id === node.parentId && agent.parentId === id,
        ));
    return this.nodes.filter(inside).flatMap((node) =>
      this.outputPorts(node.id).map((port) => ({
        nodeId: node.id,
        port,
        label: `${node.name} takes ${port}`,
      })),
    );
  }

  // Sets the exit from "<block id>:<output>", or clears it with "".
  setLoopExit(id: string, value: string): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (!node) return;
    const separator = value.lastIndexOf(":");
    node.untilNode = separator > 0 ? value.slice(0, separator) : undefined;
    node.untilPort = separator > 0 ? value.slice(separator + 1) : undefined;
  }

  // Agents, commands and loops can run where exclusive branches join.
  canWaitForAny(id: string): boolean {
    const type = this.nodes.find((node) => node.id === id)?.type;
    return type === "agent" || type === "command" || type === "loop";
  }

  setWaitForAny(id: string, waitForAny: boolean): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (node) node.waitForAny = waitForAny || undefined;
  }

  setMaxIterations(id: string, value: string): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (node) node.maxIterations = value === "" ? undefined : Number(value);
  }

  // Whether the shown run has a step, and so a log, of the block's own.
  hasLog(id: string): boolean {
    return this.runId !== null && this.runStatuses[id] !== undefined;
  }

  openLog(id: string | null): void {
    this.logNodeId = id;
  }

  // A block's own step status, or for a block without a step of its own the
  // roll-up of its children: failed wins, then any work in progress.
  statusOf(id: string): string | undefined {
    const own = this.runStatuses[id];
    if (own) return own;
    const statuses = this.nodes
      .filter((node) => node.parentId === id)
      .map((node) => this.statusOf(node.id))
      .filter((status): status is string => status !== undefined);
    if (statuses.length === 0) return undefined;
    if (statuses.includes("failed")) return "failed";
    if (statuses.every((status) => status === statuses[0])) return statuses[0];
    return "running";
  }

  // The worst problem a block has: an error, a warning, or none.
  severityOf(id: string): "error" | "warning" | undefined {
    const severities = this.problems
      .filter((problem) => problem.nodeId === id)
      .map((problem) => problem.severity);
    if (severities.includes("error")) return "error";
    if (severities.includes("warning")) return "warning";
    return undefined;
  }

  get problemCounts(): { errors: number; warnings: number } {
    const count = (severity: string) =>
      this.problems.filter((problem) => problem.severity === severity).length;
    return { errors: count("error"), warnings: count("warning") };
  }

  setAppId(id: string, appId: string): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (node) node.appId = appId;
  }

  setPrivateKeyPath(id: string, privateKeyPath: string): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (node) node.privateKeyPath = privateKeyPath;
  }
}
