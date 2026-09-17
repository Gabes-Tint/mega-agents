import { expect, test } from "@playwright/test";

test.describe("Theme", () => {
  test("follows the system theme and remembers a switch", async ({ page }) => {
    await page.emulateMedia({ colorScheme: "dark" });
    await page.goto("/");
    const root = page.locator("html");
    await expect(root).toHaveAttribute("data-theme", "dark");
    const darkBackground = await page
      .locator("body")
      .evaluate((body) => getComputedStyle(body).backgroundColor);

    await page.getByRole("button", { name: "Switch to light theme" }).click();

    await expect(root).toHaveAttribute("data-theme", "light");
    await expect
      .poll(() =>
        page
          .locator("body")
          .evaluate((body) => getComputedStyle(body).backgroundColor),
      )
      .not.toBe(darkBackground);
    await page.reload();
    await expect(root).toHaveAttribute("data-theme", "light");
    await expect(
      page.getByRole("button", { name: "Switch to dark theme" }),
    ).toBeVisible();
  });
});
