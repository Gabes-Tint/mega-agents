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

afterEach(() => {
  cleanup();
  localStorage.clear();
});

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

  test("captures the already-authenticated flag for a GitHub box", async () => {
    render(Workspace);

    await dropComponent("Project", 400, 400);
    await dropComponent("GitHub", 410, 410);
    const checkbox = screen.getByLabelText("Already authenticated (OAuth)");
    expect(checkbox).not.toBeChecked();

    await fireEvent.click(checkbox);
    await fireEvent.click(screen.getByText("Project 1"));
    await fireEvent.click(screen.getByText("GitHub 1"));

    expect(
      screen.getByLabelText("Already authenticated (OAuth)"),
    ).toBeChecked();
  });

  function repositoryLookups(
    answer: (path: string) => Response | Promise<Response>,
  ) {
    const fetchMock = vi.fn(async (input: string) => {
      const url = new URL(input, "http://localhost");
      return answer(url.searchParams.get("path") ?? "");
    });
    vi.stubGlobal("fetch", fetchMock);
    return fetchMock;
  }

  async function setSelectedPath(path: string): Promise<void> {
    await fireEvent.input(screen.getByLabelText("Path"), {
      target: { value: path },
    });
  }

  test("loads the repository from the project's clone when a GitHub block is dropped into it", async () => {
    const fetchMock = repositoryLookups(() =>
      Response.json({ repository: "acme/api", remote: "origin" }),
    );
    render(Workspace);
    await dropComponent("Project", 400, 400);
    await setSelectedPath("/home/user/api");

    await dropComponent("GitHub", 410, 410);

    await waitFor(() =>
      expect(screen.getByLabelText("Repository")).toHaveValue("acme/api"),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/git/repository?path=%2Fhome%2Fuser%2Fapi",
    );
    vi.unstubAllGlobals();
  });

  test("loads the repository when a GitHub block moves into another project", async () => {
    repositoryLookups((path) =>
      path === "/home/user/web"
        ? Response.json({ repository: "acme/web", remote: "origin" })
        : new Response("not a Git repository", { status: 404 }),
    );
    render(Workspace);
    await dropComponent("Project");
    await dropComponent("GitHub", 100, 50);
    const github = screen.getByRole("button", { name: "GitHub 1" });
    await dropComponent("Project", 400, 400);
    await setSelectedPath("/home/user/web");

    const dataTransfer = makeDataTransfer();
    await dispatchDragStart(github, { dataTransfer, clientX: 0, clientY: 0 });
    await dispatchDrop(canvas(), { dataTransfer, clientX: 430, clientY: 420 });
    await fireEvent.click(github);

    await waitFor(() =>
      expect(screen.getByLabelText("Repository")).toHaveValue("acme/web"),
    );
    vi.unstubAllGlobals();
  });

  test("keeps a repository typed before the project's clone answers", async () => {
    let answer: (response: Response) => void = () => {};
    repositoryLookups(
      () =>
        new Promise<Response>((resolve) => {
          answer = resolve;
        }),
    );
    render(Workspace);
    await dropComponent("Project", 400, 400);
    await setSelectedPath("/home/user/api");
    await dropComponent("GitHub", 410, 410);

    await fireEvent.input(screen.getByLabelText("Repository"), {
      target: { value: "acme/typed" },
    });
    answer(Response.json({ repository: "acme/api", remote: "origin" }));
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(screen.getByLabelText("Repository")).toHaveValue("acme/typed");
    vi.unstubAllGlobals();
  });

  test("leaves the repository empty when the project is not a GitHub clone", async () => {
    const fetchMock = repositoryLookups(
      () => new Response("not a Git repository", { status: 404 }),
    );
    render(Workspace);
    await dropComponent("Project", 400, 400);
    await setSelectedPath("/home/user/notes");

    await dropComponent("GitHub", 410, 410);

    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    expect(screen.getByLabelText("Repository")).toHaveValue("");
    vi.unstubAllGlobals();
  });

  test("leaves the repository empty when the backend is unreachable", async () => {
    const fetchMock = vi.fn(async () => Promise.reject(new Error("offline")));
    vi.stubGlobal("fetch", fetchMock);
    render(Workspace);
    await dropComponent("Project", 400, 400);
    await setSelectedPath("/home/user/api");

    await dropComponent("GitHub", 410, 410);

    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    expect(screen.getByLabelText("Repository")).toHaveValue("");
    vi.unstubAllGlobals();
  });

  test("does not look up a repository for a project without a path", async () => {
    const fetchMock = repositoryLookups(() =>
      Response.json({ repository: "acme/api", remote: "origin" }),
    );
    render(Workspace);
    await dropComponent("Project", 400, 400);

    await dropComponent("GitHub", 410, 410);

    expect(fetchMock).not.toHaveBeenCalled();
    vi.unstubAllGlobals();
  });

  async function buildRunnableFlow(): Promise<void> {
    await dropComponent("Project", 400, 400);
    await fireEvent.input(screen.getByLabelText("Path"), {
      target: { value: "/home/user/api" },
    });
    await dropComponent("GitHub", 410, 410);
    await fireEvent.input(screen.getByLabelText("Repository"), {
      target: { value: "acme/api" },
    });
    await fireEvent.click(
      screen.getByLabelText("Already authenticated (OAuth)"),
    );
    await fireEvent.click(screen.getByLabelText("Starting point"));
  }

  test("runs the flow on the backend and shows each step's result", async () => {
    const fetchMock = vi.fn(async () =>
      Response.json({
        status: "succeeded",
        steps: [
          {
            nodeId: "g1",
            name: "GitHub 1",
            action: "fetch",
            status: "succeeded",
            details: {
              repository: "acme/api",
              remote: "origin",
              output: "From github.com:acme/api",
            },
          },
        ],
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    render(Workspace);
    await buildRunnableFlow();

    await fireEvent.click(screen.getByRole("button", { name: "Run flow" }));

    const result = screen.getByRole("region", { name: "Run result" });
    await waitFor(() => expect(result).toHaveTextContent("✅ Run succeeded"));
    expect(result).toHaveTextContent("✅ GitHub 1: fetch succeeded");
    expect(result).toHaveTextContent("Repository: acme/api");
    expect(result).toHaveTextContent("Remote: origin");
    expect(result).toHaveTextContent("From github.com:acme/api");
    const call = fetchMock.mock.calls.at(-1) as unknown as
      [string, RequestInit] | undefined;
    expect(call?.[0]).toBe("/api/runs");
    expect(call?.[1]?.method).toBe("POST");
    expect(call?.[1]?.headers).toEqual({ "Content-Type": "application/json" });
    const body = JSON.parse(String(call?.[1]?.body)) as {
      nodes: Array<Record<string, unknown>>;
    };
    expect(body.nodes[0]).toMatchObject({
      type: "project",
      path: "/home/user/api",
    });
    expect(body.nodes[1]).toMatchObject({
      type: "github",
      repository: "acme/api",
      authenticated: true,
      start: true,
    });
    vi.unstubAllGlobals();
  });

  test("shows why a run step failed", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json({
          status: "failed",
          steps: [
            {
              nodeId: "g1",
              name: "GitHub 1",
              action: "fetch",
              status: "failed",
              error: "project path /home/user/api is not a Git repository",
            },
          ],
        }),
      ),
    );
    render(Workspace);
    await buildRunnableFlow();

    await fireEvent.click(screen.getByRole("button", { name: "Run flow" }));

    const result = screen.getByRole("region", { name: "Run result" });
    await waitFor(() => expect(result).toHaveTextContent("❌ Run failed"));
    expect(result).toHaveTextContent("❌ GitHub 1: fetch failed");
    expect(result).toHaveTextContent(
      "project path /home/user/api is not a Git repository",
    );
    vi.unstubAllGlobals();
  });

  test("surfaces a run the backend rejects", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            "flag a GitHub block as the starting point to run the flow\n",
            { status: 400 },
          ),
      ),
    );
    render(Workspace);
    await dropComponent("Project");

    await fireEvent.click(screen.getByRole("button", { name: "Run flow" }));

    const result = screen.getByRole("region", { name: "Run result" });
    await waitFor(() =>
      expect(result).toHaveTextContent(
        "flag a GitHub block as the starting point to run the flow",
      ),
    );
    vi.unstubAllGlobals();
  });

  test("surfaces a backend outage while running", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => Promise.reject(new Error("offline"))),
    );
    render(Workspace);
    await dropComponent("Project");

    await fireEvent.click(screen.getByRole("button", { name: "Run flow" }));

    const result = screen.getByRole("region", { name: "Run result" });
    await waitFor(() =>
      expect(result).toHaveTextContent("Cannot reach the backend"),
    );
    vi.unstubAllGlobals();
  });

  test("disables the run button while a run is in flight", async () => {
    let finish: (response: Response) => void = () => {};
    vi.stubGlobal(
      "fetch",
      vi.fn(
        () =>
          new Promise<Response>((resolve) => {
            finish = resolve;
          }),
      ),
    );
    render(Workspace);
    await buildRunnableFlow();

    await fireEvent.click(screen.getByRole("button", { name: "Run flow" }));

    const running = await screen.findByRole("button", { name: "Running…" });
    expect(running).toBeDisabled();
    finish(Response.json({ status: "succeeded", steps: [] }));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Run flow" })).toBeEnabled(),
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
    expect(palette).toContainElement(
      screen.getByRole("button", { name: "JSON Schema" }),
    );
    expect(palette).toContainElement(
      screen.getByRole("button", { name: "Router" }),
    );
    expect(palette).toContainElement(
      screen.getByRole("button", { name: "Command" }),
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
    // into it. It stays a child of GitHub 1, under the pointer but moved
    // down inside GitHub 1's top edge at 410, and GitHub 1 grows to hold it.
    expect(app).toHaveStyle({ left: "720px", top: "410px" });
    expect(screen.getByRole("button", { name: "GitHub 1" })).toHaveStyle({
      width: "482px",
    });
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

  test("previews where a palette component lands while it is dragged", async () => {
    render(Workspace);
    await dropComponent("Project", 30, 20);
    const dataTransfer = makeDataTransfer();
    await fireEvent.dragStart(screen.getByRole("button", { name: "Loop" }), {
      dataTransfer,
    });

    await dispatchDragOver(canvas(), {
      dataTransfer,
      clientX: 90,
      clientY: 60,
    });

    const preview = canvas().querySelector(".drop-preview");
    expect(preview).toHaveTextContent("Loop");
    expect(preview).toHaveStyle({
      left: "90px",
      top: "60px",
      width: "400px",
      height: "220px",
    });
    expect(preview).not.toHaveClass("invalid");

    await dispatchDrop(canvas(), { dataTransfer, clientX: 90, clientY: 60 });
    expect(canvas().querySelector(".drop-preview")).toBeNull();
  });

  test("marks the preview of a drop the canvas would reject", async () => {
    render(Workspace);
    const dataTransfer = makeDataTransfer();
    await fireEvent.dragStart(screen.getByRole("button", { name: "Agent" }), {
      dataTransfer,
    });

    await dispatchDragOver(canvas(), {
      dataTransfer,
      clientX: 40,
      clientY: 40,
    });

    expect(canvas().querySelector(".drop-preview")).toHaveClass("invalid");
    await dispatchDragEvent("dragleave", canvas(), {});
    expect(canvas().querySelector(".drop-preview")).toBeNull();
  });

  test("previews a moved block at its landing spot with its own size", async () => {
    render(Workspace);
    await dropComponent("Project", 30, 20);
    const project = screen.getByRole("button", { name: "Project 1" });
    const dataTransfer = makeDataTransfer();
    await dispatchDragStart(project, { dataTransfer, clientX: 10, clientY: 5 });

    await dispatchDragOver(canvas(), {
      dataTransfer,
      clientX: 210,
      clientY: 105,
    });

    const preview = canvas().querySelector(".drop-preview");
    expect(preview).toHaveTextContent("Project 1");
    expect(preview).toHaveStyle({
      left: "200px",
      top: "100px",
      width: "160px",
      height: "64px",
    });
    expect(project).toHaveClass("dragging");
  });

  test("highlights the preview and the container a block will land in", async () => {
    render(Workspace);
    await dropComponent("Project", 30, 20);
    // GitHub 1 renders at (100, 50) inside Project 1.
    await dropComponent("GitHub", 100, 50);
    const dataTransfer = makeDataTransfer();
    await fireEvent.dragStart(screen.getByRole("button", { name: "Project" }), {
      dataTransfer,
    });

    await dispatchDragOver(canvas(), {
      dataTransfer,
      clientX: 40,
      clientY: 30,
    });
    expect(canvas().querySelector(".drop-preview")).toHaveClass("nesting");
    expect(screen.getByRole("button", { name: "Project 1" })).toHaveClass(
      "drop-ok",
    );

    // A GitHub box refuses a project, which then lands on the canvas: nothing
    // is highlighted as its container.
    await dispatchDragOver(canvas(), {
      dataTransfer,
      clientX: 140,
      clientY: 70,
    });
    expect(canvas().querySelector(".drop-preview")).not.toHaveClass("nesting");
    expect(canvas().querySelector(".drop-ok, .drop-no")).toBeNull();
  });

  test("grows a container to fit a block dropped near its edge", async () => {
    render(Workspace);
    await dropComponent("Project", 30, 20);

    await dropComponent("Agent", 150, 70);

    // The agent sits at (120, 50) in the 160 × 64 project, which grows to
    // 120 + 160 + 12 wide and 50 + 64 + 12 tall.
    expect(screen.getByRole("button", { name: "Project 1" })).toHaveStyle({
      width: "292px",
      height: "126px",
    });
  });

  test("grows a container to fit a block moved inside it", async () => {
    render(Workspace);
    await dropComponent("Project", 30, 20);
    await dropComponent("Agent", 40, 30);
    const agent = screen.getByRole("button", { name: "Agent 1" });
    const dataTransfer = makeDataTransfer();
    await dispatchDragStart(agent, { dataTransfer, clientX: 0, clientY: 0 });

    await dispatchDrop(canvas(), { dataTransfer, clientX: 180, clientY: 80 });

    expect(screen.getByRole("button", { name: "Project 1" })).toHaveStyle({
      width: "322px",
      height: "136px",
    });
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

  test("stacks the block id over an icon of the block's type", async () => {
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("GitHub");
    const github = screen.getByRole("button", { name: "GitHub 1" });
    const meta = github.querySelector(".node-meta");

    expect(meta?.children[0]).toHaveClass("node-id");
    expect(meta?.children[1]).toHaveClass("block-icon", "object");
    expect(meta?.children[1]).toHaveAttribute("data-icon", "github");
    expect(meta?.children[1]?.querySelector("svg")).toBeInTheDocument();
  });

  test("shows the same block id in the properties panel", async () => {
    render(Workspace);

    await dropComponent("Project");
    await dropComponent("Agent");
    const node = screen.getByRole("button", { name: "Agent 1" });
    const headerId = node.querySelector(".node-id")?.textContent ?? "";

    const properties = screen.getByRole("complementary", {
      name: "Node properties",
    });
    const panelId = properties.querySelector(".node-id");

    expect(panelId).toHaveTextContent(headerId);
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

  async function githubWithActions(...labels: string[]): Promise<void> {
    await dropComponent("Project", 400, 400);
    await dropComponent("GitHub", 410, 410);
    for (const label of labels) {
      await fireEvent.click(screen.getByText("GitHub 1"));
      await fireEvent.change(screen.getByLabelText("New action"), {
        target: { value: label },
      });
      await fireEvent.click(screen.getByRole("button", { name: "Add action" }));
    }
  }

  test("adds Git actions to a GitHub block as a sequence", async () => {
    render(Workspace);

    await githubWithActions("fetch", "worktree", "rebase");

    for (const name of ["Fetch", "Create worktree", "Rebase"]) {
      expect(screen.getByRole("button", { name })).toBeInTheDocument();
    }
    expect(canvas().querySelectorAll(".edge-line")).toHaveLength(2);
    await fireEvent.click(screen.getByText("GitHub 1"));
    const sequence = screen.getByRole("list", { name: "Actions" });
    expect(sequence).toHaveTextContent("1. Fetch");
    expect(sequence).toHaveTextContent("2. Create worktree");
    expect(sequence).toHaveTextContent("3. Rebase");
  });

  test("selects an action from its GitHub block's sequence", async () => {
    render(Workspace);
    await githubWithActions("fetch", "worktree");
    await fireEvent.click(screen.getByText("GitHub 1"));

    await fireEvent.click(
      screen.getByRole("button", { name: "2. Create worktree" }),
    );

    expect(screen.getByText("Git action: Create worktree")).toBeInTheDocument();
  });

  test("configures the branch, base, and path of a worktree action", async () => {
    render(Workspace);
    await githubWithActions("worktree");

    const fields = {
      Branch: "feature/login",
      Base: "origin/main",
      "Worktree path": "/tmp/login",
    };
    for (const [label, value] of Object.entries(fields)) {
      await fireEvent.input(screen.getByLabelText(label), {
        target: { value },
      });
    }
    await fireEvent.click(screen.getByText("GitHub 1"));
    await fireEvent.click(
      screen.getByRole("button", { name: "Create worktree" }),
    );

    for (const [label, value] of Object.entries(fields)) {
      expect(screen.getByLabelText(label)).toHaveValue(value);
    }
    expect(screen.queryByLabelText("Onto")).not.toBeInTheDocument();
  });

  test("configures what a rebase action rebases onto", async () => {
    render(Workspace);
    await githubWithActions("worktree", "rebase");

    await fireEvent.input(screen.getByLabelText("Onto"), {
      target: { value: "origin/release" },
    });

    expect(screen.getByLabelText("Onto")).toHaveValue("origin/release");
    expect(screen.queryByLabelText("Branch")).not.toBeInTheDocument();
  });

  test("only GitHub blocks offer actions", async () => {
    render(Workspace);
    await dropComponent("Project", 400, 400);

    expect(screen.queryByRole("button", { name: "Add action" })).toBeNull();
    await dropComponent("GitLab", 410, 410);
    expect(screen.queryByRole("button", { name: "Add action" })).toBeNull();
  });

  test("shows each action's run status on the canvas and rolls it up", async () => {
    render(Workspace);
    await githubWithActions("fetch", "worktree");
    const fetchNode = screen.getByRole("button", { name: "Fetch" });
    const worktreeNode = screen.getByRole("button", {
      name: "Create worktree",
    });
    const githubNode = screen.getByRole("button", { name: "GitHub 1" });
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_url: string, init: RequestInit) => {
        const body = JSON.parse(String(init.body)) as {
          nodes: Array<{ id: string; action?: string }>;
        };
        const idOf = (action: string) =>
          body.nodes.find((node) => node.action === action)?.id;
        return Response.json({
          status: "failed",
          steps: [
            {
              nodeId: idOf("fetch"),
              name: "Fetch",
              action: "fetch",
              status: "succeeded",
            },
            {
              nodeId: idOf("worktree"),
              name: "Create worktree",
              action: "worktree",
              status: "failed",
              error: "set the branch the worktree works on",
              details: { path: "/tmp/login", branch: "feature/login" },
            },
          ],
        });
      }),
    );

    await fireEvent.click(screen.getByRole("button", { name: "Run flow" }));

    await waitFor(() => expect(fetchNode).toHaveClass("status-succeeded"));
    expect(worktreeNode).toHaveClass("status-failed");
    expect(githubNode).toHaveClass("status-failed");
    const result = screen.getByRole("region", { name: "Run result" });
    expect(result).toHaveTextContent("Path: /tmp/login");
    expect(result).toHaveTextContent("Branch: feature/login");
    vi.unstubAllGlobals();
  });

  // A fake backend answering by "METHOD path"; each route answers in turn
  // from its list and repeats its last answer.
  function fakeBackend(
    routes: Record<string, Array<() => Response | Promise<Response>>>,
  ) {
    const calls: string[] = [];
    const fetchMock = vi.fn(async (input: string, init?: RequestInit) => {
      const key = `${init?.method ?? "GET"} ${input}`;
      calls.push(key);
      const answers = routes[key];
      if (!answers) return new Response("not found", { status: 404 });
      const answer = answers.length > 1 ? answers.shift()! : answers[0]!;
      return answer();
    });
    vi.stubGlobal("fetch", fetchMock);
    return calls;
  }

  const step = (status: string, extra: Record<string, unknown> = {}) => ({
    nodeId: "g1",
    name: "GitHub 1",
    action: "fetch",
    status,
    ...extra,
  });

  const record = (status: string, steps: unknown[]) => () =>
    Response.json({ id: "run-1", workflow: "workflow", status, steps });

  async function selectedGitHubId(): Promise<string> {
    let id = "";
    await fireEvent.click(screen.getByText("GitHub 1"));
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_input: string, init?: RequestInit) => {
        const body = JSON.parse(String(init?.body)) as {
          nodes: Array<{ id: string; type: string }>;
        };
        id = body.nodes.find((node) => node.type === "github")?.id ?? "";
        return new Response("stop", { status: 400 });
      }),
    );
    await fireEvent.click(screen.getByRole("button", { name: "Run flow" }));
    await waitFor(() => expect(id).not.toBe(""));
    return id;
  }

  test("follows a started run until it finishes, updating the canvas", async () => {
    render(Workspace);
    await buildRunnableFlow();
    const id = await selectedGitHubId();
    const github = screen.getByRole("button", { name: "GitHub 1" });
    const calls = fakeBackend({
      "POST /api/runs": [
        () =>
          Response.json(
            {
              id: "run-1",
              status: "running",
              steps: [{ ...step("pending"), nodeId: id }],
            },
            { status: 202 },
          ),
      ],
      "GET /api/runs/run-1": [
        record("running", [{ ...step("running"), nodeId: id }]),
        record("succeeded", [
          {
            ...step("succeeded"),
            nodeId: id,
            details: { remote: "origin" },
          },
        ]),
      ],
    });

    await fireEvent.click(screen.getByRole("button", { name: "Run flow" }));

    await waitFor(() => expect(github).toHaveClass("status-running"));
    expect(screen.getByRole("button", { name: "Running…" })).toBeDisabled();
    const result = screen.getByRole("region", { name: "Run result" });
    await waitFor(() => expect(result).toHaveTextContent("Run succeeded"));
    expect(github).toHaveClass("status-succeeded");
    expect(result).toHaveTextContent("Remote: origin");
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Run flow" })).toBeEnabled(),
    );
    expect(calls.filter((call) => call === "GET /api/runs/run-1")).toHaveLength(
      2,
    );
    vi.unstubAllGlobals();
  });

  test("reports a lost connection while following a run", async () => {
    render(Workspace);
    await buildRunnableFlow();
    fakeBackend({
      "POST /api/runs": [record("running", [step("running")])],
      "GET /api/runs/run-1": [() => Promise.reject(new Error("offline"))],
    });

    await fireEvent.click(screen.getByRole("button", { name: "Run flow" }));

    const result = screen.getByRole("region", { name: "Run result" });
    await waitFor(() =>
      expect(result).toHaveTextContent("Lost track of run run-1"),
    );
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Run flow" })).toBeEnabled(),
    );
    vi.unstubAllGlobals();
  });

  test("shows a block's log from its properties after a run", async () => {
    render(Workspace);
    await buildRunnableFlow();
    expect(screen.queryByRole("button", { name: "Show logs" })).toBeNull();
    const id = await selectedGitHubId();
    const calls = fakeBackend({
      "POST /api/runs": [record("failed", [{ ...step("failed"), nodeId: id }])],
      [`GET /api/runs/run-1/logs/${id}`]: [
        () => new Response("$ git fetch origin\nfatal: no route\n"),
      ],
    });
    await fireEvent.click(screen.getByRole("button", { name: "Run flow" }));
    await waitFor(() =>
      expect(
        screen.getByRole("region", { name: "Run result" }),
      ).toHaveTextContent("Run failed"),
    );

    await fireEvent.click(screen.getByText("GitHub 1"));
    await fireEvent.click(screen.getByRole("button", { name: "Show logs" }));

    const dialog = await screen.findByRole("dialog", {
      name: "Logs: GitHub 1",
    });
    await waitFor(() => expect(dialog).toHaveTextContent("fatal: no route"));
    expect(calls).toContain(`GET /api/runs/run-1/logs/${id}`);
    vi.unstubAllGlobals();
  });

  test("shows a step's log from the run result", async () => {
    render(Workspace);
    await buildRunnableFlow();
    fakeBackend({
      "POST /api/runs": [record("succeeded", [step("succeeded")])],
      "GET /api/runs/run-1/logs/g1": [() => new Response("GitHub 1 started")],
    });
    await fireEvent.click(screen.getByRole("button", { name: "Run flow" }));

    await fireEvent.click(
      await screen.findByRole("button", { name: "Logs of GitHub 1" }),
    );

    const dialog = await screen.findByRole("dialog", {
      name: "Logs: GitHub 1",
    });
    await waitFor(() => expect(dialog).toHaveTextContent("GitHub 1 started"));
    await fireEvent.click(screen.getByRole("button", { name: "Close" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    vi.unstubAllGlobals();
  });

  test("explains a log that cannot be loaded", async () => {
    render(Workspace);
    await buildRunnableFlow();
    fakeBackend({
      "POST /api/runs": [record("skipped", [step("skipped")])],
    });
    await fireEvent.click(screen.getByRole("button", { name: "Run flow" }));

    await fireEvent.click(
      await screen.findByRole("button", { name: "Logs of GitHub 1" }),
    );

    const dialog = await screen.findByRole("dialog", {
      name: "Logs: GitHub 1",
    });
    await waitFor(() =>
      expect(dialog).toHaveTextContent("No log recorded for this step"),
    );
    vi.unstubAllGlobals();
  });

  async function agentInProject(): Promise<void> {
    await dropComponent("Project", 400, 400);
    await dropComponent("Agent", 410, 410);
  }

  test("configures an agent's backend, model, prompt, and limits", async () => {
    const posted: Array<Record<string, unknown>> = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_url: string, init?: RequestInit) => {
        posted.push(...(JSON.parse(String(init?.body)) as { nodes: [] }).nodes);
        return new Response("stop", { status: 400 });
      }),
    );
    render(Workspace);
    await agentInProject();

    expect(screen.getByLabelText("Backend")).toHaveValue("claude");
    await fireEvent.change(screen.getByLabelText("Backend"), {
      target: { value: "opencode" },
    });
    const text = {
      Model: "opencode-go/glm-5.3-flash",
      Effort: "high",
      Prompt: "Implement {{workspace.branch}}",
      "Output schema": '{"type": "object"}',
    };
    for (const [label, value] of Object.entries(text)) {
      await fireEvent.input(screen.getByLabelText(label), {
        target: { value },
      });
    }
    await fireEvent.input(screen.getByLabelText("Retries"), {
      target: { value: "1" },
    });
    await fireEvent.input(screen.getByLabelText("Timeout (minutes)"), {
      target: { value: "45" },
    });
    await fireEvent.click(screen.getByText("Project 1"));
    await fireEvent.click(screen.getByText("Agent 1"));

    for (const [label, value] of Object.entries(text)) {
      expect(screen.getByLabelText(label)).toHaveValue(value);
    }
    expect(screen.getByLabelText("Backend")).toHaveValue("opencode");
    await fireEvent.click(screen.getByRole("button", { name: "Run flow" }));
    await waitFor(() => expect(posted.length).toBeGreaterThan(0));
    expect(posted.find((node) => node.type === "agent")).toMatchObject({
      backend: "opencode",
      model: "opencode-go/glm-5.3-flash",
      effort: "high",
      prompt: "Implement {{workspace.branch}}",
      outputSchema: '{"type": "object"}',
      retries: 1,
      timeoutMinutes: 45,
    });
    vi.unstubAllGlobals();
  });

  test("warns about an output schema that is not JSON", async () => {
    render(Workspace);
    await agentInProject();

    await fireEvent.input(screen.getByLabelText("Output schema"), {
      target: { value: '{"type": ' },
    });

    expect(screen.getByRole("alert")).toHaveTextContent(
      "The output schema is not valid JSON",
    );
    await fireEvent.input(screen.getByLabelText("Output schema"), {
      target: { value: "" },
    });
    expect(screen.queryByRole("alert")).toBeNull();
  });

  test("lists the placeholders a prompt can use", async () => {
    render(Workspace);
    await agentInProject();

    expect(screen.getByText(/\{\{workspace\.path\}\}/)).toBeInTheDocument();
    expect(screen.getByText(/\{\{results\.<agent>\}\}/)).toBeInTheDocument();
  });

  test("shows an agent step's session, attempts, and reply", async () => {
    render(Workspace);
    await buildRunnableFlow();
    fakeBackend({
      "POST /api/runs": [
        record("succeeded", [
          {
            nodeId: "a1",
            name: "Reviewer",
            action: "agent",
            status: "succeeded",
            details: {
              backend: "claude",
              sessionId: "session-9",
              attempts: 2,
              reply: '{"verdict":"approve"}',
            },
          },
        ]),
      ],
    });

    await fireEvent.click(screen.getByRole("button", { name: "Run flow" }));

    const result = screen.getByRole("region", { name: "Run result" });
    await waitFor(() => expect(result).toHaveTextContent("Run succeeded"));
    expect(result).toHaveTextContent("Backend: claude");
    expect(result).toHaveTextContent("Session: session-9");
    expect(result).toHaveTextContent("Attempts: 2");
    expect(result).toHaveTextContent('{"verdict":"approve"}');
    vi.unstubAllGlobals();
  });

  test("configures a schema block and which output each arrow takes", async () => {
    render(Workspace);
    await dropComponent("Project", 400, 400);
    await dropComponent("Agent", 410, 410);
    // The check lands inside the agent, which grows to 410..592 × 410..496
    // and grows the project to 400..604 × 400..508.
    await dropComponent("JSON Schema", 420, 420);
    await fireEvent.input(screen.getByLabelText("Name"), {
      target: { value: "Check" },
    });
    await fireEvent.input(screen.getByLabelText("Schema"), {
      target: { value: '{"type": "object"}' },
    });
    await dropComponent("Agent", 596, 500);
    await fireEvent.input(screen.getByLabelText("Name"), {
      target: { value: "Fixer" },
    });
    await fireEvent.click(screen.getByText("Check"));
    await fireEvent.click(screen.getByRole("button", { name: "Connect" }));
    await fireEvent.click(screen.getByText("Fixer"));

    await fireEvent.click(screen.getByText("Check"));
    expect(screen.getByLabelText("Schema")).toHaveValue('{"type": "object"}');
    const port = screen.getByLabelText("Output to Fixer");
    expect(port).toHaveValue("valid");
    await fireEvent.change(port, { target: { value: "invalid" } });

    expect(canvas().querySelector(".edge-port")).toHaveTextContent("invalid");
    await fireEvent.input(screen.getByLabelText("Schema"), {
      target: { value: "{" },
    });
    expect(screen.getByRole("alert")).toHaveTextContent(
      "The schema is not valid JSON",
    );
  });

  test("shows a schema step's field errors", async () => {
    render(Workspace);
    await buildRunnableFlow();
    fakeBackend({
      "POST /api/runs": [
        record("failed", [
          {
            nodeId: "s1",
            name: "Check",
            action: "jsonschema",
            status: "failed",
            error: "the value did not satisfy the schema",
            details: {
              valid: false,
              errors: [
                {
                  path: "$.verdict",
                  message: "value must be one of 'approve'",
                },
              ],
            },
          },
        ]),
      ],
    });

    await fireEvent.click(screen.getByRole("button", { name: "Run flow" }));

    const result = screen.getByRole("region", { name: "Run result" });
    await waitFor(() => expect(result).toHaveTextContent("Run failed"));
    expect(result).toHaveTextContent("Valid: false");
    expect(result).toHaveTextContent(
      "$.verdict: value must be one of 'approve'",
    );
    vi.unstubAllGlobals();
  });

  test("edits a router's cases and routes its arrows", async () => {
    render(Workspace);
    await dropComponent("Project", 400, 400);
    await dropComponent("Router", 410, 410);
    await dropComponent("Agent", 450, 440);
    await fireEvent.input(screen.getByLabelText("Name"), {
      target: { value: "Fix" },
    });
    await fireEvent.click(screen.getByText("Router 1"));
    await fireEvent.click(screen.getByRole("button", { name: "Connect" }));
    await fireEvent.click(screen.getByText("Fix"));
    await fireEvent.click(screen.getByText("Router 1"));

    expect(screen.getByLabelText("Case 1 name")).toHaveValue("approved");
    await fireEvent.click(screen.getByRole("button", { name: "Add case" }));
    await fireEvent.input(screen.getByLabelText("Case 2 name"), {
      target: { value: "blocked" },
    });
    await fireEvent.input(screen.getByLabelText("Case 2 expression"), {
      target: { value: "size(value.findings) > 0" },
    });
    const port = screen.getByLabelText("Output to Fix");
    expect(port).toHaveValue("approved");
    await fireEvent.change(port, { target: { value: "blocked" } });
    expect(canvas().querySelector(".edge-port")).toHaveTextContent("blocked");

    await fireEvent.click(
      screen.getByRole("button", { name: "Remove case 2" }),
    );
    expect(screen.queryByLabelText("Case 2 name")).toBeNull();
    expect(screen.getByLabelText("Output to Fix")).toHaveValue("default");
  });

  test("shows the route a router step took", async () => {
    render(Workspace);
    await buildRunnableFlow();
    fakeBackend({
      "POST /api/runs": [
        record("succeeded", [
          {
            nodeId: "x1",
            name: "Route",
            action: "router",
            status: "succeeded",
            details: { case: "approved" },
          },
        ]),
      ],
    });

    await fireEvent.click(screen.getByRole("button", { name: "Run flow" }));

    const result = screen.getByRole("region", { name: "Run result" });
    await waitFor(() => expect(result).toHaveTextContent("Route: approved"));
    vi.unstubAllGlobals();
  });

  test("saves the workflow under its name", async () => {
    const calls = fakeBackend({
      "PUT /api/workflows/issue-to-pr": [
        () => Response.json({ name: "issue-to-pr", updatedAt: "now" }),
      ],
    });
    render(Workspace);
    await dropComponent("Project", 400, 400);
    await fireEvent.input(screen.getByLabelText("Workflow name"), {
      target: { value: "Issue to PR" },
    });

    await fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(
        screen.getByRole("status", { name: "Workflow file" }),
      ).toHaveTextContent("Saved as issue-to-pr"),
    );
    expect(calls).toContain("PUT /api/workflows/issue-to-pr");
    vi.unstubAllGlobals();
  });

  test("explains a save the backend refuses", async () => {
    fakeBackend({
      "PUT /api/workflows/workflow": [
        () =>
          new Response("agent cannot be placed inside root", { status: 400 }),
      ],
    });
    render(Workspace);

    await fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(
        screen.getByRole("status", { name: "Workflow file" }),
      ).toHaveTextContent("agent cannot be placed inside root"),
    );
    vi.unstubAllGlobals();
  });

  test("opens a saved workflow", async () => {
    fakeBackend({
      "GET /api/workflows": [
        () =>
          Response.json([
            { name: "nightly", updatedAt: "2026-09-17T01:00:00Z" },
          ]),
      ],
      "GET /api/workflows/nightly": [
        () =>
          Response.json({
            name: "nightly",
            nodes: [
              {
                id: "api",
                type: "project",
                name: "api",
                x: 10,
                y: 10,
                w: 300,
                h: 200,
              },
              {
                id: "planner",
                type: "agent",
                name: "Planner",
                x: 20,
                y: 40,
                w: 160,
                h: 64,
                parentId: "api",
              },
            ],
            edges: [],
          }),
      ],
    });
    render(Workspace);
    await dropComponent("Project", 400, 400);

    await fireEvent.click(screen.getByRole("button", { name: "Open…" }));
    await fireEvent.click(
      await screen.findByRole("button", { name: "nightly" }),
    );

    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Planner" }),
      ).toBeInTheDocument(),
    );
    expect(screen.queryByRole("button", { name: "Project 1" })).toBeNull();
    expect(screen.getByLabelText("Workflow name")).toHaveValue("nightly");
    vi.unstubAllGlobals();
  });

  test("shows when there is nothing to open", async () => {
    fakeBackend({ "GET /api/workflows": [() => Response.json([])] });
    render(Workspace);

    await fireEvent.click(screen.getByRole("button", { name: "Open…" }));

    expect(
      await screen.findByText("No saved workflows yet."),
    ).toBeInTheDocument();
    vi.unstubAllGlobals();
  });

  test("imports a workflow YAML file", async () => {
    let body = "";
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_url: string, init?: RequestInit) => {
        body = String(init?.body);
        return Response.json({
          name: "imported",
          nodes: [
            {
              id: "api",
              type: "project",
              name: "Imported",
              x: 0,
              y: 0,
              w: 200,
              h: 100,
            },
          ],
          edges: [],
        });
      }),
    );
    render(Workspace);
    const file = new File(["kind: Workflow"], "flow.yaml", {
      type: "application/yaml",
    });

    await fireEvent.change(screen.getByLabelText("Import YAML"), {
      target: { files: [file] },
    });

    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Imported" }),
      ).toBeInTheDocument(),
    );
    expect(body).toBe("kind: Workflow");
    vi.unstubAllGlobals();
  });

  test("keeps the graph across reloads of the page", async () => {
    const first = render(Workspace);
    await dropComponent("Project", 400, 400);
    await fireEvent.input(screen.getByLabelText("Workflow name"), {
      target: { value: "draft" },
    });
    await new Promise((resolve) => setTimeout(resolve, 0));
    first.unmount();

    render(Workspace);

    expect(
      screen.getByRole("button", { name: "Project 1" }),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("Workflow name")).toHaveValue("draft");
    localStorage.clear();
  });

  test("cancels the run in progress", async () => {
    render(Workspace);
    await buildRunnableFlow();
    let cancelled = false;
    const calls = fakeBackend({
      "POST /api/runs": [record("running", [step("running")])],
      "GET /api/runs/run-1": [
        () =>
          Response.json({
            id: "run-1",
            status: cancelled ? "cancelled" : "running",
            steps: [step(cancelled ? "failed" : "running")],
          }),
      ],
      "POST /api/runs/run-1/cancel": [
        () => {
          cancelled = true;
          return Response.json(
            { id: "run-1", status: "cancelling" },
            { status: 202 },
          );
        },
      ],
    });
    expect(screen.queryByRole("button", { name: "Cancel run" })).toBeNull();

    await fireEvent.click(screen.getByRole("button", { name: "Run flow" }));
    await fireEvent.click(
      await screen.findByRole("button", { name: "Cancel run" }),
    );

    const result = screen.getByRole("region", { name: "Run result" });
    await waitFor(() => expect(result).toHaveTextContent("Run cancelled"));
    expect(calls).toContain("POST /api/runs/run-1/cancel");
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: "Cancel run" })).toBeNull(),
    );
    vi.unstubAllGlobals();
  });

  test("opens a past run from the run history", async () => {
    render(Workspace);
    await buildRunnableFlow();
    fakeBackend({
      "GET /api/runs": [
        () =>
          Response.json([
            {
              id: "run-7",
              workflow: "nightly",
              status: "failed",
              startedAt: "2026-09-17T01:00:00Z",
              steps: [],
            },
          ]),
      ],
      "GET /api/runs/run-7": [
        () =>
          Response.json({
            id: "run-7",
            workflow: "nightly",
            status: "failed",
            steps: [step("failed", { error: "no route to github.com" })],
          }),
      ],
    });

    await fireEvent.click(screen.getByRole("button", { name: "Runs…" }));
    await fireEvent.click(
      await screen.findByRole("button", { name: /nightly · failed/ }),
    );

    const result = screen.getByRole("region", { name: "Run result" });
    await waitFor(() => expect(result).toHaveTextContent("Run failed"));
    expect(result).toHaveTextContent("no route to github.com");
    expect(
      screen.getByRole("button", { name: "Logs of GitHub 1" }),
    ).toBeInTheDocument();
    vi.unstubAllGlobals();
  });

  test("shows when no run has been recorded", async () => {
    fakeBackend({ "GET /api/runs": [() => Response.json([])] });
    render(Workspace);

    await fireEvent.click(screen.getByRole("button", { name: "Runs…" }));

    expect(
      await screen.findByText("No runs recorded yet."),
    ).toBeInTheDocument();
    vi.unstubAllGlobals();
  });

  test("configures a command gate", async () => {
    render(Workspace);
    await dropComponent("Project", 400, 400);
    await dropComponent("Command", 410, 410);

    await fireEvent.input(screen.getByLabelText("Command"), {
      target: { value: "go test ./..." },
    });
    await fireEvent.input(screen.getByLabelText("Timeout (minutes)"), {
      target: { value: "15" },
    });
    await fireEvent.click(screen.getByText("Project 1"));
    await fireEvent.click(screen.getByText("Command 1"));

    expect(screen.getByLabelText("Command")).toHaveValue("go test ./...");
    expect(screen.getByLabelText("Timeout (minutes)")).toHaveValue(15);
  });

  test("shows a command step's exit code and output", async () => {
    render(Workspace);
    await buildRunnableFlow();
    fakeBackend({
      "POST /api/runs": [
        record("failed", [
          {
            nodeId: "c1",
            name: "Tests",
            action: "command",
            status: "failed",
            error: "Tests exited 1: FAIL",
            details: { exitCode: 1, output: "--- FAIL: TestLogin" },
          },
        ]),
      ],
    });

    await fireEvent.click(screen.getByRole("button", { name: "Run flow" }));

    const result = screen.getByRole("region", { name: "Run result" });
    await waitFor(() => expect(result).toHaveTextContent("Exit code: 1"));
    expect(result).toHaveTextContent("--- FAIL: TestLogin");
    vi.unstubAllGlobals();
  });

  test("configures the delivery actions of a GitHub block", async () => {
    render(Workspace);
    await githubWithActions("issue", "commit", "pullrequest");

    await fireEvent.click(screen.getByRole("button", { name: "Read issue" }));
    await fireEvent.input(screen.getByLabelText("Issue number"), {
      target: { value: "7" },
    });
    await fireEvent.click(screen.getByRole("button", { name: "Commit" }));
    await fireEvent.input(screen.getByLabelText("Commit message"), {
      target: { value: "Implement {{workspace.branch}}" },
    });
    await fireEvent.click(
      screen.getByRole("button", { name: "Open pull request" }),
    );
    for (const [label, value] of Object.entries({
      Title: "Close #7",
      Body: "Made by agents",
      "Base branch": "develop",
    })) {
      await fireEvent.input(screen.getByLabelText(label), {
        target: { value },
      });
    }

    await fireEvent.click(screen.getByRole("button", { name: "Read issue" }));
    expect(screen.getByLabelText("Issue number")).toHaveValue(7);
    await fireEvent.click(screen.getByRole("button", { name: "Commit" }));
    expect(screen.getByLabelText("Commit message")).toHaveValue(
      "Implement {{workspace.branch}}",
    );
    await fireEvent.click(
      screen.getByRole("button", { name: "Open pull request" }),
    );
    expect(screen.getByLabelText("Base branch")).toHaveValue("develop");
  });

  test("shows the commit and pull request a run delivered", async () => {
    render(Workspace);
    await buildRunnableFlow();
    fakeBackend({
      "POST /api/runs": [
        record("succeeded", [
          {
            nodeId: "k1",
            name: "Commit",
            action: "commit",
            status: "succeeded",
            details: { commit: "abc123" },
          },
          {
            nodeId: "r1",
            name: "Open pull request",
            action: "pullrequest",
            status: "succeeded",
            details: { url: "https://github.com/acme/api/pull/42", number: 42 },
          },
        ]),
      ],
    });

    await fireEvent.click(screen.getByRole("button", { name: "Run flow" }));

    const result = screen.getByRole("region", { name: "Run result" });
    await waitFor(() => expect(result).toHaveTextContent("Commit: abc123"));
    expect(
      screen.getByRole("link", { name: "https://github.com/acme/api/pull/42" }),
    ).toHaveAttribute("href", "https://github.com/acme/api/pull/42");
    vi.unstubAllGlobals();
  });

  test("retries a failed run and follows the retry", async () => {
    render(Workspace);
    await buildRunnableFlow();
    const calls = fakeBackend({
      "POST /api/runs": [record("failed", [step("failed", { error: "boom" })])],
      "POST /api/runs/run-1/retry": [
        () =>
          Response.json(
            {
              id: "run-2",
              status: "running",
              retryOf: "run-1",
              steps: [step("pending")],
            },
            { status: 202 },
          ),
      ],
      "GET /api/runs/run-2": [
        () =>
          Response.json({
            id: "run-2",
            status: "succeeded",
            retryOf: "run-1",
            steps: [step("succeeded", { details: { reusedFrom: "run-1" } })],
          }),
      ],
    });
    await fireEvent.click(screen.getByRole("button", { name: "Run flow" }));

    await fireEvent.click(
      await screen.findByRole("button", { name: "Retry from failure" }),
    );

    const result = screen.getByRole("region", { name: "Run result" });
    await waitFor(() => expect(result).toHaveTextContent("Run succeeded"));
    expect(result).toHaveTextContent("Retry of run-1");
    expect(result).toHaveTextContent("Reused from: run-1");
    expect(calls).toContain("POST /api/runs/run-1/retry");
    expect(
      screen.queryByRole("button", { name: "Retry from failure" }),
    ).toBeNull();
    vi.unstubAllGlobals();
  });

  test("explains a retry the backend refuses", async () => {
    render(Workspace);
    await buildRunnableFlow();
    fakeBackend({
      "POST /api/runs": [record("cancelled", [step("failed")])],
      "POST /api/runs/run-1/retry": [
        () =>
          new Response("run run-1 did not record its workflow", {
            status: 409,
          }),
      ],
    });
    await fireEvent.click(screen.getByRole("button", { name: "Run flow" }));

    await fireEvent.click(
      await screen.findByRole("button", { name: "Retry from failure" }),
    );

    await waitFor(() =>
      expect(
        screen.getByRole("region", { name: "Run result" }),
      ).toHaveTextContent("did not record its workflow"),
    );
    vi.unstubAllGlobals();
  });

  test("starts a workflow from a template", async () => {
    fakeBackend({
      "GET /api/templates": [
        () =>
          Response.json([
            {
              name: "gate-and-fix",
              title: "Gate and fix",
              description: "Run the gate, fix what fails.",
            },
          ]),
      ],
      "GET /api/templates/gate-and-fix": [
        () =>
          Response.json({
            name: "gate-and-fix",
            nodes: [
              {
                id: "project",
                type: "project",
                name: "Project",
                x: 40,
                y: 40,
                w: 600,
                h: 400,
              },
              {
                id: "gate",
                type: "command",
                name: "Gate",
                x: 20,
                y: 60,
                w: 180,
                h: 64,
                parentId: "project",
                command: "make verify",
              },
            ],
            edges: [],
          }),
      ],
    });
    render(Workspace);

    await fireEvent.click(screen.getByRole("button", { name: "Templates…" }));
    expect(
      await screen.findByText("Run the gate, fix what fails."),
    ).toBeInTheDocument();
    await fireEvent.click(screen.getByRole("button", { name: "Gate and fix" }));

    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Gate" })).toBeInTheDocument(),
    );
    expect(screen.getByLabelText("Workflow name")).toHaveValue("gate-and-fix");
    expect(
      screen.getByRole("status", { name: "Workflow file" }),
    ).toHaveTextContent("Started from template Gate and fix");
    vi.unstubAllGlobals();
  });

  test("shows what each agent cost and what the run cost", async () => {
    render(Workspace);
    await buildRunnableFlow();
    const usage = (cost: number) => ({
      inputTokens: 1000,
      cachedInputTokens: 0,
      outputTokens: 50,
      costUsd: cost,
      costKnown: true,
    });
    fakeBackend({
      "POST /api/runs": [
        record("succeeded", [
          {
            nodeId: "a1",
            name: "Coder",
            action: "agent",
            status: "succeeded",
            details: { usage: usage(0.02) },
          },
          {
            nodeId: "a2",
            name: "Reviewer",
            action: "agent",
            status: "succeeded",
            details: { usage: usage(0.0125) },
          },
        ]),
      ],
    });

    await fireEvent.click(screen.getByRole("button", { name: "Run flow" }));

    const result = screen.getByRole("region", { name: "Run result" });
    await waitFor(() =>
      expect(result).toHaveTextContent(
        "Agents cost $0.0325 · 2000 input tokens · 100 output tokens",
      ),
    );
    expect(result).toHaveTextContent("Cost: $0.0200 · 1000 in / 50 out");
    vi.unstubAllGlobals();
  });

  test("sets an agent's cost budget", async () => {
    render(Workspace);
    await dropComponent("Project", 400, 400);
    await dropComponent("Agent", 410, 410);

    await fireEvent.input(screen.getByLabelText("Max cost (USD)"), {
      target: { value: "0.5" },
    });
    await fireEvent.click(screen.getByText("Project 1"));
    await fireEvent.click(screen.getByText("Agent 1"));

    expect(screen.getByLabelText("Max cost (USD)")).toHaveValue(0.5);
  });

  test("lets an agent continue the connected agent's conversation", async () => {
    const posted: Array<Record<string, unknown>> = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_url: string, init?: RequestInit) => {
        posted.push(...(JSON.parse(String(init?.body)) as { nodes: [] }).nodes);
        return new Response("stop", { status: 400 });
      }),
    );
    render(Workspace);
    await dropComponent("Project", 400, 400);
    await dropComponent("Agent", 410, 410);
    const box = screen.getByLabelText(
      "Continue the connected agent's conversation",
    );
    expect(box).not.toBeChecked();

    await fireEvent.click(box);
    await fireEvent.click(screen.getByRole("button", { name: "Run flow" }));

    await waitFor(() =>
      expect(posted.find((node) => node.type === "agent")).toMatchObject({
        continueSession: true,
      }),
    );
    vi.unstubAllGlobals();
  });
});

describe("deleting blocks", () => {
  test("the properties panel deletes the selected block", async () => {
    render(Workspace);
    await dropComponent("Project");
    await fireEvent.click(screen.getByRole("button", { name: "Project 1" }));

    await fireEvent.click(screen.getByRole("button", { name: "Delete block" }));

    expect(
      screen.queryByRole("button", { name: "Project 1" }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByText("Select a node on the canvas to edit its properties."),
    ).toBeInTheDocument();
  });

  test("the Delete key deletes the selected block on the canvas", async () => {
    render(Workspace);
    await dropComponent("Project");
    const block = screen.getByRole("button", { name: "Project 1" });
    await fireEvent.click(block);

    await fireEvent.keyDown(block, { key: "Delete" });

    expect(
      screen.queryByRole("button", { name: "Project 1" }),
    ).not.toBeInTheDocument();
  });

  test("Backspace deletes too", async () => {
    render(Workspace);
    await dropComponent("Project");
    const block = screen.getByRole("button", { name: "Project 1" });
    await fireEvent.click(block);

    await fireEvent.keyDown(block, { key: "Backspace" });

    expect(
      screen.queryByRole("button", { name: "Project 1" }),
    ).not.toBeInTheDocument();
  });

  test("other keys and keys typed in fields leave the block", async () => {
    render(Workspace);
    await dropComponent("Project");
    const block = screen.getByRole("button", { name: "Project 1" });
    await fireEvent.click(block);

    await fireEvent.keyDown(block, { key: "Enter" });
    await fireEvent.keyDown(screen.getByLabelText("Name"), {
      key: "Backspace",
    });

    expect(
      screen.getByRole("button", { name: "Project 1" }),
    ).toBeInTheDocument();
  });
});

describe("block colors", () => {
  test("a block and its palette entry carry the hue of their type", async () => {
    render(Workspace);
    await dropComponent("Project");

    const block = screen.getByRole("button", { name: "Project 1" });
    const entry = screen.getByRole("button", { name: "Project" });
    const agentEntry = screen.getByRole("button", { name: "Agent" });

    expect(block.style.getPropertyValue("--block-hue")).not.toBe("");
    expect(entry.style.getPropertyValue("--block-hue")).toBe(
      block.style.getPropertyValue("--block-hue"),
    );
    expect(agentEntry.style.getPropertyValue("--block-hue")).not.toBe(
      block.style.getPropertyValue("--block-hue"),
    );
  });
});

describe("palette tooltips", () => {
  test("hovering a component explains what it is", async () => {
    render(Workspace);
    const entry = screen.getByRole("button", { name: "Command" });

    await fireEvent.mouseEnter(entry);

    const tooltip = screen.getByRole("tooltip");
    expect(tooltip).toHaveTextContent("Runs a shell command");
    expect(entry).toHaveAccessibleDescription(/Runs a shell command/);

    await fireEvent.mouseLeave(entry);

    expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
  });

  test("focusing a component explains it too", async () => {
    render(Workspace);
    const entry = screen.getByRole("button", { name: "Router" });

    await fireEvent.focus(entry);

    expect(screen.getByRole("tooltip")).toHaveTextContent("CEL");

    await fireEvent.blur(entry);

    expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
  });

  test("dragging a component hides its tooltip", async () => {
    render(Workspace);
    const entry = screen.getByRole("button", { name: "Project" });
    await fireEvent.mouseEnter(entry);

    await fireEvent.dragStart(entry, { dataTransfer: makeDataTransfer() });

    expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
  });
});

describe("problems panel", () => {
  function problemsBackend(
    problems: { nodeId?: string; severity: string; message: string }[],
  ) {
    const fetchMock = vi.fn((url: string) =>
      Promise.resolve(
        url === "/api/workflows/problems"
          ? new Response(JSON.stringify(problems), {
              headers: { "content-type": "application/json" },
            })
          : new Response("not found", { status: 404 }),
      ),
    );
    vi.stubGlobal("fetch", fetchMock);
    return fetchMock;
  }

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  test("lists what to fix before running, checked as the graph changes", async () => {
    const fetchMock = problemsBackend([]);
    render(Workspace);
    await dropComponent("Project");
    const project = screen.getByRole("button", { name: "Project 1" });
    const id = draftIdOf(project);
    fetchMock.mockImplementation((url: string) =>
      Promise.resolve(
        new Response(
          url === "/api/workflows/problems"
            ? JSON.stringify([
                { severity: "error", message: "flag a starting point" },
                {
                  nodeId: id,
                  severity: "warning",
                  message: "Project 1: set the folder its blocks work in",
                },
              ])
            : "",
        ),
      ),
    );

    await fireEvent.input(screen.getByLabelText("Name"), {
      target: { value: "api" },
    });

    const panel = await screen.findByRole("tabpanel", { name: /Problems/ });
    await waitFor(() =>
      expect(panel).toHaveTextContent("flag a starting point"),
    );
    expect(panel).toHaveTextContent(
      "Project 1: set the folder its blocks work in",
    );
    const request = (fetchMock.mock.calls as unknown as [string, RequestInit][])
      .filter(([url]) => url === "/api/workflows/problems")
      .at(-1)!;
    expect(JSON.parse(String(request[1].body)).nodes[0].name).toBe("api");
    expect(
      screen.getByRole("button", { name: "1 error, 1 warning" }),
    ).toBeInTheDocument();
    await waitFor(() => expect(project).toHaveClass("problem-warning"));
  });

  test("choosing a problem selects its block", async () => {
    problemsBackend([]);
    render(Workspace);
    await dropComponent("Project");
    await dropComponent("Project", 400, 20);
    const first = screen.getByRole("button", { name: "Project 1" });
    const id = draftIdOf(first);
    problemsBackend([
      { nodeId: id, severity: "error", message: "Project 1 is broken" },
    ]);
    await fireEvent.input(screen.getByLabelText("Name"), {
      target: { value: "Project 2b" },
    });

    await fireEvent.click(
      await screen.findByRole("button", {
        name: "Select the block with problem 1",
        description: "Project 1 is broken",
      }),
    );

    expect(screen.getByLabelText("Name")).toHaveValue("Project 1");
  });

  test("shows that nothing needs fixing", async () => {
    problemsBackend([]);
    render(Workspace);
    await dropComponent("Project");

    const panel = screen.getByRole("tabpanel", { name: /Problems/ });

    await waitFor(() => expect(panel).toHaveTextContent("No problems found"));
    expect(
      screen.getByRole("button", { name: "0 errors, 0 warnings" }),
    ).toBeInTheDocument();
  });

  test("the Run tab shows a run once it starts", async () => {
    problemsBackend([]);
    render(Workspace);

    await fireEvent.click(screen.getByRole("tab", { name: "Run" }));

    expect(screen.getByRole("tab", { name: "Run" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await fireEvent.click(
      screen.getByRole("button", { name: "0 errors, 0 warnings" }),
    );
    expect(screen.getByRole("tab", { name: /Problems/ })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await fireEvent.click(screen.getByRole("button", { name: "Run flow" }));
    await waitFor(() =>
      expect(screen.getByRole("tab", { name: "Run" })).toHaveAttribute(
        "aria-selected",
        "true",
      ),
    );
  });
});

// The id a block on the canvas stands for, read from the draft the editor
// keeps of the graph.
function draftIdOf(block: HTMLElement): string {
  const draft = JSON.parse(
    localStorage.getItem("mega-agents:draft") ?? "{}",
  ) as {
    nodes: { id: string; name: string }[];
  };
  const node = draft.nodes.find(
    (candidate) =>
      candidate.name === block.textContent?.match(/Project \d/)?.[0],
  );
  if (!node) throw new Error("block not in the draft");
  return node.id;
}

describe("loop properties", () => {
  test("a loop's properties choose how often it repeats and what ends it", async () => {
    render(Workspace);
    await dropComponent("Project");
    await dropComponent("Loop", 60, 60);
    await fireEvent.click(screen.getByRole("button", { name: "Loop 1" }));

    expect(screen.getByLabelText("Repeat at most")).toHaveValue(3);
    expect(screen.getByLabelText("Ends when")).toBeDisabled();
    expect(
      screen.getByText(
        "Put a command or router inside the loop, or a schema check inside one of its agents, to end it.",
      ),
    ).toBeInTheDocument();

    await dropComponent("Command", 80, 120);
    await fireEvent.click(screen.getByRole("button", { name: "Loop 1" }));
    await fireEvent.change(screen.getByLabelText("Ends when"), {
      target: {
        value: screen
          .getByRole("option", { name: "Command 1 takes passed" })
          .getAttribute("value"),
      },
    });
    await fireEvent.input(screen.getByLabelText("Repeat at most"), {
      target: { value: "4" },
    });

    await waitFor(() => {
      const draft = JSON.parse(
        localStorage.getItem("mega-agents:draft") ?? "{}",
      ) as { nodes: { type: string }[] };
      expect(draft.nodes.find((node) => node.type === "loop")).toMatchObject({
        untilPort: "passed",
        maxIterations: 4,
      });
    });
  });
});

describe("waiting for any arrow in the panel", () => {
  test("a command can be set to run when any arrow arrives", async () => {
    render(Workspace);
    await dropComponent("Project");
    await dropComponent("Command", 60, 50);
    await fireEvent.click(screen.getByRole("button", { name: "Command 1" }));

    await fireEvent.click(screen.getByLabelText("Run when any arrow arrives"));

    await waitFor(() => {
      const draft = JSON.parse(
        localStorage.getItem("mega-agents:draft") ?? "{}",
      ) as { nodes: { type: string; waitForAny?: boolean }[] };
      expect(
        draft.nodes.find((node) => node.type === "command")?.waitForAny,
      ).toBe(true);
    });
  });
});
