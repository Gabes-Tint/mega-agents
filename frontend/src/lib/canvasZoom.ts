// How far the canvas zooms, and how a gesture or a button moves between
// levels. The canvas draws its content at this scale and converts pointer
// positions through it, so the arithmetic lives here on its own.

export const ZOOM_MIN = 0.25;
export const ZOOM_MAX = 2;
export const ZOOM_DEFAULT = 1;

// The levels the buttons and the keyboard step through. Full size is one of
// them, so it is always one step away from the levels on either side of it,
// however far a wheel gesture wandered from them.
const ZOOM_STOPS = [0.25, 0.33, 0.5, 0.67, 0.75, 1, 1.25, 1.5, 2];

// Rounding slack, so a level that is a stop in all but the last bit does not
// step onto itself.
const NEAR = 1e-6;

// How much of a wheel notch or a pinch turns into zoom. A notch of 100,
// which is what a mouse wheel reports, zooms by about a quarter.
const WHEEL_RATE = 0.0025;

export function clampZoom(zoom: number): number {
  if (!Number.isFinite(zoom)) return ZOOM_DEFAULT;
  return Math.min(ZOOM_MAX, Math.max(ZOOM_MIN, zoom));
}

export function zoomPercent(zoom: number): number {
  return Math.round(zoom * 100);
}

export function zoomIn(zoom: number): number {
  return ZOOM_STOPS.find((stop) => stop > zoom + NEAR) ?? ZOOM_MAX;
}

export function zoomOut(zoom: number): number {
  return (
    [...ZOOM_STOPS].reverse().find((stop) => stop < zoom - NEAR) ?? ZOOM_MIN
  );
}

// A wheel notch or a trackpad pinch zooms by a share of the level it starts
// from, so the gesture feels the same wherever it is used.
export function wheelZoom(zoom: number, deltaY: number): number {
  return clampZoom(zoom * Math.exp(-deltaY * WHEEL_RATE));
}

// A wheel notch is a hundred pixels, three lines, or a fraction of a screen,
// depending on the browser and the platform; these turn the last two into
// the pixels the rest of the canvas is measured in.
const LINE_PIXELS = 100 / 3;
const PAGE_PIXELS = 400;

// How far a wheel event moves the canvas, in pixels. Sideways it takes the
// horizontal delta when the browser reports one, and the vertical one when
// the shift key has been left to turn the wheel sideways here.
export function wheelPixels(event: WheelSignal, axis: "x" | "y" = "y"): number {
  const delta =
    axis === "x" && event.deltaX !== 0 ? event.deltaX : event.deltaY;
  if (event.deltaMode === 1) return delta * LINE_PIXELS;
  if (event.deltaMode === 2) return delta * PAGE_PIXELS;
  return delta;
}

// What a wheel gesture does to the canvas: zoom toward the pointer, scroll
// sideways, or be left to the canvas's own scrolling.
export type WheelIntent = "zoom" | "scroll-x" | "scroll-y";

// What a bare wheel is taken to mean, which the wheel button in the zoom bar
// flips and the browser remembers.
export type WheelMode = "zoom" | "scroll";

// The part of a wheel event the decision reads.
export interface WheelSignal {
  deltaX: number;
  deltaY: number;
  deltaMode?: number;
  ctrlKey?: boolean;
  metaKey?: boolean;
  shiftKey?: boolean;
  timeStamp?: number;
}

// What the run of wheel events so far has shown about the device sending
// them. A trackpad gives itself away in one event of a gesture rather than
// in all of them, so the evidence is carried from event to event.
export interface WheelGesture {
  // When the last event taken into the run arrived.
  at: number;
  // Set once anything in the run carried a trackpad's signature.
  trackpad: boolean;
}

export const NO_WHEEL_GESTURE: WheelGesture = {
  at: -Infinity,
  trackpad: false,
};

// A pause this long ends a gesture: the next event is read on its own again.
const GESTURE_GAP_MS = 250;

// The smallest step a mouse wheel reports. A notch is 100 or 120 pixels, and
// a free-spinning wheel breaks that into smaller whole steps, but a wheel
// never crawls the way two fingers starting to move do.
const WHEEL_NOTCH_FLOOR = 20;

// Whether this one event bears a trackpad's marks. A wheel reports whole
// pixels straight down, in steps no smaller than a notch, or counts them in
// lines or pages; two fingers report fractions, wander sideways, and start
// from almost nothing.
function fromTrackpad(event: WheelSignal): boolean {
  if (event.deltaX === 0 && event.deltaY === 0) return false;
  if ((event.deltaMode ?? 0) !== 0) return false;
  if (event.deltaX !== 0) return true;
  if (!Number.isInteger(event.deltaY)) return true;
  return Math.abs(event.deltaY) < WHEEL_NOTCH_FLOOR;
}

// What the canvas does with a wheel event, and what its gesture knows once
// the event has been read. The control and meta keys always zoom, which is
// how a trackpad pinch arrives; shift always goes sideways; otherwise a bare
// wheel zooms unless the gesture looks like two fingers on a trackpad, or
// unless the wheel has been set to scroll.
export function wheelIntent(
  event: WheelSignal,
  gesture: WheelGesture = NO_WHEEL_GESTURE,
  mode: WheelMode = "zoom",
): { intent: WheelIntent; gesture: WheelGesture } {
  const at = event.timeStamp ?? 0;
  const continues = at - gesture.at <= GESTURE_GAP_MS;
  const next = {
    at,
    trackpad: (continues && gesture.trackpad) || fromTrackpad(event),
  };
  return { intent: intentOf(event, next, mode), gesture: next };
}

function intentOf(
  event: WheelSignal,
  gesture: WheelGesture,
  mode: WheelMode,
): WheelIntent {
  if (event.ctrlKey || event.metaKey) return "zoom";
  if (event.shiftKey) return "scroll-x";
  if (mode === "scroll") return "scroll-y";
  return gesture.trackpad ? "scroll-y" : "zoom";
}

// What a bare wheel does is remembered beside the zoom level and the panel
// sizes, under the one key the workspace keeps its layout in. Reading and
// writing go through the whole record, so a setting saved here never forgets
// a size saved there, or the other way round.
const LAYOUT_KEY = "mega-agents:layout";

function storedLayout(): Record<string, unknown> {
  try {
    const stored: unknown = JSON.parse(
      localStorage.getItem(LAYOUT_KEY) ?? "{}",
    );
    if (stored && typeof stored === "object")
      return stored as Record<string, unknown>;
  } catch {
    // Unreadable or unavailable storage leaves the default.
  }
  return {};
}

export function loadWheelMode(): WheelMode {
  return storedLayout().wheel === "scroll" ? "scroll" : "zoom";
}

export function saveWheelMode(mode: WheelMode): void {
  try {
    localStorage.setItem(
      LAYOUT_KEY,
      JSON.stringify({ ...storedLayout(), wheel: mode }),
    );
  } catch {
    // The setting then lasts only until the page reloads.
  }
}

export interface Scroll {
  left: number;
  top: number;
}

export interface Bounds {
  x: number;
  y: number;
  w: number;
  h: number;
}

export interface Viewport {
  width: number;
  height: number;
}

// Where the canvas scrolls to so that the content under the anchor stays
// under it. The anchor is measured from the viewport's top left corner in
// screen pixels: the pointer for a wheel gesture, the middle for a button.
// Content starts at the origin, so scrolling never goes behind it.
export function scrollForZoom(
  scroll: Scroll,
  anchor: { x: number; y: number },
  from: number,
  to: number,
): Scroll {
  const ratio = to / from;
  return {
    left: Math.max(0, (scroll.left + anchor.x) * ratio - anchor.x),
    top: Math.max(0, (scroll.top + anchor.y) * ratio - anchor.y),
  };
}

// The gap left around the content when the whole flow is framed.
const FIT_PADDING = 24;

// The zoom and scroll that frame the content in the viewport, with the
// content centred on what is left over. An empty flow, or a viewport that
// has not been laid out yet, stays at full size.
export function fitToContent(
  content: Bounds,
  view: Viewport,
  padding = FIT_PADDING,
): Scroll & { zoom: number } {
  const usableWidth = view.width - 2 * padding;
  const usableHeight = view.height - 2 * padding;
  if (content.w <= 0 || content.h <= 0 || usableWidth <= 0 || usableHeight <= 0)
    return { zoom: ZOOM_DEFAULT, left: 0, top: 0 };
  const zoom = clampZoom(
    Math.min(usableWidth / content.w, usableHeight / content.h),
  );
  return {
    zoom,
    left: Math.max(0, (content.x + content.w / 2) * zoom - view.width / 2),
    top: Math.max(0, (content.y + content.h / 2) * zoom - view.height / 2),
  };
}
