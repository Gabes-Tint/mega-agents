import { rmSync } from "node:fs";
import { join } from "node:path";
import { expect, test, type Page } from "@playwright/test";
import {
  addAction,
  canvas,
  connect,
  dropInto,
  enlarge,
  mouseDrag,
  projectClone,
  selectBlock,
  type ProjectClone,
} from "./fixtures";

// Command gates run real shell commands in a real worktree.
test.describe("Command blocks", () => {
  let fixture: ProjectClone;

  test.beforeEach(async ({ page }) => {
    fixture = projectClone();
    await page.goto("/");
  });

  test.afterEach(() => {
    rmSync(fixture.root, { recursive: true, force: true });
  });

  async function gatedWorktree(page: Page, command: string) {
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Project" }),
      canvas(page),
    );
    const project = page.getByRole("button", { name: "Project 1" });
    await page.getByLabel("Path").fill(fixture.clone);
    await enlarge(page, project, 260, 200);
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
    await page.getByLabel("Starting point").check();
    await addAction(page, github, "Create worktree");
    const worktree = join(fixture.root, "worktrees", "gate");
    await page.getByLabel("Branch").fill("gate");
    await page.getByLabel("Worktree path").fill(worktree);
    const gate = await dropInto(
      page,
      "Command",
      project,
      { x: 250, y: 60 },
      "Tests",
    );
    await page.getByLabel("Command", { exact: true }).fill(command);
    await connect(
      page,
      page.getByRole("button", { name: "Create worktree", exact: true }),
      gate,
    );
    return { project, gate, worktree };
  }

  test("a passing command lets the flow continue", async ({ page }) => {
    const { gate, worktree } = await gatedWorktree(
      page,
      'test -f README.md && echo "all green in $(pwd)"',
    );

    await page.getByRole("button", { name: "Run flow" }).click();

    const result = page.getByRole("region", { name: "Run result" });
    await expect(result).toContainText("Run succeeded", { timeout: 15_000 });
    await expect(result).toContainText("Exit code: 0");
    await expect(result).toContainText(`all green in ${worktree}`);
    await expect(gate).toHaveClass(/status-succeeded/);
  });

  test("a failing command sends its output to a fixer agent", async ({
    page,
  }) => {
    const { project, gate, worktree } = await gatedWorktree(
      page,
      'echo "lint: unused import"; exit 2',
    );
    const fixer = await dropInto(
      page,
      "Agent",
      project,
      { x: 250, y: 190 },
      "Fixer",
    );
    await page.getByLabel("Prompt").fill("Fix: {{results.tests}}");
    await connect(page, gate, fixer);
    await selectBlock(gate);
    await page.getByLabel("Output to Fixer").selectOption("failed");

    await page.getByRole("button", { name: "Run flow" }).click();

    const result = page.getByRole("region", { name: "Run result" });
    await expect(result).toContainText("Run succeeded", { timeout: 15_000 });
    await expect(result).toContainText("Exit code: 2");
    await expect(result).toContainText(
      `worked in ${fixture.clone}: Fix: {"exitCode":2,"output":"lint: unused import\\n"}`,
    );
    await expect(fixer).toHaveClass(/status-succeeded/);
    expect(worktree).toContain("gate");
  });
});
