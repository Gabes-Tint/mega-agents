import { rmSync } from "node:fs";
import { expect, test } from "@playwright/test";
import {
  canvas,
  dropInto,
  mouseDrag,
  projectClone,
  type ProjectClone,
} from "./fixtures";

test.describe("Run history and cancelling", () => {
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
  });

  test.afterEach(() => {
    rmSync(fixture.root, { recursive: true, force: true });
  });

  test("a long agent turn can be cancelled and found again in the history", async ({
    page,
  }) => {
    const name = `slow-${Date.now()}`;
    await page.getByLabel("Workflow name").fill(name);
    const agent = await dropInto(
      page,
      "Agent",
      page.getByRole("button", { name: "Project 1" }),
      { x: 30, y: 40 },
      "Thinker",
    );
    await page.getByLabel("Starting point").check();
    await page.getByLabel("Prompt").fill("[slow] think hard");

    await page.getByRole("button", { name: "Run flow" }).click();
    await expect(agent).toHaveClass(/status-running/);
    await page.getByRole("button", { name: "Cancel run" }).click();

    const result = page.getByRole("region", { name: "Run result" });
    await expect(result).toContainText("Run cancelled", { timeout: 10_000 });
    await expect(agent).toHaveClass(/status-failed/);

    await page.getByRole("button", { name: "Runs…" }).click();
    await page
      .getByRole("button", { name: new RegExp(`${name} · cancelled`) })
      .click();
    await expect(result).toContainText("Run cancelled");
    await page.getByRole("button", { name: "Logs of Thinker" }).click();
    await expect(
      page.getByRole("dialog", { name: "Logs: Thinker" }),
    ).toContainText("claude turn stopped");
  });
});
