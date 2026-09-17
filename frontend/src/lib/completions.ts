import { labelFor, type GraphEdge, type GraphNode } from "./graph.svelte.js";

// Languages a property field is edited in. "text" is a prompt: placeholders
// but no syntax of its own.
export type EditorLanguage = "shell" | "json" | "text";

// One variable the field's completion popup offers.
export interface VariableCompletion {
  label: string;
  detail: string;
  kind: "workspace" | "result" | "env";
}

// The graph a completion list is derived from: the nodes and arrows only, so
// the logic stays testable without a live store.
export interface CompletionGraph {
  nodes: readonly GraphNode[];
  edges: readonly GraphEdge[];
}

// Git actions that hand a workspace to whatever follows them, matching the
// planner's worktree action and the ones that need a workspace to run.
const WORKSPACE_ACTIONS = new Set([
  "worktree",
  "rebase",
  "commit",
  "push",
  "pullrequest",
]);

// Blocks that pass the workspace they receive on to the blocks after them.
const WORKSPACE_CARRIERS = new Set(["agent", "command", "loop", "router"]);

// The workspace fields a template may name, in the order the popup lists
// them, with what each one holds.
const WORKSPACE_FIELDS: readonly { field: string; detail: string }[] = [
  { field: "path", detail: "the worktree's folder on this machine" },
  { field: "branch", detail: "the branch the worktree checked out" },
  { field: "base", detail: "the branch the worktree started from" },
  { field: "repository", detail: "the repository the worktree came from" },
];

// How a block's name is spelled inside {{results.<block>}}: the backend's
// own slug, so a completion names what the run resolves.
export function blockIdentifier(name: string): string {
  let slug = "";
  for (const char of name.toLowerCase()) {
    if (/[a-z0-9]/.test(char)) slug += char;
    else if (" -_/.".includes(char) || !/\p{L}/u.test(char)) slug += "-";
  }
  return slug || "node";
}

function nodeById(graph: CompletionGraph, id: string): GraphNode | undefined {
  return graph.nodes.find((candidate) => candidate.id === id);
}

// The agent a schema check sits in and checks the reply of.
function checkedAgent(
  graph: CompletionGraph,
  node: GraphNode,
): GraphNode | undefined {
  if (node.type !== "jsonschema") return undefined;
  const parent = node.parentId ? nodeById(graph, node.parentId) : undefined;
  return parent?.type === "agent" ? parent : undefined;
}

// The arrows a block receives, mirroring the planner: a schema check takes
// its agent's reply, and a block inside a loop that nothing beside it feeds
// receives the arrows into the loop on every repeat.
function arrowsInto(graph: CompletionGraph, node: GraphNode): GraphEdge[] {
  const agent = checkedAgent(graph, node);
  if (agent)
    return [{ id: `${agent.id}->${node.id}`, from: agent.id, to: node.id }];
  const arrows = graph.edges.filter((edge) => edge.to === node.id);
  if (arrows.length > 0) return arrows;
  const owner = agent ?? node;
  const parent = owner.parentId ? nodeById(graph, owner.parentId) : undefined;
  return parent?.type === "loop" ? arrowsInto(graph, parent) : [];
}

// Whether a block hands a workspace to the blocks after it: the Git actions
// that prepare or keep one, and the blocks that receive one and pass it on.
function passesWorkspace(
  graph: CompletionGraph,
  id: string,
  visiting: Set<string>,
): boolean {
  const node = nodeById(graph, id);
  if (!node) return false;
  if (node.type === "action")
    return node.action !== undefined && WORKSPACE_ACTIONS.has(node.action);
  if (!WORKSPACE_CARRIERS.has(node.type) || visiting.has(id)) return false;
  visiting.add(id);
  return arrowsInto(graph, node).some((edge) =>
    passesWorkspace(graph, edge.from, visiting),
  );
}

// Whether an arrow from the block carries a value a template can name, as
// opposed to a workspace or nothing but an ordering.
function carriesValue(node: GraphNode): boolean {
  if (node.type === "action")
    return node.action === "pullrequest" || node.action === "issue";
  return (
    node.type === "agent" ||
    node.type === "command" ||
    node.type === "loop" ||
    node.type === "router" ||
    node.type === "jsonschema"
  );
}

// The variables that reach the block, as the completion popup offers them:
// the workspace fields when a worktree reaches it, the values of the blocks
// connected to it, and in a shell field the same workspace as environment
// variables.
export function completionsFor(
  graph: CompletionGraph,
  nodeId: string,
  language: EditorLanguage,
): VariableCompletion[] {
  const node = nodeById(graph, nodeId);
  if (!node) return [];
  const arrows = arrowsInto(graph, node);
  const workspace = arrows.some((edge) =>
    passesWorkspace(graph, edge.from, new Set()),
  );
  // Results are keyed by identifier the way the backend keys them, so two
  // blocks whose names spell the same way collapse into one entry.
  const results = new Map<string, GraphNode>();
  for (const edge of arrows) {
    const source = nodeById(graph, edge.from);
    if (source && carriesValue(source))
      results.set(blockIdentifier(source.name), source);
  }
  const completions: VariableCompletion[] = [];
  if (workspace)
    for (const { field, detail } of WORKSPACE_FIELDS)
      completions.push({
        label: `{{workspace.${field}}}`,
        detail,
        kind: "workspace",
      });
  // {{result}} needs one connected block, or any of them for a block that
  // runs on whichever arrow arrives first.
  const only = [...results.values()][0];
  if (only && (results.size === 1 || node.waitForAny))
    completions.push({
      label: "{{result}}",
      detail: `the value from ${only.name}`,
      kind: "result",
    });
  for (const [identifier, source] of results)
    completions.push({
      label: `{{results.${identifier}}}`,
      detail: `the value from ${source.name}, ${article(labelFor(source.type))}`,
      kind: "result",
    });
  if (language === "shell" && workspace)
    for (const { field, detail } of WORKSPACE_FIELDS)
      completions.push({
        label: `$MEGA_AGENTS_WORKSPACE_${field.toUpperCase()}`,
        detail,
        kind: "env",
      });
  return completions;
}

function article(label: string): string {
  return `${/^[aeiou]/i.test(label) ? "an" : "a"} ${label}`;
}
