import { expect, test, type Locator } from "@playwright/test";
import {
  boxOf,
  canvas,
  center,
  connect,
  dragFromHandle,
  dropInto,
  enlarge,
  pressAndMove,
} from "./fixtures";

test.describe("drawing arrows by dragging", () => {
  test.use({ viewport: { width: 1600, height: 1000 } });

  let project: Locator;
  let coder: Locator;
  let reviewer: Locator;
  let tests: Locator;
  let fixer: Locator;

  // A project in the canvas corner holding two agents above a command gate
  // and a fixer agent.
  test.beforeEach(async ({ page }) => {
    await page.goto("/");
    const area = await boxOf(canvas(page));
    const palette = center(
      await boxOf(page.getByRole("button", { name: "Project" })),
    );
    await page.mouse.move(palette.x, palette.y);
    await page.mouse.down();
    await page.mouse.move(area.x + 40, area.y + 40, { steps: 10 });
    await page.mouse.up();
    project = page.getByRole("button", { name: "Project 1" });
    await enlarge(page, project, 440, 300);
    coder = await dropInto(page, "Agent", project, { x: 30, y: 60 }, "Coder");
    reviewer = await dropInto(
      page,
      "Agent",
      project,
      { x: 330, y: 60 },
      "Reviewer",
    );
    tests = await dropInto(
      page,
      "Command",
      project,
      { x: 30, y: 220 },
      "Tests",
    );
    fixer = await dropInto(page, "Agent", project, { x: 330, y: 220 }, "Fixer");
  });

  test("hovering a block shows a handle outside each side", async ({
    page,
  }) => {
    const handles = canvas(page).locator(".link-handle");
    const area = await boxOf(canvas(page));
    await page.mouse.move(area.x + area.width - 20, area.y + area.height - 20);
    await expect(handles).toHaveCount(4);

    const block = await boxOf(coder);
    const middle = center(block);
    await page.mouse.move(middle.x, middle.y);
    await expect(handles).toHaveCount(8);

    // The selected block's handles come first, then the hovered one's.
    const right = await boxOf(handles.nth(5));
    expect(right.x).toBeGreaterThanOrEqual(block.x + block.width - 1);
    expect(center(right).y).toBeCloseTo(middle.y, 0);
    await page.mouse.move(block.x + block.width + 10, middle.y, { steps: 4 });
    await expect(handles).toHaveCount(8);
  });

  test("dragging from a handle onto another agent draws an arrow", async ({
    page,
  }) => {
    await dragFromHandle(page, coder, center(await boxOf(reviewer)));
    await expect(reviewer).toHaveClass(/drop-ok/);
    await expect(canvas(page).locator(".edge-preview")).toHaveCount(1);
    await page.mouse.up();

    await expect(canvas(page).locator(".edge-line")).toHaveCount(1);
    await expect(
      page.getByRole("option", { name: "Arrow from Coder to Reviewer" }),
    ).toHaveCount(1);
    await expect(canvas(page).locator(".edge-preview")).toHaveCount(0);
  });

  test("a block on another level is refused and gets no arrow", async ({
    page,
  }) => {
    const box = await boxOf(project);
    const empty = { x: box.x + 260, y: box.y + box.height - 20 };

    await dragFromHandle(page, coder, empty, "bottom");
    await expect(project).toHaveClass(/drop-no/);
    await page.mouse.up();

    await expect(project).not.toHaveClass(/drop-no/);
    await expect(canvas(page).locator(".edge-line")).toHaveCount(0);
  });

  test("releasing on the empty canvas or pressing Escape draws nothing", async ({
    page,
  }) => {
    const area = await boxOf(canvas(page));
    await dragFromHandle(page, coder, {
      x: area.x + area.width - 40,
      y: area.y + 40,
    });
    await page.mouse.up();
    await expect(canvas(page).locator(".edge-line")).toHaveCount(0);

    await dragFromHandle(page, coder, center(await boxOf(reviewer)));
    await page.keyboard.press("Escape");
    await expect(canvas(page).locator(".edge-preview")).toHaveCount(0);
    await page.mouse.up();
    await expect(canvas(page).locator(".edge-line")).toHaveCount(0);
  });

  test("an arrow from a command asks which output it takes", async ({
    page,
  }) => {
    await dragFromHandle(page, tests, center(await boxOf(fixer)));
    await page.mouse.up();

    const menu = page.getByRole("menu", { name: "Output" });
    await expect(menu.getByRole("menuitem")).toHaveText(["passed", "failed"]);
    await expect(canvas(page).locator(".edge-line")).toHaveCount(0);
    await menu.getByRole("menuitem", { name: "failed" }).click();

    await expect(menu).toHaveCount(0);
    await expect(canvas(page).locator(".edge-port")).toHaveText("failed");
  });

  test("a clicked arrow is selected and Delete removes it", async ({
    page,
  }) => {
    await connect(page, coder, reviewer);
    const arrow = page.getByRole("option", {
      name: "Arrow from Coder to Reviewer",
    });
    const from = center(await boxOf(coder));
    const to = center(await boxOf(reviewer));

    await page.mouse.click((from.x + to.x) / 2, (from.y + to.y) / 2);
    await expect(arrow).toHaveAttribute("aria-selected", "true");
    await expect(
      page.getByRole("button", { name: "Delete arrow" }),
    ).toBeVisible();
    await page.keyboard.press("Delete");

    await expect(arrow).toHaveCount(0);
    await expect(canvas(page).locator(".edge-line")).toHaveCount(0);
    await expect(reviewer).toBeVisible();
  });

  test("dragging a selected arrow's head moves it to a third block", async ({
    page,
  }) => {
    await connect(page, coder, reviewer);
    const from = center(await boxOf(coder));
    const to = center(await boxOf(reviewer));
    await page.mouse.click((from.x + to.x) / 2, (from.y + to.y) / 2);

    await pressAndMove(
      page,
      canvas(page).locator('.edge-end[data-end="to"]'),
      center(await boxOf(fixer)),
    );
    await expect(fixer).toHaveClass(/drop-ok/);
    await page.mouse.up();

    await expect(
      page.getByRole("option", { name: "Arrow from Coder to Fixer" }),
    ).toHaveCount(1);
    await expect(canvas(page).locator(".edge-line")).toHaveCount(1);
  });

  test("a hovered arrow's end drags without selecting the arrow", async ({
    page,
  }) => {
    await connect(page, coder, reviewer);
    const ends = canvas(page).locator(".edge-end");
    const from = center(await boxOf(coder));
    const to = center(await boxOf(reviewer));
    await expect(ends).toHaveCount(0);

    await page.mouse.move((from.x + to.x) / 2, (from.y + to.y) / 2);
    await expect(ends).toHaveCount(2);
    const area = await boxOf(canvas(page));
    await page.mouse.move(area.x + area.width - 20, area.y + area.height - 20);
    await expect(ends).toHaveCount(0);

    await page.mouse.move((from.x + to.x) / 2, (from.y + to.y) / 2);
    await pressAndMove(
      page,
      canvas(page).locator('.edge-end[data-end="to"]'),
      center(await boxOf(fixer)),
    );
    await page.mouse.up();

    const moved = page.getByRole("option", {
      name: "Arrow from Coder to Fixer",
    });
    await expect(moved).toHaveCount(1);
    await expect(moved).toHaveAttribute("aria-selected", "false");
  });

  test("the preview turns red over a block the arrow cannot reach", async ({
    page,
  }) => {
    const preview = canvas(page).locator(".edge-preview");
    const box = await boxOf(project);

    await dragFromHandle(page, coder, center(await boxOf(reviewer)));
    await expect(preview).not.toHaveClass(/invalid/);
    await page.mouse.move(box.x + 260, box.y + box.height - 20, { steps: 6 });
    await expect(preview).toHaveClass(/invalid/);
    await expect(preview).toHaveAttribute(
      "marker-end",
      "url(#edge-arrowhead-invalid)",
    );
    await page.mouse.up();

    await connect(page, coder, reviewer);
    const from = center(await boxOf(coder));
    const to = center(await boxOf(reviewer));
    await page.mouse.move((from.x + to.x) / 2, (from.y + to.y) / 2);
    await pressAndMove(page, canvas(page).locator('.edge-end[data-end="to"]'), {
      x: box.x + 260,
      y: box.y + box.height - 20,
    });
    await expect(preview).toHaveClass(/invalid/);
    await page.mouse.up();
    await expect(
      page.getByRole("option", { name: "Arrow from Coder to Reviewer" }),
    ).toHaveCount(1);
  });
});
