import { describe, expect, test } from "vitest";
import {
  ARROW_INSET,
  arrowScope,
  routeArrow,
  type Box,
  type LevelNode,
  type Point,
  type Route,
} from "./routing.js";

// Walks the polyline and reports whether any point of it lies inside the
// box. Sampling is deliberately independent of the routing module's own
// crossing test, so a mistake in that test cannot hide a broken route.
function entersBox(points: Point[], box: Box, margin = 1): boolean {
  const inside = (point: Point) =>
    point.x > box.x + margin &&
    point.x < box.x + box.w - margin &&
    point.y > box.y + margin &&
    point.y < box.y + box.h - margin;
  for (let index = 1; index < points.length; index++) {
    const from = points[index - 1]!;
    const to = points[index]!;
    for (let step = 0; step <= 200; step++) {
      const at = step / 200;
      if (
        inside({
          x: from.x + (to.x - from.x) * at,
          y: from.y + (to.y - from.y) * at,
        })
      )
        return true;
    }
  }
  return false;
}

function bends(points: Point[]): number {
  return Math.max(0, points.length - 2);
}

// Every segment of an orthogonal route runs along one axis only.
function isOrthogonal(points: Point[]): boolean {
  for (let index = 1; index < points.length; index++) {
    const from = points[index - 1]!;
    const to = points[index]!;
    if (from.x !== to.x && from.y !== to.y) return false;
  }
  return true;
}

// The point of an SVG path's opening "M" command.
function pathStart(path: string): Point {
  const found = /^M\s*(-?[\d.]+)\s+(-?[\d.]+)/.exec(path);
  if (!found) throw new Error(`no move command in ${path}`);
  return { x: Number(found[1]), y: Number(found[2]) };
}

const LEFT: Box = { x: 0, y: 0, w: 100, h: 60 };

describe("routing an arrow between two blocks", () => {
  test("blocks side by side are joined by one straight run between the facing sides", () => {
    const right: Box = { x: 300, y: 0, w: 100, h: 60 };

    const route = routeArrow(LEFT, right);

    expect(route.points).toEqual([
      { x: 100, y: 30 },
      { x: 300, y: 30 },
    ]);
    expect(route.clean).toBe(true);
    expect(route.path).not.toContain("Q");
  });

  test("a block below is left from the bottom side and entered at the top", () => {
    const below: Box = { x: 0, y: 200, w: 100, h: 60 };

    const route = routeArrow(LEFT, below);

    expect(route.start).toEqual({ x: 50, y: 60 });
    expect(route.end).toEqual({ x: 50, y: 200 });
  });

  test("a block set off on both axes is reached in orthogonal segments with rounded corners", () => {
    const away: Box = { x: 300, y: 200, w: 100, h: 60 };

    const route = routeArrow(LEFT, away);

    expect(isOrthogonal(route.points)).toBe(true);
    expect(route.start).toEqual({ x: 100, y: 30 });
    expect(route.end).toEqual({ x: 300, y: 230 });
    expect(bends(route.points)).toBe(2);
    // One rounded corner per bend, so no hard right angle is drawn.
    expect(route.path.match(/Q/g)).toHaveLength(2);
  });

  test("an arrow goes around a block standing between its ends", () => {
    const far: Box = { x: 400, y: 0, w: 100, h: 60 };
    const between: Box = { x: 180, y: -40, w: 100, h: 140 };

    const route = routeArrow(LEFT, far, { obstacles: [between] });

    expect(route.clean).toBe(true);
    expect(entersBox(route.points, between)).toBe(false);
    expect(isOrthogonal(route.points)).toBe(true);
    expect(bends(route.points)).toBeGreaterThanOrEqual(2);
    expect(route.start).toEqual({ x: 100, y: 30 });
    expect(route.end).toEqual({ x: 400, y: 30 });
  });

  test("an arrow goes around a wall of blocks rather than through its gapless middle", () => {
    const far: Box = { x: 500, y: 0, w: 100, h: 60 };
    const wall: Box[] = [
      { x: 200, y: -200, w: 120, h: 200 },
      { x: 200, y: 0, w: 120, h: 120 },
    ];

    const route = routeArrow(LEFT, far, { obstacles: wall });

    expect(route.clean).toBe(true);
    for (const block of wall)
      expect(entersBox(route.points, block)).toBe(false);
  });

  test("an arrow never cuts back through the blocks it joins", () => {
    const above: Box = { x: 20, y: -200, w: 100, h: 60 };

    const route = routeArrow(LEFT, above);

    expect(entersBox(route.points, LEFT)).toBe(false);
    expect(entersBox(route.points, above)).toBe(false);
  });

  test("the same blocks route the same way whatever order the obstacles arrive in", () => {
    const far: Box = { x: 600, y: 0, w: 100, h: 60 };
    const obstacles: Box[] = [
      { x: 180, y: -40, w: 100, h: 140 },
      { x: 320, y: -40, w: 100, h: 140 },
      { x: 460, y: -40, w: 100, h: 140 },
    ];

    const route = routeArrow(LEFT, far, { obstacles });
    const reversed = routeArrow(LEFT, far, {
      obstacles: [...obstacles].reverse(),
    });

    expect(reversed.path).toBe(route.path);
    expect(reversed.points).toEqual(route.points);
  });

  test("a route prefers staying inside the container that holds both blocks", () => {
    const bounds: Box = { x: -40, y: -40, w: 640, h: 200 };
    const far: Box = { x: 400, y: 0, w: 100, h: 60 };
    const between: Box = { x: 180, y: -40, w: 100, h: 140 };

    const route = routeArrow(LEFT, far, { obstacles: [between], bounds });

    expect(route.clean).toBe(true);
    for (const point of route.points) {
      expect(point.x).toBeGreaterThanOrEqual(bounds.x);
      expect(point.x).toBeLessThanOrEqual(bounds.x + bounds.w);
      expect(point.y).toBeGreaterThanOrEqual(bounds.y);
      expect(point.y).toBeLessThanOrEqual(bounds.y + bounds.h);
    }
  });

  test("an arrow with no way through is still drawn, and says that it crosses", () => {
    const walled: Box = { x: 400, y: 0, w: 100, h: 60 };
    // A sealed ring of blocks around the target, with no gap to slip through.
    const ring: Box[] = [
      { x: 340, y: -60, w: 260, h: 10 },
      { x: 340, y: 110, w: 260, h: 10 },
      { x: 340, y: -50, w: 10, h: 160 },
      { x: 590, y: -50, w: 10, h: 160 },
    ];

    const route = routeArrow(LEFT, walled, { obstacles: ring });

    expect(route.clean).toBe(false);
    expect(route.points.length).toBeGreaterThanOrEqual(2);
    expect(route.path).toMatch(/^M/);
    expect(route.path).not.toMatch(/NaN/);
  });
});

describe("routes between blocks in degenerate positions", () => {
  test("overlapping blocks still give a drawable path", () => {
    const over: Box = { x: 40, y: 20, w: 100, h: 60 };

    const route = routeArrow(LEFT, over);

    expect(route.points.length).toBeGreaterThanOrEqual(2);
    expect(isOrthogonal(route.points)).toBe(true);
    expect(route.path).not.toMatch(/NaN/);
  });

  test("a block sitting inside another still gives a drawable path", () => {
    const inner: Box = { x: 20, y: 15, w: 30, h: 20 };

    const route = routeArrow(LEFT, inner);

    expect(route.points.length).toBeGreaterThanOrEqual(2);
    expect(route.path).not.toMatch(/NaN/);
    expect(route.label.x).not.toBeNaN();
  });

  test("blocks at the very same place still give a drawable path", () => {
    const route = routeArrow(LEFT, { ...LEFT });

    expect(route.points.length).toBeGreaterThanOrEqual(2);
    expect(route.path).not.toMatch(/NaN/);
    expect(route.hitPath).not.toMatch(/NaN/);
  });

  test("a target with no size, such as the pointer, is reached like any other", () => {
    const pointer: Box = { x: 400, y: 30, w: 0, h: 0 };

    const route = routeArrow(LEFT, pointer);

    expect(route.end).toEqual({ x: 400, y: 30 });
    expect(route.path).not.toMatch(/NaN/);
  });
});

describe("what hangs off a routed arrow", () => {
  test("the click target starts along the route, clear of the block's resize strip", () => {
    const right: Box = { x: 300, y: 0, w: 100, h: 60 };

    const route = routeArrow(LEFT, right);

    expect(pathStart(route.hitPath)).toEqual({ x: 100 + ARROW_INSET, y: 30 });
    expect(route.hitPath).toMatch(
      new RegExp(`${300 - ARROW_INSET}\\s+30\\s*$`),
    );
  });

  test("an arrow too short to inset keeps a click target of its own", () => {
    const close: Box = { x: 104, y: 0, w: 100, h: 60 };

    const route = routeArrow(LEFT, close);

    expect(route.hitPath).toMatch(/^M/);
    expect(route.hitPath).not.toMatch(/NaN/);
    expect(pathStart(route.hitPath)).toEqual({ x: 100, y: 30 });
  });

  test("the label of a straight arrow sits above its middle", () => {
    const right: Box = { x: 300, y: 0, w: 100, h: 60 };

    const route = routeArrow(LEFT, right);

    expect(route.label.anchor).toBe("middle");
    expect(route.label.x).toBe(200);
    expect(route.label.y).toBeLessThan(30);
  });

  test("the label of an arrow running down sits beside its longest run", () => {
    const below: Box = { x: 0, y: 600, w: 100, h: 60 };

    const route = routeArrow(LEFT, below);

    expect(route.label.anchor).toBe("start");
    expect(route.label.x).toBeGreaterThan(50);
    expect(route.label.y).toBe(330);
  });

  test("the label of a bent arrow sits on its longest run, not at the middle of the ends", () => {
    const away: Box = { x: 900, y: 200, w: 100, h: 60 };

    const route = routeArrow(LEFT, away);

    // The label rides the first long run, well away from the midpoint of
    // the two ends, which the route itself only passes through sideways.
    expect(route.label.anchor).toBe("middle");
    expect(route.label.x).toBe(300);
    expect(route.label.y).toBeLessThan(30);
  });
});

// The blocks an arrow has to get around: the ones drawn at its own level.
describe("which blocks an arrow has to avoid", () => {
  const nodes: LevelNode[] = [
    { id: "project", box: { x: 0, y: 0, w: 800, h: 600 } },
    { id: "coder", parentId: "project", box: { x: 20, y: 40, w: 100, h: 60 } },
    { id: "gate", parentId: "project", box: { x: 300, y: 40, w: 100, h: 60 } },
    { id: "fixer", parentId: "project", box: { x: 600, y: 40, w: 100, h: 60 } },
    {
      id: "github",
      parentId: "project",
      box: { x: 20, y: 300, w: 200, h: 200 },
    },
    { id: "commit", parentId: "github", box: { x: 40, y: 340, w: 150, h: 40 } },
    { id: "push", parentId: "github", box: { x: 40, y: 400, w: 150, h: 40 } },
    { id: "loose", box: { x: 900, y: 40, w: 100, h: 60 } },
  ];

  test("the blocks beside the two ends are the obstacles, their container is not", () => {
    const scope = arrowScope(nodes, "coder", "fixer");

    expect(scope.obstacles).toEqual([
      { x: 300, y: 40, w: 100, h: 60 },
      { x: 20, y: 300, w: 200, h: 200 },
    ]);
    expect(scope.bounds).toEqual({ x: 0, y: 0, w: 800, h: 600 });
  });

  test("an arrow out of a nested block avoids the blocks at both levels it crosses", () => {
    const scope = arrowScope(nodes, "commit", "fixer");

    // The GitHub block the arrow comes out of is not in its own way, but
    // the action beside it is, and so are the blocks at the project's level
    // that the arrow then runs across.
    expect(scope.obstacles).toEqual([
      { x: 20, y: 40, w: 100, h: 60 },
      { x: 300, y: 40, w: 100, h: 60 },
      { x: 40, y: 400, w: 150, h: 40 },
    ]);
    expect(scope.bounds).toEqual({ x: 0, y: 0, w: 800, h: 600 });
  });

  test("blocks nested inside an obstacle are covered by it and not listed again", () => {
    const scope = arrowScope(nodes, "coder", "gate");

    expect(scope.obstacles).toContainEqual({ x: 20, y: 300, w: 200, h: 200 });
    expect(scope.obstacles).not.toContainEqual({
      x: 40,
      y: 340,
      w: 150,
      h: 40,
    });
  });

  test("two actions inside the same block only have each other in the way", () => {
    const scope = arrowScope(nodes, "commit", "push");

    expect(scope.obstacles).toEqual([]);
    expect(scope.bounds).toEqual({ x: 20, y: 300, w: 200, h: 200 });
  });

  test("an arrow on the canvas itself has no container to stay inside", () => {
    const scope = arrowScope(
      [...nodes, { id: "other", box: { x: 900, y: 300, w: 100, h: 60 } }],
      "project",
      "loose",
    );

    expect(scope.bounds).toBeUndefined();
    expect(scope.obstacles).toEqual([{ x: 900, y: 300, w: 100, h: 60 }]);
  });

  test("an arrow still being drawn avoids the blocks beside the one it leaves", () => {
    const scope = arrowScope(nodes, "coder");

    expect(scope.obstacles).toHaveLength(3);
    expect(scope.bounds).toEqual({ x: 0, y: 0, w: 800, h: 600 });
  });

  test("a block that is gone leaves the arrow without obstacles", () => {
    expect(arrowScope(nodes, "missing", "fixer").obstacles).toEqual([]);
  });
});
const FIT_BLOCKS: LevelNode[] = [
  { id: "fit", box: { x: 40, y: 40, w: 2000, h: 1040 } },
  { id: "repository", parentId: "fit", box: { x: 60, y: 100, w: 250, h: 500 } },
  { id: "sync", parentId: "repository", box: { x: 80, y: 140, w: 210, h: 64 } },
  {
    id: "story-worktree",
    parentId: "repository",
    box: { x: 80, y: 230, w: 210, h: 64 },
  },
  {
    id: "freeze",
    parentId: "repository",
    box: { x: 80, y: 320, w: 210, h: 64 },
  },
  {
    id: "push-integration",
    parentId: "repository",
    box: { x: 80, y: 410, w: 210, h: 64 },
  },
  {
    id: "open-pull-request",
    parentId: "repository",
    box: { x: 80, y: 500, w: 210, h: 64 },
  },
  { id: "pick-story", parentId: "fit", box: { x: 340, y: 100, w: 180, h: 64 } },
  { id: "whose-call", parentId: "fit", box: { x: 340, y: 210, w: 180, h: 64 } },
  {
    id: "gabriels-call",
    parentId: "fit",
    box: { x: 340, y: 320, w: 180, h: 64 },
  },
  { id: "hold", parentId: "fit", box: { x: 340, y: 430, w: 180, h: 64 } },
  { id: "slice", parentId: "fit", box: { x: 340, y: 540, w: 180, h: 64 } },
  {
    id: "failing-tests",
    parentId: "fit",
    box: { x: 300, y: 650, w: 440, h: 210 },
  },
  {
    id: "tests-fail",
    parentId: "failing-tests",
    box: { x: 320, y: 700, w: 180, h: 64 },
  },
  {
    id: "test-writer",
    parentId: "failing-tests",
    box: { x: 540, y: 780, w: 180, h: 64 },
  },
  { id: "delegate", parentId: "fit", box: { x: 820, y: 100, w: 180, h: 64 } },
  {
    id: "capability-rung",
    parentId: "fit",
    box: { x: 820, y: 210, w: 180, h: 64 },
  },
  {
    id: "mechanic-implements",
    parentId: "fit",
    box: { x: 800, y: 330, w: 440, h: 210 },
  },
  {
    id: "mechanic-gates",
    parentId: "mechanic-implements",
    box: { x: 820, y: 380, w: 180, h: 64 },
  },
  {
    id: "mechanic",
    parentId: "mechanic-implements",
    box: { x: 1040, y: 460, w: 180, h: 64 },
  },
  {
    id: "builder-implements",
    parentId: "fit",
    box: { x: 800, y: 570, w: 440, h: 210 },
  },
  {
    id: "builder-gates",
    parentId: "builder-implements",
    box: { x: 820, y: 620, w: 180, h: 64 },
  },
  {
    id: "builder",
    parentId: "builder-implements",
    box: { x: 1040, y: 700, w: 180, h: 64 },
  },
  {
    id: "solver-implements",
    parentId: "fit",
    box: { x: 800, y: 810, w: 440, h: 210 },
  },
  {
    id: "solver-gates",
    parentId: "solver-implements",
    box: { x: 820, y: 860, w: 180, h: 64 },
  },
  {
    id: "solver",
    parentId: "solver-implements",
    box: { x: 1040, y: 940, w: 180, h: 64 },
  },
  {
    id: "final-barrier",
    parentId: "fit",
    box: { x: 1300, y: 100, w: 180, h: 64 },
  },
  {
    id: "review-rounds",
    parentId: "fit",
    box: { x: 1280, y: 220, w: 460, h: 330 },
  },
  {
    id: "reviewer",
    parentId: "review-rounds",
    box: { x: 1300, y: 270, w: 180, h: 64 },
  },
  {
    id: "verdict",
    parentId: "review-rounds",
    box: { x: 1300, y: 380, w: 180, h: 64 },
  },
  {
    id: "review-fixer",
    parentId: "review-rounds",
    box: { x: 1530, y: 380, w: 180, h: 64 },
  },
  {
    id: "push-review-fixes",
    parentId: "review-rounds",
    box: { x: 1530, y: 470, w: 180, h: 64 },
  },
  {
    id: "ci-fix-rounds",
    parentId: "fit",
    box: { x: 1280, y: 580, w: 440, h: 210 },
  },
  {
    id: "required-checks",
    parentId: "ci-fix-rounds",
    box: { x: 1300, y: 630, w: 180, h: 64 },
  },
  {
    id: "ci-fixer",
    parentId: "ci-fix-rounds",
    box: { x: 1520, y: 710, w: 180, h: 64 },
  },
  { id: "merge", parentId: "fit", box: { x: 1300, y: 830, w: 180, h: 64 } },
  { id: "qa-deploy", parentId: "fit", box: { x: 1800, y: 100, w: 180, h: 64 } },
  { id: "main-ci", parentId: "fit", box: { x: 1800, y: 210, w: 180, h: 64 } },
  {
    id: "production-deploy",
    parentId: "fit",
    box: { x: 1800, y: 320, w: 180, h: 64 },
  },
  { id: "cleanup", parentId: "fit", box: { x: 1800, y: 430, w: 180, h: 64 } },
  {
    id: "hand-to-gabriel",
    parentId: "fit",
    box: { x: 1800, y: 580, w: 180, h: 64 },
  },
  {
    id: "mark-blocked",
    parentId: "fit",
    box: { x: 1800, y: 690, w: 180, h: 64 },
  },
];

const FIT_ARROWS: [string, string][] = [
  ["sync", "story-worktree"],
  ["final-barrier", "freeze"],
  ["freeze", "push-integration"],
  ["push-integration", "open-pull-request"],
  ["story-worktree", "pick-story"],
  ["pick-story", "whose-call"],
  ["whose-call", "gabriels-call"],
  ["gabriels-call", "hold"],
  ["whose-call", "slice"],
  ["hold", "slice"],
  ["slice", "failing-tests"],
  ["tests-fail", "test-writer"],
  ["failing-tests", "delegate"],
  ["slice", "delegate"],
  ["delegate", "capability-rung"],
  ["capability-rung", "mechanic-implements"],
  ["mechanic-gates", "mechanic"],
  ["capability-rung", "builder-implements"],
  ["mechanic-implements", "builder-implements"],
  ["builder-gates", "builder"],
  ["capability-rung", "solver-implements"],
  ["builder-implements", "solver-implements"],
  ["solver-gates", "solver"],
  ["mechanic-implements", "final-barrier"],
  ["builder-implements", "final-barrier"],
  ["solver-implements", "final-barrier"],
  ["open-pull-request", "review-rounds"],
  ["reviewer", "verdict"],
  ["verdict", "review-fixer"],
  ["review-fixer", "push-review-fixes"],
  ["review-rounds", "ci-fix-rounds"],
  ["required-checks", "ci-fixer"],
  ["ci-fix-rounds", "merge"],
  ["merge", "qa-deploy"],
  ["qa-deploy", "main-ci"],
  ["main-ci", "production-deploy"],
  ["production-deploy", "cleanup"],
  ["main-ci", "cleanup"],
  ["gabriels-call", "hand-to-gabriel"],
  ["capability-rung", "hand-to-gabriel"],
  ["review-rounds", "hand-to-gabriel"],
  ["ci-fix-rounds", "hand-to-gabriel"],
  ["merge", "hand-to-gabriel"],
  ["qa-deploy", "hand-to-gabriel"],
  ["production-deploy", "hand-to-gabriel"],
  ["failing-tests", "mark-blocked"],
  ["solver-implements", "mark-blocked"],
];

describe("routing the Fit_ development flow", () => {
  // One drag frame: every arrow of the template routed from scratch.
  function frame(blocks: LevelNode[]): Route[] {
    const boxes = new Map(blocks.map((block) => [block.id, block.box]));
    return FIT_ARROWS.map(([fromId, toId]) => {
      const scope = arrowScope(blocks, fromId, toId);
      return routeArrow(boxes.get(fromId)!, boxes.get(toId)!, scope);
    });
  }

  test("every arrow of the template is routed", () => {
    const routes = frame(FIT_BLOCKS);

    expect(routes).toHaveLength(47);
    for (const route of routes) {
      expect(route.path).toMatch(/^M/);
      expect(route.path).not.toMatch(/NaN/);
    }
  });

  // The dragged block moves every frame, so no route can be reused.
  function measure(route: (blocks: LevelNode[]) => unknown): number {
    const frames = 60;
    const start = performance.now();
    for (let step = 0; step < frames; step++)
      route(
        FIT_BLOCKS.map((block) =>
          block.id === "reviewer"
            ? { ...block, box: { ...block.box, x: block.box.x + step } }
            : block,
        ),
      );
    return (performance.now() - start) / frames;
  }

  test("a drag frame routes the whole template well inside a frame", () => {
    // Here a frame takes about 1.5 ms for all 47 arrows. A shared CI runner
    // with a dozen workers on it is several times slower, so the budget is
    // wide enough to survive that and still catch a routing cost that grows
    // by an order of magnitude.
    expect(measure(frame)).toBeLessThan(40);
  });

  test("the blocks in the way cost only a few times a clear run", () => {
    // Routing the same arrows with nothing to avoid is the floor. Staying
    // near it is what keeps a drag smooth however loaded the machine is,
    // and it is what a search that widened out of hand would lose.
    const clear = measure((blocks) => {
      const boxes = new Map(blocks.map((block) => [block.id, block.box]));
      return FIT_ARROWS.map(([fromId, toId]) =>
        routeArrow(boxes.get(fromId)!, boxes.get(toId)!),
      );
    });

    expect(measure(frame)).toBeLessThan(Math.max(clear, 0.05) * 25);
  });
});
