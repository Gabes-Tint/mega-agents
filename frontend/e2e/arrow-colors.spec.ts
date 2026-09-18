import { expect, test, type Locator, type Page } from "@playwright/test";
import {
  boxOf,
  canvas,
  center,
  connect,
  dragFromHandle,
  dropInto,
  enlarge,
  routePoint,
  routePoints,
} from "./fixtures";

type Rgb = [number, number, number];
interface Point {
  x: number;
  y: number;
}

// The red, green and blue a CSS colour paints. The block tints are written in
// OKLCH, which the browser keeps as written in computed styles, so painting a
// value is the only way to compare a token with what the screen shows.
function painted(page: Page, value: string): Promise<Rgb> {
  return page.evaluate((color) => {
    const surface = document.createElement("canvas");
    const context = surface.getContext("2d");
    if (!context) throw new Error("no 2d context");
    context.fillStyle = color;
    context.fillRect(0, 0, 1, 1);
    const [red, green, blue] = context.getImageData(0, 0, 1, 1).data;
    return [red, green, blue] as [number, number, number];
  }, value);
}

// What the page actually shows at the given viewport points, read back from a
// screenshot: an SVG marker paints no element of its own to measure.
async function shown(page: Page, points: Point[]): Promise<Rgb[]> {
  const shot = (await page.screenshot()).toString("base64");
  return page.evaluate(
    async ({ shot, points }) => {
      const image = new Image();
      image.src = `data:image/png;base64,${shot}`;
      await image.decode();
      const surface = document.createElement("canvas");
      surface.width = image.width;
      surface.height = image.height;
      const context = surface.getContext("2d");
      if (!context) throw new Error("no 2d context");
      context.drawImage(image, 0, 0);
      return points.map(({ x, y }) => {
        const [red, green, blue] = context.getImageData(
          Math.round(x),
          Math.round(y),
          1,
          1,
        ).data;
        return [red, green, blue] as [number, number, number];
      });
    },
    { shot, points },
  );
}

const styleOf = (element: Locator, property: string) =>
  element.evaluate(
    (node, name) => getComputedStyle(node).getPropertyValue(name).trim(),
    property,
  );

const rootStyle = (page: Page, property: string) =>
  styleOf(page.locator(":root"), property);

// The unit vector along the last stretch of a route, and the one across it.
async function heading(arrow: Locator) {
  const points = await routePoints(arrow, 120);
  const end = points.at(-1);
  const back = [...points].reverse().find((point) => {
    return end && Math.hypot(point.x - end.x, point.y - end.y) > 4;
  });
  if (!end || !back) throw new Error("the arrow has no route to measure");
  const length = Math.hypot(end.x - back.x, end.y - back.y);
  const along = { x: (end.x - back.x) / length, y: (end.y - back.y) / length };
  return { end, along, across: { x: -along.y, y: along.x } };
}

// Two points inside the solid part of the arrowhead and clear of the line's
// own stroke. The head is a marker, so it is drawn in stroke widths: its base
// sits 7 widths behind the end of the route and is 8 widths tall, which puts
// 4 widths back and 1.5 widths to either side well inside it.
async function arrowheadPoints(arrow: Locator): Promise<Point[]> {
  const { end, along, across } = await heading(arrow);
  const width = Number.parseFloat(await styleOf(arrow, "stroke-width"));
  const back = {
    x: end.x - along.x * 4 * width,
    y: end.y - along.y * 4 * width,
  };
  const side = { x: across.x * 1.5 * width, y: across.y * 1.5 * width };
  return [
    { x: back.x + side.x, y: back.y + side.y },
    { x: back.x - side.x, y: back.y - side.y },
  ];
}

// Points either side of the middle of a route, far enough out to miss the
// line itself but within any band drawn around it.
async function besideRoute(arrow: Locator): Promise<Point[]> {
  const middle = await routePoint(arrow, 0.5);
  const { across } = await heading(arrow);
  return [
    { x: middle.x + across.x * 3, y: middle.y + across.y * 3 },
    { x: middle.x - across.x * 3, y: middle.y - across.y * 3 },
  ];
}

// How far apart two colours are, so a test can say a colour changed without
// depending on the exact tint a theme happens to use.
const apart = (left: Rgb, right: Rgb) =>
  Math.max(...left.map((value, index) => Math.abs(value - right[index])));

// WCAG relative luminance, for the contrast an arrow has against the canvas
// it is drawn on.
function luminance([red, green, blue]: Rgb): number {
  const channel = (value: number) => {
    const part = value / 255;
    return part <= 0.04045 ? part / 12.92 : ((part + 0.055) / 1.055) ** 2.4;
  };
  return (
    0.2126 * channel(red) + 0.7152 * channel(green) + 0.0722 * channel(blue)
  );
}

function contrast(left: Rgb, right: Rgb): number {
  const [lighter, darker] = [luminance(left), luminance(right)].sort(
    (first, second) => second - first,
  );
  return ((lighter ?? 0) + 0.05) / ((darker ?? 0) + 0.05);
}

// Leaves the pointer in an empty corner so no hovered block draws its arrow
// handles over the pixels a test reads. A selected block keeps its four.
async function restPointer(page: Page, handles = 4) {
  const area = await boxOf(canvas(page));
  await page.mouse.move(area.x + area.width - 20, area.y + area.height - 20);
  await expect(canvas(page).locator(".link-handle")).toHaveCount(handles);
}

test.describe("arrows take the colour of the block they leave", () => {
  test.use({ viewport: { width: 1600, height: 1000 } });

  let project: Locator;
  let coder: Locator;
  let reviewer: Locator;
  let tests: Locator;
  let fixer: Locator;

  // Two agents above a command gate and a fixer agent, all in one project, so
  // arrows can leave blocks of two different types.
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

  test("an arrow is drawn in its source block's colour, not the neutral one", async ({
    page,
  }) => {
    await connect(page, coder, reviewer);
    const arrow = canvas(page).locator(".edge-line");

    expect(await painted(page, await styleOf(arrow, "stroke"))).toEqual(
      await painted(page, await styleOf(coder, "--block-accent")),
    );
    expect(await painted(page, await styleOf(arrow, "stroke"))).not.toEqual(
      await painted(page, await rootStyle(page, "--edge")),
    );
  });

  test("an arrow from another kind of block is drawn in another colour", async ({
    page,
  }) => {
    await connect(page, coder, reviewer);
    await connect(page, tests, fixer);
    const arrows = canvas(page).locator(".edge-line");

    const fromAgent = await painted(
      page,
      await styleOf(arrows.nth(0), "stroke"),
    );
    const fromCommand = await painted(
      page,
      await styleOf(arrows.nth(1), "stroke"),
    );
    expect(fromCommand).toEqual(
      await painted(page, await styleOf(tests, "--block-accent")),
    );
    expect(apart(fromAgent, fromCommand)).toBeGreaterThan(16);
  });

  test("the arrowhead is painted like the line it ends", async ({ page }) => {
    await connect(page, coder, reviewer);
    await connect(page, tests, fixer);
    const arrows = canvas(page).locator(".edge-line");
    await restPointer(page);

    for (const index of [0, 1]) {
      const arrow = arrows.nth(index);
      const line = await painted(page, await styleOf(arrow, "stroke"));
      const head = await shown(page, await arrowheadPoints(arrow));
      expect(head[0]).toEqual(line);
      expect(head[1]).toEqual(line);
    }
  });

  test("a selected arrow keeps its colour and still stands out", async ({
    page,
  }) => {
    await connect(page, coder, reviewer);
    const arrow = canvas(page).locator(".edge-line");
    const beside = await besideRoute(arrow);
    await restPointer(page);
    const quiet = await shown(page, beside);
    const colour = await painted(page, await styleOf(arrow, "stroke"));

    const middle = await routePoint(arrow, 0.5);
    await page.mouse.click(middle.x, middle.y);
    await expect(
      page.getByRole("option", { name: "Arrow from Coder to Reviewer" }),
    ).toHaveAttribute("aria-selected", "true");
    await restPointer(page, 0);

    // The arrow leaves an agent, whose hue is close to the selection accent,
    // so selection has to read as more than a change of colour.
    expect(await painted(page, await styleOf(arrow, "stroke"))).toEqual(colour);
    const marked = await shown(page, beside);
    expect(apart(marked[0], quiet[0])).toBeGreaterThan(16);
    expect(apart(marked[1], quiet[1])).toBeGreaterThan(16);
    const head = await shown(page, await arrowheadPoints(arrow));
    expect(head[0]).toEqual(colour);
  });

  test("the arrow being dragged from a handle takes the source's colour", async ({
    page,
  }) => {
    await dragFromHandle(page, coder, center(await boxOf(reviewer)));
    const preview = canvas(page).locator(".edge-preview");

    expect(await painted(page, await styleOf(preview, "stroke"))).toEqual(
      await painted(page, await styleOf(coder, "--block-accent")),
    );
    await page.mouse.up();
  });

  test("a preview over a block that refuses it reads as refused", async ({
    page,
  }) => {
    const box = await boxOf(project);
    const preview = canvas(page).locator(".edge-preview");
    await dragFromHandle(page, coder, center(await boxOf(reviewer)));
    const allowed = {
      stroke: await styleOf(preview, "stroke"),
      dashes: await styleOf(preview, "stroke-dasharray"),
    };

    await page.mouse.move(box.x + 260, box.y + box.height - 20, { steps: 6 });
    await expect(preview).toHaveClass(/invalid/);

    expect(await painted(page, await styleOf(preview, "stroke"))).toEqual(
      await painted(page, await rootStyle(page, "--fail")),
    );
    expect(await styleOf(preview, "stroke-dasharray")).not.toBe(allowed.dashes);
    expect(await painted(page, await styleOf(preview, "stroke"))).not.toEqual(
      await painted(page, allowed.stroke),
    );
    await page.mouse.up();
  });

  test("an arrow changes with the theme and its head follows", async ({
    page,
  }) => {
    await connect(page, coder, reviewer);
    const arrow = canvas(page).locator(".edge-line");
    await restPointer(page);
    const light = await painted(page, await styleOf(arrow, "stroke"));

    await page.getByRole("button", { name: "Switch to dark theme" }).click();
    await restPointer(page);

    const dark = await painted(page, await styleOf(arrow, "stroke"));
    expect(apart(light, dark)).toBeGreaterThan(16);
    expect(dark).toEqual(
      await painted(page, await styleOf(coder, "--block-accent")),
    );
    const head = await shown(page, await arrowheadPoints(arrow));
    expect(head[0]).toEqual(dark);
  });

  test("every block's arrow colour stands out from the canvas in both themes", async ({
    page,
  }) => {
    const swatches = page
      .getByRole("complementary", { name: "Component palette" })
      .locator("button[style*='--block-hue']");
    await expect(swatches).not.toHaveCount(0);

    for (const theme of ["light", "dark"]) {
      const behind = await painted(
        page,
        await styleOf(canvas(page), "background-color"),
      );
      for (const swatch of await swatches.all()) {
        const accent = await painted(
          page,
          await styleOf(swatch, "--block-accent"),
        );
        expect(contrast(accent, behind)).toBeGreaterThanOrEqual(3);
      }
      if (theme === "light")
        await page
          .getByRole("button", { name: "Switch to dark theme" })
          .click();
    }
  });
});
