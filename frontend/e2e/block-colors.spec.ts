import { expect, test, type Locator } from "@playwright/test";
import { canvas, dropInto, enlarge, mouseDrag } from "./fixtures";

const headerColor = (block: Locator) =>
  block
    .locator(".node-title")
    .evaluate((title) => getComputedStyle(title).backgroundColor);

test("each block type has its own color in both themes", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "light" });
  await page.goto("/");
  await mouseDrag(
    page,
    page.getByRole("button", { name: "Project" }),
    canvas(page),
  );
  const project = page.getByRole("button", { name: "Project 1" });
  await enlarge(page, project, 260, 160);
  const agent = await dropInto(
    page,
    "Agent",
    project,
    { x: 90, y: 90 },
    "Coder",
  );
  const command = await dropInto(
    page,
    "Command",
    project,
    { x: 330, y: 150 },
    "Tests",
  );

  const light = [
    await headerColor(project),
    await headerColor(agent),
    await headerColor(command),
  ];
  expect(new Set(light).size).toBe(3);

  await page.getByRole("button", { name: "Switch to dark theme" }).click();

  const dark = [
    await headerColor(project),
    await headerColor(agent),
    await headerColor(command),
  ];
  expect(new Set(dark).size).toBe(3);
  expect(dark[1]).not.toBe(light[1]);
});
