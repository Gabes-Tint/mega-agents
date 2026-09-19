import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/svelte";
import { afterEach, describe, expect, test, vi } from "vitest";
import Schedules from "./Schedules.svelte";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const workflows = [
  {
    name: "nightly-triage",
    updatedAt: "2026-09-17T10:00:00Z",
    schedule: "0 9 * * 1-5",
    nextRuns: ["2026-09-18T09:00:00Z", "2026-09-21T09:00:00Z"],
  },
  { name: "on-demand", updatedAt: "2026-09-16T08:30:00Z" },
];

// answers routes the page's requests to canned replies, and records the
// ones it sends.
function answers(
  routes: Record<
    string,
    { ok?: boolean; status?: number; body?: unknown; text?: string }
  >,
) {
  const sent: { url: string; method: string; body: string }[] = [];
  const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
    sent.push({
      url,
      method: init?.method ?? "GET",
      body: String(init?.body ?? ""),
    });
    // The most specific route wins, so /api/workflows does not answer for
    // /api/workflows/<name>/schedule.
    const match = Object.keys(routes)
      .filter((route) => url.startsWith(route))
      .sort((one, other) => other.length - one.length)[0];
    const reply = match
      ? routes[match]!
      : { ok: false, status: 404, text: "not found" };
    return {
      ok: reply.ok ?? true,
      status: reply.status ?? 200,
      json: async () => reply.body,
      text: async () => reply.text ?? "",
    } as Response;
  });
  vi.stubGlobal("fetch", fetchMock);
  return sent;
}

function renderPage() {
  return render(Schedules, { previewDelayMs: 0 });
}

describe("the workflows page", () => {
  test("lists every saved workflow and the schedule it runs on", async () => {
    answers({ "/api/workflows": { body: workflows } });

    renderPage();

    expect(await screen.findByText("nightly-triage")).toBeInTheDocument();
    expect(screen.getByText("on-demand")).toBeInTheDocument();
    const scheduled = screen.getByLabelText("Schedule for nightly-triage");
    expect(scheduled).toHaveValue("0 9 * * 1-5");
    expect(screen.getByLabelText("Schedule for on-demand")).toHaveValue("");
  });

  test("a workflow with a schedule shows the runs it has coming", async () => {
    answers({ "/api/workflows": { body: workflows } });

    const { container } = renderPage();

    await screen.findByText("nightly-triage");
    await waitFor(() =>
      expect(container.querySelectorAll("time")).toHaveLength(2),
    );
    const shown = [...container.querySelectorAll("time")].map((time) =>
      time.getAttribute("datetime"),
    );
    expect(shown).toEqual(workflows[0]!.nextRuns);
  });

  test("a workflow with no schedule says it only runs when asked", async () => {
    answers({ "/api/workflows": { body: workflows } });

    renderPage();

    await screen.findByText("on-demand");
    expect(screen.getByText(/runs when you ask/i)).toBeInTheDocument();
  });

  test("typing an expression shows the next runs it would start", async () => {
    const sent = answers({
      "/api/workflows": { body: workflows },
      "/api/schedule/preview": {
        body: {
          schedule: "*/30 * * * *",
          nextRuns: [
            "2026-09-18T09:30:00Z",
            "2026-09-18T10:00:00Z",
            "2026-09-18T10:30:00Z",
          ],
        },
      },
    });

    const { container } = renderPage();

    await screen.findByText("on-demand");
    const field = screen.getByLabelText("Schedule for on-demand");
    await fireEvent.input(field, { target: { value: "*/30 * * * *" } });

    await waitFor(() =>
      expect(
        [...container.querySelectorAll("time")].map((time) =>
          time.getAttribute("datetime"),
        ),
      ).toContain("2026-09-18T10:30:00Z"),
    );
    expect(
      sent.some((request) =>
        request.url.includes("/api/schedule/preview?schedule=*%2F30+*+*+*+*"),
      ),
    ).toBe(true);
  });

  test("an expression that cannot run is explained and cannot be saved", async () => {
    answers({
      "/api/workflows": { body: workflows },
      "/api/schedule/preview": {
        ok: false,
        status: 400,
        text: "a schedule has five fields, minute hour day-of-month month day-of-week, and this one has 4\n",
      },
    });

    renderPage();

    await screen.findByText("on-demand");
    await fireEvent.input(screen.getByLabelText("Schedule for on-demand"), {
      target: { value: "0 9 * *" },
    });

    expect(await screen.findByText(/five fields/)).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Save schedule for on-demand" }),
    ).toBeDisabled();
  });

  test("saving puts the workflow on the schedule", async () => {
    const sent = answers({
      "/api/workflows": { body: workflows },
      "/api/schedule/preview": {
        body: { schedule: "@daily", nextRuns: ["2026-09-19T00:00:00Z"] },
      },
      "/api/workflows/on-demand/schedule": {
        body: {
          name: "on-demand",
          updatedAt: "2026-09-18T09:00:00Z",
          schedule: "@daily",
          nextRuns: ["2026-09-19T00:00:00Z"],
        },
      },
    });

    renderPage();

    await screen.findByText("on-demand");
    await fireEvent.input(screen.getByLabelText("Schedule for on-demand"), {
      target: { value: "@daily" },
    });
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Save schedule for on-demand" }),
      ).toBeEnabled(),
    );
    await fireEvent.click(
      screen.getByRole("button", { name: "Save schedule for on-demand" }),
    );

    await waitFor(() => {
      const save = sent.find((request) => request.method === "PUT");
      expect(save?.url).toBe("/api/workflows/on-demand/schedule");
      expect(JSON.parse(save?.body ?? "{}")).toEqual({ schedule: "@daily" });
    });
    expect(await screen.findByText(/on the schedule/i)).toBeInTheDocument();
  });

  test("clearing a schedule takes the workflow off it", async () => {
    const sent = answers({
      "/api/workflows": { body: workflows },
      "/api/workflows/nightly-triage/schedule": {
        body: { name: "nightly-triage", updatedAt: "2026-09-18T09:00:00Z" },
      },
    });

    renderPage();

    await screen.findByText("nightly-triage");
    await fireEvent.click(
      screen.getByRole("button", { name: "Clear schedule for nightly-triage" }),
    );

    await waitFor(() => {
      const save = sent.find((request) => request.method === "PUT");
      expect(save?.url).toBe("/api/workflows/nightly-triage/schedule");
      expect(JSON.parse(save?.body ?? "{}")).toEqual({ schedule: "" });
    });
    expect(screen.getByLabelText("Schedule for nightly-triage")).toHaveValue(
      "",
    );
  });

  test("a save the server refuses is reported and leaves the field alone", async () => {
    answers({
      "/api/workflows": { body: workflows },
      "/api/schedule/preview": {
        body: { schedule: "@daily", nextRuns: ["2026-09-19T00:00:00Z"] },
      },
      "/api/workflows/on-demand/schedule": {
        ok: false,
        status: 400,
        text: "workflow on-demand not found\n",
      },
    });

    renderPage();

    await screen.findByText("on-demand");
    await fireEvent.input(screen.getByLabelText("Schedule for on-demand"), {
      target: { value: "@daily" },
    });
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Save schedule for on-demand" }),
      ).toBeEnabled(),
    );
    await fireEvent.click(
      screen.getByRole("button", { name: "Save schedule for on-demand" }),
    );

    expect(await screen.findByText(/not found/)).toBeInTheDocument();
    expect(screen.getByLabelText("Schedule for on-demand")).toHaveValue(
      "@daily",
    );
  });

  test("with nothing saved the page says where workflows come from", async () => {
    answers({ "/api/workflows": { body: [] } });

    renderPage();

    expect(await screen.findByText(/no saved workflows/i)).toBeInTheDocument();
  });

  test("a workflow opens in the editor from its name", async () => {
    answers({ "/api/workflows": { body: workflows } });

    renderPage();

    const link = await screen.findByRole("link", { name: "nightly-triage" });
    expect(link).toHaveAttribute("href", "/workflows/nightly-triage");
  });
});
