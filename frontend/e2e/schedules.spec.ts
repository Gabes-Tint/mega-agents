import { rmSync } from "node:fs";
import { expect, test, type Page } from "@playwright/test";
import { canvas, mouseDrag, projectClone, type ProjectClone } from "./fixtures";

// Schedules are cron expressions kept in the workflow file and read by the
// real Go backend, which is also what starts the runs they are due for.
test.describe("Schedules", () => {
  let fixture: ProjectClone;

  test.beforeEach(async ({ page }) => {
    fixture = projectClone();
    await page.goto("/");
  });

  test.afterEach(() => {
    rmSync(fixture.root, { recursive: true, force: true });
  });

  // Saves a one-agent workflow the fake Claude answers, and returns its name.
  async function saveWorkflow(page: Page, prefix: string): Promise<string> {
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Project" }),
      canvas(page),
    );
    const project = page.getByRole("button", { name: "Project 1" });
    await page.getByLabel("Path").fill(fixture.clone);
    const box = await project.boundingBox();
    const palette = await page
      .getByRole("button", { name: "Agent", exact: true })
      .boundingBox();
    if (!box || !palette) throw new Error("not visible");
    await page.mouse.move(palette.x + 20, palette.y + 10);
    await page.mouse.down();
    await page.mouse.move(box.x + box.width - 50, box.y + box.height - 30, {
      steps: 10,
    });
    await page.mouse.up();
    await page.getByLabel("Name", { exact: true }).fill("Planner");
    await page.getByLabel("Prompt").fill("Plan the work");
    await page.getByLabel("Starting point").check();
    const name = `${prefix}-${Date.now()}`;
    await page.getByLabel("Workflow name").fill(name);
    await page.getByRole("button", { name: "Save" }).click();
    await expect(
      page.getByRole("status", { name: "Workflow file" }),
    ).toHaveText(`Saved as ${name}`);
    return name;
  }

  test("a workflow is put on a schedule, which says when it will run", async ({
    page,
  }) => {
    const name = await saveWorkflow(page, "scheduled");

    await page.getByRole("link", { name: "Schedules…" }).click();

    await expect(page).toHaveURL(/\/workflows$/);
    const field = page.getByLabel(`Schedule for ${name}`, { exact: true });
    await expect(field).toHaveValue("");
    const row = page.locator(".workflow").filter({ hasText: name });
    await expect(row).toContainText("Runs when you ask it to");

    // The runs appear while the expression is being typed, before anything
    // is saved.
    await field.fill("30 9 * * 1-5");
    await expect(row).toContainText("Next runs once saved");
    await expect(row.locator("time")).toHaveCount(3);
    await expect(row.locator("time").first()).toContainText("09:30");

    await page
      .getByRole("button", { name: `Save schedule for ${name}` })
      .click();
    await expect(row).toContainText("Now on the schedule");

    // The schedule is in the workflow file, so it is still there in a fresh
    // page, and the workflow opens in the editor from its name.
    await page.reload();
    await expect(
      page.getByLabel(`Schedule for ${name}`, { exact: true }),
    ).toHaveValue("30 9 * * 1-5");
    await page.getByRole("link", { name, exact: true }).click();
    await expect(page.getByLabel("Workflow name")).toHaveValue(name);
  });

  test("an expression that cannot run is explained and is not saved", async ({
    page,
  }) => {
    const name = await saveWorkflow(page, "invalid");
    await page.getByRole("link", { name: "Schedules…" }).click();
    const row = page.locator(".workflow").filter({ hasText: name });

    await page
      .getByLabel(`Schedule for ${name}`, { exact: true })
      .fill("0 9 * *");

    await expect(row).toContainText("five fields");
    await expect(
      page.getByRole("button", { name: `Save schedule for ${name}` }),
    ).toBeDisabled();
  });

  test("a schedule is taken off again", async ({ page }) => {
    const name = await saveWorkflow(page, "cleared");
    await page.getByRole("link", { name: "Schedules…" }).click();
    const row = page.locator(".workflow").filter({ hasText: name });
    await page
      .getByLabel(`Schedule for ${name}`, { exact: true })
      .fill("@daily");
    await page
      .getByRole("button", { name: `Save schedule for ${name}` })
      .click();
    await expect(row).toContainText("Now on the schedule");

    await page
      .getByRole("button", { name: `Clear schedule for ${name}` })
      .click();

    await expect(row).toContainText("Taken off the schedule");
    await expect(
      page.getByLabel(`Schedule for ${name}`, { exact: true }),
    ).toHaveValue("");
    await page.reload();
    await expect(
      page.getByLabel(`Schedule for ${name}`, { exact: true }),
    ).toHaveValue("");
  });

  // The whole point of a schedule: the server starts the run itself. A
  // schedule due every minute is waited out, so this test runs past the
  // usual timeout.
  test("a workflow due every minute is started by the server", async ({
    page,
  }) => {
    test.setTimeout(180_000);
    const name = await saveWorkflow(page, "every-minute");
    await page.getByRole("link", { name: "Schedules…" }).click();
    await page
      .getByLabel(`Schedule for ${name}`, { exact: true })
      .fill("* * * * *");
    await page
      .getByRole("button", { name: `Save schedule for ${name}` })
      .click();
    await expect(
      page.locator(".workflow").filter({ hasText: name }),
    ).toContainText("Now on the schedule");

    await expect
      .poll(
        async () => {
          const response = await page.request.get("/api/runs");
          const runs = (await response.json()) as {
            workflow: string;
            trigger?: string;
          }[];
          return runs.filter(
            (run) => run.workflow === name && run.trigger === "schedule",
          ).length;
        },
        { timeout: 150_000, intervals: [2_000] },
      )
      .toBeGreaterThan(0);
  });
});
