import { rmSync } from "node:fs";
import { join } from "node:path";
import { expect, test, type Locator, type Page } from "@playwright/test";
import {
  addAction,
  canvas,
  connect,
  dropInto,
  enlarge,
  mouseDrag,
  projectClone,
  selectBlock,
  type ProjectClone,
} from "./fixtures";

// The command, prompt and schema fields are code editors: they highlight what
// is typed and offer the variables that reach the selected block.
test.describe("Property code editors", () => {
  let fixture: ProjectClone;

  test.beforeEach(async ({ page }) => {
    fixture = projectClone();
    await page.goto("/");
  });

  test.afterEach(() => {
    rmSync(fixture.root, { recursive: true, force: true });
  });

  // A project on the clone with a worktree action feeding a command named
  // Tests, so a workspace really reaches the fields the editors are tried on.
  async function gateOnAWorktree(page: Page) {
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Project" }),
      canvas(page),
    );
    const project = page.getByRole("button", { name: "Project 1" });
    await page.getByLabel("Path").fill(fixture.clone);
    await enlarge(page, project, 260, 200);
    const box = await project.boundingBox();
    const palette = await page
      .getByRole("button", { name: "GitHub", exact: true })
      .boundingBox();
    if (!box || !palette) throw new Error("not visible");
    await page.mouse.move(palette.x + 20, palette.y + 10);
    await page.mouse.down();
    await page.mouse.move(box.x + 30, box.y + 45, { steps: 10 });
    await page.mouse.up();
    const github = page.getByRole("button", { name: "GitHub 1" });
    await page.getByLabel("Starting point").check();
    await addAction(page, github, "Create worktree");
    const worktree = join(fixture.root, "worktrees", "editor");
    await page.getByLabel("Branch").fill("editor");
    await page.getByLabel("Worktree path").fill(worktree);
    const gate = await dropInto(
      page,
      "Command",
      project,
      { x: 250, y: 60 },
      "Tests",
    );
    await connect(
      page,
      page.getByRole("button", { name: "Create worktree", exact: true }),
      gate,
    );
    // Connecting leaves the arrow's source selected; the fields under test
    // belong to the gate.
    await selectBlock(gate);
    return { project, gate, worktree };
  }

  const commandField = (page: Page) =>
    page.getByLabel("Command", { exact: true });

  const popup = (page: Page) => page.locator(".cm-tooltip-autocomplete");

  const suggestions = (page: Page) =>
    page.locator(".cm-tooltip-autocomplete .cm-completionLabel");

  // Types into an empty field the way a user does, from its own caret.
  async function type(field: Locator, text: string) {
    await field.click();
    await field.pressSequentially(text);
  }

  // The popup ignores keys for a moment after it opens so a keystroke already
  // on its way cannot pick an entry the user never saw.
  const settle = (page: Page) => page.waitForTimeout(150);

  test("runs a command written over several lines", async ({ page }) => {
    const { gate, worktree } = await gateOnAWorktree(page);
    const field = commandField(page);

    await field.fill(
      'cd {{workspace.path}}\ntest -f README.md\necho "in $(pwd)"',
    );

    await expect(field.locator(".cm-line")).toHaveCount(3);
    await page.getByRole("button", { name: "Run flow" }).click();
    const result = page.getByRole("region", { name: "Run result" });
    await expect(result).toContainText("Run succeeded", { timeout: 15_000 });
    await expect(result).toContainText(`in ${worktree}`);
    await expect(gate).toHaveClass(/status-succeeded/);
  });

  test("colours the shell words and marks the placeholders apart", async ({
    page,
  }) => {
    await gateOnAWorktree(page);
    const field = commandField(page);

    await field.fill("cd {{workspace.path}} && make verify");

    await expect(field.locator(".tok-builtin").first()).toHaveText("cd");
    await expect(field.locator(".tok-template")).toHaveText(
      "{{workspace.path}}",
    );
  });

  test("offers the workspace a worktree hands the command", async ({
    page,
  }) => {
    await gateOnAWorktree(page);
    const field = commandField(page);

    await type(field, "cd {{");

    await expect(suggestions(page).first()).toHaveText("{{workspace.path}}");
    await settle(page);
    await page.keyboard.press("Enter");
    // The braces already typed are replaced, not repeated.
    await expect(field).toHaveText("cd {{workspace.path}}");
  });

  test("offers the workspace environment variables to a command", async ({
    page,
  }) => {
    await gateOnAWorktree(page);
    const field = commandField(page);

    await type(field, "echo $MEGA");

    await expect(suggestions(page).first()).toHaveText(
      "$MEGA_AGENTS_WORKSPACE_PATH",
    );
    await page.keyboard.press("Escape");
    await expect(popup(page)).toHaveCount(0);
  });

  test("offers a prompt the value of the block connected to it", async ({
    page,
  }) => {
    const { project, gate } = await gateOnAWorktree(page);
    const fixer = await dropInto(
      page,
      "Agent",
      project,
      { x: 250, y: 190 },
      "Fixer",
    );
    await connect(page, gate, fixer);
    await selectBlock(fixer);
    const prompt = page.getByLabel("Prompt");

    await type(prompt, "Fix {{results");

    // Tab accepts the highlighted variable while the list is open.
    await expect(suggestions(page).first()).toHaveText("{{results.tests}}");
    await settle(page);
    await page.keyboard.press("Tab");
    await expect(prompt).toHaveText("Fix {{results.tests}}");
    // A prompt is filled in before anything runs, so it has no shell of its
    // own; only the placeholder is marked.
    await expect(prompt.locator(".tok-template")).toHaveText(
      "{{results.tests}}",
    );
    await expect(prompt.locator(".tok-builtin")).toHaveCount(0);
    await expect(fixer).toBeVisible();
  });
});

test.describe("Schema code editors", () => {
  test("still warn about a schema that is not JSON", async ({ page }) => {
    await page.goto("/");
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Project" }),
      canvas(page),
    );
    const project = page.getByRole("button", { name: "Project 1" });
    await enlarge(page, project, 200, 160);
    await dropInto(page, "Agent", project, { x: 40, y: 60 }, "Ask");
    const schema = page.getByLabel("Output schema");

    await schema.fill('{"type": ');

    await expect(page.getByRole("alert")).toHaveText(
      "The output schema is not valid JSON",
    );
    await schema.fill('{"type": "object"}');
    await expect(page.getByRole("alert")).toHaveCount(0);
    await expect(schema.locator(".tok-property")).toHaveText('"type"');
  });
});
