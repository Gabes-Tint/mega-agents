// Which workflow the address bar points at, and the address a workflow has.
//
// Every saved workflow has its own URL, so it can be bookmarked, shared and
// reopened by link, and the browser's Back button walks between workflows.
// The mapping is pure so the workspace only has to follow what it returns.

// Where saved workflows live. The name in the path is the same identifier
// the workflow file and the workflow API use (internal/app/workflow_store.go),
// so no second slug rule is introduced here.
const WORKFLOW_PREFIX = "/workflows/";

export type Route =
  // The unsaved graph in progress, at "/".
  | { kind: "scratch" }
  // The list of saved workflows and the schedules they run on, at
  // "/workflows".
  | { kind: "schedules" }
  // A saved workflow, at /workflows/<name>. The name is not checked against
  // the store: the backend answers whether it exists.
  | { kind: "workflow"; name: string }
  // Anything else, such as a path the editor has no page for.
  | { kind: "unknown"; path: string };

// Where the list of workflows and their schedules is.
export const SCHEDULES_PATH = "/workflows";

// The address the named workflow is opened at.
export function workflowPath(name: string): string {
  return WORKFLOW_PREFIX + encodeURIComponent(name);
}

export function parseRoute(path: string): Route {
  // A trailing slash names the same page, except at the root.
  const trimmed = path.endsWith("/") ? path.slice(0, -1) : path;
  if (trimmed === "") return { kind: "scratch" };
  if (trimmed === SCHEDULES_PATH) return { kind: "schedules" };
  if (!trimmed.startsWith(WORKFLOW_PREFIX)) return { kind: "unknown", path };
  const segment = trimmed.slice(WORKFLOW_PREFIX.length);
  if (segment === "" || segment.includes("/")) return { kind: "unknown", path };
  try {
    return { kind: "workflow", name: decodeURIComponent(segment) };
  } catch {
    // A malformed escape names no workflow.
    return { kind: "unknown", path };
  }
}
