import { existsSync, rmSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "@playwright/test";
import {
  addAction,
  git,
  projectClone,
  projectWithGitHub,
  selectBlock,
  type ProjectClone,
} from "./fixtures";

// These scenarios run against the real Go backend, which executes Git in a
// temporary clone; see playwright.config.ts.
test.describe("Git actions inside a GitHub block", () => {
  let fixture: ProjectClone;

  test.beforeEach(async ({ page }) => {
    fixture = projectClone();
    await page.goto("/");
  });

  test.afterEach(() => {
    rmSync(fixture.root, { recursive: true, force: true });
  });

  test("fills the GitHub repository from the project's clone", async ({
    page,
  }) => {
    await projectWithGitHub(page, fixture.clone);

    await expect(page.getByLabel("Repository")).toHaveValue("acme/api");
  });

  test("fetches, creates a worktree, and rebases it in one run", async ({
    page,
  }) => {
    const github = await projectWithGitHub(page, fixture.clone);
    await page.getByLabel("Already authenticated (OAuth)").check();
    await page.getByLabel("Starting point").check();
    await addAction(page, github, "Fetch");
    await addAction(page, github, "Create worktree");
    const worktreePath = join(fixture.root, "worktrees", "login");
    await page.getByLabel("Branch").fill("feature/login");
    await page.getByLabel("Worktree path").fill(worktreePath);
    await addAction(page, github, "Rebase");

    await selectBlock(github);
    await expect(page.getByRole("list", { name: "Actions" })).toContainText(
      "1. Fetch2. Create worktree3. Rebase",
    );
    await page.getByRole("button", { name: "Run flow" }).click();

    const result = page.getByRole("region", { name: "Run result" });
    await expect(result).toContainText("Run succeeded", { timeout: 15_000 });
    await expect(result).toContainText("Create worktree: worktree succeeded");
    await expect(result).toContainText(`Path: ${worktreePath}`);
    for (const name of ["Fetch", "Create worktree", "Rebase", "GitHub 1"]) {
      await expect(page.getByRole("button", { name, exact: true })).toHaveClass(
        /status-succeeded/,
      );
    }
    // The worktree branches from the commit the fetch brought in.
    expect(git(worktreePath, "rev-parse", "HEAD")).toBe(fixture.unfetched);
    expect(git(worktreePath, "branch", "--show-current")).toBe("feature/login");
  });

  test("marks a failed action and skips the actions after it", async ({
    page,
  }) => {
    const github = await projectWithGitHub(page, fixture.clone);
    await page.getByLabel("Starting point").check();
    await addAction(page, github, "Create worktree");
    await addAction(page, github, "Rebase");

    await page.getByRole("button", { name: "Run flow" }).click();

    const result = page.getByRole("region", { name: "Run result" });
    await expect(result).toContainText("Run failed", { timeout: 15_000 });
    await expect(result).toContainText("set the branch the worktree works on");
    await expect(
      page.getByRole("button", { name: "Create worktree", exact: true }),
    ).toHaveClass(/status-failed/);
    await expect(
      page.getByRole("button", { name: "Rebase", exact: true }),
    ).toHaveClass(/status-skipped/);
    await expect(page.getByRole("button", { name: "GitHub 1" })).toHaveClass(
      /status-failed/,
    );
    expect(existsSync(join(fixture.root, "worktrees"))).toBe(false);
  });

  test("explains a flow that cannot start", async ({ page }) => {
    const github = await projectWithGitHub(page, fixture.clone);
    await page.getByLabel("Starting point").check();
    await addAction(page, github, "Fetch");
    await addAction(page, github, "Rebase");

    await page.getByRole("button", { name: "Run flow" }).click();

    await expect(
      page.getByRole("region", { name: "Run result" }),
    ).toContainText(
      "Rebase needs a workspace; connect a Create worktree action before it",
    );
  });
});
