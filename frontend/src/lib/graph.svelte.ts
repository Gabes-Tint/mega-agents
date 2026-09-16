export type NodeType =
  "agent" | "tool" | "project" | "gatebase" | "github" | "gitlab" | "githubapp";

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
  appId?: string;
  privateKeyPath?: string;
}

export const DEFAULT_NODE_WIDTH = 160;
export const DEFAULT_NODE_HEIGHT = 64;
export const MIN_NODE_WIDTH = 120;
export const MIN_NODE_HEIGHT = 48;

export const PALETTE: readonly PaletteItem[] = [
  { type: "agent", label: "Agent" },
  { type: "tool", label: "Tool" },
  { type: "project", label: "Project" },
  { type: "gatebase", label: "GateBase" },
  { type: "github", label: "GitHub" },
  { type: "gitlab", label: "GitLab" },
  { type: "githubapp", label: "GitHub App" },
];

// Boxes that can host nested children one level deep.
const CONTAINER_TYPES: readonly NodeType[] = ["project", "gatebase", "github"];

export function isContainerType(type: NodeType): boolean {
  return CONTAINER_TYPES.includes(type);
}

// General containers accept every component except the GitHub App, which
// nests only inside a GitHub controller; a GitHub hosts nothing else.
export function canHostChild(
  parentType: NodeType,
  childType: NodeType,
): boolean {
  if (childType === "githubapp") return parentType === "github";
  if (parentType === "github") return false;
  return isContainerType(parentType);
}

function paletteLabel(type: NodeType): string {
  return PALETTE.find((item) => item.type === type)?.label ?? type;
}

export class GraphStore {
  nodes = $state<GraphNode[]>([]);
  edges = $state<GraphEdge[]>([]);
  selectedId = $state<string | null>(null);
  connecting = $state(false);
  connectFromId = $state<string | null>(null);

  get selected(): GraphNode | undefined {
    return this.nodes.find((node) => node.id === this.selectedId);
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
    this.nodes.push(node);
    this.selectedId = node.id;
    return node;
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
      // Nesting is one level deep: neither the host nor the incoming box may
      // already participate in a parent-child relationship, and the child
      // type must be allowed inside this container.
      parent.parentId !== undefined ||
      !canHostChild(parent.type, node.type) ||
      this.hasChildren(id)
    )
      return;
    // Children live in their parent's coordinate space, so the content-space
    // drop position converts to parent-relative coordinates here. A flagged
    // start point cannot cross containers without breaking the single-start
    // rule, so it arrives unflagged.
    node.parentId = parentId;
    node.x = absoluteX - parent.x;
    node.y = absoluteY - parent.y;
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
    if ((from.parentId ?? null) !== (to.parentId ?? null)) return false;
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

  // Topmost (last added) top-level container whose box contains the content
  // point; only top-level containers can host children because nesting is
  // one level deep.
  containerAt(x: number, y: number): GraphNode | undefined {
    for (let index = this.nodes.length - 1; index >= 0; index--) {
      const node = this.nodes[index];
      if (
        node &&
        isContainerType(node.type) &&
        node.parentId === undefined &&
        x >= node.x &&
        x < node.x + node.w &&
        y >= node.y &&
        y < node.y + node.h
      )
        return node;
    }
    return undefined;
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

  setAppId(id: string, appId: string): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (node) node.appId = appId;
  }

  setPrivateKeyPath(id: string, privateKeyPath: string): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (node) node.privateKeyPath = privateKeyPath;
  }
}
