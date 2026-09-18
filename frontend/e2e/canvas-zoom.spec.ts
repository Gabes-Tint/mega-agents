import { expect, test, type Locator, type Page } from "@playwright/test";
import { boxOf, canvas, center, connect, mouseDrag } from "./fixtures";

// Zooming the canvas: the gestures and controls that change the scale, and
// the interactions that have to keep landing under the pointer once they do.

const level = (page: Page) => page.getByRole("button", { name: "Reset zoom" });

async function press(page: Page, name: string) {
  await page.getByRole("button", { name }).click();
}

// Steps down to half size, which the buttons reach in three steps.
async function halve(page: Page) {
  for (let step = 0; step < 3; step++) await press(page, "Zoom out");
  await expect(level(page)).toHaveText("50%");
}

// Drags a palette component onto the canvas and releases it at the point.
async function dropAt(
  page: Page,
  palette: string,
  to: { x: number; y: number },
) {
  const from = center(
    await boxOf(page.getByRole("button", { name: palette, exact: true })),
  );
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  await page.mouse.move(to.x, to.y, { steps: 10 });
  await page.mouse.up();
}

// A point inside the canvas, measured from its top left corner.
async function pointIn(page: Page, dx: number, dy: number) {
  const box = await boxOf(canvas(page));
  return { x: box.x + dx, y: box.y + dy };
}

async function block(page: Page, name: string): Promise<Locator> {
  const found = page.getByRole("button", { name, exact: true });
  await expect(found).toBeVisible();
  return found;
}

test.describe("Zooming the canvas", () => {
  test("the buttons and the keyboard scale the content and reset it", async ({
    page,
  }) => {
    await page.goto("/");
    await dropAt(page, "Project", await pointIn(page, 120, 120));
    const project = await block(page, "Project 1");
    const full = await boxOf(project);

    await press(page, "Zoom in");
    await expect(level(page)).toHaveText("125%");
    expect((await boxOf(project)).width).toBeCloseTo(full.width * 1.25, 0);

    await press(page, "Zoom out");
    await press(page, "Zoom out");
    await expect(level(page)).toHaveText("75%");
    expect((await boxOf(project)).width).toBeCloseTo(full.width * 0.75, 0);

    await page.keyboard.press("Control+0");
    await expect(level(page)).toHaveText("100%");
    expect((await boxOf(project)).width).toBeCloseTo(full.width, 0);
  });

  test("Ctrl and the wheel zooms toward the pointer", async ({ page }) => {
    await page.goto("/");
    await dropAt(page, "Project", await pointIn(page, 200, 180));
    const project = await block(page, "Project 1");
    const before = await boxOf(project);

    // The pointer rests on the block's top left corner, which is where the
    // content under it has to stay while the canvas zooms in.
    await page.mouse.move(before.x, before.y);
    await page.keyboard.down("Control");
    await page.mouse.wheel(0, -120);
    await page.keyboard.up("Control");

    await expect(level(page)).not.toHaveText("100%");
    const after = await boxOf(project);
    expect(after.width).toBeGreaterThan(before.width);
    expect(after.x).toBeCloseTo(before.x, 0);
    expect(after.y).toBeCloseTo(before.y, 0);
  });

  test("a block dropped at half size lands under the pointer", async ({
    page,
  }) => {
    await page.goto("/");
    await halve(page);
    const at = await pointIn(page, 260, 200);

    await dropAt(page, "Project", at);

    const dropped = await boxOf(await block(page, "Project 1"));
    expect(dropped.x).toBeCloseTo(at.x, 0);
    expect(dropped.y).toBeCloseTo(at.y, 0);
  });

  test("a block moved at half size follows the pointer", async ({ page }) => {
    await page.goto("/");
    await dropAt(page, "Project", await pointIn(page, 150, 120));
    const project = await block(page, "Project 1");
    await halve(page);
    const before = await boxOf(project);
    const grab = { x: before.x + 20, y: before.y + 15 };
    const to = { x: grab.x + 180, y: grab.y + 90 };

    await page.mouse.move(grab.x, grab.y);
    await page.mouse.down();
    for (let step = 1; step <= 10; step++) {
      await page.mouse.move(
        grab.x + ((to.x - grab.x) * step) / 10,
        grab.y + ((to.y - grab.y) * step) / 10,
      );
      await page.waitForTimeout(20);
    }
    await page.mouse.up();

    // The block keeps the point it was grabbed by under the pointer, so it
    // travels exactly as far on screen as the pointer did.
    const after = await boxOf(project);
    expect(after.x).toBeCloseTo(before.x + 180, 0);
    expect(after.y).toBeCloseTo(before.y + 90, 0);
    expect(after.width).toBeCloseTo(before.width, 0);
  });

  test("a block resized at half size follows the pointer", async ({ page }) => {
    await page.goto("/");
    await dropAt(page, "Project", await pointIn(page, 150, 120));
    const project = await block(page, "Project 1");
    await halve(page);
    const before = await boxOf(project);
    const grip = center(await boxOf(project.locator(".resize-handle")));

    await page.mouse.move(grip.x, grip.y);
    await page.mouse.down();
    await page.mouse.move(grip.x + 120, grip.y + 60, { steps: 8 });
    await page.mouse.up();

    const after = await boxOf(project);
    expect(after.width).toBeCloseTo(before.width + 120, 0);
    expect(after.height).toBeCloseTo(before.height + 60, 0);
  });

  test("an arrow drawn at half size reaches the block under the pointer", async ({
    page,
  }) => {
    await page.goto("/");
    await dropAt(page, "Project", await pointIn(page, 120, 120));
    await dropAt(page, "Project", await pointIn(page, 520, 320));
    await halve(page);

    await connect(
      page,
      await block(page, "Project 1"),
      await block(page, "Project 2"),
    );

    await expect(
      page.getByRole("option", { name: "Arrow from Project 1 to Project 2" }),
    ).toBeVisible();
  });

  test("fit to content frames the whole Fit_ development flow", async ({
    page,
  }) => {
    await page.goto("/");
    await press(page, "Templates…");
    await page.getByRole("button", { name: "Fit_ development flow" }).click();
    await expect(
      page.getByRole("button", { name: "Solver implements", exact: true }),
    ).toBeVisible();

    await press(page, "Fit to content");

    const percentage = Number((await level(page).innerText()).replace("%", ""));
    expect(percentage).toBeLessThan(100);
    const view = await boxOf(canvas(page));
    const blocks = canvas(page).locator(".node");
    const count = await blocks.count();
    expect(count).toBeGreaterThan(40);
    for (let index = 0; index < count; index++) {
      const box = await boxOf(blocks.nth(index));
      expect(box.x).toBeGreaterThanOrEqual(view.x - 1);
      expect(box.y).toBeGreaterThanOrEqual(view.y - 1);
      expect(box.x + box.width).toBeLessThanOrEqual(view.x + view.width + 1);
      expect(box.y + box.height).toBeLessThanOrEqual(view.y + view.height + 1);
    }
  });

  test("the controls step aside for a block dropped under them", async ({
    page,
  }) => {
    await page.goto("/");
    const middle = center(
      await boxOf(page.getByRole("group", { name: "Zoom" })),
    );
    const bar = { x: Math.round(middle.x), y: Math.round(middle.y) };

    await dropAt(page, "Project", bar);

    const dropped = await boxOf(await block(page, "Project 1"));
    expect(dropped.x).toBeCloseTo(bar.x, 0);
    expect(dropped.y).toBeCloseTo(bar.y, 0);
  });

  test("the zoom level survives a reload", async ({ page }) => {
    await page.goto("/");
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Project" }),
      canvas(page),
    );
    const project = await block(page, "Project 1");
    await halve(page);
    const zoomed = await boxOf(project);

    await page.reload();

    await expect(level(page)).toHaveText("50%");
    const restored = await boxOf(await block(page, "Project 1"));
    expect(restored.width).toBeCloseTo(zoomed.width, 0);
  });
});
