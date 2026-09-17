import { execFileSync } from "node:child_process";
import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { expect, type Locator, type Page } from "@playwright/test";

export const canvas = (page: Page) =>
  page.getByRole("region", { name: "Graph canvas" });

// Drives a real HTML5 drag through the browser input pipeline (mousedown,
// dragstart, dragover, drop), unlike synthetic event dispatch.
export async function mouseDrag(
  page: Page,
  source: Locator,
  target: Locator,
  steps = 12,
) {
  const from = await source.boundingBox();
  const to = await target.boundingBox();
  if (!from || !to) throw new Error("drag endpoints are not visible");
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

export function git(dir: string, ...args: string[]): string {
  const environment = { ...process.env };
  for (const name of ["GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE"])
    delete environment[name];
  return execFileSync(
    "git",
    [
      "-c",
      "user.name=E2E",
      "-c",
      "user.email=e2e@example.com",
      "-c",
      "init.defaultBranch=main",
      "-c",
      "commit.gpgsign=false",
      ...args,
    ],
    { cwd: dir, env: environment, encoding: "utf8" },
  ).trim();
}

export interface ProjectClone {
  root: string;
  clone: string;
  // The commit on the remote that the clone has not fetched yet.
  unfetched: string;
}

// A clone whose origin reads https://github.com/acme/api but is rewritten
// through insteadOf to a local bare repository, which already holds a commit
// the clone has not fetched. Git runs for real, without the network.
export function projectClone(): ProjectClone {
  const root = mkdtempSync(join(tmpdir(), "mega-agents-e2e-"));
  const bare = join(root, "remote.git");
  const seed = join(root, "seed");
  const clone = join(root, "clone");
  git(root, "init", "--bare", "--quiet", bare);
  git(root, "init", "--quiet", seed);
  writeFileSync(join(seed, "README.md"), "api\n");
  git(seed, "add", "README.md");
  git(seed, "commit", "--quiet", "-m", "first");
  git(seed, "push", "--quiet", bare, "HEAD:refs/heads/main");
  git(root, "clone", "--quiet", bare, clone);
  git(clone, "remote", "set-url", "origin", "https://github.com/acme/api");
  git(clone, "config", `url.${bare}.insteadOf`, "https://github.com/acme/api");
  git(clone, "config", "user.name", "E2E");
  git(clone, "config", "user.email", "e2e@example.com");
  git(seed, "commit", "--quiet", "--allow-empty", "-m", "second");
  git(seed, "push", "--quiet", bare, "HEAD:refs/heads/main");
  return { root, clone, unfetched: git(seed, "rev-parse", "HEAD") };
}

// Drops a project onto the canvas, points it at the folder, and drops a
// GitHub block into it; the editor fills the repository from the clone.
export async function projectWithGitHub(page: Page, path: string) {
  await mouseDrag(
    page,
    page.getByRole("button", { name: "Project" }),
    canvas(page),
  );
  await page.getByLabel("Path").fill(path);
  const project = page.getByRole("button", { name: "Project 1" });
  await mouseDrag(
    page,
    page.getByRole("button", { name: "GitHub", exact: true }),
    project,
  );
  const github = page.getByRole("button", { name: "GitHub 1" });
  await expect(github).toBeVisible();
  return github;
}

// Selects a block by its header: a container's body is covered by the blocks
// nested inside it, so the header is where a click reliably lands.
export async function selectBlock(block: Locator) {
  await block.locator(".node-title").click();
}

export async function addAction(page: Page, github: Locator, label: string) {
  await selectBlock(github);
  await page.getByLabel("New action").selectOption({ label });
  await page.getByRole("button", { name: "Add action" }).click();
}

// Grows a block by dragging its corner handle.
export async function enlarge(
  page: Page,
  block: Locator,
  dx: number,
  dy: number,
) {
  const handle = await block.locator(".resize-handle").boundingBox();
  if (!handle) throw new Error("resize handle not visible");
  await page.mouse.move(
    handle.x + handle.width / 2,
    handle.y + handle.height / 2,
  );
  await page.mouse.down();
  await page.mouse.move(
    handle.x + handle.width / 2 + dx,
    handle.y + handle.height / 2 + dy,
    { steps: 8 },
  );
  await page.mouse.up();
}

// Drops a palette block at an offset inside a container and names it.
export async function dropInto(
  page: Page,
  palette: string,
  container: Locator,
  at: { x: number; y: number },
  name: string,
) {
  const box = await container.boundingBox();
  const from = await page
    .getByRole("button", { name: palette, exact: true })
    .boundingBox();
  if (!box || !from) throw new Error("drag endpoints are not visible");
  await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2);
  await page.mouse.down();
  await page.mouse.move(box.x + at.x, box.y + at.y, { steps: 10 });
  await page.mouse.up();
  await page.getByLabel("Name", { exact: true }).fill(name);
  return page.getByRole("button", { name, exact: true });
}

export interface Box {
  x: number;
  y: number;
  width: number;
  height: number;
}

export async function boxOf(locator: Locator): Promise<Box> {
  const box = await locator.boundingBox();
  if (!box) throw new Error("not visible");
  return box;
}

export function center(box: Box) {
  return { x: box.x + box.width / 2, y: box.y + box.height / 2 };
}

// Presses on the element and moves the real mouse to the point, leaving the
// button down so the drag can be inspected before it is released.
export async function pressAndMove(
  page: Page,
  element: Locator,
  to: { x: number; y: number },
) {
  const from = center(await boxOf(element));
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  await page.mouse.move(to.x, to.y, { steps: 12 });
}

// Selects the block and drags from its arrow handle on the side to the
// point, leaving the button down.
export async function dragFromHandle(
  page: Page,
  block: Locator,
  to: { x: number; y: number },
  side = "right",
) {
  await selectBlock(block);
  await pressAndMove(
    page,
    canvas(page).locator(`.link-handle[data-side="${side}"]`),
    to,
  );
}

// The side of the block whose handle faces the point, so the drag starts on
// the near side rather than reaching around the block.
function sideToward(box: Box, to: { x: number; y: number }) {
  const from = center(box);
  const dx = to.x - from.x;
  const dy = to.y - from.y;
  if (Math.abs(dx) >= Math.abs(dy)) return dx >= 0 ? "right" : "left";
  return dy >= 0 ? "bottom" : "top";
}

// Draws an arrow between two blocks the way a user does: press the source's
// handle on the side facing the target and release the pointer over it. A
// source with several outputs asks which one the arrow takes, and it takes
// the first.
export async function connect(page: Page, from: Locator, to: Locator) {
  const edges = canvas(page).locator(".edge-line");
  const drawn = await edges.count();
  // The arrow lands on the block under the pointer, and a container's body
  // is covered by the blocks nested inside it, so aim at its header.
  const target = center(await boxOf(to.locator(".node-title")));
  const side = sideToward(await boxOf(from), target);
  await dragFromHandle(page, from, target, side);
  await page.mouse.up();

  const menu = page.getByRole("menu", { name: "Output" });
  await expect
    .poll(async () => (await menu.count()) > 0 || (await edges.count()) > drawn)
    .toBe(true);
  if ((await menu.count()) > 0)
    await menu.getByRole("menuitem").first().click();
  await expect(edges).toHaveCount(drawn + 1);
}
