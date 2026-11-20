export const GRID_COLS = 24;
export const GRID_ROW_HEIGHT = 48;
export const GRID_GAP = 12;

export interface GridRect {
  id: string;
  x: number;
  y: number;
  w: number;
  h: number;
}

export function collides(a: GridRect, b: GridRect): boolean {
  if (a.id === b.id) return false;
  if (a.x + a.w <= b.x) return false;
  if (a.x >= b.x + b.w) return false;
  if (a.y + a.h <= b.y) return false;
  if (a.y >= b.y + b.h) return false;
  return true;
}

export function clampRect<T extends GridRect>(
  item: T,
  minW = 4,
  minH = 3,
  cols = GRID_COLS
): T {
  const w = Math.max(minW, Math.min(cols, Math.round(item.w)));
  const h = Math.max(minH, Math.round(item.h));
  const x = Math.max(0, Math.min(cols - w, Math.round(item.x)));
  const y = Math.max(0, Math.round(item.y));
  return { ...item, x, y, w, h };
}

export function sortByPosition<T extends GridRect>(items: T[]): T[] {
  return [...items].sort((a, b) => {
    if (a.y !== b.y) return a.y - b.y;
    if (a.x !== b.x) return a.x - b.x;
    return a.id.localeCompare(b.id);
  });
}

/**
 * Compacts all items upward (Grafana-style vertical gravity) while preventing overlaps.
 * If pinnedId is supplied, that item keeps its y >= desired position while others compact around it.
 */
export function compactLayout<T extends GridRect>(items: T[], cols = GRID_COLS, pinnedId?: string): T[] {
  const sorted = sortByPosition(items.map((it) => ({ ...it })));
  const placed: T[] = [];

  // Place pinned item first if present so other items flow around/below it
  if (pinnedId) {
    const pinnedIdx = sorted.findIndex((it) => it.id === pinnedId);
    if (pinnedIdx !== -1) {
      const [pinned] = sorted.splice(pinnedIdx, 1);
      pinned.x = Math.max(0, Math.min(cols - pinned.w, pinned.x));
      pinned.y = Math.max(0, pinned.y);
      placed.push(pinned);
    }
  }

  for (const item of sorted) {
    item.x = Math.max(0, Math.min(cols - item.w, item.x));
    item.y = Math.max(0, item.y);

    // Push down if colliding with any already-placed item (e.g. pinned item)
    while (placed.some((p) => collides(item, p))) {
      item.y += 1;
    }

    // Float upward as far as possible without colliding
    while (item.y > 0) {
      const candidate = { ...item, y: item.y - 1 };
      if (placed.some((p) => collides(candidate, p))) {
        break;
      }
      item.y -= 1;
    }

    placed.push(item);
  }

  // If pinned item was placed high with empty space above it, allow it to settle cleanly too
  // only when not actively dragging (callers pass pinnedId during active drag/resize).
  return sortByPosition(placed);
}

/**
 * Moves a widget to (targetX, targetY), pushing colliding widgets down and compacting vertically.
 */
export function moveGridItem<T extends GridRect>(
  items: T[],
  id: string,
  targetX: number,
  targetY: number,
  cols = GRID_COLS,
  pinMoved = true
): T[] {
  const cloned = items.map((it) => ({ ...it }));
  const target = cloned.find((it) => it.id === id);
  if (!target) return cloned;

  target.x = Math.max(0, Math.min(cols - target.w, Math.round(targetX)));
  target.y = Math.max(0, Math.round(targetY));

  return compactLayout(cloned, cols, pinMoved ? id : undefined);
}

/**
 * Resizes a widget to (targetW, targetH) respecting minW/minH and grid bounds,
 * pushing colliding widgets down and compacting vertically.
 */
export function resizeGridItem<T extends GridRect>(
  items: T[],
  id: string,
  targetW: number,
  targetH: number,
  minW = 4,
  minH = 3,
  cols = GRID_COLS
): T[] {
  const cloned = items.map((it) => ({ ...it }));
  const target = cloned.find((it) => it.id === id);
  if (!target) return cloned;

  const maxW = cols - target.x;
  target.w = Math.max(minW, Math.min(maxW, Math.round(targetW)));
  target.h = Math.max(minH, Math.round(targetH));

  return compactLayout(cloned, cols, id);
}

/**
 * Finds the first available (x, y) slot scanning left-to-right, top-to-bottom.
 */
export function findOpenSlot(
  items: GridRect[],
  w: number,
  h: number,
  cols = GRID_COLS
): { x: number; y: number } {
  const width = Math.max(1, Math.min(cols, Math.round(w)));
  const height = Math.max(1, Math.round(h));
  const maxY = getGridTotalRows(items);

  for (let y = 0; y <= maxY + 1; y++) {
    for (let x = 0; x <= cols - width; x++) {
      const probe: GridRect = { id: '__probe__', x, y, w: width, h: height };
      if (!items.some((it) => collides(probe, it))) {
        return { x, y };
      }
    }
  }
  return { x: 0, y: maxY };
}

export function getGridTotalRows(items: GridRect[]): number {
  let maxRow = 0;
  for (const it of items) {
    const end = it.y + it.h;
    if (end > maxRow) maxRow = end;
  }
  return maxRow;
}
