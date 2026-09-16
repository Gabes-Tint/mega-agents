export type NodeType = "agent" | "tool" | "project";

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
  path?: string;
}

export const DEFAULT_NODE_WIDTH = 160;
export const DEFAULT_NODE_HEIGHT = 64;
export const MIN_NODE_WIDTH = 120;
export const MIN_NODE_HEIGHT = 48;

export const PALETTE: readonly PaletteItem[] = [
  { type: "agent", label: "Agent" },
  { type: "tool", label: "Tool" },
  { type: "project", label: "Project" },
];

function paletteLabel(type: NodeType): string {
  return PALETTE.find((item) => item.type === type)?.label ?? type;
}

export class GraphStore {
  nodes = $state<GraphNode[]>([]);
  selectedId = $state<string | null>(null);

  get selected(): GraphNode | undefined {
    return this.nodes.find((node) => node.id === this.selectedId);
  }

  addNode(type: NodeType, x: number, y: number, parentId?: string): GraphNode {
    const count = this.nodes.filter((node) => node.type === type).length + 1;
    const node: GraphNode = {
      id: crypto.randomUUID(),
      type,
      name: `${paletteLabel(type)} ${count}`,
      x,
      y,
      w: DEFAULT_NODE_WIDTH,
      h: DEFAULT_NODE_HEIGHT,
      parentId,
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

  attachToProject(
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
      // already participate in a parent-child relationship.
      parent.parentId !== undefined ||
      this.hasChildren(id)
    )
      return;
    // Children live in their parent's coordinate space, so the content-space
    // drop position converts to parent-relative coordinates here.
    node.parentId = parentId;
    node.x = absoluteX - parent.x;
    node.y = absoluteY - parent.y;
  }

  detachNode(id: string, absoluteX: number, absoluteY: number): void {
    const node = this.nodes.find((candidate) => candidate.id === id);
    if (!node || node.parentId === undefined) return;
    node.parentId = undefined;
    node.x = absoluteX;
    node.y = absoluteY;
  }

  hasChildren(id: string): boolean {
    return this.nodes.some((candidate) => candidate.parentId === id);
  }

  // Topmost (last added) top-level project whose box contains the content
  // point; only top-level projects can host children because nesting is one
  // level deep.
  projectAt(x: number, y: number): GraphNode | undefined {
    for (let index = this.nodes.length - 1; index >= 0; index--) {
      const node = this.nodes[index];
      if (
        node &&
        node.type === "project" &&
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
}
