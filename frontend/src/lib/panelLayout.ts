import { clampZoom, ZOOM_DEFAULT } from "./canvasZoom.js";

// Sizes in pixels of the panels around the canvas, which people drag to
// suit their screen. They are remembered in this browser only.
export interface PanelLayout {
  palette: number;
  properties: number;
  panel: number;
}

export type PanelName = keyof PanelLayout;

export const PANEL_LIMITS: Record<
  PanelName,
  { default: number; min: number; max: number }
> = {
  palette: { default: 220, min: 160, max: 480 },
  properties: { default: 300, min: 240, max: 640 },
  panel: { default: 200, min: 96, max: 720 },
};

const LAYOUT_KEY = "mega-agents:layout";

export function clampPanel(name: PanelName, size: number): number {
  const { min, max } = PANEL_LIMITS[name];
  return Math.round(Math.min(max, Math.max(min, size)));
}

function defaults(): PanelLayout {
  return {
    palette: PANEL_LIMITS.palette.default,
    properties: PANEL_LIMITS.properties.default,
    panel: PANEL_LIMITS.panel.default,
  };
}

// Everything the workspace remembers about its layout lives under one key,
// so reading and writing go through the whole record and a panel drag never
// forgets the zoom, or the other way round.
function readStored(): Record<string, unknown> {
  try {
    const stored: unknown = JSON.parse(
      localStorage.getItem(LAYOUT_KEY) ?? "{}",
    );
    if (stored && typeof stored === "object")
      return stored as Record<string, unknown>;
  } catch {
    // Unreadable or unavailable storage leaves the defaults.
  }
  return {};
}

function writeStored(changes: Record<string, number>): void {
  try {
    localStorage.setItem(
      LAYOUT_KEY,
      JSON.stringify({ ...readStored(), ...changes }),
    );
  } catch {
    // The settings then last only until the page reloads.
  }
}

export function loadLayout(): PanelLayout {
  const layout = defaults();
  const stored = readStored();
  for (const name of Object.keys(layout) as PanelName[]) {
    const size = stored[name];
    if (typeof size === "number" && Number.isFinite(size))
      layout[name] = clampPanel(name, size);
  }
  return layout;
}

export function saveLayout(layout: PanelLayout): void {
  writeStored({ ...layout });
}

// How far the canvas was zoomed, kept beside the panel sizes.
export function loadZoom(): number {
  const zoom = readStored().zoom;
  return typeof zoom === "number" ? clampZoom(zoom) : ZOOM_DEFAULT;
}

export function saveZoom(zoom: number): void {
  writeStored({ zoom });
}
