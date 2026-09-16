import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/svelte";
import { afterEach, describe, expect, test, vi } from "vitest";
import Workspace from "./Workspace.svelte";

afterEach(cleanup);

interface FakeDataTransfer {
  setData(type: string, value: string): void;
  getData(type: string): string;
  effectAllowed: string;
}

function makeDataTransfer(type = ""): FakeDataTransfer {
  const data = new Map<string, string>();
  return {
    setData: (name, value) => data.set(name, value),
    getData: (name) => data.get(name) ?? type,
    effectAllowed: "copy",
  };
}

function canvas() {
  return screen.getByRole("region", { name: "Graph canvas" });
}

// fireEvent does not propagate coordinates in this environment, so drag
// events that depend on clientX/clientY are dispatched manually. Svelte
// applies DOM updates in a microtask after dispatchEvent returns, so the
// helper yields a task tick before returning.
async function dispatchDragEvent(
  type: "dragstart" | "drop" | "dragover" | "dragleave",
  target: Element,
  init: { dataTransfer?: FakeDataTransfer; clientX?: number; clientY?: number },
) {
  const event = new Event(type, { bubbles: true, cancelable: true });
  Object.defineProperty(event, "dataTransfer", { value: init.dataTransfer });
  Object.defineProperty(event, "clientX", { value: init.clientX });
  Object.defineProperty(event, "clientY", { value: init.clientY });
  target.dispatchEvent(event);
  await new Promise((resolve) => setTimeout(resolve, 0));
}

function dispatchDragStart(
  target: Element,
  init: { dataTransfer?: FakeDataTransfer; clientX?: number; clientY?: number },
) {
  dispatchDragEvent("dragstart", target, init);
}

function dispatchDrop(
  target: Element,
  init: { dataTransfer?: FakeDataTransfer; clientX?: number; clientY?: number },
) {
  dispatchDragEvent("drop", target, init);
}

function dispatchDragOver(
  target: Element,
  init: { dataTransfer?: FakeDataTransfer; clientX?: number; clientY?: number },
) {
  dispatchDragEvent("dragover", target, init);
}

async function dropComponent(label: string, clientX = 30, clientY = 20) {
  const dataTransfer = makeDataTransfer();
  await fireEvent.dragStart(screen.getByRole("button", { name: label }), {
    dataTransfer,
  });
  await fireEvent.dragOver(canvas(), {});
  await dispatchDrop(canvas(), {
    dataTransfer,
    clientX,
    clientY,
  });
}

describe("graph builder workspace", () => {
  test("downloads the workflow YAML from the toolbar", async () => {
    const fetchJson = () =>
      Promise.resolve(
        new Response("apiVersion: megaagents.dev/v1alpha1\n", {
          headers: {
            "content-type": "application/x-yaml",
            "content-disposition":
              'attachment; filename="issue-to-pull-request.yaml"',
          },
        }),
      );
    const fetchMock = vi.fn(fetchJson);
    vi.stubGlobal("fetch", fetchMock);
    const createObjectURL = vi.fn(() => "blob:yaml");
    const revokeObjectURL = vi.fn();
    vi.stubGlobal("URL", { createObjectURL, revokeObjectURL });
    const clicks: HTMLAnchorElement[] = [];
    const originalCreate = document.createElement.bind(document);
    vi.spyOn(document, "createElement").mockImplementation((tag) => {
      const element = originalCreate(tag);
      if (tag === "a") clicks.push(element as HTMLAnchorElement);
      return element;
    });
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Agent");
    await fireEvent.click(
      screen.getByRole("button", { name: "Download YAML" }),
    );

    const call = fetchMock.mock.calls.at(-1) as unknown as
      [string, RequestInit] | undefined;
    const request = call?.[1];
    expect(request?.method).toBe("POST");
    const body = JSON.parse(String(request?.body)) as {
      nodes: Array<{ name: string }>;
      edges: unknown[];
    };
    expect(body.nodes.map((node) => node.name)).toEqual([
      "Project 1",
      "Agent 1",
    ]);
    expect(body.edges).toEqual([]);
    await waitFor(() => expect(createObjectURL).toHaveBeenCalled());
    expect(clicks[0]?.download).toBe("issue-to-pull-request.yaml");
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  test("surfaces an export error from the backend", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () => new Response("unknown component type", { status: 400 }),
      ),
    );
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Agent");
    await fireEvent.click(
      screen.getByRole("button", { name: "Download YAML" }),
    );

    await waitFor(() =>
      expect(screen.getByText("unknown component type")).toBeInTheDocument(),
    );
    vi.unstubAllGlobals();
  });

  test("surfaces a backend outage while exporting", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => Promise.reject(new Error("offline"))),
    );
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Agent");
    await fireEvent.click(
      screen.getByRole("button", { name: "Download YAML" }),
    );

    await waitFor(() =>
      expect(screen.getByText("Cannot reach the backend")).toBeInTheDocument(),
    );
    vi.unstubAllGlobals();
  });

  test("lists the draggable components in the left palette", () => {
    render(Workspace);

    const palette = screen.getByRole("complementary", {
      name: "Component palette",
    });
    expect(palette).toContainElement(
      screen.getByRole("button", { name: "Agent" }),
    );
    expect(palette).toContainElement(
      screen.getByRole("button", { name: "Project" }),
    );
    expect(palette).toContainElement(
      screen.getByRole("button", { name: "GitHub" }),
    );
    expect(palette).toContainElement(
      screen.getByRole("button", { name: "GitLab" }),
    );
  });

  test("shows repository and secret key fields only for forge boxes", async () => {
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Agent");
    expect(screen.queryByLabelText("Repository")).not.toBeInTheDocument();

    await dropComponent("Project", 400, 400);
    await dropComponent("GitHub", 410, 410);

    expect(screen.getByText("Type: github")).toBeInTheDocument();
    expect(screen.getByLabelText("Repository")).toBeInTheDocument();
    expect(screen.getByLabelText("Secret key")).toBeInTheDocument();
    expect(screen.queryByLabelText("Path")).not.toBeInTheDocument();
  });

  test("captures repository and secret key values for a GitHub box", async () => {
    render(Workspace);

    await dropComponent("Project", 400, 400);
    await dropComponent("GitHub", 410, 410);

    await fireEvent.input(screen.getByLabelText("Repository"), {
      target: { value: "https://github.com/example/project" },
    });
    await fireEvent.input(screen.getByLabelText("Secret key"), {
      target: { value: "secret://github-bot" },
    });

    await dropComponent("Project", 700, 700);
    await fireEvent.click(screen.getByText("Project 2"));
    await fireEvent.click(screen.getByText("GitHub 1"));

    expect(screen.getByLabelText("Repository")).toHaveValue(
      "https://github.com/example/project",
    );
    expect(screen.getByLabelText("Secret key")).toHaveValue(
      "secret://github-bot",
    );
  });

  test("captures repository and secret key values for a GitLab box", async () => {
    render(Workspace);

    await dropComponent("Project", 400, 400);
    await dropComponent("GitLab", 410, 410);

    await fireEvent.input(screen.getByLabelText("Repository"), {
      target: { value: "https://gitlab.com/example/project" },
    });
    await fireEvent.input(screen.getByLabelText("Secret key"), {
      target: { value: "secret://gitlab-bot" },
    });

    await dropComponent("Project", 700, 700);
    await fireEvent.click(screen.getByText("Project 2"));
    await fireEvent.click(screen.getByText("GitLab 1"));

    expect(screen.getByLabelText("Repository")).toHaveValue(
      "https://gitlab.com/example/project",
    );
    expect(screen.getByLabelText("Secret key")).toHaveValue(
      "secret://gitlab-bot",
    );
  });

  test("rejects a GitHub App dropped outside a GitHub box", async () => {
    render(Workspace);

    await dropComponent("GitHub App", 30, 20);

    // A GitHub App cannot exist at the top level: it only lives inside a
    // GitHub box, so the drop is ignored.
    expect(
      screen.queryByRole("button", { name: "GitHub App 1" }),
    ).not.toBeInTheDocument();
  });

  test("explains why a rejected GitHub App drop disappears", async () => {
    render(Workspace);

    await dropComponent("GitHub App", 30, 20);

    expect(
      screen.getByText("A GitHub App can only be dropped inside a GitHub box."),
    ).toBeInTheDocument();

    // The hint clears on the next drop so it cannot go stale.
    await dropComponent("Project", 400, 400);
    expect(
      screen.queryByText(
        "A GitHub App can only be dropped inside a GitHub box.",
      ),
    ).not.toBeInTheDocument();
  });

  test("adds a node to the canvas when a palette component is dropped", async () => {
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Agent");

    expect(canvas()).toContainElement(screen.getByText("Agent 1"));
  });

  test("accounts for canvas scroll when placing a dropped component", async () => {
    render(Workspace);
    const scrolled = canvas() as HTMLElement;
    Object.defineProperty(scrolled, "scrollLeft", { get: () => 50 });
    Object.defineProperty(scrolled, "scrollTop", { get: () => 40 });

    await dropComponent("Project");
    await dropComponent("Agent");

    expect(screen.getByRole("button", { name: "Agent 1" })).toHaveStyle({
      left: "80px",
      top: "60px",
    });
  });

  test("selects the dropped node so its properties appear on the right", async () => {
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Agent");

    const properties = screen.getByRole("complementary", {
      name: "Node properties",
    });
    expect(properties).toContainElement(screen.getByText("Type: agent"));
    expect(screen.getByLabelText("Name")).toHaveValue("Agent 1");
  });

  test("ignores drops of unknown component types", async () => {
    render(Workspace);

    fireEvent.drop(canvas(), {
      dataTransfer: makeDataTransfer("nonsense"),
      clientX: 10,
      clientY: 10,
    });

    expect(
      screen.getByText("Drag components here to build your graph"),
    ).toBeInTheDocument();
    expect(screen.queryByText("Agent 1")).not.toBeInTheDocument();
  });

  test("stages the component type on the drag data when picked up", () => {
    render(Workspace);

    const dataTransfer = makeDataTransfer();
    fireEvent.dragStart(screen.getByRole("button", { name: "Agent" }), {
      dataTransfer,
    });

    expect(dataTransfer.getData("text/plain")).toBe("agent");
  });

  test("tolerates a drag start without drag data", () => {
    render(Workspace);

    expect(() =>
      fireEvent.dragStart(screen.getByRole("button", { name: "Project" }), {}),
    ).not.toThrow();
  });

  test("ignores drops without drag data", async () => {
    render(Workspace);

    await fireEvent.drop(canvas(), { clientX: 10, clientY: 10 });

    expect(
      screen.getByText("Drag components here to build your graph"),
    ).toBeInTheDocument();
  });

  test("moves an existing node when it is dragged to a new position", async () => {
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Agent");
    const node = screen.getByRole("button", { name: "Agent 1" });

    const dataTransfer = makeDataTransfer();
    await dispatchDragStart(node, { dataTransfer, clientX: 0, clientY: 0 });
    await dispatchDrop(canvas(), { dataTransfer, clientX: 100, clientY: 80 });

    expect(node).toHaveStyle({ left: "100px", top: "80px" });
    expect(canvas()).toContainElement(node);
  });

  test("accounts for canvas scroll when moving a node", async () => {
    render(Workspace);
    const scrolled = canvas() as HTMLElement;
    Object.defineProperty(scrolled, "scrollLeft", { get: () => 50 });
    Object.defineProperty(scrolled, "scrollTop", { get: () => 40 });

    await dropComponent("Project");
    await dropComponent("Agent");
    const node = screen.getByRole("button", { name: "Agent 1" });

    const dataTransfer = makeDataTransfer();
    await dispatchDragStart(node, { dataTransfer, clientX: 0, clientY: 0 });
    await dispatchDrop(scrolled, { dataTransfer, clientX: 100, clientY: 80 });

    expect(node).toHaveStyle({ left: "150px", top: "120px" });
  });

  test("keeps a node in place when drag data does not match the drop", async () => {
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Agent");
    const node = screen.getByRole("button", { name: "Agent 1" });

    await dispatchDragStart(node, {
      dataTransfer: makeDataTransfer(),
      clientX: 0,
      clientY: 0,
    });
    await dispatchDrop(canvas(), {
      dataTransfer: makeDataTransfer("node:other-node"),
      clientX: 100,
      clientY: 80,
    });

    expect(node).toHaveStyle({ left: "30px", top: "20px" });
  });

  test("tolerates dragging a node without drag data", async () => {
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Agent");
    const node = screen.getByRole("button", { name: "Agent 1" });

    expect(() => dispatchDragStart(node, {})).not.toThrow();

    await dispatchDrop(canvas(), {
      dataTransfer: makeDataTransfer(),
      clientX: 100,
      clientY: 80,
    });

    expect(node).toHaveStyle({ left: "30px", top: "20px" });
  });

  test("keeps a node in place when a drop references it without a drag", async () => {
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Agent");
    const node = screen.getByRole("button", { name: "Agent 1" });

    await dispatchDrop(canvas(), {
      dataTransfer: makeDataTransfer("node:stale"),
      clientX: 100,
      clientY: 80,
    });

    expect(node).toHaveStyle({ left: "30px", top: "20px" });
  });

  test("renders a resize handle on the dropped node", async () => {
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Agent");
    const node = screen.getByRole("button", { name: "Agent 1" });

    expect(node.querySelector(".resize-handle")).toBeInTheDocument();
  });

  test("resizes a node by dragging its handle", async () => {
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Agent");
    const node = screen.getByRole("button", { name: "Agent 1" });
    const handle = node.querySelector(".resize-handle");
    if (!handle) throw new Error("resize handle missing");

    await fireEvent.pointerDown(handle, {
      button: 0,
      pointerId: 1,
      clientX: 200,
      clientY: 100,
    });
    await fireEvent.pointerMove(window, { clientX: 250, clientY: 140 });
    await fireEvent.pointerUp(window);

    expect(node).toHaveStyle({ width: "210px", height: "104px" });
  });

  test("resizing keeps the box dimensions above their minimums", async () => {
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Agent");
    const node = screen.getByRole("button", { name: "Agent 1" });
    const handle = node.querySelector(".resize-handle");
    if (!handle) throw new Error("resize handle missing");

    await fireEvent.pointerDown(handle, {
      button: 0,
      pointerId: 1,
      clientX: 200,
      clientY: 100,
    });
    await fireEvent.pointerMove(window, { clientX: 20, clientY: 20 });
    await fireEvent.pointerUp(window);

    expect(node).toHaveStyle({ width: "120px", height: "48px" });
  });

  test("resizing a node keeps its size when it is later moved", async () => {
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Agent");
    const node = screen.getByRole("button", { name: "Agent 1" });
    const handle = node.querySelector(".resize-handle");
    if (!handle) throw new Error("resize handle missing");

    await fireEvent.pointerDown(handle, {
      button: 0,
      pointerId: 1,
      clientX: 200,
      clientY: 100,
    });
    await fireEvent.pointerMove(window, { clientX: 250, clientY: 140 });
    await fireEvent.pointerUp(window);

    const dataTransfer = makeDataTransfer();
    await dispatchDragStart(node, { dataTransfer, clientX: 0, clientY: 0 });
    await dispatchDrop(canvas(), { dataTransfer, clientX: 40, clientY: 30 });

    expect(node).toHaveStyle({ width: "210px", height: "104px" });
    expect(node).toHaveStyle({ left: "40px", top: "30px" });
  });

  test("selects the node when its resize handle is pressed", async () => {
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Agent");
    await dropComponent("Project", 400, 400);
    await fireEvent.click(screen.getByText("Project 2"));

    const node = screen.getByRole("button", { name: "Agent 1" });
    const handle = node.querySelector(".resize-handle");
    if (!handle) throw new Error("resize handle missing");

    await fireEvent.pointerDown(handle, {
      button: 0,
      pointerId: 1,
      clientX: 200,
      clientY: 100,
    });
    await fireEvent.pointerUp(window);

    expect(node).toBePressed();
  });

  test("ignores resize presses that are not the primary button", async () => {
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Agent");
    const node = screen.getByRole("button", { name: "Agent 1" });
    const handle = node.querySelector(".resize-handle");
    if (!handle) throw new Error("resize handle missing");

    await fireEvent.pointerDown(handle, {
      button: 2,
      pointerId: 1,
      clientX: 200,
      clientY: 100,
    });
    await fireEvent.pointerMove(window, { clientX: 260, clientY: 150 });
    await fireEvent.pointerUp(window);

    expect(node).toHaveStyle({ width: "160px", height: "64px" });
  });

  test("ends a resize gesture when the pointer is cancelled", async () => {
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Agent");
    const node = screen.getByRole("button", { name: "Agent 1" });
    const handle = node.querySelector(".resize-handle");
    if (!handle) throw new Error("resize handle missing");

    await fireEvent.pointerDown(handle, {
      button: 0,
      pointerId: 1,
      clientX: 200,
      clientY: 100,
    });
    await fireEvent.pointerMove(window, { clientX: 230, clientY: 120 });
    await fireEvent.pointerCancel(window);
    await fireEvent.pointerMove(window, { clientX: 300, clientY: 200 });

    expect(node).toHaveStyle({ width: "190px", height: "84px" });
  });

  test("places a dropped component inside a project when it lands on one", async () => {
    render(Workspace);

    await dropComponent("Project");

    const dataTransfer = makeDataTransfer();
    await fireEvent.dragStart(screen.getByRole("button", { name: "Agent" }), {
      dataTransfer,
    });
    await fireEvent.dragOver(canvas(), {});
    // Content point (100, 50) falls inside Project 1's box (30..190, 20..84).
    await dispatchDrop(canvas(), { dataTransfer, clientX: 100, clientY: 50 });

    expect(canvas()).toContainElement(screen.getByText("Agent 1"));
    // The child renders at its parent-relative position inside the project box.
    expect(screen.getByRole("button", { name: "Agent 1" })).toHaveStyle({
      left: "100px",
      top: "50px",
    });
    const properties = screen.getByRole("complementary", {
      name: "Node properties",
    });
    expect(properties).toContainElement(screen.getByText("Type: agent"));
  });

  test("moves a project's children when the project is dragged", async () => {
    render(Workspace);

    await dropComponent("Project");
    const project = screen.getByRole("button", { name: "Project 1" });

    const dataTransfer = makeDataTransfer();
    await fireEvent.dragStart(screen.getByRole("button", { name: "Agent" }), {
      dataTransfer,
    });
    await fireEvent.dragOver(canvas(), {});
    await dispatchDrop(canvas(), { dataTransfer, clientX: 100, clientY: 50 });
    const child = screen.getByRole("button", { name: "Agent 1" });

    await dispatchDragStart(project, { dataTransfer, clientX: 0, clientY: 0 });
    await dispatchDrop(canvas(), { dataTransfer, clientX: 70, clientY: 60 });

    expect(project).toHaveStyle({ left: "70px", top: "60px" });
    expect(child).toHaveStyle({ left: "140px", top: "90px" });
  });

  test("does not nest a project that already has children inside another project", async () => {
    render(Workspace);

    await dropComponent("Project");
    const first = screen.getByRole("button", { name: "Project 1" });
    const dataTransfer = makeDataTransfer();
    await fireEvent.dragStart(screen.getByRole("button", { name: "Agent" }), {
      dataTransfer,
    });
    await fireEvent.dragOver(canvas(), {});
    await dispatchDrop(canvas(), { dataTransfer, clientX: 100, clientY: 50 });

    await dropComponent("Project", 400, 400);
    const second = screen.getByRole("button", { name: "Project 2" });

    await dispatchDragStart(first, { dataTransfer, clientX: 0, clientY: 0 });
    await dispatchDrop(canvas(), { dataTransfer, clientX: 420, clientY: 420 });

    // The project moves to where it was dropped but is not nested: nesting a
    // box that already holds children would put them two levels deep.
    expect(first).toHaveStyle({ left: "420px", top: "420px" });
    expect(second).toHaveStyle({ left: "400px", top: "400px" });
  });

  test("re-parents a child dropped onto another project", async () => {
    render(Workspace);

    await dropComponent("Project");
    const dataTransfer = makeDataTransfer();
    await fireEvent.dragStart(screen.getByRole("button", { name: "Agent" }), {
      dataTransfer,
    });
    await fireEvent.dragOver(canvas(), {});
    await dispatchDrop(canvas(), { dataTransfer, clientX: 100, clientY: 50 });
    const child = screen.getByRole("button", { name: "Agent 1" });

    await dropComponent("Project", 400, 400);
    const second = screen.getByRole("button", { name: "Project 2" });

    await dispatchDragStart(child, { dataTransfer, clientX: 0, clientY: 0 });
    await dispatchDrop(canvas(), { dataTransfer, clientX: 430, clientY: 420 });

    // Project 2 spans (400..560, 400..464); the child lands inside it at
    // parent-relative (30, 20) and renders at the absolute drop position.
    expect(child).toHaveStyle({ left: "430px", top: "420px" });
    expect(second).toHaveStyle({ left: "400px", top: "400px" });
  });

  test("detaches a child dropped onto the empty canvas", async () => {
    render(Workspace);

    await dropComponent("Project");
    const dataTransfer = makeDataTransfer();
    await fireEvent.dragStart(screen.getByRole("button", { name: "Agent" }), {
      dataTransfer,
    });
    await fireEvent.dragOver(canvas(), {});
    await dispatchDrop(canvas(), { dataTransfer, clientX: 100, clientY: 50 });
    const child = screen.getByRole("button", { name: "Agent 1" });

    await dispatchDragStart(child, { dataTransfer, clientX: 0, clientY: 0 });
    await dispatchDrop(canvas(), { dataTransfer, clientX: 400, clientY: 300 });

    expect(child).toHaveStyle({ left: "400px", top: "300px" });
  });

  test("moves a child inside its own project when dragged", async () => {
    render(Workspace);

    await dropComponent("Project");
    const dataTransfer = makeDataTransfer();
    await fireEvent.dragStart(screen.getByRole("button", { name: "Agent" }), {
      dataTransfer,
    });
    await fireEvent.dragOver(canvas(), {});
    await dispatchDrop(canvas(), { dataTransfer, clientX: 100, clientY: 50 });
    const child = screen.getByRole("button", { name: "Agent 1" });

    await dispatchDragStart(child, { dataTransfer, clientX: 0, clientY: 0 });
    await dispatchDrop(canvas(), { dataTransfer, clientX: 90, clientY: 60 });

    expect(child).toHaveStyle({ left: "90px", top: "60px" });
  });

  test("updates the node label when the name property is edited", async () => {
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Agent");

    await fireEvent.input(screen.getByLabelText("Name"), {
      target: { value: "Fetcher" },
    });

    expect(screen.getByText("Fetcher")).toBeInTheDocument();
    expect(screen.queryByText("Agent 1")).not.toBeInTheDocument();
  });

  test("selects a different node by clicking it on the canvas", async () => {
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Agent");
    await dropComponent("Project", 400, 400);
    await fireEvent.click(screen.getByText("Agent 1"));

    expect(screen.getByLabelText("Name")).toHaveValue("Agent 1");
    expect(screen.getByRole("button", { name: "Agent 1" })).toBePressed();
    expect(screen.getByRole("button", { name: "Project 2" })).not.toBePressed();
  });

  test("creates a project node from the palette", async () => {
    render(Workspace);

    await dropComponent("Project");

    expect(canvas()).toContainElement(screen.getByText("Project 1"));
  });

  test("shows folder controls only for project nodes", async () => {
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Agent");
    expect(screen.queryByLabelText("Path")).not.toBeInTheDocument();

    await dropComponent("Project", 400, 400);

    expect(screen.getByText("Type: project")).toBeInTheDocument();
    expect(screen.getByLabelText("Path")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Browse…" })).toBeInTheDocument();
  });

  test("captures a filesystem path from the directory browser", async () => {
    const fetchJson = (body: unknown) =>
      Promise.resolve(
        new Response(JSON.stringify(body), {
          headers: { "content-type": "application/json" },
        }),
      );
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string | URL) => {
        const target = new URL(input, "http://localhost");
        if (target.searchParams.get("path") === "/home/user/my-agent") {
          return fetchJson({
            path: "/home/user/my-agent",
            directories: ["src"],
          });
        }
        return fetchJson({ path: "/home/user", directories: ["my-agent"] });
      }),
    );
    render(Workspace);

    await dropComponent("Project");

    await fireEvent.click(screen.getByRole("button", { name: "Browse…" }));
    const browser = screen.getByRole("dialog", { name: "Directory browser" });
    await waitFor(() => expect(browser).toHaveTextContent("/home/user"));

    await fireEvent.click(screen.getByRole("button", { name: "my-agent" }));
    await fireEvent.click(
      screen.getByRole("button", { name: "Choose this folder" }),
    );

    expect(screen.getByLabelText("Path")).toHaveValue("/home/user/my-agent");
    expect(screen.getByLabelText("Name")).toHaveValue("my-agent");
    await waitFor(() =>
      expect(
        screen.queryByRole("dialog", { name: "Directory browser" }),
      ).not.toBeInTheDocument(),
    );
    vi.unstubAllGlobals();
  });

  test("navigates up to the parent directory while browsing", async () => {
    const fetchJson = (body: unknown) =>
      Promise.resolve(
        new Response(JSON.stringify(body), {
          headers: { "content-type": "application/json" },
        }),
      );
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string | URL) => {
        const target = new URL(input, "http://localhost");
        const path = target.searchParams.get("path");
        if (path === "/home/user/my-agent") {
          return fetchJson({
            path: "/home/user/my-agent",
            directories: ["src"],
          });
        }
        if (path === "/home") {
          return fetchJson({ path: "/home", directories: ["user"] });
        }
        return fetchJson({ path: "/home/user", directories: ["my-agent"] });
      }),
    );
    render(Workspace);

    await dropComponent("Project");
    await fireEvent.click(screen.getByRole("button", { name: "Browse…" }));
    await waitFor(() =>
      expect(
        screen.getByRole("dialog", { name: "Directory browser" }),
      ).toHaveTextContent("/home/user"),
    );
    await fireEvent.click(screen.getByRole("button", { name: "my-agent" }));
    await waitFor(() =>
      expect(
        screen.getByRole("dialog", { name: "Directory browser" }),
      ).toHaveTextContent("/home/user/my-agent"),
    );

    await fireEvent.click(screen.getByRole("button", { name: "Up" }));
    await waitFor(() =>
      expect(
        screen.getByRole("dialog", { name: "Directory browser" }),
      ).toHaveTextContent("/home"),
    );
    vi.unstubAllGlobals();
  });

  test("shows an error when the browsed path is not readable", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () => new Response("not a readable directory", { status: 400 }),
      ),
    );
    render(Workspace);

    await dropComponent("Project");
    await fireEvent.click(screen.getByRole("button", { name: "Browse…" }));
    await waitFor(() =>
      expect(
        screen.getByRole("dialog", { name: "Directory browser" }),
      ).toHaveTextContent("not a readable directory"),
    );

    await fireEvent.keyDown(document, { key: "Escape" });
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );

    vi.stubGlobal(
      "fetch",
      vi.fn(async () => Promise.reject(new Error("offline"))),
    );
    await fireEvent.click(screen.getByRole("button", { name: "Browse…" }));
    await waitFor(() =>
      expect(
        screen.getByRole("dialog", { name: "Directory browser" }),
      ).toHaveTextContent("Cannot reach the backend"),
    );
    vi.unstubAllGlobals();
  });

  test("closes the directory browser when Escape is pressed", async () => {
    render(Workspace);

    await dropComponent("Project");
    await fireEvent.click(screen.getByRole("button", { name: "Browse…" }));
    await waitFor(() =>
      expect(
        screen.getByRole("dialog", { name: "Directory browser" }),
      ).toBeInTheDocument(),
    );

    await fireEvent.keyDown(document, { key: "Escape" });
    await waitFor(() =>
      expect(
        screen.queryByRole("dialog", { name: "Directory browser" }),
      ).not.toBeInTheDocument(),
    );
  });

  test("marks the selected node as the start point from properties", async () => {
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Agent");

    await fireEvent.click(screen.getByLabelText("Starting point"));

    const node = screen.getByRole("button", { name: "Agent 1" });
    expect(node).toHaveTextContent("▶");
    expect(screen.getByLabelText("Starting point")).toBeChecked();
  });

  test("marking another sibling start clears the previous marker", async () => {
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Agent", 100, 50);
    const first = screen.getByRole("button", { name: "Agent 1" });
    await fireEvent.click(screen.getByLabelText("Starting point"));

    // Agent 1's box spans (100..260, 50..114); (30, 30) stays inside the
    // project but outside Agent 1, so Agent 2 becomes its sibling.
    await dropComponent("Agent", 30, 30);
    await fireEvent.click(screen.getByLabelText("Starting point"));

    expect(screen.getByRole("button", { name: "Agent 2" })).toHaveTextContent(
      "▶",
    );
    expect(first).not.toHaveTextContent("▶");
  });

  test("start markers in separate projects coexist", async () => {
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Agent", 100, 50);
    const first = screen.getByRole("button", { name: "Agent 1" });
    await fireEvent.click(screen.getByLabelText("Starting point"));

    await dropComponent("Project", 400, 400);
    await dropComponent("Agent", 430, 430);
    await fireEvent.click(screen.getByLabelText("Starting point"));

    expect(first).toHaveTextContent("▶");
    expect(screen.getByRole("button", { name: "Agent 2" })).toHaveTextContent(
      "▶",
    );
  });

  test("shows the checked state when a start node is selected", async () => {
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Agent");
    await fireEvent.click(screen.getByLabelText("Starting point"));
    await dropComponent("Project", 400, 400);
    expect(screen.getByLabelText("Starting point")).not.toBeChecked();

    await fireEvent.click(screen.getByText("Agent 1"));

    expect(screen.getByLabelText("Starting point")).toBeChecked();
  });

  test("renders the node name in a header with a separator below it", async () => {
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Agent");
    const node = screen.getByRole("button", { name: "Agent 1" });
    const title = node.querySelector(".node-title");
    expect(title).toHaveTextContent("Agent 1");
    expect(node.querySelector(".node-separator")).toBeInTheDocument();
  });

  test("rejects an agent dropped on a box that cannot host it", async () => {
    render(Workspace);

    await dropComponent("Project");
    // GitHub 1 nests inside Project 1 (spans 30..190, 20..84) and renders at
    // (100, 50), spanning (100..260, 50..114).
    await dropComponent("GitHub", 100, 50);

    // The agent's drop point sits inside the GitHub box: per the matrix the
    // GitHub cannot host it and the empty canvas will not take it either.
    await dropComponent("Agent", 140, 70);

    expect(
      screen.queryByRole("button", { name: "Agent 1" }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByText("An Agent cannot be placed inside a GitHub box."),
    ).toBeInTheDocument();
  });

  test("places a project dropped over an incompatible container at the top level", async () => {
    render(Workspace);

    await dropComponent("Project");
    // GitHub 1 nests in Project 1 and renders at (100, 50), spanning
    // (100..260, 50..114).
    await dropComponent("GitHub", 100, 50);

    // The GitHub box cannot host a project, but the empty canvas accepts
    // one: the project lands at the exact content position.
    await dropComponent("Project", 140, 70);

    expect(screen.getByRole("button", { name: "Project 2" })).toHaveStyle({
      left: "140px",
      top: "70px",
    });
  });

  test("keeps an incompatible child drop inside its own parent", async () => {
    render(Workspace);

    await dropComponent("Project", 400, 400);
    await dropComponent("GitHub", 410, 410);
    await dropComponent("GitHub App", 420, 420);
    const app = screen.getByRole("button", { name: "GitHub App 1" });

    await dropComponent("Project", 700, 300);

    const dataTransfer = makeDataTransfer();
    await dispatchDragStart(app, { dataTransfer, clientX: 0, clientY: 0 });
    await dispatchDrop(canvas(), { dataTransfer, clientX: 720, clientY: 320 });

    // Project 2 spans (700..860, 300..364); the GitHub App must not nest
    // into it — it stays a child of GitHub 1 and renders at the pointer.
    expect(app).toHaveStyle({ left: "720px", top: "320px" });
  });

  test("highlights the target container while a palette drag is in flight", async () => {
    render(Workspace);

    await dropComponent("Project");
    // GitHub 1 nests inside Project 1 and renders at (100, 50), spanning
    // (100..260, 50..114).
    await dropComponent("GitHub", 100, 50);

    const dataTransfer = makeDataTransfer();
    await fireEvent.dragStart(screen.getByRole("button", { name: "Agent" }), {
      dataTransfer,
    });

    // A point inside Project 1 but outside GitHub 1 previews as valid.
    await dispatchDragOver(canvas(), {
      dataTransfer,
      clientX: 140,
      clientY: 30,
    });
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Project 1" })).toHaveClass(
        /drop-ok/,
      ),
    );

    // The GitHub box cannot host an agent and agents are forbidden at the
    // root, so the preview flips to the invalid state.
    await dispatchDragOver(canvas(), {
      dataTransfer,
      clientX: 140,
      clientY: 70,
    });
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "GitHub 1" })).toHaveClass(
        /drop-no/,
      ),
    );

    // Leaving the canvas clears the preview.
    await dispatchDragEvent("dragleave", canvas(), {});
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "GitHub 1" })).not.toHaveClass(
        /drop-no/,
      ),
    );
  });

  test("nests a GitHub App inside a GitHub box that sits in a project", async () => {
    render(Workspace);

    await dropComponent("Project");
    // GitHub 1 lands inside Project 1 (spans 30..190, 20..84) and renders at
    // its absolute position (100, 50) spanning (100..260, 50..114).
    await dropComponent("GitHub", 100, 50);

    await dropComponent("GitHub App", 140, 70);

    // The drop resolves to the nested GitHub box, not the enclosing project.
    expect(screen.getByRole("button", { name: "GitHub App 1" })).toHaveStyle({
      left: "140px",
      top: "70px",
    });
  });

  test("shows a tiny three-character id at the right of the block header", async () => {
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Agent");
    const node = screen.getByRole("button", { name: "Agent 1" });
    const id = node.querySelector(".node-id");

    expect(id).toBeInTheDocument();
    expect(id).toHaveTextContent(/^[0-9a-z]{3}$/);
  });

  test("keeps a root-forbidden box in its parent when dropped over an incompatible container", async () => {
    render(Workspace);

    await dropComponent("Project", 400, 400);
    // GitHub 1 nests in Project 1 and renders at (410, 410), spanning
    // (410..570, 410..474).
    await dropComponent("GitHub", 410, 410);
    // (400, 405) is inside Project 1 but just outside the GitHub box.
    await dropComponent("Agent", 400, 405);
    const agent = screen.getByRole("button", { name: "Agent 1" });

    const dataTransfer = makeDataTransfer();
    await dispatchDragStart(agent, { dataTransfer, clientX: 0, clientY: 0 });
    await dispatchDrop(canvas(), { dataTransfer, clientX: 450, clientY: 420 });

    // The GitHub box cannot host an agent and the matrix forbids agents at
    // the root, so the agent stays a child of Project 1 at the drop point.
    expect(agent).toHaveStyle({ left: "450px", top: "420px" });
  });

  test("keeps the start badge inside the header", async () => {
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Agent");
    await fireEvent.click(screen.getByLabelText("Starting point"));

    const node = screen.getByRole("button", { name: "Agent 1" });
    const title = node.querySelector(".node-title");
    expect(title).not.toBeNull();
    expect(title?.querySelector(".start-flag")).toBeInTheDocument();
  });

  test("connects two top-level boxes with an arrow from properties", async () => {
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Project", 400, 400);
    await fireEvent.click(screen.getByText("Project 1"));

    const connect = screen.getByRole("button", { name: "Connect" });
    await fireEvent.click(connect);
    expect(connect).toBePressed();

    await fireEvent.click(screen.getByText("Project 2"));

    expect(canvas().querySelectorAll(".edge-line")).toHaveLength(1);
    expect(connect).not.toBePressed();
  });

  test("rejects connecting a box to a node on another level", async () => {
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Agent");
    const child = screen.getByRole("button", { name: "Agent 1" });
    await fireEvent.click(screen.getByRole("button", { name: "Connect" }));

    await fireEvent.click(screen.getByText("Project 1"));

    expect(canvas().querySelectorAll(".edge-line")).toHaveLength(0);
    expect(child).toBeInTheDocument();
  });

  test("ignores a second connection attempt for the same pair", async () => {
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Project", 400, 400);
    await fireEvent.click(screen.getByText("Project 1"));

    await fireEvent.click(screen.getByRole("button", { name: "Connect" }));
    await fireEvent.click(screen.getByText("Project 2"));
    await fireEvent.click(screen.getByRole("button", { name: "Connect" }));
    await fireEvent.click(screen.getByText("Project 2"));

    expect(canvas().querySelectorAll(".edge-line")).toHaveLength(1);
  });

  test("ends the arrow at the target box border so the head stays visible", async () => {
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Project", 400, 400);
    await fireEvent.click(screen.getByText("Project 1"));

    await fireEvent.click(screen.getByRole("button", { name: "Connect" }));
    await fireEvent.click(screen.getByText("Project 2"));

    // Project 2 centers at (480, 432); the trimmed line must stop on its top
    // edge (y = 400) rather than at the hidden center point.
    const line = canvas().querySelector(".edge-line");
    expect(line).not.toBeNull();
    expect(parseFloat(line!.getAttribute("y2") ?? "")).toBeCloseTo(400, 0);
    const x2 = parseFloat(line!.getAttribute("x2") ?? "");
    expect(x2).toBeGreaterThan(400);
    expect(x2).toBeLessThan(560);
  });

  test("canceling connect mode deselects without drawing an edge", async () => {
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Project", 400, 400);

    const connect = screen.getByRole("button", { name: "Connect" });
    await fireEvent.click(connect);
    await fireEvent.click(connect);

    expect(connect).not.toBePressed();
    await fireEvent.click(screen.getByText("Project 2"));
    expect(canvas().querySelectorAll(".edge-line")).toHaveLength(0);
  });
});
