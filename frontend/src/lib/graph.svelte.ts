export type NodeType =
  "agent" | "project" | "github" | "gitlab" | "githubapp" | "action";

export type GitAction = "fetch" | "worktree" | "rebase";

export type ActionField = "branch" | "base" | "worktreePath" | "onto";

// Git actions a GitHub block runs as its own internal sequence. They are
// added from the block's properties rather than dragged from the palette.
export const GIT_ACTIONS: readonly { action: GitAction; label: string }[] = [
  { action: "fetch", label: "Fetch" },
  { action: "worktree", label: "Create worktree" },
  { action: "rebase", label: "Rebase" },
];

export type AgentField =
  "backend" | "model" | "effort" | "prompt" | "outputSchema";

export type AgentNumber = "retries" | "timeoutMinutes";

// Coding-agent CLIs an Agent block can run, as the backend names them.
export const AGENT_BACKENDS: readonly { backend: string; label: string }[] = [
  { backend: "claude", label: "Claude Code" },
  { backend: "codex", label: "Codex" },
  { backend: "grok", label: "Grok" },
  { backend: "opencode", label: "OpenCode" },
];

export interface RunStepStatus {
  nodeId: string;
  status: string;
}

// The canvas root as a pseudo target so the containment matrix covers
// top-level drops in the same table as nested drops.
export type DropTarget = NodeType | "root";

export interface GraphEdge {
  id: string;
  from: string;
  to: string;
}

export interface PaletteItem {
  type: NodeType;
  label: string;
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
}

export const DEFAULT_NODE_WIDTH = 160;
export const DEFAULT_NODE_HEIGHT = 64;
export const MIN_NODE_WIDTH = 120;
export const MIN_NODE_HEIGHT = 48;

// Layout of a GitHub block's action sequence: below the block header, one
// action under the other with room for the arrow between them.
const ACTION_INSET = 12;
const ACTION_TOP = 40;
const ACTION_GAP = 24;

export const PALETTE: readonly PaletteItem[] = [
  { type: "agent", label: "Agent" },
  { type: "project", label: "Project" },
  { type: "github", label: "GitHub" },
  { type: "gitlab", label: "GitLab" },
  { type: "githubapp", label: "GitHub App" },
];

// Single source of truth for where a block may go, agreed with the product
// owner. Keys are the block being dropped; the arrays are every target that
// accepts it, including the canvas root as a pseudo target. Anything absent
// from a target's list is rejected.
export const CONTAINMENT_MATRIX: Record<NodeType, readonly DropTarget[]> = {
  agent: ["project", "agent"],
  project: ["root", "project"],
  github: ["project"],
  gitlab: ["project"],
  githubapp: ["github"],
  action: ["github"],
};

// Boxes that can host children: every target that appears as a parent in
// the containment matrix.
export const CONTAINER_TYPES: readonly NodeType[] = (
  ["agent", "project", "github", "gitlab", "githubapp", "action"] as NodeType[]
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
};

// Human-readable list of the targets a block may be dropped into, used to
// explain rejected drops.
export function allowedTargetsLabel(type: NodeType): string {
  const labels = CONTAINMENT_MATRIX[type]
    .filter((target): target is NodeType => target !== "root")
    .map((target) => {
      if (target === type) return `another ${TARGET_LABELS[type]}`;
      return `a ${TARGET_LABELS[target]} box`;
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

export class GraphStore {
  nodes = $state<GraphNode[]>([]);
  edges = $state<GraphEdge[]>([]);
  selectedId = $state<string | null>(null);
  connecting = $state(false);
  connectFromId = $state<string | null>(null);
  // Component type currently dragged from the palette; dataTransfer.getData
  // is protected while a drag is in flight, so the type travels through the
  // store to power live dragover validation on the canvas.
  draggingType = $state<NodeType | null>(null);
  runStatuses = $state<Record<string, string>>({});
  // The run whose statuses the canvas shows, and the block whose log of that
  // run is open.
  runId = $state<string | null>(null);
  logNodeId = $state<string | null>(null);

  get selected(): GraphNode | undefined {
    return this.nodes.find((node) => node.id === this.selectedId);
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
      w: DEFAULT_NODE_WIDTH,
      h: DEFAULT_NODE_HEIGHT,
      parentId: effectiveParent,
    };
    if (type === "agent") node.backend = "claude";
    this.nodes.push(node);
    this.selectedId = node.id;
    // The store holds a reactive proxy of the node; hand that back so later
    // changes through the store are visible to the caller.
    return this.nodes.at(-1) ?? node;
  }

  select(id: string | null): void {
    this.selectedId = id;
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

  startConnect(id: string): void {
    this.connectFromId = id;
    this.connecting = true;
  }

  cancelConnect(): void {
    this.connecting = false;
    this.connectFromId = null;
  }

  // Boxes connect through arrows only within their own level: two top-level
  // boxes or two children of the same project. Level membership is validated
  // at creation only; edges follow their endpoints afterward. Any number of
  // arrows may leave or enter a box, but a pair is connected at most once in
  // either direction.
  connect(fromId: string, toId: string): boolean {
    const from = this.nodes.find((candidate) => candidate.id === fromId);
    const to = this.nodes.find((candidate) => candidate.id === toId);
    if (!from || !to || fromId === toId) return false;
    // A Git action's output also leaves its GitHub block toward the blocks
    // beside that GitHub block, such as an agent that takes the workspace.
    const provider =
      from.type === "action"
        ? this.nodes.find((candidate) => candidate.id === from.parentId)
        : undefined;
    const sameLevel = (from.parentId ?? null) === (to.parentId ?? null);
    const besideProvider =
      provider !== undefined && provider.parentId === to.parentId;
    if (!sameLevel && !besideProvider) return false;
    if (
      this.edges.some(
        (edge) =>
          (edge.from === fromId && edge.to === toId) ||
          (edge.from === toId && edge.to === fromId),
      )
    )
      return false;
    this.edges.push({ id: crypto.randomUUID(), from: fromId, to: toId });
    return true;
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
    };
    this.nodes.push(node);
    const added = this.nodes.at(-1) ?? node;
    if (previous) this.connect(previous.id, added.id);
    this.growToFit(added);
    this.selectedId = added.id;
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

  setAgentField(id: string, field: AgentField, value: string): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (node) node[field] = value;
  }

  // An empty field clears the setting so the backend default applies.
  setAgentNumber(id: string, field: AgentNumber, value: string): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (node) node[field] = value === "" ? undefined : Number(value);
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

  setAppId(id: string, appId: string): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (node) node.appId = appId;
  }

  setPrivateKeyPath(id: string, privateKeyPath: string): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (node) node.privateKeyPath = privateKeyPath;
  }
}
