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
  path?: string;
}

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

  addNode(type: NodeType, x: number, y: number): GraphNode {
    const count = this.nodes.filter((node) => node.type === type).length + 1;
    const node: GraphNode = {
      id: crypto.randomUUID(),
      type,
      name: `${paletteLabel(type)} ${count}`,
      x,
      y,
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
      node.x = x;
      node.y = y;
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
