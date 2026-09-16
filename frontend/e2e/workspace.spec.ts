import { expect, type Locator, type Page, test } from "@playwright/test";

const canvas = (page: Page) =>
  page.getByRole("region", { name: "Graph canvas" });

// Drives a real HTML5 drag through the browser input pipeline (mousedown,
// dragstart, dragover, drop), unlike synthetic event dispatch.
async function mouseDrag(
  page: Page,
  source: Locator,
  target: Locator,
  steps = 12,
) {
  const from = await source.boundingBox();
  const to = await target.boundingBox();
  const sx = from.x + from.width / 2;
  const sy = from.y + from.height / 2;
  const dx = to.x + to.width / 2;
  const dy = to.y + to.height / 2;
  await page.mouse.move(sx, sy);
  await page.mouse.down();
  for (let step = 1; step <= steps; step++) {
    await page.mouse.move(
      sx + ((dx - sx) * step) / steps,
      sy + ((dy - sy) * step) / steps,
    );
    await page.waitForTimeout(25);
  }
  await page.mouse.up();
}

test.describe("graph builder workspace", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/");
  });

  test("drops a palette component onto the canvas", async ({ page }) => {
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Agent" }),
      canvas(page),
    );

    await expect(page.getByRole("button", { name: "Agent 1" })).toBeVisible();
  });

  test("moves an existing node when dragged across the canvas", async ({
    page,
  }) => {
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Agent" }),
      canvas(page),
    );
    const node = page.getByRole("button", { name: "Agent 1" });
    const before = await node.boundingBox();

    const target = canvas(page);
    const box = await target.boundingBox();
    await page.mouse.move(
      before.x + before.width / 2,
      before.y + before.height / 2,
    );
    await page.mouse.down();
    await page.mouse.move(box.x + 30, box.y + 30, { steps: 8 });
    await page.mouse.up();

    await expect
      .poll(() =>
        node
          .boundingBox()
          .then((b) => b.x < before.x - 50 && b.y < before.y - 50),
      )
      .toBe(true);
  });

  test("places drops in content coordinates on a scrolled canvas", async ({
    page,
  }) => {
    await page.evaluate(() => {
      const canvas = document.querySelector('[aria-label="Graph canvas"]');
      const filler = document.createElement("div");
      filler.style.height = "2000px";
      filler.style.width = "2000px";
      filler.style.pointerEvents = "none";
      canvas.appendChild(filler);
      canvas.scrollTop = 150;
      canvas.scrollLeft = 200;
    });

    await mouseDrag(
      page,
      page.getByRole("button", { name: "Agent" }),
      canvas(page),
    );
    const node = page.getByRole("button", { name: "Agent 1" });

    await expect
      .poll(async () => {
        const area = await canvas(page).boundingBox();
        const left = parseFloat(await node.evaluate((el) => el.style.left));
        const top = parseFloat(await node.evaluate((el) => el.style.top));
        return Math.max(
          Math.abs(left - (area.width / 2 + 200)),
          Math.abs(top - (area.height / 2 + 150)),
        );
      })
      .toBeLessThanOrEqual(2);
  });

  test("closes the directory browser when clicking outside", async ({
    page,
  }) => {
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Project" }),
      canvas(page),
    );
    const node = page.getByRole("button", { name: "Project 1" });
    await expect(node).toBeVisible();

    await page.getByRole("button", { name: "Browse…" }).click();
    const dialog = page.getByRole("dialog", { name: "Directory browser" });
    // bits-ui registers its outside-click listeners asynchronously just after
    // mount, so settle before simulating an outside click.
    await expect(dialog.getByRole("button", { name: "Up" })).toBeVisible();
    await page.waitForFunction(
      () => (globalThis.bitsDismissableLayers?.size ?? 0) > 0,
    );

    await page.mouse.click(20, 400);
    await expect(dialog).not.toBeVisible();
  });
});
