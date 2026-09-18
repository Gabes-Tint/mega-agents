import { describe, expect, test } from "vitest";
import {
  clampZoom,
  fitToContent,
  scrollForZoom,
  wheelZoom,
  ZOOM_DEFAULT,
  ZOOM_MAX,
  ZOOM_MIN,
  zoomIn,
  zoomOut,
  zoomPercent,
} from "./canvasZoom.js";

describe("the zoom range", () => {
  test("keeps a level inside the range it offers", () => {
    expect(clampZoom(0.1)).toBe(ZOOM_MIN);
    expect(clampZoom(5)).toBe(ZOOM_MAX);
    expect(clampZoom(0.6)).toBe(0.6);
  });

  test("falls back to full size for a level that is not a number", () => {
    expect(clampZoom(Number.NaN)).toBe(ZOOM_DEFAULT);
    expect(clampZoom(Number.POSITIVE_INFINITY)).toBe(ZOOM_DEFAULT);
  });

  test("reads as a whole percentage", () => {
    expect(zoomPercent(1)).toBe(100);
    expect(zoomPercent(0.335)).toBe(34);
  });
});

describe("stepping the zoom", () => {
  test("steps up and down through the levels the buttons offer", () => {
    expect(zoomIn(0.5)).toBe(0.67);
    expect(zoomOut(0.5)).toBe(0.33);
    expect(zoomIn(1.5)).toBe(2);
    expect(zoomOut(1.25)).toBe(1);
  });

  test("stops at the ends of the range", () => {
    expect(zoomIn(ZOOM_MAX)).toBe(ZOOM_MAX);
    expect(zoomOut(ZOOM_MIN)).toBe(ZOOM_MIN);
  });

  test("reaches exactly full size from a level a gesture left behind", () => {
    expect(zoomIn(0.87)).toBe(1);
    expect(zoomOut(1.13)).toBe(1);
  });

  test("a wheel notch zooms by a share of the level it starts from", () => {
    expect(wheelZoom(1, -100)).toBeGreaterThan(1);
    expect(wheelZoom(1, 100)).toBeLessThan(1);
    // Zooming out then in by the same notch returns to where it started.
    expect(wheelZoom(wheelZoom(1, 100), -100)).toBeCloseTo(1, 6);
    expect(wheelZoom(ZOOM_MIN, 400)).toBe(ZOOM_MIN);
    expect(wheelZoom(ZOOM_MAX, -400)).toBe(ZOOM_MAX);
  });
});

describe("zooming toward a point", () => {
  test("keeps the content under the pointer where it is", () => {
    const scroll = scrollForZoom({ left: 0, top: 0 }, { x: 100, y: 60 }, 1, 2);

    // The content at (100, 60) is drawn at (200, 120) once doubled, so the
    // canvas scrolls by the difference to leave it under the pointer.
    expect(scroll).toEqual({ left: 100, top: 60 });
  });

  test("keeps the content under the pointer on a scrolled canvas", () => {
    const scroll = scrollForZoom(
      { left: 400, top: 200 },
      { x: 100, y: 60 },
      1,
      0.5,
    );

    expect(scroll).toEqual({ left: 150, top: 70 });
  });

  test("never scrolls past the top left corner of the content", () => {
    expect(
      scrollForZoom({ left: 0, top: 0 }, { x: 100, y: 60 }, 1, 0.5),
    ).toEqual({ left: 0, top: 0 });
  });
});

describe("framing the whole flow", () => {
  test("shrinks the content until it fits, and centres it", () => {
    const fit = fitToContent(
      { x: 0, y: 0, w: 2000, h: 1000 },
      { width: 1048, height: 548 },
      24,
    );

    // The width is the tighter of the two: 1000 usable pixels over 2000.
    expect(fit.zoom).toBe(0.5);
    expect(fit.left).toBe(0);
    expect(fit.top).toBe(0);
  });

  test("scrolls to the content when it sits away from the origin", () => {
    const fit = fitToContent(
      { x: 1000, y: 500, w: 400, h: 200 },
      { width: 448, height: 248 },
      24,
    );

    expect(fit.zoom).toBe(1);
    expect(fit.left).toBe(976);
    expect(fit.top).toBe(476);
  });

  test("never grows the content past the top of the range", () => {
    const fit = fitToContent(
      { x: 0, y: 0, w: 10, h: 10 },
      { width: 1000, height: 1000 },
      24,
    );

    expect(fit.zoom).toBe(ZOOM_MAX);
  });

  test("leaves an empty flow at full size", () => {
    expect(
      fitToContent({ x: 0, y: 0, w: 0, h: 0 }, { width: 800, height: 600 }),
    ).toEqual({ zoom: ZOOM_DEFAULT, left: 0, top: 0 });
    expect(
      fitToContent({ x: 0, y: 0, w: 100, h: 100 }, { width: 0, height: 0 }),
    ).toEqual({ zoom: ZOOM_DEFAULT, left: 0, top: 0 });
  });
});
