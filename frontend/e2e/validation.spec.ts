import { rmSync } from "node:fs";
import { expect, test } from "@playwright/test";
import {
  canvas,
  connect,
  dropInto,
  enlarge,
  mouseDrag,
  projectClone,
  selectBlock,
  type ProjectClone,
} from "./fixtures";

// Schema blocks validate agent results through the real backend, with the
// fake Claude Code CLI answering "worked in <folder>: <prompt>".
test.describe("JSON Schema blocks", () => {
  let fixture: ProjectClone;

  test.beforeEach(async ({ page }) => {
    fixture = projectClone();
    await page.goto("/");
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Project" }),
      canvas(page),
    );
    await page.getByLabel("Path").fill(fixture.clone);
    await enlarge(
      page,
      page.getByRole("button", { name: "Project 1" }),
      260,
      220,
    );
  });

  test.afterEach(() => {
    rmSync(fixture.root, { recursive: true, force: true });
  });

  async function writerChecked(
    page: import("@playwright/test").Page,
    pattern: string,
  ) {
    const project = page.getByRole("button", { name: "Project 1" });
    const writer = await dropInto(
      page,
      "Agent",
      project,
      { x: 20, y: 50 },
      "Writer",
    );
    await page.getByLabel("Starting point").check();
    await page.getByLabel("Prompt").fill("Write the summary");
    const check = await dropInto(
      page,
      "JSON Schema",
      project,
      { x: 20, y: 170 },
      "Check",
    );
    await page
      .getByLabel("Schema")
      .fill(
        `{"type":"object","required":["text"],"properties":{"text":{"type":"string","pattern":"${pattern}"}}}`,
      );
    await connect(page, writer, check);
    return { project, check };
  }

  test("an invalid result takes the invalid branch to a fixer", async ({
    page,
  }) => {
    const { project, check } = await writerChecked(page, "^approved$");
    const fixer = await dropInto(
      page,
      "Agent",
      project,
      { x: 250, y: 170 },
      "Fixer",
    );
    await page.getByLabel("Prompt").fill("Fix {{result}}");
    await connect(page, check, fixer);
    await selectBlock(check);
    await page.getByLabel("Output to Fixer").selectOption("invalid");
    await expect(canvas(page).locator(".edge-port")).toHaveText("invalid");

    await page.getByRole("button", { name: "Run flow" }).click();

    const result = page.getByRole("region", { name: "Run result" });
    await expect(result).toContainText("Run succeeded", { timeout: 15_000 });
    await expect(result).toContainText("Valid: false");
    await expect(result).toContainText("$.text: 'worked in");
    await expect(result).toContainText('Fix {"errors":[{"path":"$.text"');
    await expect(check).toHaveClass(/status-succeeded/);
  });

  test("an invalid result nothing handles fails the run", async ({ page }) => {
    const { check } = await writerChecked(page, "^approved$");

    await page.getByRole("button", { name: "Run flow" }).click();

    const result = page.getByRole("region", { name: "Run result" });
    await expect(result).toContainText("Run failed", { timeout: 15_000 });
    await expect(result).toContainText("the value did not satisfy the schema");
    await expect(check).toHaveClass(/status-failed/);
  });

  test("a valid result passes", async ({ page }) => {
    const { check } = await writerChecked(page, "^worked in ");

    await page.getByRole("button", { name: "Run flow" }).click();

    const result = page.getByRole("region", { name: "Run result" });
    await expect(result).toContainText("Run succeeded", { timeout: 15_000 });
    await expect(result).toContainText("Valid: true");
    await expect(check).toHaveClass(/status-succeeded/);
  });
});
