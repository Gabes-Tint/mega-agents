import { expect, test, type Page } from "@playwright/test";
import { canvas, dropInto, enlarge, mouseDrag, selectBlock } from "./fixtures";

// The editor's code is a chunk of its own, fetched when a field first shows
// one. Until it lands the field is a plain box, and the swap has to keep what
// was typed, where the caret was and which control the keyboard reaches.
test.describe("A property field before its editor arrives", () => {
  const editorChunk = "**/codeEditorView*";

  // A command block on the canvas, whose Command field is the one under test.
  async function commandBlock(page: Page) {
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Project" }),
      canvas(page),
    );
    const project = page.getByRole("button", { name: "Project 1" });
    await enlarge(page, project, 220, 160);
    const gate = await dropInto(
      page,
      "Command",
      project,
      { x: 40, y: 60 },
      "Tests",
    );
    await selectBlock(gate);
    return gate;
  }

  const field = (page: Page) => page.getByLabel("Command", { exact: true });

  test("takes typing, and the editor picks it up where it was left", async ({
    page,
  }) => {
    // Held, not failed: the chunk is still on its way while the user types.
    let deliver = () => {};
    const delivered = new Promise<void>((resolve) => {
      deliver = resolve;
    });
    await page.route(editorChunk, async (route) => {
      await delivered;
      await route.continue();
    });
    await page.goto("/");
    await commandBlock(page);

    const box = field(page);
    await expect(box).toHaveJSProperty("tagName", "TEXTAREA");
    await box.click();
    await box.pressSequentially("cd repo");
    // The caret is left between "cd " and "repo", where the swap must find it.
    await page.keyboard.press("ArrowLeft");
    await page.keyboard.press("ArrowLeft");
    await page.keyboard.press("ArrowLeft");
    await page.keyboard.press("ArrowLeft");
    await expect(box).toHaveValue("cd repo");

    deliver();

    const editor = field(page);
    await expect(editor).toHaveAttribute("contenteditable", "true");
    // The keyboard still reaches the field, at the caret it held.
    await page.keyboard.type("X");
    await expect(editor).toHaveText("cd Xrepo");
  });

  test("keeps working as a plain box when the editor never arrives", async ({
    page,
  }) => {
    await page.route(editorChunk, (route) => route.abort());
    await page.goto("/");
    const gate = await commandBlock(page);

    const box = field(page);
    await expect(page.getByText(/could not load/)).toBeVisible();
    await box.fill("make verify");

    // The text still reaches the block it belongs to.
    await expect(box).toHaveValue("make verify");
    await selectBlock(page.getByRole("button", { name: "Project 1" }));
    await selectBlock(gate);
    await expect(field(page)).toHaveValue("make verify");
  });
});
