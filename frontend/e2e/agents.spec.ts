import { rmSync } from "node:fs";
import { join } from "node:path";
import { expect, test, type Page } from "@playwright/test";
import {
  addAction,
  canvas,
  enlarge,
  mouseDrag,
  projectClone,
  selectBlock,
  type ProjectClone,
} from "./fixtures";

// Agents run through the real backend against a fake Claude Code CLI (see
// e2e/fake-agents), in temporary clones.
test.describe("Agent blocks", () => {
  let fixture: ProjectClone;

  test.beforeEach(async ({ page }) => {
    fixture = projectClone();
    await page.goto("/");
  });

  test.afterEach(() => {
    rmSync(fixture.root, { recursive: true, force: true });
  });

  async function dropAgent(
    page: Page,
    into: string,
    name: string,
    at?: { x: number; y: number },
  ) {
    const project = page.getByRole("button", { name: into, exact: true });
    const box = await project.boundingBox();
    const palette = page.getByRole("button", { name: "Agent", exact: true });
    const from = await palette.boundingBox();
    if (!box || !from) throw new Error("not visible");
    await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2);
    await page.mouse.down();
    const target = at
      ? { x: box.x + at.x, y: box.y + at.y }
      : { x: box.x + box.width - 40, y: box.y + box.height - 30 };
    await page.mouse.move(target.x, target.y, { steps: 10 });
    await page.mouse.up();
    await page.getByLabel("Name").fill(name);
    return page.getByRole("button", { name, exact: true });
  }

  test("an agent implements in the worktree its GitHub block prepares", async ({
    page,
  }) => {
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Project" }),
      canvas(page),
    );
    const project = page.getByRole("button", { name: "Project 1" });
    await page.getByLabel("Path").fill(fixture.clone);
    // Room beside the GitHub block for the agent.
    await enlarge(page, project, 260, 160);
    const projectBox = await project.boundingBox();
    const palette = await page
      .getByRole("button", { name: "GitHub", exact: true })
      .boundingBox();
    if (!projectBox || !palette) throw new Error("not visible");
    await page.mouse.move(palette.x + 20, palette.y + 10);
    await page.mouse.down();
    await page.mouse.move(projectBox.x + 30, projectBox.y + 45, { steps: 10 });
    await page.mouse.up();
    const github = page.getByRole("button", { name: "GitHub 1" });
    await page.getByLabel("Starting point").check();
    await addAction(page, github, "Create worktree");
    const worktreePath = join(fixture.root, "worktrees", "agent");
    await page.getByLabel("Branch").fill("feature/agent");
    await page.getByLabel("Worktree path").fill(worktreePath);
    // Beside the GitHub block, inside the visible part of the project.
    const agent = await dropAgent(page, "Project 1", "Implementer", {
      x: 260,
      y: 70,
    });
    await page.getByLabel("Model").fill("haiku");
    await page
      .getByLabel("Prompt")
      .fill("Implement login on {{workspace.branch}}");

    await selectBlock(
      page.getByRole("button", { name: "Create worktree", exact: true }),
    );
    await page.getByRole("button", { name: "Connect" }).click();
    await selectBlock(agent);
    await expect(canvas(page).locator(".edge-line")).toHaveCount(1);
    await page.getByRole("button", { name: "Run flow" }).click();

    const result = page.getByRole("region", { name: "Run result" });
    await expect(result).toContainText("Run succeeded", { timeout: 15_000 });
    await expect(result).toContainText(
      `worked in ${worktreePath}: Implement login on feature/agent`,
    );
    await expect(result).toContainText("Backend: claude");
    await expect(agent).toHaveClass(/status-succeeded/);
    await page.getByRole("button", { name: "Logs of Implementer" }).click();
    const log = page.getByRole("dialog", { name: "Logs: Implementer" });
    await expect(log).toContainText("$ claude --print");
    await expect(log).toContainText('Tool call: Bash {"command":"git status"}');
  });

  test("a reviewer's validated verdict feeds the next agent", async ({
    page,
  }) => {
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Project" }),
      canvas(page),
    );
    const project = page.getByRole("button", { name: "Project 1" });
    await project.click();
    await page.getByLabel("Path").fill(fixture.clone);
    const reviewer = await dropAgent(page, "Project 1", "Reviewer");
    await page.getByLabel("Starting point").check();
    await page.getByLabel("Prompt").fill("Review the change");
    await page
      .getByLabel("Output schema")
      .fill(
        '{"type":"object","additionalProperties":false,"required":["verdict"],"properties":{"verdict":{"type":"string"}}}',
      );
    // Move the reviewer to the project's top-left so the merger has room.
    const box = await project.boundingBox();
    const reviewerBox = await reviewer.boundingBox();
    if (!box || !reviewerBox) throw new Error("not visible");
    await page.mouse.move(reviewerBox.x + 20, reviewerBox.y + 10);
    await page.mouse.down();
    await page.mouse.move(box.x + 30, box.y + 45, { steps: 10 });
    await page.mouse.up();
    const merger = await dropAgent(page, "Project 1", "Merger");
    await page.getByLabel("Prompt").fill("Merge if {{result}}");

    await selectBlock(reviewer);
    await page.getByRole("button", { name: "Connect" }).click();
    await selectBlock(merger);
    await page.getByRole("button", { name: "Run flow" }).click();

    const result = page.getByRole("region", { name: "Run result" });
    await expect(result).toContainText("Run succeeded", { timeout: 15_000 });
    await expect(result).toContainText(
      `worked in ${fixture.clone}: Merge if {"verdict":"approve"}`,
    );
  });

  test("a prompt naming a missing workspace is rejected before running", async ({
    page,
  }) => {
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Project" }),
      canvas(page),
    );
    await page.getByLabel("Path").fill(fixture.clone);
    await dropAgent(page, "Project 1", "Implementer");
    await page.getByLabel("Starting point").check();
    await page.getByLabel("Prompt").fill("Work in {{workspace.path}}");

    await page.getByRole("button", { name: "Run flow" }).click();

    await expect(
      page.getByRole("region", { name: "Run result" }),
    ).toContainText(
      "Implementer: {{workspace.path}} needs a workspace; connect a Create worktree action to this agent",
    );
  });
});
