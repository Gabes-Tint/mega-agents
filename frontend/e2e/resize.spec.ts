import { expect, test, type Locator, type Page } from "@playwright/test";
import { canvas, mouseDrag } from "./fixtures";

// Presses at a point on the block's border and drags by (dx, dy).
async function dragBorder(
  page: Page,
  block: Locator,
  at: (box: { x: number; y: number; width: number; height: number }) => {
    x: number;
    y: number;
  },
  dx: number,
  dy: number,
) {
  const box = await block.boundingBox();
  if (!box) throw new Error("block is not visible");
  const point = at(box);
  await page.mouse.move(point.x, point.y);
  await page.mouse.down();
  await page.mouse.move(point.x + dx, point.y + dy, { steps: 6 });
  await page.mouse.up();
}

test.describe("resizing a block from its edges", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/");
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Project" }),
      canvas(page),
    );
  });

  test("the pointer turns into a resize cursor over each edge", async ({
    page,
  }) => {
    const project = page.getByRole("button", { name: "Project 1" });
    const cursors = await project
      .locator("[data-edge]")
      .evaluateAll((handles) =>
        Object.fromEntries(
          handles.map((handle) => [
            handle.getAttribute("data-edge"),
            getComputedStyle(handle).cursor,
          ]),
        ),
      );

    expect(cursors).toEqual({
      top: "ns-resize",
      bottom: "ns-resize",
      left: "ew-resize",
      right: "ew-resize",
      "top left": "nwse-resize",
      "bottom right": "nwse-resize",
      "top right": "nesw-resize",
      "bottom left": "nesw-resize",
    });
  });

  test("dragging the left, right and bottom edges resizes the block", async ({
    page,
  }) => {
    const project = page.getByRole("button", { name: "Project 1" });
    const before = await project.boundingBox();
    if (!before) throw new Error("project is not visible");

    await dragBorder(
      page,
      project,
      (box) => ({ x: box.x + 2, y: box.y + box.height / 2 }),
      -40,
      0,
    );
    await dragBorder(
      page,
      project,
      (box) => ({ x: box.x + box.width - 2, y: box.y + box.height / 2 }),
      60,
      0,
    );
    await dragBorder(
      page,
      project,
      (box) => ({ x: box.x + box.width / 2, y: box.y + box.height - 2 }),
      0,
      50,
    );

    const after = await project.boundingBox();
    expect(after?.x).toBeCloseTo(before.x - 40, 0);
    expect(after?.y).toBeCloseTo(before.y, 0);
    expect(after?.width).toBeCloseTo(before.width + 100, 0);
    expect(after?.height).toBeCloseTo(before.height + 50, 0);
  });
});
