import { expect, test } from "@playwright/test";
import { addAction, canvas, dropInto, enlarge, mouseDrag } from "./fixtures";

test("a block header shows its id over an icon of its type", async ({
  page,
}) => {
  await page.goto("/");
  await mouseDrag(
    page,
    page.getByRole("button", { name: "Project" }),
    canvas(page),
  );
  const project = page.getByRole("button", { name: "Project 1" });
  await enlarge(page, project, 300, 200);
  const github = await dropInto(
    page,
    "GitHub",
    project,
    { x: 20, y: 50 },
    "GitHub 1",
  );
  const agent = await dropInto(
    page,
    "Agent",
    project,
    { x: 260, y: 60 },
    "Coder",
  );
  await addAction(page, github, "Commit");

  const icon = github.locator(".node-meta > .block-icon");
  await expect(icon).toHaveAttribute("data-icon", "github");
  await expect(icon).toHaveClass(/object/);
  const id = await github.locator(".node-meta > .node-id").boundingBox();
  const mark = await icon.boundingBox();
  expect(mark!.y).toBeGreaterThanOrEqual(id!.y + id!.height);
  await expect(agent.locator(".block-icon")).toHaveAttribute(
    "data-icon",
    "agent",
  );
  await expect(agent.locator(".block-icon")).toHaveClass(/action/);
  await expect(
    page
      .getByRole("button", { name: "Commit", exact: true })
      .locator(".block-icon"),
  ).toHaveAttribute("data-icon", "commit");
});
