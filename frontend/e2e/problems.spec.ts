import { expect, test } from "@playwright/test";
import { canvas, dropInto, enlarge, mouseDrag, selectBlock } from "./fixtures";

test.describe("Problems", () => {
  test("lists what to fix before running and clears as it is fixed", async ({
    page,
  }) => {
    await page.goto("/");
    const problems = page.getByRole("tabpanel", { name: /Problems/ });
    await expect(problems).toContainText("Nothing to check yet");

    await mouseDrag(
      page,
      page.getByRole("button", { name: "Project" }),
      canvas(page),
    );
    const project = page.getByRole("button", { name: "Project 1" });
    await enlarge(page, project, 200, 120);
    const coder = await dropInto(
      page,
      "Agent",
      project,
      { x: 90, y: 90 },
      "Coder",
    );

    await expect(problems).toContainText(
      "flag a GitHub block or an agent as the starting point",
    );
    await expect(
      page.getByRole("button", { name: "1 error, 0 warnings" }),
    ).toBeVisible();

    await selectBlock(coder);
    await page.getByLabel("Starting point").check();

    await expect(problems).toContainText(
      "Coder: write the prompt the agent receives",
    );
    await expect(problems).toContainText(
      "Project 1: set the folder its blocks work in",
    );
    await expect(coder).toHaveClass(/problem-error/);
    await expect(project).toHaveClass(/problem-warning/);

    await page.getByLabel("Prompt").fill("Say hello");
    await selectBlock(project);
    await page.getByLabel("Path").fill("/tmp");

    await expect(problems).toContainText("No problems found");
    await expect(
      page.getByRole("button", { name: "0 errors, 0 warnings" }),
    ).toBeVisible();
    await expect(coder).not.toHaveClass(/problem-error/);
  });

  test("choosing a problem selects the block to fix", async ({ page }) => {
    await page.goto("/");
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Project" }),
      canvas(page),
    );
    const project = page.getByRole("button", { name: "Project 1" });
    await enlarge(page, project, 200, 120);
    const coder = await dropInto(
      page,
      "Agent",
      project,
      { x: 90, y: 90 },
      "Coder",
    );
    await page.getByLabel("Starting point").check();
    await selectBlock(project);

    await page
      .getByRole("listitem")
      .filter({ hasText: "Coder: write the prompt" })
      .getByRole("button", { name: /Select the block with problem/ })
      .click();

    await expect(page.getByLabel("Name", { exact: true })).toHaveValue("Coder");
    await expect(coder).toHaveAttribute("aria-pressed", "true");
  });
});
