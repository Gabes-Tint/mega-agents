import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import App from "./App.svelte";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

// The address bar chooses the page: the editor at the root, the list of
// workflows and their schedules at /workflows.
describe("which page the address opens", () => {
  test("the workflows path opens the list of workflows", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({ ok: true, json: async () => [] }),
    );
    globalThis.history.replaceState(null, "", "/workflows");

    render(App);

    expect(
      await screen.findByRole("heading", { name: "Workflows" }),
    ).toBeInTheDocument();
  });

  test("the root opens the editor", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({ message: "Mega Agents backend is running" }),
      }),
    );
    globalThis.history.replaceState(null, "", "/");

    render(App);

    expect(await screen.findByLabelText("Workflow name")).toBeInTheDocument();
  });
});

describe("status display", () => {
  beforeEach(() => globalThis.history.replaceState(null, "", "/"));

  test("shows the backend status", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({ message: "Mega Agents backend is running" }),
      }),
    );

    render(App);

    expect(
      await screen.findByText("Mega Agents backend is running"),
    ).toBeInTheDocument();
  });

  test("shows a failure state when the backend cannot be reached", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("offline")));

    render(App);

    expect(
      await screen.findByText("Could not reach the Go backend"),
    ).toBeInTheDocument();
  });

  test("shows a failure state for an unsuccessful backend response", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue({ ok: false, status: 503, json: async () => ({}) }),
    );

    render(App);

    expect(
      await screen.findByText("Could not reach the Go backend"),
    ).toBeInTheDocument();
  });
});
