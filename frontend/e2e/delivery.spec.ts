import { rmSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "@playwright/test";
import {
  addAction,
  canvas,
  connect,
  dropInto,
  enlarge,
  git,
  mouseDrag,
  projectClone,
  type ProjectClone,
} from "./fixtures";

// Issue to pull request through the real backend: real Git (pushing to a
// local bare remote) and a fake GitHub CLI.
test.describe("GitHub delivery", () => {
  let fixture: ProjectClone;

  test.beforeEach(async ({ page }) => {
    fixture = projectClone();
    await page.goto("/");
  });

  test.afterEach(() => {
    rmSync(fixture.root, { recursive: true, force: true });
  });

  test("an issue becomes a pushed branch and a pull request", async ({
    page,
  }) => {
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Project" }),
      canvas(page),
    );
    const project = page.getByRole("button", { name: "Project 1" });
    await page.getByLabel("Path").fill(fixture.clone);
    await enlarge(page, project, 260, 420);
    const box = await project.boundingBox();
    const palette = await page
      .getByRole("button", { name: "GitHub", exact: true })
      .boundingBox();
    if (!box || !palette) throw new Error("not visible");
    await page.mouse.move(palette.x + 20, palette.y + 10);
    await page.mouse.down();
    await page.mouse.move(box.x + 30, box.y + 45, { steps: 10 });
    await page.mouse.up();
    const github = page.getByRole("button", { name: "GitHub 1" });
    await page.getByLabel("Already authenticated (OAuth)").check();
    await page.getByLabel("Starting point").check();
    await addAction(page, github, "Read issue");
    await page.getByLabel("Issue number").fill("7");
    await addAction(page, github, "Create worktree");
    await page.getByLabel("Branch").fill("issue-7");
    await page
      .getByLabel("Worktree path")
      .fill(join(fixture.root, "worktrees", "issue-7"));
    await addAction(page, github, "Commit");
    await page
      .getByLabel("Commit message")
      .fill("Close issue on {{workspace.branch}}");
    await addAction(page, github, "Push");
    await addAction(page, github, "Open pull request");
    await page.getByLabel("Title").fill("Add a changelog");
    const implement = await dropInto(
      page,
      "Command",
      project,
      { x: 250, y: 60 },
      "Implement",
    );
    await page
      .getByLabel("Command", { exact: true })
      .fill("echo changelog > CHANGELOG.md");
    await connect(
      page,
      page.getByRole("button", { name: "Create worktree", exact: true }),
      implement,
    );
    await connect(
      page,
      implement,
      page.getByRole("button", { name: "Commit", exact: true }),
    );

    await page.getByRole("button", { name: "Run flow" }).click();

    const result = page.getByRole("region", { name: "Run result" });
    await expect(result).toContainText("Run succeeded", { timeout: 20_000 });
    await expect(
      page.getByRole("link", { name: "https://github.com/acme/api/pull/42" }),
    ).toBeVisible();
    await expect(result).toContainText("Commit: ");
    const pushed = git(
      fixture.clone,
      "ls-remote",
      "origin",
      "refs/heads/issue-7",
    );
    expect(pushed).toContain("refs/heads/issue-7");
  });
});
