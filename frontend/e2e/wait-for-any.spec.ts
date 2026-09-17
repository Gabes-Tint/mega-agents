import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { expect, test } from "@playwright/test";
import {
  canvas,
  connect,
  dropInto,
  enlarge,
  mouseDrag,
  selectBlock,
} from "./fixtures";

test.describe("Joining branches", () => {
  test.use({ viewport: { width: 1440, height: 1100 } });
  let folder: string;

  test.beforeEach(async ({ page }) => {
    folder = mkdtempSync(join(tmpdir(), "mega-agents-join-"));
    await page.goto("/");
  });

  test.afterEach(() => {
    rmSync(folder, { recursive: true, force: true });
  });

  test("a block waiting for any arrow runs after the branch taken", async ({
    page,
  }) => {
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Project" }),
      canvas(page),
    );
    const project = page.getByRole("button", { name: "Project 1" });
    await page.getByLabel("Path").fill(folder);
    await enlarge(page, project, 300, 320);
    const reviewer = await dropInto(
      page,
      "Agent",
      project,
      { x: 20, y: 60 },
      "Reviewer",
    );
    await page.getByLabel("Starting point").check();
    await page.getByLabel("Prompt").fill("Review");
    const route = await dropInto(
      page,
      "Router",
      project,
      { x: 20, y: 170 },
      "Route",
    );
    await page
      .getByLabel("Case 1 expression")
      .fill('value.text.contains("never")');
    const ship = await dropInto(
      page,
      "Command",
      project,
      { x: 230, y: 120 },
      "Ship",
    );
    await page.getByLabel("Command", { exact: true }).fill("echo shipped");
    const hold = await dropInto(
      page,
      "Command",
      project,
      { x: 230, y: 230 },
      "Hold",
    );
    await page.getByLabel("Command", { exact: true }).fill("echo held");
    const next = await dropInto(
      page,
      "Agent",
      project,
      { x: 120, y: 330 },
      "Next",
    );
    await page.getByLabel("Prompt").fill("Got {{result}}");
    await page.getByLabel("Run when any arrow arrives").check();
    await connect(page, reviewer, route);
    await connect(page, route, ship);
    await connect(page, route, hold);
    await selectBlock(route);
    await page.getByLabel("Output to Hold").selectOption("default");
    await connect(page, ship, next);
    await connect(page, hold, next);

    await page.getByRole("button", { name: "Run flow" }).click();

    const result = page.getByRole("region", { name: "Run result" });
    await expect(result).toContainText("Run succeeded", { timeout: 15_000 });
    await expect(result).toContainText(/Ship: command\s+skipped/);
    await expect(result).toContainText('Got {"exitCode":0,"output":"held\\n"}');
    await expect(next).toHaveClass(/status-succeeded/);
  });
});
