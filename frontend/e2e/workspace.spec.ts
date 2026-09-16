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

  test("resizes a node by dragging its corner handle", async ({ page }) => {
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Agent" }),
      canvas(page),
    );
    const node = page.getByRole("button", { name: "Agent 1" });
    const before = await node.boundingBox();

    const handle = node.locator(".resize-handle");
    const handleBox = await handle.boundingBox();
    await page.mouse.move(
      handleBox.x + handleBox.width / 2,
      handleBox.y + handleBox.height / 2,
    );
    await page.mouse.down();
    await page.mouse.move(before.x + 240, before.y + 160, { steps: 8 });
    await page.mouse.up();

    await expect
      .poll(async () => {
        const after = await node.boundingBox();
        return after.width > before.width + 50 && after.height > before.height;
      })
      .toBe(true);

    const moved = await node.boundingBox();
    expect(moved.x).toBeCloseTo(before.x, 0);
    expect(moved.y).toBeCloseTo(before.y, 0);
  });

  test("nests a dropped component inside a project box", async ({ page }) => {
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Project" }),
      canvas(page),
    );
    const project = page.getByRole("button", { name: "Project 1" });
    await expect(project).toBeVisible();

    const projectBox = await project.boundingBox();
    await mouseDrag(page, page.getByRole("button", { name: "Agent" }), project);
    const child = page.getByRole("button", { name: "Agent 1" });
    await expect(child).toBeVisible();

    await expect
      .poll(async () => {
        const childBox = await child.boundingBox();
        return (
          childBox.x >= projectBox.x &&
          childBox.y >= projectBox.y &&
          childBox.x < projectBox.x + projectBox.width &&
          childBox.y < projectBox.y + projectBox.height
        );
      })
      .toBe(true);
  });

  test("moves a project's children when the project is dragged", async ({
    page,
  }) => {
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Project" }),
      canvas(page),
    );
    const project = page.getByRole("button", { name: "Project 1" });
    await mouseDrag(page, page.getByRole("button", { name: "Agent" }), project);
    const child = page.getByRole("button", { name: "Agent 1" });
    await expect(child).toBeVisible();
    const before = await child.boundingBox();

    // Drag the project by its header area (center of its top strip) so the
    // grab point misses the child.
    const projectBox = await project.boundingBox();
    await page.mouse.move(projectBox.x + 20, projectBox.y + 12);
    await page.mouse.down();
    await page.mouse.move(projectBox.x + 60, projectBox.y + 40, { steps: 8 });
    await page.mouse.up();

    await expect
      .poll(async () => {
        const after = await child.boundingBox();
        return (
          Math.abs(after.x - (before.x + 40)) <= 2 &&
          Math.abs(after.y - (before.y + 28)) <= 2
        );
      })
      .toBe(true);
  });

  test("marks one start point per project", async ({ page }) => {
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Project" }),
      canvas(page),
    );
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Agent" }),
      page.getByRole("button", { name: "Project 1" }),
    );
    const first = page.getByRole("button", { name: "Agent 1" });
    await expect(first).toBeVisible();

    await page.getByLabel("Starting point").check();
    await expect(first).toContainText("▶");

    const project = page.getByRole("button", { name: "Project 1" });
    const projectBox = await project.boundingBox();
    // Aim inside the project's box: Agent 1's center sits on the project's
    // corner, which is outside the containment test.
    const paletteAgent = page.getByRole("button", {
      name: "Agent",
      exact: true,
    });
    const paletteBox = await paletteAgent.boundingBox();
    await page.mouse.move(
      paletteBox.x + paletteBox.width / 2,
      paletteBox.y + paletteBox.height / 2,
    );
    await page.mouse.down();
    await page.mouse.move(projectBox.x + 60, projectBox.y + 40, { steps: 10 });
    await page.mouse.up();
    const second = page.getByRole("button", { name: "Agent 2" });
    await expect(second).toBeVisible();

    // The invariant assertion below is what proves containment: only a
    // sibling of Agent 1 (same project group) can take over its start flag.
    await expect
      .poll(async () => {
        const box = await second.boundingBox();
        return (
          box.x >= projectBox.x + 20 &&
          box.y >= projectBox.y + 20 &&
          box.x < projectBox.x + projectBox.width &&
          box.y < projectBox.y + projectBox.height
        );
      })
      .toBe(true);

    await page.getByLabel("Starting point").check();
    await expect(second).toContainText("▶");
    await expect(first).not.toContainText("▶");
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
    await expect(
      dialog.getByRole("button", { name: "Up", exact: true }),
    ).toBeVisible();
    await page.waitForFunction(
      () => (globalThis.bitsDismissableLayers?.size ?? 0) > 0,
    );

    await page.mouse.click(20, 400);
    await expect(dialog).not.toBeVisible();
  });
});
