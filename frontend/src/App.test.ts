import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/svelte";
import { afterEach, describe, expect, test, vi } from "vitest";
import App from "./App.svelte";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("status display", () => {
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
