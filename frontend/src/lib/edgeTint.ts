// What colours an arrow: the block it leaves. Giving a line the hue of its
// source makes it possible to see at a glance where it comes from, without
// following it back around the blocks it goes past.
//
// The rules are pure so the canvas only has to hand the hue to the stylesheet,
// which derives the drawn colour from it the same way a block does.

import { BLOCK_HUES, type NodeType } from "./graph.svelte.js";

// The hue of the block an arrow leaves, in OKLCH degrees. A source that has
// not resolved yet, and a block type no hue was chosen for, give none: the
// canvas then leaves the arrow its neutral colour rather than drawing it in
// whatever a missing hue would compute to.
export function edgeHue(type: NodeType | undefined): number | undefined {
  return type === undefined ? undefined : BLOCK_HUES[type];
}

// An arrow being drawn or reconnected: the block at the fixed end, which end
// follows the pointer, and the block under the pointer, if any.
export interface LinkEnds {
  anchorId: string;
  end: "from" | "to";
  targetId?: string;
}

// The block an arrow in flight leaves. Dragging a head away from a block
// keeps that block as the source; dragging a tail instead looks for the block
// under the pointer, and has no source until the pointer finds one.
export function linkSourceId(link: LinkEnds): string | undefined {
  return link.end === "to" ? link.anchorId : link.targetId;
}
