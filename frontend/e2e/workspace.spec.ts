import { expect, test } from "@playwright/test";
import { canvas, connect, mouseDrag, routePoints } from "./fixtures";

test.describe("graph builder workspace", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/");
  });

  // The containment matrix allows only projects at the canvas root, so most
  // tests seed a project and drop the working block inside it.
  async function seedProject(page: Page) {
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Project" }),
      canvas(page),
    );
  }

  test("drops a palette component onto the canvas", async ({ page }) => {
    await seedProject(page);
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Agent" }),
      page.getByRole("button", { name: "Project 1" }),
    );

    await expect(page.getByRole("button", { name: "Agent 1" })).toBeVisible();
  });

  test("moves an existing node when dragged across the canvas", async ({
    page,
  }) => {
    // A block inside a project stays inside it, so the project itself moves.
    await seedProject(page);
    const node = page.getByRole("button", { name: "Project 1" });
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

    await seedProject(page);
    // Drop the agent at the project's top-left corner so the nested block
    // keeps the project's own content position.
    const project = page.getByRole("button", { name: "Project 1" });
    const projectBox = await project.boundingBox();
    const paletteAgent = page.getByRole("button", { name: "Agent" });
    const paletteBox = await paletteAgent.boundingBox();
    await page.mouse.move(
      paletteBox.x + paletteBox.width / 2,
      paletteBox.y + paletteBox.height / 2,
    );
    await page.mouse.down();
    await page.mouse.move(projectBox.x + 2, projectBox.y + 2, { steps: 10 });
    await page.mouse.up();
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
    await seedProject(page);
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Agent" }),
      page.getByRole("button", { name: "Project 1" }),
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
    await expect(first.locator('[data-mark="start"]')).toHaveCount(1);

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
    await expect(second.locator('[data-mark="start"]')).toHaveCount(1);
    await expect(first.locator('[data-mark="start"]')).toHaveCount(0);
  });

  test("styles each node with a title header and separator", async ({
    page,
  }) => {
    await seedProject(page);
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Agent", exact: true }),
      page.getByRole("button", { name: "Project 1" }),
    );
    const node = page.getByRole("button", { name: "Agent 1" });
    await expect(node).toBeVisible();

    const title = node.locator(".node-title");
    await expect(title).toContainText("Agent 1");
    const separator = node.locator(".node-separator");
    await expect(separator).toBeVisible();

    // The separator renders below the header inside the box, leaving room in
    // the body for nested children.
    await expect
      .poll(async () => {
        const titleBox = await title.boundingBox();
        const separatorBox = await separator.boundingBox();
        const nodeBox = await node.boundingBox();
        if (!titleBox || !separatorBox || !nodeBox) return false;
        return (
          separatorBox.y >= titleBox.y + titleBox.height - 1 &&
          separatorBox.height >= 1 &&
          nodeBox.height > titleBox.height + separatorBox.height
        );
      })
      .toBe(true);
  });

  test("draws an arrow between two same-level boxes", async ({ page }) => {
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Project" }),
      canvas(page),
    );
    // Drop the second project away from Project 1 so they do not overlap.
    const area = await canvas(page).boundingBox();
    const paletteProject = page.getByRole("button", {
      name: "Project",
      exact: true,
    });
    const paletteBox = await paletteProject.boundingBox();
    await page.mouse.move(
      paletteBox.x + paletteBox.width / 2,
      paletteBox.y + paletteBox.height / 2,
    );
    await page.mouse.down();
    await page.mouse.move(area.x + 300, area.y + 100, { steps: 10 });
    await page.mouse.up();
    await expect(page.getByRole("button", { name: "Project 2" })).toBeVisible();

    await connect(
      page,
      page.getByRole("button", { name: "Project 1" }),
      page.getByRole("button", { name: "Project 2" }),
    );

    await expect(page.locator(".edge-line")).toHaveCount(1);

    // Moving a connected box keeps the arrow attached: both endpoints move
    // with the boxes.
    const node = page.getByRole("button", { name: "Project 2" });
    const before = await node.boundingBox();
    await page.mouse.move(before.x + before.width / 2, before.y + 30);
    await page.mouse.down();
    await page.mouse.move(before.x + 120, before.y + 60, { steps: 8 });
    await page.mouse.up();

    const line = page.locator(".edge-line");
    await expect(line).toHaveCount(1);
    // The arrow stops on the border of the project box it points at, so its
    // head stays visible instead of hiding under the box.
    await expect
      .poll(async () => {
        const points = await routePoints(line, 40);
        const tip = points[points.length - 1]!;
        const box = await node.boundingBox();
        const onSide =
          Math.abs(tip.x - box.x) < 3 ||
          Math.abs(tip.x - (box.x + box.width)) < 3 ||
          Math.abs(tip.y - box.y) < 3 ||
          Math.abs(tip.y - (box.y + box.height)) < 3;
        return (
          onSide &&
          tip.x > box.x - 3 &&
          tip.x < box.x + box.width + 3 &&
          tip.y > box.y - 3 &&
          tip.y < box.y + box.height + 3
        );
      })
      .toBe(true);
  });

  test("captures repository and secret key properties for a GitHub box", async ({
    page,
  }) => {
    await seedProject(page);
    await mouseDrag(
      page,
      page.getByRole("button", { name: "GitHub", exact: true }),
      page.getByRole("button", { name: "Project 1" }),
    );
    const github = page.getByRole("button", { name: "GitHub 1" });
    await expect(github).toBeVisible();

    await page
      .getByLabel("Repository")
      .fill("https://github.com/example/project");
    await page.getByLabel("Secret key").fill("secret://github-bot");

    // Re-selecting the box shows the persisted values again.
    await github.click();
    await expect(page.getByLabel("Repository")).toHaveValue(
      "https://github.com/example/project",
    );
    await expect(page.getByLabel("Secret key")).toHaveValue(
      "secret://github-bot",
    );
  });

  test("nests a GitHub App inside a GitHub box and captures its properties", async ({
    page,
  }) => {
    await seedProject(page);
    await mouseDrag(
      page,
      page.getByRole("button", { name: "GitHub", exact: true }),
      page.getByRole("button", { name: "Project 1" }),
    );
    const github = page.getByRole("button", { name: "GitHub 1" });
    await expect(github).toBeVisible();

    await mouseDrag(
      page,
      page.getByRole("button", { name: "GitHub App", exact: true }),
      github,
    );
    const app = page.getByRole("button", { name: "GitHub App 1" });
    await expect(app).toBeVisible();

    // The app must render inside the GitHub box, not beside it.
    await expect
      .poll(async () => {
        const githubBox = await github.boundingBox();
        const appBox = await app.boundingBox();
        return (
          appBox.x >= githubBox.x &&
          appBox.y >= githubBox.y &&
          appBox.x < githubBox.x + githubBox.width &&
          appBox.y < githubBox.y + githubBox.height
        );
      })
      .toBe(true);

    await page.getByLabel("App ID").fill("123456");
    await page
      .getByLabel("Private key")
      .fill("/home/user/.keys/github-app.pem");
    await expect(page.getByLabel("App ID")).toHaveValue("123456");
    await expect(page.getByLabel("Private key")).toHaveValue(
      "/home/user/.keys/github-app.pem",
    );
  });

  test("nests a GitHub App inside a GitHub box that sits in a project", async ({
    page,
  }) => {
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Project" }),
      canvas(page),
    );
    const project = page.getByRole("button", { name: "Project 1" });
    await expect(project).toBeVisible();

    await mouseDrag(
      page,
      page.getByRole("button", { name: "GitHub", exact: true }),
      project,
    );
    const github = page.getByRole("button", { name: "GitHub 1" });
    await expect(github).toBeVisible();
    const githubBox = await github.boundingBox();

    // The reported failure: dropping on the GitHub box's center resolved to
    // the enclosing project and rejected the app outright.
    await mouseDrag(
      page,
      page.getByRole("button", { name: "GitHub App", exact: true }),
      github,
    );
    const app = page.getByRole("button", { name: "GitHub App 1" });
    await expect(app).toBeVisible();

    await expect
      .poll(async () => {
        const appBox = await app.boundingBox();
        return (
          appBox.x >= githubBox.x &&
          appBox.y >= githubBox.y &&
          appBox.x < githubBox.x + githubBox.width &&
          appBox.y < githubBox.y + githubBox.height
        );
      })
      .toBe(true);
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
