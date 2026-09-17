import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/svelte";
import { afterEach, describe, expect, test, vi } from "vitest";
import AgentHealth from "./AgentHealth.svelte";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const checked = "2026-09-17T10:00:00Z";

function agents(overrides: Record<string, object> = {}) {
  return {
    agents: [
      {
        backend: "claude",
        label: "Claude Code",
        state: "available",
        replyMs: 2100,
        checkedAt: checked,
        acknowledged: true,
      },
      {
        backend: "codex",
        label: "Codex",
        state: "unavailable",
        reason: "not installed",
        checkedAt: checked,
        acknowledged: false,
      },
      {
        backend: "grok",
        label: "Grok",
        state: "checking",
        acknowledged: false,
      },
      {
        backend: "opencode",
        label: "OpenCode",
        state: "available",
        replyMs: 850,
        checkedAt: checked,
        acknowledged: false,
      },
    ].map((agent) => ({ ...agent, ...overrides[agent.backend] })),
  };
}

function serve(...bodies: object[]) {
  const fetchMock = vi.fn<
    (url: string, init?: { method: string }) => Promise<Response>
  >(async () => {
    const body = bodies.length > 1 ? bodies.shift() : bodies[0];
    return new Response(JSON.stringify(body));
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

describe("agent health", () => {
  test("shows each backend's logo with what its check found", async () => {
    serve(agents());

    render(AgentHealth, { pollMs: 60_000 });

    const claude = await screen.findByRole("button", {
      name: "Claude Code: responded in 2.1 s",
    });
    expect(claude).toHaveAttribute("data-state", "available");
    expect(claude).toHaveAttribute("title", "Claude Code: responded in 2.1 s");
    expect(
      screen.getByRole("button", { name: "Codex: not installed" }),
    ).toHaveAttribute("data-state", "unavailable");
    expect(
      screen.getByRole("button", { name: "Grok: checking…" }),
    ).toHaveAttribute("data-state", "checking");
    expect(
      screen.getByRole("button", { name: "OpenCode: responded in 850 ms" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("group", { name: "Agent backends" }),
    ).toBeInTheDocument();
  });

  test("checks again when a logo is clicked", async () => {
    const fetchMock = serve(
      agents({ grok: { state: "unavailable", reason: "exited 1" } }),
      agents({
        grok: { state: "unavailable", reason: "exited 1" },
        codex: { state: "checking", reason: undefined },
      }),
    );
    render(AgentHealth, { pollMs: 60_000 });

    await fireEvent.click(
      await screen.findByRole("button", { name: "Codex: not installed" }),
    );

    expect(
      await screen.findByRole("button", { name: "Codex: checking…" }),
    ).toBeInTheDocument();
    expect(fetchMock).toHaveBeenLastCalledWith("/api/agents/health", {
      method: "POST",
    });
  });

  test("polls only while a check is running", async () => {
    const fetchMock = serve(
      agents(),
      agents({ grok: { state: "unavailable", reason: "timed out after 60s" } }),
    );

    render(AgentHealth, { pollMs: 10 });

    expect(
      await screen.findByRole("button", { name: "Grok: timed out after 60s" }),
    ).toBeInTheDocument();
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  test("shows nothing when the backend cannot be reached", async () => {
    const fetchMock = vi.fn(async () => Promise.reject(new Error("offline")));
    vi.stubGlobal("fetch", fetchMock);

    render(AgentHealth);

    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    expect(screen.queryByRole("group")).not.toBeInTheDocument();
  });
});
