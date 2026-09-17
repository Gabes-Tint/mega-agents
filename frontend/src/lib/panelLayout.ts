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

export function loadLayout(): PanelLayout {
  const layout = defaults();
  try {
    const stored = JSON.parse(
      localStorage.getItem(LAYOUT_KEY) ?? "{}",
    ) as Partial<Record<PanelName, unknown>>;
    for (const name of Object.keys(layout) as PanelName[]) {
      const size = stored[name];
      if (typeof size === "number" && Number.isFinite(size))
        layout[name] = clampPanel(name, size);
    }
  } catch {
    // Unreadable or unavailable storage leaves the default sizes.
  }
  return layout;
}

export function saveLayout(layout: PanelLayout): void {
  try {
    localStorage.setItem(LAYOUT_KEY, JSON.stringify(layout));
  } catch {
    // The sizes then last only until the page reloads.
  }
}
