import { expect, test, type Page } from "@playwright/test";
import { canvas, mouseDrag } from "./fixtures";

// Every saved workflow has its own address, so it can be bookmarked, shared
// and reopened by link, and Back walks between the workflows that were open.
test.describe("Workflow addresses", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/");
    await page.evaluate(() => localStorage.clear());
    await page.reload();
  });

  // Draws a one-block workflow and saves it under the name.
  async function saveWorkflow(page: Page, block: string, name: string) {
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Project" }),
      canvas(page),
    );
    await page.getByLabel("Name", { exact: true }).fill(block);
    await page.getByLabel("Workflow name").fill(name);
    await page.getByRole("button", { name: "Save" }).click();
    await expect(
      page.getByRole("status", { name: "Workflow file" }),
    ).toHaveText(`Saved as ${name}`);
  }

  // A fresh editor on the scratch page, with no graph in progress.
  async function emptyEditor(page: Page) {
    await page.goto("/");
    await page.evaluate(() => localStorage.clear());
    await page.reload();
  }

  test("saving moves the address to the workflow, and Back walks between workflows", async ({
    page,
  }) => {
    const stamp = Date.now();
    const first = `urls-alpha-${stamp}`;
    const second = `urls-beta-${stamp}`;
    await saveWorkflow(page, "Alpha", first);
    await expect(page).toHaveURL(new RegExp(`/workflows/${first}$`));
    await emptyEditor(page);
    await saveWorkflow(page, "Beta", second);
    await expect(page).toHaveURL(new RegExp(`/workflows/${second}$`));

    await page.getByRole("button", { name: "Open…" }).click();
    await page.getByRole("button", { name: first, exact: true }).click();

    await expect(page).toHaveURL(new RegExp(`/workflows/${first}$`));
    await expect(page.getByRole("button", { name: "Alpha" })).toBeVisible();

    await page.goBack();

    await expect(page).toHaveURL(new RegExp(`/workflows/${second}$`));
    await expect(page.getByRole("button", { name: "Beta" })).toBeVisible();

    await page.goForward();

    await expect(page).toHaveURL(new RegExp(`/workflows/${first}$`));
    await expect(page.getByRole("button", { name: "Alpha" })).toBeVisible();
  });

  test("a deep link reopens the workflow it names", async ({ page }) => {
    const name = `urls-link-${Date.now()}`;
    await saveWorkflow(page, "Linked", name);
    await emptyEditor(page);

    await page.goto(`/workflows/${name}`);

    await expect(page.getByRole("button", { name: "Linked" })).toBeVisible();
    await expect(page.getByLabel("Workflow name")).toHaveValue(name);

    await page.reload();

    await expect(page.getByRole("button", { name: "Linked" })).toBeVisible();
  });

  test("an address with no workflow names it and leads back to the editor", async ({
    page,
  }) => {
    const missing = `urls-missing-${Date.now()}`;

    await page.goto(`/workflows/${missing}`);

    const message = page.getByRole("region", { name: "Page not found" });
    await expect(message).toContainText(missing);

    await message.getByRole("button", { name: "Back to the editor" }).click();

    await expect(page).toHaveURL(/\/$/);
    await expect(message).toHaveCount(0);
    await expect(canvas(page)).toBeVisible();
  });
});
