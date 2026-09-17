import { rmSync } from "node:fs";
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

test.describe("Parallel runs", () => {
  let fixture: ProjectClone;

  test.beforeEach(async ({ page }) => {
    fixture = projectClone();
    await page.goto("/");
  });

  test.afterEach(() => {
    rmSync(fixture.root, { recursive: true, force: true });
  });

  test("independent agents run at the same time", async ({ page }) => {
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Project" }),
      canvas(page),
    );
    const project = page.getByRole("button", { name: "Project 1" });
    await page.getByLabel("Path").fill(fixture.clone);
    await enlarge(page, project, 240, 160);
    const planner = await dropInto(
      page,
      "Agent",
      project,
      { x: 20, y: 50 },
      "Planner",
    );
    await page.getByLabel("Starting point").check();
    await page.getByLabel("Prompt").fill("Split the story");
    const domain = await dropInto(
      page,
      "Agent",
      project,
      { x: 20, y: 170 },
      "Domain",
    );
    await page.getByLabel("Prompt").fill("[wait] domain slice");
    const ui = await dropInto(page, "Agent", project, { x: 220, y: 170 }, "UI");
    await page.getByLabel("Prompt").fill("[wait] ui slice");
    await connect(page, planner, domain);
    await connect(page, planner, ui);

    await page.getByRole("button", { name: "Run flow" }).click();

    await expect(domain).toHaveClass(/status-running/);
    await expect(ui).toHaveClass(/status-running/);
    await expect(
      page.getByRole("region", { name: "Run result" }),
    ).toContainText("Run succeeded", { timeout: 15_000 });
  });
});
