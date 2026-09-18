import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { expect, test, type Locator, type Page } from "@playwright/test";
import {
  canvas,
  connect,
  dropInto,
  enlarge,
  mouseDrag,
  selectBlock,
} from "./fixtures";

const markOf = (block: Locator, kind: string) =>
  block.locator(`[data-mark="${kind}"]`);

const pausedPanel = (page: Page) =>
  page.getByRole("region", { name: "Paused at a breakpoint" });

const runResult = (page: Page) =>
  page.getByRole("region", { name: "Run result" });

// A run reaching a breakpoint waits for as long as it takes, but the blocks
// before it still run an agent, which the fakes make quick.
const RUN = { timeout: 20_000 };

test.describe("Breakpoints in the editor", () => {
  test.use({ viewport: { width: 1440, height: 1000 } });

  let folder: string;

  test.beforeEach(async ({ page }) => {
    folder = mkdtempSync(join(tmpdir(), "mega-agents-breakpoints-"));
    await page.goto("/");
    await page.evaluate(() => localStorage.clear());
    await page.reload();
  });

  test.afterEach(() => {
    rmSync(folder, { recursive: true, force: true });
  });

  // A project holding a coder the reviewer reads after.
  async function coderAndReviewer(page: Page) {
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Project" }),
      canvas(page),
    );
    const project = page.getByRole("button", { name: "Project 1" });
    await page.getByLabel("Path").fill(folder);
    await enlarge(page, project, 620, 300);
    const coder = await dropInto(
      page,
      "Agent",
      project,
      { x: 60, y: 70 },
      "Coder",
    );
    await page.getByLabel("Starting point").check();
    await page.getByLabel("Prompt", { exact: true }).fill("Write the fix");
    const reviewer = await dropInto(
      page,
      "Agent",
      project,
      { x: 380, y: 70 },
      "Reviewer",
    );
    await page.getByLabel("Prompt", { exact: true }).fill("Review {{result}}");
    await connect(page, coder, reviewer);
    return { coder, reviewer };
  }

  async function armBreakpoint(page: Page, block: Locator) {
    await selectBlock(block);
    await page.getByLabel("Breakpoint").check();
  }

  test("a block wears the breakpoint dot, the run stops in front of it, and Continue takes it on", async ({
    page,
  }) => {
    const { coder, reviewer } = await coderAndReviewer(page);

    await armBreakpoint(page, coder);

    await expect(markOf(coder, "breakpoint")).toBeVisible();
    // The dot sits in the header beside the start flag, not over the name.
    await expect(markOf(coder, "start")).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Coder", exact: true }),
    ).toHaveCount(1);

    await page.getByRole("button", { name: "Run flow" }).click();

    await expect(pausedPanel(page)).toContainText("Paused before Coder", RUN);
    await expect(coder).toHaveClass(/status-paused/);
    await expect(markOf(coder, "paused")).toBeVisible();
    // The pause shows the text the block is about to run.
    await expect(page.getByLabel("Prompt for Coder in this run")).toContainText(
      "Write the fix",
    );

    await page.getByRole("button", { name: "Continue Coder" }).click();

    await expect(runResult(page)).toContainText("Run succeeded", RUN);
    await expect(pausedPanel(page)).toHaveCount(0);
    await expect(markOf(reviewer, "succeeded")).toBeVisible();
  });

  test("Step runs the block and stops before the next one, showing what reached it", async ({
    page,
  }) => {
    const { coder } = await coderAndReviewer(page);
    await armBreakpoint(page, coder);
    await page.getByRole("button", { name: "Run flow" }).click();
    await expect(pausedPanel(page)).toContainText("Paused before Coder", RUN);

    await page.getByRole("button", { name: "Step Coder" }).click();

    await expect(pausedPanel(page)).toContainText(
      "Paused before Reviewer",
      RUN,
    );
    await expect(markOf(coder, "succeeded")).toBeVisible();
    // What the coder replied is both the value that arrived and the
    // placeholder filled in the prompt the reviewer would run.
    await expect(pausedPanel(page)).toContainText("worked in");
    await expect(
      page.getByLabel("Prompt for Reviewer in this run"),
    ).toContainText(/^Review .*Write the fix/);

    await page.getByRole("button", { name: "Continue Reviewer" }).click();

    await expect(runResult(page)).toContainText("Run succeeded", RUN);
  });

  test("Skip settles the block, and what follows it, without running either", async ({
    page,
  }) => {
    const { coder, reviewer } = await coderAndReviewer(page);
    await armBreakpoint(page, coder);
    await page.getByRole("button", { name: "Run flow" }).click();
    await expect(pausedPanel(page)).toContainText("Paused before Coder", RUN);

    await page.getByRole("button", { name: "Skip Coder" }).click();

    await expect(markOf(coder, "skipped")).toBeVisible(RUN);
    await expect(markOf(reviewer, "skipped")).toBeVisible();
    await expect(runResult(page)).toContainText("skipped at a breakpoint");
  });

  test("an edited prompt runs in this run only and the saved workflow keeps its own", async ({
    page,
  }) => {
    const { coder } = await coderAndReviewer(page);
    await armBreakpoint(page, coder);
    const name = `breakpoints-${Date.now()}`;
    await page.getByLabel("Workflow name").fill(name);
    await page.getByRole("button", { name: "Save" }).click();
    await expect(
      page.getByRole("status", { name: "Workflow file" }),
    ).toHaveText(`Saved as ${name}`);
    await page.getByRole("button", { name: "Run flow" }).click();
    await expect(pausedPanel(page)).toContainText("Paused before Coder", RUN);

    await page
      .getByLabel("Prompt for Coder in this run")
      .fill("Write the smallest fix");

    // The panel says the text belongs to the run, and offers the way back.
    await expect(pausedPanel(page)).toContainText("this run only");
    await expect(
      page.getByRole("button", { name: "Undo the edit to Coder" }),
    ).toBeVisible();
    await page.getByRole("button", { name: "Continue Coder" }).click();

    // The fake agent repeats what it was asked, so the run shows what ran.
    await expect(runResult(page)).toContainText("Write the smallest fix", RUN);
    await expect(runResult(page)).toContainText("Run succeeded");

    // The workflow on disk still holds the block's own prompt. Saving moved
    // the address to the workflow, so the fresh editor starts at "/" again.
    await page.goto("/");
    await page.evaluate(() => localStorage.clear());
    await page.reload();
    await page.getByRole("button", { name: "Open…" }).click();
    await page.getByRole("button", { name, exact: true }).click();
    await selectBlock(page.getByRole("button", { name: "Coder", exact: true }));

    await expect(page.getByLabel("Prompt", { exact: true })).toContainText(
      "Write the fix",
    );
    await expect(page.getByLabel("Prompt", { exact: true })).not.toContainText(
      "smallest",
    );
    // The breakpoint is a property of the block, so it came back too.
    await expect(page.getByLabel("Breakpoint")).toBeChecked();
  });

  test("two blocks paused at once are taken on one at a time", async ({
    page,
  }) => {
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Project" }),
      canvas(page),
    );
    const project = page.getByRole("button", { name: "Project 1" });
    await page.getByLabel("Path").fill(folder);
    await enlarge(page, project, 620, 420);
    const coder = await dropInto(
      page,
      "Agent",
      project,
      { x: 50, y: 150 },
      "Coder",
    );
    await page.getByLabel("Starting point").check();
    await page.getByLabel("Prompt", { exact: true }).fill("Write the fix");
    const reviewer = await dropInto(
      page,
      "Agent",
      project,
      { x: 380, y: 60 },
      "Reviewer",
    );
    await page.getByLabel("Prompt", { exact: true }).fill("Review it");
    await page.getByLabel("Breakpoint").check();
    const tester = await dropInto(
      page,
      "Agent",
      project,
      { x: 380, y: 250 },
      "Tester",
    );
    await page.getByLabel("Prompt", { exact: true }).fill("Test it");
    await page.getByLabel("Breakpoint").check();
    await connect(page, coder, reviewer);
    await connect(page, coder, tester);

    await page.getByRole("button", { name: "Run flow" }).click();

    await expect(pausedPanel(page)).toContainText(
      "Paused before Reviewer",
      RUN,
    );
    await expect(pausedPanel(page)).toContainText("Paused before Tester");

    // Each card resumes its own block, so neither branch has to guess.
    await page.getByRole("button", { name: "Skip Tester" }).click();

    await expect(pausedPanel(page)).not.toContainText("Paused before Tester", {
      timeout: 10_000,
    });
    await expect(pausedPanel(page)).toContainText("Paused before Reviewer");
    await expect(markOf(tester, "skipped")).toBeVisible();

    await page.getByRole("button", { name: "Continue Reviewer" }).click();

    await expect(runResult(page)).toContainText("Run succeeded", RUN);
    await expect(markOf(reviewer, "succeeded")).toBeVisible();
  });

  test("a paused run can still be cancelled", async ({ page }) => {
    const { coder } = await coderAndReviewer(page);
    await armBreakpoint(page, coder);
    await page.getByRole("button", { name: "Run flow" }).click();
    await expect(pausedPanel(page)).toContainText("Paused before Coder", RUN);

    await page.getByRole("button", { name: "Cancel run" }).click();

    await expect(runResult(page)).toContainText("Run cancelled", RUN);
    await expect(pausedPanel(page)).toHaveCount(0);
  });

  const animationOf = (element: Locator) =>
    element.evaluate((node) => getComputedStyle(node).animationName);

  test("the paused block breathes to be noticed", async ({ page }) => {
    const { coder } = await coderAndReviewer(page);
    await armBreakpoint(page, coder);
    await page.getByRole("button", { name: "Run flow" }).click();
    await expect(pausedPanel(page)).toContainText("Paused before Coder", RUN);

    expect(await animationOf(coder)).not.toBe("none");
    expect(await animationOf(markOf(coder, "paused").locator("svg"))).not.toBe(
      "none",
    );
  });

  test.describe("with motion turned down", () => {
    test.use({ reducedMotion: "reduce" });

    test("the paused block keeps its mark and ring, and holds still", async ({
      page,
    }) => {
      const { coder } = await coderAndReviewer(page);
      await armBreakpoint(page, coder);
      await page.getByRole("button", { name: "Run flow" }).click();
      await expect(pausedPanel(page)).toContainText("Paused before Coder", RUN);

      await expect(markOf(coder, "paused")).toBeVisible();
      await expect(coder).toHaveClass(/status-paused/);
      expect(await animationOf(coder)).toBe("none");
      expect(await animationOf(markOf(coder, "paused").locator("svg"))).toBe(
        "none",
      );
    });
  });
});
