import { expect, test } from "@playwright/test";
import {
  canvas,
  connect,
  dropInto,
  enlarge,
  mouseDrag,
  selectBlock,
} from "./fixtures";

test.describe("Deleting blocks", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/");
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Project" }),
      canvas(page),
    );
    await enlarge(
      page,
      page.getByRole("button", { name: "Project 1" }),
      260,
      160,
    );
  });

  test("deletes a block and its arrows from the properties panel", async ({
    page,
  }) => {
    const project = page.getByRole("button", { name: "Project 1" });
    const coder = await dropInto(
      page,
      "Agent",
      project,
      { x: 90, y: 90 },
      "Coder",
    );
    const reviewer = await dropInto(
      page,
      "Agent",
      project,
      { x: 330, y: 150 },
      "Reviewer",
    );
    await connect(page, coder, reviewer);
    await expect(canvas(page).locator(".edge-line")).toHaveCount(1);

    await selectBlock(coder);
    await page.getByRole("button", { name: "Delete block" }).click();

    await expect(coder).toHaveCount(0);
    await expect(reviewer).toBeVisible();
    await expect(canvas(page).locator(".edge-line")).toHaveCount(0);
    await page.reload();
    await expect(page.getByRole("button", { name: "Reviewer" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Coder" })).toHaveCount(0);
  });

  test("the Delete key deletes a container with everything inside", async ({
    page,
  }) => {
    const project = page.getByRole("button", { name: "Project 1" });
    await dropInto(page, "Agent", project, { x: 90, y: 90 }, "Coder");

    await selectBlock(project);
    await page.keyboard.press("Delete");

    await expect(project).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Coder" })).toHaveCount(0);
    await expect(
      page.getByText("Drag components here to build your graph"),
    ).toBeVisible();
  });
});
