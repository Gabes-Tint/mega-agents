import { expect, test, type Locator, type Page } from "@playwright/test";
import { boxOf, canvas, center } from "./fixtures";

// The wheel and the pan gestures: what a bare wheel does to the canvas, how
// the setting in the zoom bar flips it, and the three ways the view is moved
// now that the wheel no longer scrolls it.

const level = (page: Page) => page.getByRole("button", { name: "Reset zoom" });
const wheelSetting = (page: Page) =>
  page.getByRole("button", { name: "Wheel zooms" });

async function press(page: Page, name: string) {
  await page.getByRole("button", { name }).click();
}

// A point inside the canvas, measured from its top left corner.
async function pointIn(page: Page, dx: number, dy: number) {
  const box = await boxOf(canvas(page));
  return { x: box.x + dx, y: box.y + dy };
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

async function block(page: Page, name: string): Promise<Locator> {
  const found = page.getByRole("button", { name, exact: true });
  await expect(found).toBeVisible();
  return found;
}

function scrollOf(page: Page) {
  return canvas(page).evaluate((view) => ({
    left: view.scrollLeft,
    top: view.scrollTop,
  }));
}

// Zooms in until the content is wider and taller than the canvas, so there
// is somewhere for a scroll or a pan to go.
async function makeScrollable(page: Page) {
  await press(page, "Zoom in");
  await press(page, "Zoom in");
  await expect(level(page)).toHaveText("150%");
}

// Presses at the point, moves by the offset and releases, which is how every
// pan gesture is driven; the button and any held key are the caller's.
async function drag(
  page: Page,
  from: { x: number; y: number },
  by: { x: number; y: number },
  button: "left" | "middle" = "left",
) {
  await page.mouse.move(from.x, from.y);
  await page.mouse.down({ button });
  await page.mouse.move(from.x + by.x, from.y + by.y, { steps: 10 });
  await page.mouse.up({ button });
}

test.describe("The wheel over the canvas", () => {
  test("a bare wheel zooms toward the pointer", async ({ page }) => {
    await page.goto("/");
    await dropAt(page, "Project", await pointIn(page, 200, 180));
    const project = await block(page, "Project 1");
    const before = await boxOf(project);

    // The pointer rests on the block's top left corner, which is where the
    // content under it has to stay while the canvas zooms in.
    await page.mouse.move(before.x, before.y);
    await page.mouse.wheel(0, -120);

    await expect(level(page)).not.toHaveText("100%");
    const after = await boxOf(project);
    expect(after.width).toBeGreaterThan(before.width);
    expect(after.x).toBeCloseTo(before.x, 0);
    expect(after.y).toBeCloseTo(before.y, 0);
  });

  test("shift and the wheel scrolls sideways instead of zooming", async ({
    page,
  }) => {
    await page.goto("/");
    await makeScrollable(page);
    const before = await scrollOf(page);
    const at = await pointIn(page, 200, 150);
    await page.mouse.move(at.x, at.y);

    await page.keyboard.down("Shift");
    await page.mouse.wheel(0, 200);
    await page.keyboard.up("Shift");

    await expect(level(page)).toHaveText("150%");
    await expect
      .poll(async () => (await scrollOf(page)).left)
      .toBeGreaterThan(before.left);
    expect((await scrollOf(page)).top).toBe(before.top);
  });

  test("the setting flips what the wheel does, and survives a reload", async ({
    page,
  }) => {
    await page.goto("/");
    await makeScrollable(page);
    const before = await scrollOf(page);
    const at = await pointIn(page, 200, 150);
    await page.mouse.move(at.x, at.y);

    await expect(wheelSetting(page)).toHaveAttribute("aria-pressed", "true");
    await wheelSetting(page).click();
    await expect(wheelSetting(page)).toHaveAttribute("aria-pressed", "false");

    await page.mouse.move(at.x, at.y);
    await page.mouse.wheel(0, 200);
    await expect(level(page)).toHaveText("150%");
    await expect
      .poll(async () => (await scrollOf(page)).top)
      .toBeGreaterThan(before.top);

    await page.reload();

    await expect(wheelSetting(page)).toHaveAttribute("aria-pressed", "false");
    await page.mouse.move(at.x, at.y);
    await page.mouse.wheel(0, 200);
    await expect(level(page)).toHaveText("150%");

    // And turning it back on zooms again.
    await wheelSetting(page).click();
    await page.mouse.move(at.x, at.y);
    await page.mouse.wheel(0, 200);
    await expect(level(page)).not.toHaveText("150%");
  });
});

test.describe("Panning the canvas", () => {
  test("dragging the background moves the view, not the blocks", async ({
    page,
  }) => {
    await page.goto("/");
    await dropAt(page, "Project", await pointIn(page, 120, 100));
    const project = await block(page, "Project 1");
    await makeScrollable(page);
    const before = await boxOf(project);
    const from = await scrollOf(page);

    await drag(page, await pointIn(page, 420, 300), { x: -150, y: -100 });

    const scroll = await scrollOf(page);
    expect(scroll.left - from.left).toBeCloseTo(150, 0);
    expect(scroll.top - from.top).toBeCloseTo(100, 0);
    // The block travelled with the view, so it never moved on the canvas.
    const after = await boxOf(project);
    expect(after.x).toBeCloseTo(before.x - 150, 0);
    expect(after.y).toBeCloseTo(before.y - 100, 0);
  });

  test("the middle button pans from anywhere, blocks included", async ({
    page,
  }) => {
    await page.goto("/");
    await dropAt(page, "Project", await pointIn(page, 120, 100));
    const project = await block(page, "Project 1");
    await makeScrollable(page);
    const before = await boxOf(project);
    const from = await scrollOf(page);

    await drag(page, center(before), { x: -120, y: -80 }, "middle");

    expect((await scrollOf(page)).left - from.left).toBeCloseTo(120, 0);
    expect((await boxOf(project)).x).toBeCloseTo(before.x - 120, 0);
  });

  test("space and a drag pans over a block without moving it", async ({
    page,
  }) => {
    await page.goto("/");
    await dropAt(page, "Project", await pointIn(page, 120, 100));
    const project = await block(page, "Project 1");
    await makeScrollable(page);
    const before = await boxOf(project);
    const from = await scrollOf(page);

    await page.keyboard.down(" ");
    await expect(canvas(page)).toHaveClass(/grab/);
    await drag(page, center(before), { x: -120, y: -80 });
    await page.keyboard.up(" ");

    expect((await scrollOf(page)).left - from.left).toBeCloseTo(120, 0);
    expect((await boxOf(project)).x).toBeCloseTo(before.x - 120, 0);
    await expect(canvas(page)).not.toHaveClass(/grab/);
  });
});
