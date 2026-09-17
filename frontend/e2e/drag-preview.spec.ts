import { expect, test, type Page } from "@playwright/test";
import { canvas, mouseDrag } from "./fixtures";

// Presses on the source and moves the pointer to (x, y) inside the canvas
// without releasing, so the drag is still in flight.
async function dragTo(
  page: Page,
  from: { x: number; y: number },
  x: number,
  y: number,
) {
  const box = await canvas(page).boundingBox();
  if (!box) throw new Error("canvas is not visible");
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  await page.mouse.move(box.x + x, box.y + y, { steps: 10 });
  return { x: box.x + x, y: box.y + y };
}

async function centerOf(page: Page, name: string) {
  const box = await page
    .getByRole("button", { name, exact: true })
    .boundingBox();
  if (!box) throw new Error(`${name} is not visible`);
  return { x: box.x + box.width / 2, y: box.y + box.height / 2 };
}

test.describe("drag preview", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/");
  });

  test("shows a palette block where it will land", async ({ page }) => {
    const pointer = await dragTo(
      page,
      await centerOf(page, "Project"),
      120,
      90,
    );

    const preview = canvas(page).locator(".drop-preview");
    await expect(preview).toContainText("Project");
    const ghost = await preview.boundingBox();
    expect(ghost?.x).toBeCloseTo(pointer.x, 0);
    expect(ghost?.y).toBeCloseTo(pointer.y, 0);

    await page.mouse.up();
    await expect(preview).toHaveCount(0);
    const block = await page
      .getByRole("button", { name: "Project 1" })
      .boundingBox();
    expect(block).toEqual(ghost);
  });

  test("shows a moved block at its new spot and dims it where it was", async ({
    page,
  }) => {
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Project" }),
      canvas(page),
    );
    const project = page.getByRole("button", { name: "Project 1" });
    const before = await project.boundingBox();
    if (!before) throw new Error("project is not visible");

    await dragTo(page, { x: before.x + 20, y: before.y + 10 }, 60, 300);

    const preview = canvas(page).locator(".drop-preview");
    await expect(preview).toContainText("Project 1");
    await expect(project).toHaveClass(/dragging/);
    const ghost = await preview.boundingBox();
    expect(ghost?.width).toBeCloseTo(before.width, 0);

    await page.mouse.up();
    await expect(preview).toHaveCount(0);
    await expect(project).not.toHaveClass(/dragging/);
    await expect.poll(() => project.boundingBox()).toEqual(ghost);
  });

  test("marks a spot the block cannot land on", async ({ page }) => {
    await dragTo(page, await centerOf(page, "Agent"), 300, 300);

    await expect(canvas(page).locator(".drop-preview")).toHaveClass(/invalid/);
    await page.mouse.up();
  });
});
