import { expect, test } from "@playwright/test";

test("hovering a palette component explains it", async ({ page }) => {
  await page.goto("/");
  const entry = page.getByRole("button", { name: "JSON Schema" });

  await entry.hover();

  const tooltip = page.getByRole("tooltip");
  await expect(tooltip).toBeVisible();
  await expect(tooltip).toContainText("valid or invalid");
  const entryBox = await entry.boundingBox();
  const tooltipBox = await tooltip.boundingBox();
  expect(tooltipBox!.x).toBeGreaterThanOrEqual(entryBox!.x + entryBox!.width);

  await page.mouse.move(700, 400);

  await expect(tooltip).toHaveCount(0);
});
