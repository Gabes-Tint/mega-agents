// How arrows find their way between blocks: orthogonal segments with
// rounded corners that leave a block from the side facing the other one and
// go around the blocks in the way, the way draw.io draws them.
//
// The routing is pure geometry so the canvas only has to draw what it
// returns, and so it can be tested without a browser.

export interface Box {
  x: number;
  y: number;
  w: number;
  h: number;
}

export interface Point {
  x: number;
  y: number;
}

// A block as the router sees it: where it is in canvas content space, and
// the block it sits in.
export interface LevelNode {
  id: string;
  parentId?: string;
  box: Box;
}

// The blocks an arrow has to get around, and the container it should stay
// inside while doing so.
export interface ArrowScope {
  obstacles: Box[];
  bounds?: Box;
}

// Where an arrow's output label goes, and how it is anchored there.
export interface RouteLabel {
  x: number;
  y: number;
  anchor: "middle" | "start";
}

export interface Route {
  // The corners of the route, from the source's border to the target's.
  points: Point[];
  // The drawn path, with a rounded corner at every bend.
  path: string;
  // The same path pulled in at both ends for the click target.
  hitPath: string;
  start: Point;
  end: Point;
  label: RouteLabel;
  // Whether the route gets through without crossing a block.
  clean: boolean;
}

export interface RouteOptions {
  obstacles?: readonly Box[];
  bounds?: Box;
}

// How far an arrow stays clear of a block it goes around.
export const ARROW_CLEARANCE = 16;
// How far an arrow leaves its block before it may turn.
export const ARROW_STUB = 12;
// How far the click target stops short of each end, so it leaves the resize
// strips of the blocks it joins alone.
export const ARROW_INSET = 8;
// The radius of a bend, which reads as a curve rather than a right angle.
export const ARROW_RADIUS = 8;

// How far around the two blocks a block still counts when the corridors an
// arrow may run along are collected.
const REACH = 120;
// How many corridors beyond the natural ones a route tries per axis. The
// nearest ones to the straight way through come first.
const CORRIDORS = 4;
// What it costs an arrow to leave or arrive on a side other than the one
// facing the block at its other end. It outweighs a couple of extra bends,
// so an arrow only gives up the facing sides to get through at all.
const SIDE_PAIR_PENALTY = 40;

type Side = "top" | "right" | "bottom" | "left";

const OUTWARD: Record<Side, Point> = {
  top: { x: 0, y: -1 },
  right: { x: 1, y: 0 },
  bottom: { x: 0, y: 1 },
  left: { x: -1, y: 0 },
};

const OPPOSITE: Record<Side, Side> = {
  top: "bottom",
  right: "left",
  bottom: "top",
  left: "right",
};

function centerX(box: Box): number {
  return box.x + box.w / 2;
}

function centerY(box: Box): number {
  return box.y + box.h / 2;
}

// Where along a side an arrow attaches: the middle of the stretch the two
// blocks share, so blocks that line up are joined by one straight run, and
// the middle of the side itself when they share nothing.
function sharedCenter(
  from: number,
  fromSize: number,
  to: number,
  toSize: number,
): number {
  const low = Math.max(from, to);
  const high = Math.min(from + fromSize, to + toSize);
  return high >= low ? (low + high) / 2 : from + fromSize / 2;
}

function anchorOn(box: Box, side: Side, other: Box): Point {
  if (side === "left" || side === "right")
    return {
      x: side === "right" ? box.x + box.w : box.x,
      y: sharedCenter(box.y, box.h, other.y, other.h),
    };
  return {
    x: sharedCenter(box.x, box.w, other.x, other.w),
    y: side === "bottom" ? box.y + box.h : box.y,
  };
}

function stubEnd(point: Point, side: Side): Point {
  return {
    x: point.x + OUTWARD[side].x * ARROW_STUB,
    y: point.y + OUTWARD[side].y * ARROW_STUB,
  };
}

// Drops the points a straight run does not need: repeats, and corners where
// the route carries straight on.
function simplify(points: Point[]): Point[] {
  const kept: Point[] = [];
  for (const point of points) {
    const last = kept[kept.length - 1];
    if (
      last &&
      Math.abs(last.x - point.x) < 0.01 &&
      Math.abs(last.y - point.y) < 0.01
    )
      continue;
    kept.push(point);
  }
  const straight: Point[] = [];
  for (let index = 0; index < kept.length; index++) {
    const before = straight[straight.length - 1];
    const after = kept[index + 1];
    const point = kept[index]!;
    if (
      before &&
      after &&
      ((before.x === point.x && point.x === after.x) ||
        (before.y === point.y && point.y === after.y))
    )
      continue;
    straight.push(point);
  }
  return straight;
}

// Whether an axis-aligned segment passes through the box. Running along a
// border does not count, so an arrow may hug a block.
function segmentEnters(from: Point, to: Point, box: Box): boolean {
  const edge = 0.5;
  const left = box.x + edge;
  const right = box.x + box.w - edge;
  const top = box.y + edge;
  const bottom = box.y + box.h - edge;
  if (left >= right || top >= bottom) return false;
  return (
    Math.max(from.x, to.x) > left &&
    Math.min(from.x, to.x) < right &&
    Math.max(from.y, to.y) > top &&
    Math.min(from.y, to.y) < bottom
  );
}

function routeEnters(points: Point[], box: Box): boolean {
  for (let index = 1; index < points.length; index++)
    if (segmentEnters(points[index - 1]!, points[index]!, box)) return true;
  return false;
}

function outsideBounds(point: Point, bounds: Box): boolean {
  return (
    point.x < bounds.x ||
    point.y < bounds.y ||
    point.x > bounds.x + bounds.w ||
    point.y > bounds.y + bounds.h
  );
}

function routeLength(points: Point[]): number {
  let total = 0;
  for (let index = 1; index < points.length; index++)
    total +=
      Math.abs(points[index]!.x - points[index - 1]!.x) +
      Math.abs(points[index]!.y - points[index - 1]!.y);
  return total;
}

// The corridors a route may turn onto along one axis: the way straight
// through first, then each end's own line, then the lanes just clear of the
// blocks nearby, nearest the straight way first.
function corridors(
  from: number,
  to: number,
  blocks: readonly Box[],
  axis: "x" | "y",
): number[] {
  const middle = (from + to) / 2;
  const natural = [middle, from, to];
  const extra: number[] = [];
  for (const box of blocks) {
    const low = (axis === "x" ? box.x : box.y) - ARROW_CLEARANCE;
    const high =
      (axis === "x" ? box.x + box.w : box.y + box.h) + ARROW_CLEARANCE;
    extra.push(low, high);
  }
  const seen = new Set<number>();
  const values: number[] = [];
  for (const value of natural)
    if (!seen.has(value)) {
      seen.add(value);
      values.push(value);
    }
  const sorted = extra
    .filter((value) => !seen.has(value))
    .sort(
      (one, other) =>
        Math.abs(one - middle) - Math.abs(other - middle) || one - other,
    );
  for (const value of sorted) {
    if (values.length >= natural.length + CORRIDORS) break;
    if (seen.has(value)) continue;
    seen.add(value);
    values.push(value);
  }
  return values;
}

function inflated(from: Box, to: Box, by: number): Box {
  const x = Math.min(from.x, to.x) - by;
  const y = Math.min(from.y, to.y) - by;
  return {
    x,
    y,
    w: Math.max(from.x + from.w, to.x + to.w) + by - x,
    h: Math.max(from.y + from.h, to.y + to.h) + by - y,
  };
}

function overlaps(one: Box, other: Box): boolean {
  return (
    one.x < other.x + other.w &&
    other.x < one.x + one.w &&
    one.y < other.y + other.h &&
    other.y < one.y + one.h
  );
}

function format(value: number): string {
  return String(Math.round(value * 100) / 100);
}

// The drawn path: straight runs joined by a short curve at each bend, so a
// corner reads as a curve rather than a right angle.
function toPath(points: Point[]): string {
  const first = points[0];
  if (!first) return "";
  let path = `M ${format(first.x)} ${format(first.y)}`;
  for (let index = 1; index < points.length - 1; index++) {
    const before = points[index - 1]!;
    const corner = points[index]!;
    const after = points[index + 1]!;
    const into = Math.abs(corner.x - before.x) + Math.abs(corner.y - before.y);
    const away = Math.abs(after.x - corner.x) + Math.abs(after.y - corner.y);
    const radius = Math.min(ARROW_RADIUS, into / 2, away / 2);
    if (radius < 0.01) {
      path += ` L ${format(corner.x)} ${format(corner.y)}`;
      continue;
    }
    const start = {
      x: corner.x + ((before.x - corner.x) / into) * radius,
      y: corner.y + ((before.y - corner.y) / into) * radius,
    };
    const end = {
      x: corner.x + ((after.x - corner.x) / away) * radius,
      y: corner.y + ((after.y - corner.y) / away) * radius,
    };
    path += ` L ${format(start.x)} ${format(start.y)}`;
    path += ` Q ${format(corner.x)} ${format(corner.y)} ${format(end.x)} ${format(end.y)}`;
  }
  const last = points[points.length - 1]!;
  if (points.length > 1) path += ` L ${format(last.x)} ${format(last.y)}`;
  return path;
}

// Pulls the route in by the same distance at both ends. A route too short
// to spare it keeps its full length rather than losing its click target.
function trimmed(points: Point[], by: number): Point[] {
  if (routeLength(points) < by * 2 + 4) return points;
  const walk = (ordered: Point[]): Point[] => {
    const rest = [...ordered];
    let left = by;
    while (rest.length > 1 && left > 0) {
      const first = rest[0]!;
      const next = rest[1]!;
      const length = Math.abs(next.x - first.x) + Math.abs(next.y - first.y);
      if (length > left) {
        const at = left / length;
        rest[0] = {
          x: first.x + (next.x - first.x) * at,
          y: first.y + (next.y - first.y) * at,
        };
        left = 0;
      } else {
        rest.shift();
        left -= length;
      }
    }
    return rest;
  };
  return walk([...walk(points)].reverse()).reverse();
}

// The label rides the longest run of the route, where there is room to read
// it: above a horizontal run, and beside a vertical one.
function labelOn(points: Point[]): RouteLabel {
  let best = -1;
  let at = 0;
  for (let index = 1; index < points.length; index++) {
    const length =
      Math.abs(points[index]!.x - points[index - 1]!.x) +
      Math.abs(points[index]!.y - points[index - 1]!.y);
    if (length > best) {
      best = length;
      at = index;
    }
  }
  const from = points[at - 1] ?? points[0]!;
  const to = points[at] ?? points[0]!;
  if (from.y === to.y)
    return { x: (from.x + to.x) / 2, y: from.y - 4, anchor: "middle" };
  return { x: from.x + 6, y: (from.y + to.y) / 2, anchor: "start" };
}

interface Candidate {
  points: Point[];
  combo: number;
}

// The sides an arrow may leave and arrive on, the pair facing the other
// block first and the ones it only falls back on after it.
function sidePairs(from: Box, to: Box): [Side, Side][] {
  const gapX = Math.max(to.x - (from.x + from.w), from.x - (to.x + to.w));
  const gapY = Math.max(to.y - (from.y + from.h), from.y - (to.y + to.h));
  const sideways: Side = centerX(to) >= centerX(from) ? "right" : "left";
  const upright: Side = centerY(to) >= centerY(from) ? "bottom" : "top";
  return gapX >= gapY
    ? [
        [sideways, OPPOSITE[sideways]],
        [sideways, OPPOSITE[upright]],
        [upright, OPPOSITE[sideways]],
        [upright, OPPOSITE[upright]],
      ]
    : [
        [upright, OPPOSITE[upright]],
        [upright, OPPOSITE[sideways]],
        [sideways, OPPOSITE[upright]],
        [sideways, OPPOSITE[sideways]],
      ];
}

// The routes to try for one pair of sides: the ways across on a vertical
// corridor, then the ways along a horizontal one. Both leave and arrive
// along the side of the block, so every shape is orthogonal.
function candidatesFor(
  from: Box,
  to: Box,
  pair: [Side, Side],
  combo: number,
  near: readonly Box[],
): Candidate[] {
  const [exit, entry] = pair;
  const head = anchorOn(from, exit, to);
  const tail = anchorOn(to, entry, from);
  const out = stubEnd(head, exit);
  const back = stubEnd(tail, entry);
  const candidates: Candidate[] = [];
  for (const x of corridors(out.x, back.x, near, "x"))
    candidates.push({
      combo,
      points: simplify([
        head,
        out,
        { x, y: out.y },
        { x, y: back.y },
        back,
        tail,
      ]),
    });
  for (const y of corridors(out.y, back.y, near, "y"))
    candidates.push({
      combo,
      points: simplify([
        head,
        out,
        { x: out.x, y },
        { x: back.x, y },
        back,
        tail,
      ]),
    });
  return candidates;
}

function finish(points: Point[], clean: boolean): Route {
  const start = points[0] ?? { x: 0, y: 0 };
  const end = points[points.length - 1] ?? start;
  return {
    points,
    path: toPath(points),
    hitPath: toPath(trimmed(points, ARROW_INSET)),
    start,
    end,
    label: labelOn(points),
    clean,
  };
}

// The way an arrow runs from one block to the other: the first route that
// gets through without crossing anything, and otherwise the one that
// crosses least. Blocks the arrow does not join, and the two it does, are
// all in its way; the two it joins only at their borders.
export function routeArrow(
  from: Box,
  to: Box,
  options: RouteOptions = {},
): Route {
  const obstacles = options.obstacles ?? [];
  const blockers: Box[] = [from, to, ...obstacles];
  const reach = inflated(from, to, REACH);
  const near = obstacles.filter((box) => overlaps(box, reach));
  const pairs = sidePairs(from, to);
  let best: Candidate | undefined;
  let bestCost = Number.POSITIVE_INFINITY;
  let bestClean = false;
  for (let combo = 0; combo < pairs.length; combo++) {
    // No route on a later pair of sides can beat what its penalty alone
    // already costs, so the sides facing each other are usually the only
    // ones tried at all.
    if (bestCost <= combo * SIDE_PAIR_PENALTY) break;
    for (const candidate of candidatesFor(
      from,
      to,
      pairs[combo]!,
      combo,
      near,
    )) {
      let crossings = 0;
      for (const box of blockers)
        if (routeEnters(candidate.points, box)) crossings++;
      let outside = 0;
      if (options.bounds)
        for (const point of candidate.points)
          if (outsideBounds(point, options.bounds)) outside++;
      const bends = Math.max(0, candidate.points.length - 2);
      // A plain route through open ground is what draw.io draws, so take
      // it as soon as one turns up rather than weighing every other shape.
      if (crossings === 0 && outside === 0 && bends <= 2)
        return finish(candidate.points, true);
      const cost =
        crossings * 1000 +
        outside * 120 +
        bends * 8 +
        routeLength(candidate.points) * 0.01 +
        combo * SIDE_PAIR_PENALTY;
      if (cost < bestCost) {
        bestCost = cost;
        best = candidate;
        bestClean = crossings === 0;
      }
    }
  }
  return finish(best?.points ?? [], bestClean);
}

function chainOf(nodes: Map<string, LevelNode>, node: LevelNode): LevelNode[] {
  const chain: LevelNode[] = [node];
  let current = node;
  while (current.parentId) {
    const parent = nodes.get(current.parentId);
    if (!parent || chain.includes(parent)) break;
    chain.push(parent);
    current = parent;
  }
  return chain;
}

// The blocks an arrow has to get around, and the container it should stay
// inside.
//
// An arrow crosses the smallest container holding both of its ends, so the
// blocks in its way are that container's own children, plus the blocks
// beside each end inside the containers the arrow starts or finishes in.
// Left out are the two ends, every container around them, which the arrow
// has to come out of, and the blocks nested inside the children it passes:
// a container covers what it holds, and the arrow never gets in there.
export function arrowScope(
  nodes: readonly LevelNode[],
  fromId: string,
  toId?: string,
): ArrowScope {
  const byId = new Map(nodes.map((node) => [node.id, node]));
  const from = byId.get(fromId);
  const to = toId === undefined ? undefined : byId.get(toId);
  if (!from || (toId !== undefined && !to)) return { obstacles: [] };
  const above = chainOf(byId, from);
  const below = to ? chainOf(byId, to) : [];
  let scope: string | undefined;
  if (to) {
    const ancestors = new Set(above.map((node) => node.id));
    scope = below.find((node) => ancestors.has(node.id))?.id;
  } else scope = from.parentId;
  // The arrow's own ends and the containers it comes out of.
  const ends = new Set([...above, ...below].map((node) => node.id));
  // The levels it is drawn across: the shared one, and the inside of each
  // container it leaves or enters on the way there.
  const crossed = new Set<string | undefined>([scope]);
  for (const chain of [above, below])
    for (const node of chain) {
      if (node.id === scope) break;
      crossed.add(node.parentId);
    }
  return {
    obstacles: nodes
      .filter((node) => crossed.has(node.parentId) && !ends.has(node.id))
      .map((node) => node.box),
    bounds: scope === undefined ? undefined : byId.get(scope)?.box,
  };
}
