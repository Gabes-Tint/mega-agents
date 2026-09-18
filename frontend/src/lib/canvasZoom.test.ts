import { describe, expect, test } from "vitest";
import {
  clampZoom,
  fitToContent,
  loadWheelMode,
  NO_WHEEL_GESTURE,
  saveWheelMode,
  wheelIntent,
  wheelPixels,
  type WheelIntent,
  type WheelMode,
  type WheelSignal,
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

  test("a notch counts the same however the browser measures it", () => {
    expect(wheelPixels({ deltaX: 0, deltaY: 100 })).toBe(100);
    // Firefox counts the same notch as three lines, and a page as a screen.
    expect(wheelPixels({ deltaX: 0, deltaY: 3, deltaMode: 1 })).toBe(100);
    expect(wheelPixels({ deltaX: 0, deltaY: -1, deltaMode: 2 })).toBe(-400);
  });

  test("sideways, a wheel takes whichever axis the browser reports", () => {
    expect(wheelPixels({ deltaX: -60, deltaY: 0 }, "x")).toBe(-60);
    // A wheel with shift held reports its steps down the page on most
    // platforms, and they move the canvas sideways.
    expect(wheelPixels({ deltaX: 0, deltaY: 100 }, "x")).toBe(100);
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

describe("telling a mouse wheel from a trackpad", () => {
  // Recorded shapes of the gestures the canvas has to tell apart, as the
  // browser delivers them: a run of wheel events about a frame apart.
  const MOUSE_NOTCHES: WheelSignal[] = [
    { deltaX: 0, deltaY: 100 },
    { deltaX: 0, deltaY: 100 },
    { deltaX: 0, deltaY: 100 },
  ];
  // Firefox reports the same wheel in lines rather than pixels.
  const MOUSE_LINES: WheelSignal[] = [
    { deltaX: 0, deltaY: 3, deltaMode: 1 },
    { deltaX: 0, deltaY: 3, deltaMode: 1 },
  ];
  // A free-spinning wheel sends its notches faster and smaller, but still in
  // whole pixels, straight down, and above the smallest step a wheel takes.
  const FINE_WHEEL: WheelSignal[] = [
    { deltaX: 0, deltaY: 24 },
    { deltaX: 0, deltaY: 32 },
    { deltaX: 0, deltaY: 48 },
    { deltaX: 0, deltaY: 32 },
  ];
  // Two fingers on a trackpad: the deltas ramp up from nothing and back
  // down, they are not whole pixels, and the fingers wander sideways.
  const TRACKPAD_SWIPE: WheelSignal[] = [
    { deltaX: 0, deltaY: 1.5 },
    { deltaX: -1, deltaY: 4.5 },
    { deltaX: -2, deltaY: 12 },
    { deltaX: 0, deltaY: 26 },
    { deltaX: 1, deltaY: 44 },
    { deltaX: 0, deltaY: 18 },
    { deltaX: 0, deltaY: 3 },
  ];
  // A trackpad pinch arrives as a wheel with the control key held.
  const TRACKPAD_PINCH: WheelSignal[] = [
    { deltaX: 0, deltaY: -2.7, ctrlKey: true },
    { deltaX: 0, deltaY: -6.1, ctrlKey: true },
    { deltaX: 0, deltaY: -4.3, ctrlKey: true },
  ];
  const SHIFT_WHEEL: WheelSignal[] = [
    { deltaX: 0, deltaY: 100, shiftKey: true },
    { deltaX: 0, deltaY: 100, shiftKey: true },
  ];

  // Reads a whole gesture, feeding each event the run it belongs to.
  function intents(
    events: WheelSignal[],
    mode: WheelMode = "zoom",
    gap = 16,
  ): WheelIntent[] {
    let gesture = NO_WHEEL_GESTURE;
    return events.map((event, index) => {
      const step = wheelIntent(
        { timeStamp: index * gap, ...event },
        gesture,
        mode,
      );
      gesture = step.gesture;
      return step.intent;
    });
  }

  function only(read: WheelIntent[]): WheelIntent[] {
    return [...new Set(read)];
  }

  test("a mouse wheel zooms, in pixels or in lines", () => {
    expect(only(intents(MOUSE_NOTCHES))).toEqual(["zoom"]);
    expect(only(intents(MOUSE_LINES))).toEqual(["zoom"]);
    expect(only(intents(FINE_WHEEL))).toEqual(["zoom"]);
  });

  test("a trackpad swipe scrolls, however far its deltas run up", () => {
    expect(only(intents(TRACKPAD_SWIPE))).toEqual(["scroll-y"]);
  });

  test("a pinch zooms, and so does the control key with a wheel", () => {
    expect(only(intents(TRACKPAD_PINCH))).toEqual(["zoom"]);
    expect(intents([{ deltaX: 0, deltaY: 100, ctrlKey: true }])).toEqual([
      "zoom",
    ]);
    expect(intents([{ deltaX: 0, deltaY: 100, metaKey: true }])).toEqual([
      "zoom",
    ]);
  });

  test("the shift key sends a wheel sideways instead", () => {
    expect(only(intents(SHIFT_WHEEL))).toEqual(["scroll-x"]);
  });

  test("a gesture is read as a whole, and a pause starts a new one", () => {
    // The sideways drift arrives in the second event, and the first one has
    // already zoomed; the rest of the swipe scrolls.
    expect(
      intents([
        { deltaX: 0, deltaY: 30 },
        { deltaX: -3, deltaY: 30 },
        { deltaX: 0, deltaY: 30 },
      ]),
    ).toEqual(["zoom", "scroll-y", "scroll-y"]);

    // Once the hand has come off the trackpad, a wheel notch zooms again.
    expect(
      wheelIntent(
        { deltaX: 0, deltaY: 100, timeStamp: 1000 },
        { at: 0, trackpad: true },
      ).intent,
    ).toBe("zoom");
  });

  test("the scroll setting leaves every wheel to the canvas", () => {
    expect(only(intents(MOUSE_NOTCHES, "scroll"))).toEqual(["scroll-y"]);
    expect(only(intents(FINE_WHEEL, "scroll"))).toEqual(["scroll-y"]);
    // A pinch and the shift key keep what they do in either setting.
    expect(only(intents(TRACKPAD_PINCH, "scroll"))).toEqual(["zoom"]);
    expect(only(intents(SHIFT_WHEEL, "scroll"))).toEqual(["scroll-x"]);
  });

  test("a wheel too fine to tell from a trackpad scrolls", () => {
    // Steps below the smallest notch a wheel reports, or fractions of a
    // pixel, read as a trackpad. The setting is the way out of it.
    expect(intents([{ deltaX: 0, deltaY: 8 }])).toEqual(["scroll-y"]);
    expect(intents([{ deltaX: 0, deltaY: 100.5 }])).toEqual(["scroll-y"]);
  });

  test("a wheel that reports nothing leaves the gesture as it found it", () => {
    expect(wheelIntent({ deltaX: 0, deltaY: 0 }).gesture.trackpad).toBe(false);
  });
});

describe("remembering what the wheel does", () => {
  test("zooms until it is told to scroll, and back again", () => {
    localStorage.clear();
    expect(loadWheelMode()).toBe("zoom");

    saveWheelMode("scroll");
    expect(loadWheelMode()).toBe("scroll");

    saveWheelMode("zoom");
    expect(loadWheelMode()).toBe("zoom");
  });

  test("is kept beside the zoom level, and keeps it", () => {
    localStorage.clear();
    localStorage.setItem(
      "mega-agents:layout",
      JSON.stringify({ zoom: 0.5, palette: 200 }),
    );

    saveWheelMode("scroll");

    expect(
      JSON.parse(localStorage.getItem("mega-agents:layout") ?? "{}"),
    ).toEqual({ zoom: 0.5, palette: 200, wheel: "scroll" });
  });

  test("falls back to zooming when the setting is unreadable", () => {
    localStorage.clear();
    localStorage.setItem("mega-agents:layout", "not json");
    expect(loadWheelMode()).toBe("zoom");

    localStorage.setItem("mega-agents:layout", JSON.stringify({ wheel: 7 }));
    expect(loadWheelMode()).toBe("zoom");
  });
});
