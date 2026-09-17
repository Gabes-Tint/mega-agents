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
  await page.getByLabel("Name").fill(name);
  return page.getByRole("button", { name, exact: true });
}

export async function connect(page: Page, from: Locator, to: Locator) {
  await selectBlock(from);
  await page.getByRole("button", { name: "Connect" }).click();
  await selectBlock(to);
}
