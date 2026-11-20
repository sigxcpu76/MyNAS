import { describe, expect, it } from 'vitest';
import {
  GRID_COLS,
  collides,
  compactLayout,
  findOpenSlot,
  moveGridItem,
  resizeGridItem,
  type GridRect,
} from '../src/ui/dashboard/gridEngine';

describe('Grafana Fluid Grid Engine', () => {
  it('detects collisions accurately', () => {
    const a: GridRect = { id: 'a', x: 0, y: 0, w: 8, h: 4 };
    const b: GridRect = { id: 'b', x: 8, y: 0, w: 8, h: 4 };
    const c: GridRect = { id: 'c', x: 4, y: 2, w: 8, h: 4 };

    expect(collides(a, b)).toBe(false);
    expect(collides(a, c)).toBe(true);
    expect(collides(a, a)).toBe(false);
  });

  it('compacts layout upward without creating overlaps', () => {
    const items: GridRect[] = [
      { id: 'a', x: 0, y: 5, w: 12, h: 4 },
      { id: 'b', x: 12, y: 8, w: 12, h: 3 },
      { id: 'c', x: 0, y: 12, w: 24, h: 2 },
    ];
    const compacted = compactLayout(items, GRID_COLS);
    const byId = Object.fromEntries(compacted.map((i) => [i.id, i]));

    expect(byId.a.y).toBe(0);
    expect(byId.b.y).toBe(0);
    expect(byId.c.y).toBe(4);
  });

  it('moves a widget and pushes colliding widgets down cleanly', () => {
    const items: GridRect[] = [
      { id: 'a', x: 0, y: 0, w: 12, h: 4 },
      { id: 'b', x: 12, y: 0, w: 12, h: 4 },
    ];
    const moved = moveGridItem(items, 'b', 0, 0, GRID_COLS, true);
    const byId = Object.fromEntries(moved.map((i) => [i.id, i]));

    expect(byId.b).toMatchObject({ x: 0, y: 0 });
    expect(byId.a).toMatchObject({ x: 0, y: 4 });
    expect(collides(byId.a, byId.b)).toBe(false);
  });

  it('resizes a widget within bounds and reflows colliding neighbors', () => {
    const items: GridRect[] = [
      { id: 'a', x: 0, y: 0, w: 8, h: 4 },
      { id: 'b', x: 8, y: 0, w: 8, h: 4 },
    ];
    const resized = resizeGridItem(items, 'a', 16, 6, 4, 3, GRID_COLS);
    const byId = Object.fromEntries(resized.map((i) => [i.id, i]));

    expect(byId.a).toMatchObject({ x: 0, y: 0, w: 16, h: 6 });
    expect(byId.b.y).toBeGreaterThanOrEqual(6);
    expect(collides(byId.a, byId.b)).toBe(false);
  });

  it('finds the first open slot for a new widget', () => {
    const items: GridRect[] = [
      { id: 'a', x: 0, y: 0, w: 12, h: 4 },
    ];
    expect(findOpenSlot(items, 8, 4, GRID_COLS)).toEqual({ x: 12, y: 0 });
    expect(findOpenSlot(items, 16, 4, GRID_COLS)).toEqual({ x: 0, y: 4 });
  });
});
