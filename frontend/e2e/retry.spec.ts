import { rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "@playwright/test";
import {
  canvas,
  connect,
  dropInto,
  enlarge,
  mouseDrag,
  projectClone,
  type ProjectClone,
} from "./fixtures";

// A failed run is retried through the real backend: the agent that already
// succeeded is reused, and only the failed gate runs again.
test.describe("Retrying a failed run", () => {
  let fixture: ProjectClone;

  test.beforeEach(async ({ page }) => {
    fixture = projectClone();
    await page.goto("/");
  });

  test.afterEach(() => {
    rmSync(fixture.root, { recursive: true, force: true });
  });

  test("only the failed gate runs again", async ({ page }) => {
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Project" }),
      canvas(page),
    );
    const project = page.getByRole("button", { name: "Project 1" });
    await page.getByLabel("Path").fill(fixture.clone);
    await enlarge(page, project, 240, 160);
    const agent = await dropInto(
      page,
      "Agent",
      project,
      { x: 20, y: 50 },
      "Coder",
    );
    await page.getByLabel("Starting point").check();
    await page.getByLabel("Prompt").fill("Write the code");
    const marker = join(fixture.root, "fixed");
    const gate = await dropInto(
      page,
      "Command",
      project,
      { x: 220, y: 150 },
      "Gate",
    );
    await page.getByLabel("Command", { exact: true }).fill(`test -e ${marker}`);
    await connect(page, agent, gate);

    await page.getByRole("button", { name: "Run flow" }).click();
    const result = page.getByRole("region", { name: "Run result" });
    await expect(result).toContainText("Run failed", { timeout: 15_000 });

    writeFileSync(marker, "");
    await page.getByRole("button", { name: "Retry from failure" }).click();

    await expect(result).toContainText("Run succeeded", { timeout: 15_000 });
    await expect(result).toContainText("Retry of ");
    await expect(result).toContainText("Reused from: ");
    await expect(gate).toHaveClass(/status-succeeded/);
    await page.getByRole("button", { name: "Logs of Coder" }).click();
    await expect(
      page.getByRole("dialog", { name: "Logs: Coder" }),
    ).toContainText("Reused the result of Coder from run");
  });
});
