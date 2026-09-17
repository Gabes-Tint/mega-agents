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
    // The check goes inside the writer and takes its reply with no arrow;
    // the writer grows to hold it.
    const check = await dropInto(
      page,
      "JSON Schema",
      writer,
      { x: 20, y: 45 },
      "Check",
    );
    await page
      .getByLabel("Schema")
      .fill(
        `{"type":"object","required":["text"],"properties":{"text":{"type":"string","pattern":"${pattern}"}}}`,
      );
    return { project, check };
  }

  test("a check goes only inside an agent", async ({ page }) => {
    const project = page.getByRole("button", { name: "Project 1" });
    const box = await project.boundingBox();
    const palette = await page
      .getByRole("button", { name: "JSON Schema", exact: true })
      .boundingBox();
    if (!box || !palette) throw new Error("drag endpoints are not visible");
    await page.mouse.move(palette.x + 10, palette.y + 10);
    await page.mouse.down();
    await page.mouse.move(box.x + 200, box.y + 150, { steps: 10 });

    await expect(canvas(page).locator(".drop-preview")).toHaveClass(/invalid/);
    await expect(project).toHaveClass(/drop-no/);
    await page.mouse.up();
    await expect(
      page.getByRole("button", { name: "JSON Schema 1" }),
    ).toHaveCount(0);

    const { check } = await writerChecked(page, "^x$");
    const writer = page.getByRole("button", { name: "Writer", exact: true });
    // The writer eases into its new size.
    await expect
      .poll(async () => {
        const [inner, outer] = await Promise.all([
          check.boundingBox(),
          writer.boundingBox(),
        ]);
        return (
          !!inner &&
          !!outer &&
          inner.x >= outer.x &&
          inner.y + inner.height <= outer.y + outer.height
        );
      })
      .toBe(true);
  });

  test("an invalid result takes the invalid branch to a fixer", async ({
    page,
  }) => {
    const { project, check } = await writerChecked(page, "^approved$");
    const fixer = await dropInto(
      page,
      "Agent",
      project,
      { x: 290, y: 60 },
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
