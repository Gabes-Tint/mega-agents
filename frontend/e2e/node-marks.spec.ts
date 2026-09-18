import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { expect, test, type Locator, type Page } from "@playwright/test";
import { canvas, dropInto, enlarge, mouseDrag, selectBlock } from "./fixtures";

function center(box: { x: number; y: number; width: number; height: number }) {
  return { x: box.x + box.width / 2, y: box.y + box.height / 2 };
}

async function boxOf(locator: Locator) {
  const box = await locator.boundingBox();
  if (!box) throw new Error("not visible");
  return box;
}

const markOf = (block: Locator, kind: string) =>
  block.locator(`[data-mark="${kind}"]`);

// Draws an arrow by dragging from the source's right handle onto the target's
// header, which is the part of a container the blocks inside it never cover.
async function drawArrow(page: Page, from: Locator, to: Locator) {
  await selectBlock(from);
  const handle = center(
    await boxOf(canvas(page).locator('.link-handle[data-side="right"]')),
  );
  const target = center(await boxOf(to.locator(".node-title")));
  await page.mouse.move(handle.x, handle.y);
  await page.mouse.down();
  await page.mouse.move(target.x, target.y, { steps: 12 });
  await page.mouse.up();
}

test.describe("the flags that say where a run starts and ends", () => {
  test.use({ viewport: { width: 1600, height: 1100 } });

  let project: Locator;
  let coder: Locator;
  let reviewer: Locator;

  // A roomy project holding two agents, the first of them the flow's start.
  test.beforeEach(async ({ page }) => {
    await page.goto("/");
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Project" }),
      canvas(page),
    );
    project = page.getByRole("button", { name: "Project 1" });
    await enlarge(page, project, 560, 480);
    coder = await dropInto(page, "Agent", project, { x: 30, y: 60 }, "Coder");
    reviewer = await dropInto(
      page,
      "Agent",
      project,
      { x: 330, y: 60 },
      "Reviewer",
    );
    await selectBlock(coder);
    await page.getByLabel("Starting point").check();
  });

  test("a starting block that leads nowhere waves both flags", async ({
    page,
  }) => {
    await expect(markOf(coder, "start")).toBeVisible();
    await expect(markOf(coder, "end")).toBeVisible();

    // The two sit side by side inside the header rather than on top of one
    // another or below the title.
    const header = await boxOf(coder.locator(".node-title"));
    const start = await boxOf(markOf(coder, "start"));
    const finish = await boxOf(markOf(coder, "end"));
    expect(finish.x).toBeGreaterThanOrEqual(start.x + start.width);
    expect(finish.x + finish.width).toBeLessThanOrEqual(
      header.x + header.width,
    );
    expect(start.y).toBeGreaterThanOrEqual(header.y);
    expect(finish.y + finish.height).toBeLessThanOrEqual(
      header.y + header.height,
    );

    // A container does no work of its own, and no start leads to the second
    // agent, so neither can end the flow.
    await expect(markOf(project, "end")).toHaveCount(0);
    await expect(markOf(reviewer, "end")).toHaveCount(0);
    // The flags are drawings, so the blocks keep their own names.
    await expect(
      page.getByRole("button", { name: "Coder", exact: true }),
    ).toHaveCount(1);
  });

  test("an arrow out of a block hands the finish flag on, and losing it takes it back", async ({
    page,
  }) => {
    await drawArrow(page, coder, reviewer);

    await expect(canvas(page).locator(".edge-line")).toHaveCount(1);
    await expect(markOf(coder, "end")).toHaveCount(0);
    await expect(markOf(coder, "start")).toHaveCount(1);
    await expect(markOf(reviewer, "end")).toHaveCount(1);
    await expect(markOf(reviewer, "start")).toHaveCount(0);

    const from = center(await boxOf(coder));
    const to = center(await boxOf(reviewer));
    await page.mouse.click((from.x + to.x) / 2, (from.y + to.y) / 2);
    await page.keyboard.press("Delete");

    await expect(canvas(page).locator(".edge-line")).toHaveCount(0);
    await expect(markOf(coder, "end")).toHaveCount(1);
    await expect(markOf(reviewer, "end")).toHaveCount(0);
  });

  test("a loop ends the flow; the blocks it repeats do not", async ({
    page,
  }) => {
    const loop = await dropInto(
      page,
      "Loop",
      project,
      { x: 30, y: 220 },
      "Until green",
    );
    const gate = await dropInto(
      page,
      "Command",
      loop,
      { x: 60, y: 90 },
      "Gate",
    );
    await drawArrow(page, coder, loop);

    await expect(canvas(page).locator(".edge-line")).toHaveCount(1);
    await expect(markOf(loop, "end")).toHaveCount(1);
    await expect(markOf(gate, "end")).toHaveCount(0);
    await expect(markOf(coder, "end")).toHaveCount(0);
  });
});

test.describe("the mark a block carries during a run", () => {
  let folder: string;

  test.beforeEach(async ({ page }) => {
    folder = mkdtempSync(join(tmpdir(), "mega-agents-marks-"));
    await page.goto("/");
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Project" }),
      canvas(page),
    );
    await page.getByLabel("Path").fill(folder);
  });

  test.afterEach(() => {
    rmSync(folder, { recursive: true, force: true });
  });

  // Starts a flow of one agent whose turn takes long enough to watch.
  async function runOneAgent(page: Page): Promise<Locator> {
    const agent = await dropInto(
      page,
      "Agent",
      page.getByRole("button", { name: "Project 1" }),
      { x: 30, y: 40 },
      "Thinker",
    );
    await page.getByLabel("Starting point").check();
    await page.getByLabel("Prompt").fill("[wait] think a little");
    await page.getByRole("button", { name: "Run flow" }).click();
    return agent;
  }

  const animationOf = (mark: Locator) =>
    mark.locator("svg").evaluate((svg) => getComputedStyle(svg).animationName);

  test("the block spins while it runs and is ticked off when it succeeds", async ({
    page,
  }) => {
    const agent = await runOneAgent(page);

    const running = markOf(agent, "running");
    await expect(running).toBeVisible();
    expect(await animationOf(running)).not.toBe("none");
    // A reader hears the state as the block's description, so the mark does
    // not push its way into the block's name.
    await expect(running).toHaveText("Running");
    await expect(agent).toHaveAttribute("aria-describedby", /^status-/);
    await expect(
      page.getByRole("button", { name: "Thinker", exact: true }),
    ).toHaveCount(1);

    await expect(
      page.getByRole("region", { name: "Run result" }),
    ).toContainText("Run succeeded", { timeout: 15_000 });
    await expect(markOf(agent, "succeeded")).toBeVisible();
    await expect(running).toHaveCount(0);
  });

  test.describe("with motion turned down", () => {
    test.use({ reducedMotion: "reduce" });

    test("the running block keeps a still mark rather than none", async ({
      page,
    }) => {
      const agent = await runOneAgent(page);

      const running = markOf(agent, "running");
      await expect(running).toBeVisible();
      expect(await animationOf(running)).toBe("none");
    });
  });
});
