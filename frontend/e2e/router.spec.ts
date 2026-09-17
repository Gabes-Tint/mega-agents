import { rmSync } from "node:fs";
import { expect, test, type Page } from "@playwright/test";
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

// Routers route agent verdicts through the real backend, with the fake
// Claude Code CLI answering {"verdict":"approve"} to schema turns.
test.describe("Router blocks", () => {
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
      300,
      220,
    );
  });

  test.afterEach(() => {
    rmSync(fixture.root, { recursive: true, force: true });
  });

  async function reviewRouted(page: Page, expression: string) {
    const project = page.getByRole("button", { name: "Project 1" });
    const reviewer = await dropInto(
      page,
      "Agent",
      project,
      { x: 20, y: 50 },
      "Reviewer",
    );
    await page.getByLabel("Starting point").check();
    await page.getByLabel("Prompt").fill("Review");
    await page
      .getByLabel("Output schema")
      .fill(
        '{"type":"object","additionalProperties":false,"required":["verdict"],"properties":{"verdict":{"type":"string"}}}',
      );
    const route = await dropInto(
      page,
      "Router",
      project,
      { x: 20, y: 170 },
      "Route",
    );
    await page.getByLabel("Case 1 expression").fill(expression);
    const ship = await dropInto(
      page,
      "Agent",
      project,
      { x: 250, y: 60 },
      "Ship",
    );
    await page.getByLabel("Prompt").fill("Ship {{result}}");
    const fix = await dropInto(
      page,
      "Agent",
      project,
      { x: 250, y: 190 },
      "Fix",
    );
    await page.getByLabel("Prompt").fill("Fix {{result}}");
    await connect(page, reviewer, route);
    await connect(page, route, ship);
    await connect(page, route, fix);
    await selectBlock(route);
    await page.getByLabel("Output to Fix").selectOption("default");
    await expect(canvas(page).locator(".edge-port")).toHaveText([
      "approved",
      "default",
    ]);
    await page.getByRole("button", { name: "Run flow" }).click();
    const result = page.getByRole("region", { name: "Run result" });
    await expect(result).toContainText("Run succeeded", { timeout: 15_000 });
    return { result, ship, fix };
  }

  test("a matching case routes the verdict to its branch", async ({ page }) => {
    const { result, ship, fix } = await reviewRouted(
      page,
      'value.verdict == "approve"',
    );

    await expect(result).toContainText("Route: approved");
    await expect(result).toContainText(
      'Ship {"case":"approved","value":{"verdict":"approve"}}',
    );
    await expect(ship).toHaveClass(/status-succeeded/);
    await expect(fix).toHaveClass(/status-skipped/);
  });

  test("with no matching case the verdict takes the default route", async ({
    page,
  }) => {
    const { result, ship, fix } = await reviewRouted(
      page,
      'value.verdict == "reject"',
    );

    await expect(result).toContainText("Route: default");
    await expect(fix).toHaveClass(/status-succeeded/);
    await expect(ship).toHaveClass(/status-skipped/);
  });

  test("an expression that does not compile is rejected before running", async ({
    page,
  }) => {
    const project = page.getByRole("button", { name: "Project 1" });
    const writer = await dropInto(
      page,
      "Agent",
      project,
      { x: 20, y: 50 },
      "Writer",
    );
    await page.getByLabel("Starting point").check();
    await page.getByLabel("Prompt").fill("Write");
    const route = await dropInto(
      page,
      "Router",
      project,
      { x: 20, y: 170 },
      "Route",
    );
    await page.getByLabel("Case 1 expression").fill("value.verdict ==");
    await connect(page, writer, route);

    await page.getByRole("button", { name: "Run flow" }).click();

    await expect(
      page.getByRole("region", { name: "Run result" }),
    ).toContainText('Route: case "approved": ERROR');
  });
});
