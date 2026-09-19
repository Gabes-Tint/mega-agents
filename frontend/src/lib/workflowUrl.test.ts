import { describe, expect, test } from "vitest";
import { parseRoute, workflowPath } from "./workflowUrl.js";

describe("what the address bar points at", () => {
  test("the root is the scratch graph", () => {
    expect(parseRoute("/")).toEqual({ kind: "scratch" });
  });

  test("an empty path is the scratch graph too", () => {
    expect(parseRoute("")).toEqual({ kind: "scratch" });
  });

  test("the workflows path is the list of workflows and their schedules", () => {
    expect(parseRoute("/workflows")).toEqual({ kind: "schedules" });
  });

  test("a trailing slash still opens the list", () => {
    expect(parseRoute("/workflows/")).toEqual({ kind: "schedules" });
  });

  test("a workflow path names the saved workflow it opens", () => {
    expect(parseRoute("/workflows/issue-to-pr")).toEqual({
      kind: "workflow",
      name: "issue-to-pr",
    });
  });

  test("a trailing slash still opens the workflow", () => {
    expect(parseRoute("/workflows/demo/")).toEqual({
      kind: "workflow",
      name: "demo",
    });
  });

  test("an escaped name is read as the name it stands for", () => {
    expect(parseRoute("/workflows/my%20flow")).toEqual({
      kind: "workflow",
      name: "my flow",
    });
  });

  test("a name that cannot be decoded is not a workflow", () => {
    expect(parseRoute("/workflows/%zz")).toEqual({
      kind: "unknown",
      path: "/workflows/%zz",
    });
  });

  test("a path below a workflow is not a workflow", () => {
    expect(parseRoute("/workflows/demo/runs")).toEqual({
      kind: "unknown",
      path: "/workflows/demo/runs",
    });
  });

  test("any other page is unknown, including the runs a URL may one day get", () => {
    expect(parseRoute("/runs/42")).toEqual({
      kind: "unknown",
      path: "/runs/42",
    });
    expect(parseRoute("/nope")).toEqual({ kind: "unknown", path: "/nope" });
  });
});

describe("the address a workflow gets", () => {
  test("a saved workflow lives under its name", () => {
    expect(workflowPath("issue-to-pr")).toBe("/workflows/issue-to-pr");
  });

  test("a name with characters a path cannot hold is escaped", () => {
    expect(workflowPath("my flow")).toBe("/workflows/my%20flow");
  });

  test("the address of a workflow reads back as that workflow", () => {
    expect(parseRoute(workflowPath("gate-and-fix"))).toEqual({
      kind: "workflow",
      name: "gate-and-fix",
    });
  });
});
