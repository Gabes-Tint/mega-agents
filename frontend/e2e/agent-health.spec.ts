import { expect, type Locator, test } from "@playwright/test";

// The backend checks each agent CLI when it starts. On the fake CLIs Claude
// Code, Codex and OpenCode answer, and Grok fails after a moment.
const markStyle = (logo: Locator) =>
  logo.locator("svg").evaluate((svg) => {
    const style = getComputedStyle(svg);
    return { filter: style.filter, opacity: style.opacity };
  });

test.describe("Agent health", () => {
  test("shows answering backends in color and the others greyed out, and checks again on click", async ({
    page,
  }) => {
    await page.goto("/");
    const agents = page
      .getByRole("contentinfo")
      .getByRole("group", { name: "Agent backends" });

    const claude = agents.getByRole("button", {
      name: /^Claude Code: responded in /,
    });
    const grok = agents.getByRole("button", {
      name: "Grok: grok reported an error: no credits",
    });
    await expect(claude).toBeVisible({ timeout: 15_000 });
    await expect(grok).toBeVisible({ timeout: 15_000 });
    await expect(
      agents.getByRole("button", { name: /^Codex: responded in / }),
    ).toBeVisible();
    await expect(
      agents.getByRole("button", { name: /^OpenCode: responded in / }),
    ).toBeVisible();
    expect(await markStyle(claude)).toEqual({ filter: "none", opacity: "1" });
    expect(await markStyle(grok)).toEqual({
      filter: "grayscale(1)",
      opacity: "0.35",
    });

    await grok.click();

    await expect(
      agents.getByRole("button", { name: "Grok: checking…" }),
    ).toBeVisible();
    await expect(grok).toBeVisible({ timeout: 15_000 });
    await expect(claude).toBeVisible();
  });
});
