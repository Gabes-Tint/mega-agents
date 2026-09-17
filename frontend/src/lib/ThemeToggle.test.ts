import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/svelte";
import { afterEach, describe, expect, test, vi } from "vitest";
import ThemeToggle from "./ThemeToggle.svelte";

function prefersDark(dark: boolean) {
  vi.stubGlobal(
    "matchMedia",
    vi.fn((query: string) => ({
      matches: dark && query === "(prefers-color-scheme: dark)",
    })),
  );
}

afterEach(() => {
  cleanup();
  localStorage.clear();
  delete document.documentElement.dataset.theme;
  vi.unstubAllGlobals();
});

describe("theme toggle", () => {
  test("follows a dark system preference until the user chooses", () => {
    prefersDark(true);

    render(ThemeToggle);

    expect(document.documentElement.dataset.theme).toBe("dark");
    expect(
      screen.getByRole("button", { name: "Switch to light theme" }),
    ).toBeInTheDocument();
  });

  test("follows a light system preference", () => {
    prefersDark(false);

    render(ThemeToggle);

    expect(document.documentElement.dataset.theme).toBe("light");
  });

  test("switches the theme and remembers the choice", async () => {
    prefersDark(false);
    render(ThemeToggle);

    await fireEvent.click(
      screen.getByRole("button", { name: "Switch to dark theme" }),
    );

    expect(document.documentElement.dataset.theme).toBe("dark");
    expect(localStorage.getItem("mega-agents:theme")).toBe("dark");
    expect(
      screen.getByRole("button", { name: "Switch to light theme" }),
    ).toBeInTheDocument();
  });

  test("a remembered choice wins over the system preference", () => {
    prefersDark(true);
    localStorage.setItem("mega-agents:theme", "light");

    render(ThemeToggle);

    expect(document.documentElement.dataset.theme).toBe("light");
  });

  test("works where the system preference cannot be read", () => {
    vi.stubGlobal("matchMedia", undefined);

    render(ThemeToggle);

    expect(document.documentElement.dataset.theme).toBe("light");
  });
});
