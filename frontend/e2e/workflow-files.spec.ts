import { rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "@playwright/test";
import {
  addAction,
  projectClone,
  projectWithGitHub,
  type ProjectClone,
} from "./fixtures";

// Workflows are saved as YAML under the backend's temporary Mega Agents home.
test.describe("Workflow files", () => {
  let fixture: ProjectClone;

  test.beforeEach(async ({ page }) => {
    fixture = projectClone();
    await page.goto("/");
    await page.evaluate(() => localStorage.clear());
    await page.reload();
  });

  test.afterEach(() => {
    rmSync(fixture.root, { recursive: true, force: true });
  });

  test("a saved workflow opens in a fresh editor and runs", async ({
    page,
  }) => {
    const github = await projectWithGitHub(page, fixture.clone);
    await page.getByLabel("Already authenticated (OAuth)").check();
    await page.getByLabel("Starting point").check();
    await addAction(page, github, "Fetch");
    const name = `saved-${Date.now()}`;
    await page.getByLabel("Workflow name").fill(name);
    await page.getByRole("button", { name: "Save" }).click();
    await expect(
      page.getByRole("status", { name: "Workflow file" }),
    ).toHaveText(`Saved as ${name}`);

    // Saving moved the address to the workflow, so a fresh editor starts
    // from the scratch page again.
    await page.goto("/");
    await page.evaluate(() => localStorage.clear());
    await page.reload();
    await expect(page.getByRole("button", { name: "GitHub 1" })).toHaveCount(0);
    await page.getByRole("button", { name: "Open…" }).click();
    await page.getByRole("button", { name, exact: true }).click();

    await expect(
      page.getByRole("button", { name: "Fetch", exact: true }),
    ).toBeVisible();
    await expect(page.getByLabel("Workflow name")).toHaveValue(name);
    await page.getByRole("button", { name: "Run flow" }).click();
    await expect(
      page.getByRole("region", { name: "Run result" }),
    ).toContainText("Run succeeded", { timeout: 15_000 });
  });

  test("the graph in progress survives a page reload", async ({ page }) => {
    await projectWithGitHub(page, fixture.clone);

    await page.reload();

    await expect(page.getByRole("button", { name: "GitHub 1" })).toBeVisible();
  });

  test("an older Read issue ignores the default labels until they are cleared", async ({
    page,
  }) => {
    const name = `old-${Date.now()}`;
    const file = join(fixture.root, "old.yaml");
    // Written before Read issue had labels to ignore. The fake GitHub CLI
    // labels issue #8 "paused".
    writeFileSync(
      file,
      `apiVersion: megaagents.dev/v1alpha1
kind: Workflow
metadata:
  name: ${name}
nodes:
  api:
    uses: project@v1
    with:
      path: "${fixture.clone}"
    layout: { x: 40, y: 40, w: 400, h: 300 }
    children:
      github:
        uses: github@v1
        start: true
        with:
          authenticated: true
        layout: { x: 20, y: 40, w: 220, h: 200 }
        children:
          read-issue:
            uses: git/issue@v1
            name: "Read issue"
            start: true
            with:
              issue: 8
            layout: { x: 12, y: 40, w: 160, h: 64 }
`,
    );
    await page.getByLabel("Import YAML").setInputFiles(file);
    const readIssue = page.getByRole("button", {
      name: "Read issue",
      exact: true,
    });
    await readIssue.click();
    const labels = page.getByLabel("Labels to ignore");
    await expect(labels).toHaveValue("paused, draft, needs-attention");
    const result = page.getByRole("region", { name: "Run result" });
    await page.getByRole("button", { name: "Run flow" }).click();
    await expect(result).toContainText(
      'issue #8 is labeled "paused", one of the labels to ignore',
      { timeout: 15_000 },
    );

    await labels.fill("");
    await page.getByRole("button", { name: "Save" }).click();
    await expect(
      page.getByRole("status", { name: "Workflow file" }),
    ).toHaveText(`Saved as ${name}`);
    await page.goto("/");
    await page.evaluate(() => localStorage.clear());
    await page.reload();
    await page.getByRole("button", { name: "Open…" }).click();
    await page.getByRole("button", { name, exact: true }).click();
    await readIssue.click();

    await expect(labels).toHaveValue("");
    await page.getByRole("button", { name: "Run flow" }).click();
    await expect(result).toContainText("Run succeeded", { timeout: 15_000 });
  });

  test("a workflow YAML file imports into the editor", async ({ page }) => {
    const file = join(fixture.root, "imported.yaml");
    writeFileSync(
      file,
      `apiVersion: megaagents.dev/v1alpha1
kind: Workflow
metadata:
  name: imported
nodes:
  api:
    uses: project@v1
    name: "Imported API"
    with:
      path: "${fixture.clone}"
    layout:
      x: 40
      y: 40
      w: 300
      h: 200
`,
    );

    await page.getByLabel("Import YAML").setInputFiles(file);

    await expect(
      page.getByRole("button", { name: "Imported API" }),
    ).toBeVisible();
    await expect(
      page.getByRole("status", { name: "Workflow file" }),
    ).toHaveText("Imported imported.yaml");
  });
});
