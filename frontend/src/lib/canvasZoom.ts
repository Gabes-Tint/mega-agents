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
