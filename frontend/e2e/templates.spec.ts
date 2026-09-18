import { expect, test } from "@playwright/test";
import { routePoint } from "./fixtures";

// Templates come from the real backend's embedded workflows.
test.describe("Templates", () => {
  test("every template opens as a flow on the canvas", async ({ page }) => {
    await page.goto("/");
    for (const [title, block] of [
      ["Issue to pull request", "Open pull request"],
      ["Review and route", "Route"],
      ["Fit_ development flow", "Solver implements"],
      ["Gate and fix", "Gate again"],
    ]) {
      await page.getByRole("button", { name: "Templates…" }).click();
      await page.getByRole("button", { name: title, exact: true }).click();
      await expect(
        page.getByRole("button", { name: block, exact: true }),
      ).toBeVisible();
      await expect(
        page.getByRole("status", { name: "Workflow file" }),
      ).toHaveText(`Started from template ${title}`);
      if (title === "Issue to pull request") {
        // It reads the next available issue.
        await page
          .getByRole("button", { name: "Read issue", exact: true })
          .click();
        await expect(page.getByLabel("Issue number")).toHaveValue("0");
      }
    }
    // Arrows between nested blocks are drawn above their container. Hit
    // testing skips pointer-transparent layers, so let it see the arrows
    // while checking which element paints on top.
    await page.addStyleTag({
      content: ".edges, .edge-line { pointer-events: auto !important; }",
    });
    const line = page.locator(".edge-line").first();
    // Arrows bend around the blocks in the way, so the middle of the box
    // around one need not be on it: walk the route instead.
    const middle = await routePoint(line);
    const topmost = await page.evaluate(
      ([x, y]) => document.elementsFromPoint(x, y).map((el) => el.tagName),
      [middle.x, middle.y],
    );
    expect(topmost[0]).toBe("path");
    await page.screenshot({ path: "test-results/template-gate-and-fix.png" });
  });
});
