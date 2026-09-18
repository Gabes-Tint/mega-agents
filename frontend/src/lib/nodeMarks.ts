import {
  outputPortsOf,
  type GraphEdge,
  type GraphNode,
  type NodeType,
} from "./graph.svelte.js";

// The marks a block's header carries beside its name: where a run starts,
// where it can finish, where a run stops to be looked at, and how the block
// fared in the run being shown.
export type MarkKind =
  | "start"
  | "end"
  | "breakpoint"
  | "running"
  | "paused"
  | "succeeded"
  | "failed"
  | "skipped";

// What each mark says, as its tooltip and as the block's description, in the
// order a header carries them: what the block itself is, then what the run
// being shown made of it.
export const MARK_LABELS: Record<MarkKind, string> = {
  start: "Starting point",
  end: "Ending block",
  breakpoint: "Breakpoint",
  running: "Running",
  paused: "Paused at a breakpoint",
  succeeded: "Succeeded",
  failed: "Failed",
  skipped: "Skipped",
};

// The mark for a block's state in the run being shown. A block no run
// covers, or one still waiting its turn, carries none: the canvas stays
// quiet until the run reaches it.
export function statusMark(status: string | undefined): MarkKind | undefined {
  return status === "running" ||
    status === "paused" ||
    status === "succeeded" ||
    status === "failed" ||
    status === "skipped"
    ? status
    : undefined;
}

// The blocks a run executes once a starting point reaches them, as the
// planner lists them. Containers hold work but do none of their own.
const EXECUTABLE: readonly NodeType[] = [
  "action",
  "agent",
  "jsonschema",
  "router",
  "command",
  "loop",
];

type Index = Map<string, GraphNode>;

// The agent whose reply a schema check sitting inside it reads, or nothing.
function checkedAgent(node: GraphNode, index: Index): GraphNode | undefined {
  const parent = node.parentId ? index.get(node.parentId) : undefined;
  return node.type === "jsonschema" && parent?.type === "agent"
    ? parent
    : undefined;
}

// The loop a block repeats in, or nothing. A schema check repeats with the
// agent it checks.
function loopOf(node: GraphNode, index: Index): GraphNode | undefined {
  const owner = checkedAgent(node, index) ?? node;
  const parent = owner.parentId ? index.get(owner.parentId) : undefined;
  return parent?.type === "loop" ? parent : undefined;
}

// The schema checks a block holds; an agent hands them its reply without an
// arrow, so they carry the flow on from it.
function checksIn(id: string, nodes: readonly GraphNode[]): GraphNode[] {
  return nodes.filter(
    (node) => node.parentId === id && node.type === "jsonschema",
  );
}

// The blocks a run reaches from the flow's starting points, mirroring the
// planner: an agent flagged as the starting point, or the starting action of
// a GitHub block flagged as one, and everything their arrows lead to. A
// block flagged inside a loop starts nothing, which the problems panel
// reports as an error.
export function reachableNodeIds(
  nodes: readonly GraphNode[],
  edges: readonly GraphEdge[],
): Set<string> {
  const index: Index = new Map(nodes.map((node) => [node.id, node]));
  const reached = new Set<string>();
  const include = (id: string) => {
    const node = index.get(id);
    if (!node || reached.has(id) || !EXECUTABLE.includes(node.type)) return;
    reached.add(id);
    // A loop repeats every block inside it, so they all run without an arrow
    // reaching them from outside.
    if (node.type === "loop")
      for (const child of nodes)
        if (child.parentId === id && EXECUTABLE.includes(child.type)) {
          reached.add(child.id);
          for (const check of checksIn(child.id, nodes)) reached.add(check.id);
        }
    if (node.type === "agent")
      for (const check of checksIn(id, nodes)) include(check.id);
    for (const edge of edges) if (edge.from === id) include(edge.to);
  };
  for (const node of nodes) {
    if (!node.start || loopOf(node, index)) continue;
    if (node.type === "agent") include(node.id);
    if (node.type === "github") {
      const first = nodes.find(
        (child) =>
          child.parentId === node.id && child.type === "action" && child.start,
      );
      if (first) include(first.id);
    }
  }
  return reached;
}

// Whether a run that reaches the block can stop there: one of its outputs
// carries no arrow away. A block with a single output ends the flow when no
// arrow leaves it at all, and an agent holding a schema check hands its
// reply to that check instead of ending.
function hasFreeOutput(
  node: GraphNode,
  nodes: readonly GraphNode[],
  edges: readonly GraphEdge[],
): boolean {
  const ports = outputPortsOf(node);
  const taken = new Set(
    edges
      .filter((edge) => edge.from === node.id)
      .map((edge) => edge.fromPort ?? ports[0]),
  );
  if (ports.length > 0) return ports.some((port) => !taken.has(port));
  return taken.size === 0 && checksIn(node.id, nodes).length === 0;
}

// The blocks a run of the flow can finish at: the ones it reaches that do
// work of their own and leave one of their outputs without an arrow. The
// blocks a loop repeats are never among them — the loop takes the flow on
// through its own done and exhausted outputs.
export function endingNodeIds(
  nodes: readonly GraphNode[],
  edges: readonly GraphEdge[],
): Set<string> {
  const index: Index = new Map(nodes.map((node) => [node.id, node]));
  const reached = reachableNodeIds(nodes, edges);
  return new Set(
    nodes
      .filter(
        (node) =>
          reached.has(node.id) &&
          !loopOf(node, index) &&
          hasFreeOutput(node, nodes, edges),
      )
      .map((node) => node.id),
  );
}
