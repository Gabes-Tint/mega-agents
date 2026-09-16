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
  type: "dragstart" | "drop",
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
  test("lists the draggable components in the left palette", () => {
    render(Workspace);

    const palette = screen.getByRole("complementary", {
      name: "Component palette",
    });
    expect(palette).toContainElement(
      screen.getByRole("button", { name: "Agent" }),
    );
    expect(palette).toContainElement(
      screen.getByRole("button", { name: "Tool" }),
    );
    expect(palette).toContainElement(
      screen.getByRole("button", { name: "Project" }),
    );
  });

  test("adds a node to the canvas when a palette component is dropped", async () => {
    render(Workspace);

    await dropComponent("Agent");

    expect(canvas()).toContainElement(screen.getByText("Agent 1"));
  });

  test("accounts for canvas scroll when placing a dropped component", async () => {
    render(Workspace);
    const scrolled = canvas() as HTMLElement;
    Object.defineProperty(scrolled, "scrollLeft", { get: () => 50 });
    Object.defineProperty(scrolled, "scrollTop", { get: () => 40 });

    await dropComponent("Agent");

    expect(screen.getByText("Agent 1")).toHaveStyle({
      left: "80px",
      top: "60px",
    });
  });

  test("selects the dropped node so its properties appear on the right", async () => {
    render(Workspace);

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
      fireEvent.dragStart(screen.getByRole("button", { name: "Tool" }), {}),
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

    await dropComponent("Agent");
    const node = screen.getByRole("button", { name: "Agent 1" });

    const dataTransfer = makeDataTransfer();
    await dispatchDragStart(node, { dataTransfer, clientX: 0, clientY: 0 });
    await dispatchDrop(scrolled, { dataTransfer, clientX: 100, clientY: 80 });

    expect(node).toHaveStyle({ left: "150px", top: "120px" });
  });

  test("keeps a node in place when drag data does not match the drop", async () => {
    render(Workspace);

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

    await dropComponent("Agent");
    const node = screen.getByRole("button", { name: "Agent 1" });

    expect(node.querySelector(".resize-handle")).toBeInTheDocument();
  });

  test("resizes a node by dragging its handle", async () => {
    render(Workspace);

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

    await dropComponent("Agent");
    await dropComponent("Tool");
    await fireEvent.click(screen.getByText("Tool 1"));

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
    expect(screen.getByText("Agent 1")).toHaveStyle({
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

    await dropComponent("Agent");

    await fireEvent.input(screen.getByLabelText("Name"), {
      target: { value: "Fetcher" },
    });

    expect(screen.getByText("Fetcher")).toBeInTheDocument();
    expect(screen.queryByText("Agent 1")).not.toBeInTheDocument();
  });

  test("selects a different node by clicking it on the canvas", async () => {
    render(Workspace);

    await dropComponent("Agent");
    await dropComponent("Tool");
    await fireEvent.click(screen.getByText("Agent 1"));

    expect(screen.getByLabelText("Name")).toHaveValue("Agent 1");
    expect(screen.getByRole("button", { name: "Agent 1" })).toBePressed();
    expect(screen.getByRole("button", { name: "Tool 1" })).not.toBePressed();
  });

  test("creates a project node from the palette", async () => {
    render(Workspace);

    await dropComponent("Project");

    expect(canvas()).toContainElement(screen.getByText("Project 1"));
  });

  test("shows folder controls only for project nodes", async () => {
    render(Workspace);

    await dropComponent("Agent");
    expect(screen.queryByLabelText("Path")).not.toBeInTheDocument();

    await dropComponent("Project");

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

    await dropComponent("Agent");

    await fireEvent.click(screen.getByLabelText("Starting point"));

    const node = screen.getByRole("button", { name: "Agent 1" });
    expect(node).toHaveTextContent("▶");
    expect(screen.getByLabelText("Starting point")).toBeChecked();
  });

  test("marking another sibling start clears the previous marker", async () => {
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Agent");
    const first = screen.getByRole("button", { name: "Agent 1" });
    await fireEvent.click(screen.getByLabelText("Starting point"));

    await dropComponent("Agent", 100, 50);
    await fireEvent.click(screen.getByLabelText("Starting point"));

    expect(screen.getByRole("button", { name: "Agent 2" })).toHaveTextContent(
      "▶",
    );
    expect(first).not.toHaveTextContent("▶");
  });

  test("start markers in separate projects coexist", async () => {
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Agent");
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

    await dropComponent("Agent");
    await fireEvent.click(screen.getByLabelText("Starting point"));
    await dropComponent("Tool", 400, 400);
    expect(screen.getByLabelText("Starting point")).not.toBeChecked();

    await fireEvent.click(screen.getByText("Agent 1"));

    expect(screen.getByLabelText("Starting point")).toBeChecked();
  });
});
