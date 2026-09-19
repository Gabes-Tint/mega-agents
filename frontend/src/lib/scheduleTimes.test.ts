import { describe, expect, test } from "vitest";
import { describeRun } from "./scheduleTimes.js";

const now = new Date("2026-09-18T09:00:00Z");

describe("how a run still to come is written", () => {
  test("a run within the hour is counted in minutes", () => {
    expect(describeRun("2026-09-18T09:25:00Z", now, "en", "UTC").relative).toBe(
      "in 25 minutes",
    );
  });

  test("a run later today is counted in hours", () => {
    expect(describeRun("2026-09-18T14:00:00Z", now, "en", "UTC").relative).toBe(
      "in 5 hours",
    );
  });

  test("a run further off is counted in days", () => {
    expect(describeRun("2026-09-21T09:00:00Z", now, "en", "UTC").relative).toBe(
      "in 3 days",
    );
  });

  test("the next minute is counted as one minute, never as none", () => {
    expect(describeRun("2026-09-18T09:00:30Z", now, "en", "UTC").relative).toBe(
      "in 1 minute",
    );
  });

  test("the run itself is written out in full", () => {
    const run = describeRun("2026-09-18T14:30:00Z", now, "en", "UTC");
    expect(run.absolute).toContain("14:30");
    expect(run.absolute).toContain("Sep");
  });

  test("a time that is not a time says so rather than showing NaN", () => {
    expect(describeRun("not a time", now, "en", "UTC")).toEqual({
      absolute: "not a time",
      relative: "",
    });
  });
});
