import { expect, test, type Locator } from "@playwright/test";
import {
  boxOf,
  canvas,
  center,
  connect,
  dropInto,
  enlarge,
  pressAndMove,
  routePoint,
  routePoints,
  type Box,
} from "./fixtures";

type Spot = { x: number; y: number };

function inside(point: Spot, box: Box, margin = 1): boolean {
  return (
    point.x > box.x + margin &&
    point.x < box.x + box.width - margin &&
    point.y > box.y + margin &&
    point.y < box.y + box.height - margin
  );
}

// How far the route strays from the straight line between its two ends: 0
// for a straight arrow, and the depth of the detour for a bent one.
function detour(points: Spot[]): number {
  const first = points[0]!;
  const last = points[points.length - 1]!;
  const length = Math.hypot(last.x - first.x, last.y - first.y) || 1;
  let worst = 0;
  for (const point of points) {
    const away =
      Math.abs(
        (last.x - first.x) * (first.y - point.y) -
          (first.x - point.x) * (last.y - first.y),
      ) / length;
    worst = Math.max(worst, away);
  }
  return worst;
}

test.describe("arrows routed around the blocks in the way", () => {
  test.use({ viewport: { width: 1600, height: 1000 } });

  let project: Locator;
  let tests: Locator;
  let fixer: Locator;

  // A wide project with a command gate on the left and an agent on the far
  // right, with room between them for a block to get in the way.
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
    await enlarge(page, project, 780, 380);
    tests = await dropInto(page, "Command", project, { x: 30, y: 60 }, "Tests");
    fixer = await dropInto(page, "Agent", project, { x: 700, y: 60 }, "Fixer");
  });

  async function interpose(page: Parameters<typeof dropInto>[0]) {
    return dropInto(page, "Agent", project, { x: 380, y: 60 }, "Middle");
  }

  test("an arrow bends around a block dropped in its way", async ({ page }) => {
    await connect(page, tests, fixer);
    const line = canvas(page).locator(".edge-line");
    expect(detour(await routePoints(line, 40))).toBeLessThan(2);

    const middle = await interpose(page);

    const bent = await routePoints(line, 120);
    expect(detour(bent)).toBeGreaterThan(20);
    const blocking = await boxOf(middle);
    for (const point of bent) expect(inside(point, blocking)).toBe(false);
    // The corners are curves rather than right angles.
    await expect(line).toHaveAttribute("d", /Q/);
  });

  test("the output label stays readable on the routed arrow", async ({
    page,
  }) => {
    await connect(page, tests, fixer);
    const middle = await interpose(page);
    const line = canvas(page).locator(".edge-line");

    const label = canvas(page).locator(".edge-port");
    await expect(label).toHaveText("passed");
    const at = center(await boxOf(label));
    const points = await routePoints(line, 120);
    const nearest = Math.min(
      ...points.map((point) => Math.hypot(point.x - at.x, point.y - at.y)),
    );

    // It rides the route itself, and no block is printed over it.
    expect(nearest).toBeLessThan(24);
    for (const block of [tests, fixer, middle])
      expect(inside(at, await boxOf(block))).toBe(false);
  });

  test("a routed arrow is selected by clicking anywhere along it", async ({
    page,
  }) => {
    await connect(page, tests, fixer);
    await interpose(page);
    const line = canvas(page).locator(".edge-line");
    const arrow = page.getByRole("option", {
      name: "Arrow from Tests to Fixer",
    });

    const along = await routePoint(line, 0.5);
    await page.mouse.click(along.x, along.y);

    await expect(arrow).toHaveAttribute("aria-selected", "true");
    await expect(
      page.getByRole("button", { name: "Delete arrow" }),
    ).toBeVisible();
  });

  test("the ends of a routed arrow sit on it and still move it", async ({
    page,
  }) => {
    await connect(page, tests, fixer);
    const middle = await interpose(page);
    const line = canvas(page).locator(".edge-line");
    const points = await routePoints(line, 120);

    const along = await routePoint(line, 0.5);
    await page.mouse.move(along.x, along.y);
    const head = canvas(page).locator('.edge-end[data-end="to"]');
    await expect(head).toHaveCount(1);
    const at = center(await boxOf(head));
    const tip = points[points.length - 1]!;
    expect(Math.hypot(at.x - tip.x, at.y - tip.y)).toBeLessThan(3);

    await pressAndMove(page, head, center(await boxOf(middle)));
    await expect(middle).toHaveClass(/drop-ok/);
    await page.mouse.up();

    await expect(
      page.getByRole("option", { name: "Arrow from Tests to Middle" }),
    ).toHaveCount(1);
    await expect(canvas(page).locator(".edge-line")).toHaveCount(1);
  });
});

// The template the routing is judged on: 42 blocks in seven containers,
// joined by 47 arrows, all of it drawn from the real backend.
test.describe("the Fit_ development flow template", () => {
  test.use({ viewport: { width: 2400, height: 1400 } });

  test("no arrow crosses a block it does not join", async ({ page }) => {
    await page.goto("/");
    await page.getByRole("button", { name: "Templates…" }).click();
    await page
      .getByRole("button", { name: "Fit_ development flow", exact: true })
      .click();
    await expect(
      page.getByRole("button", { name: "Solver implements" }),
    ).toBeVisible();

    const blocks = await canvas(page)
      .locator(".node")
      .evaluateAll((elements) =>
        elements.map((element) => {
          const box = element.getBoundingClientRect();
          return {
            name: element.querySelector(".node-title")?.textContent ?? "",
            x: box.x,
            y: box.y,
            width: box.width,
            height: box.height,
          };
        }),
      );
    const names = await canvas(page)
      .locator(".edge-hit")
      .evaluateAll((elements) =>
        elements.map((element) => element.getAttribute("aria-label") ?? ""),
      );
    const arrows = canvas(page).locator(".edge-line");
    // The whole flow is on the canvas, so the walk below is not vacuous.
    expect(names.length).toBeGreaterThan(40);
    expect(blocks.length).toBeGreaterThan(40);
    const crossings: string[] = [];
    for (let index = 0; index < names.length; index++) {
      const points = await routePoints(arrows.nth(index), 200);
      const ends = [points[0]!, points[points.length - 1]!];
      for (const block of blocks) {
        // A block one end of the arrow sits on, or a container it comes out
        // of, is not something the arrow has to avoid.
        if (ends.some((end) => inside(end, block, -2))) continue;
        if (points.some((point) => inside(point, block, 2)))
          crossings.push(`${names[index]} crosses ${block.name.trim()}`);
      }
    }

    expect(crossings).toEqual([]);
  });
});
