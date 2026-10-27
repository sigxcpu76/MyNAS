/**
 * Reusable client-side table sorting (`useTableSort.ts`).
 *
 * Usage:
 *   const sort = useTableSort(rows, {
 *     name: (r) => r.name,
 *     size: (r) => r.sizeBytes,
 *   }, { key: 'name', dir: 'asc' });
 *   // template: v-for="r in sort.sorted.value", <SortableTh :sort="sort" sort-key="name">Name</SortableTh>
 *
 * Comparison rules:
 *  - numbers / booleans / Dates compare numerically
 *  - strings compare with natural (numeric-aware), case-insensitive collation
 *    so "disk2" < "disk10" and "sda" < "sdb"
 *  - null / undefined / '' always sort last regardless of direction
 *  - sorting is stable (ties keep original order)
 */
import { computed, ref, unref, type ComputedRef, type MaybeRef, type Ref } from 'vue';

export type SortDirection = 'asc' | 'desc';
export type SortValue = string | number | boolean | Date | null | undefined;
export type SortAccessors<T> = Record<string, (row: T) => SortValue>;

export interface TableSortState {
  key: Ref<string | null>;
  dir: Ref<SortDirection>;
  toggle: (key: string) => void;
}

export interface TableSort<T> extends TableSortState {
  sorted: ComputedRef<T[]>;
  compare: (a: T, b: T) => number;
}

const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' });

function isEmpty(v: SortValue): boolean {
  return v === null || v === undefined || v === '' || (typeof v === 'number' && Number.isNaN(v));
}

export function compareSortValues(a: SortValue, b: SortValue): number {
  if (a instanceof Date) a = a.getTime();
  if (b instanceof Date) b = b.getTime();
  if (typeof a === 'boolean') a = a ? 1 : 0;
  if (typeof b === 'boolean') b = b ? 1 : 0;
  if (typeof a === 'number' && typeof b === 'number') return a - b;
  return collator.compare(String(a), String(b));
}

export function useTableSort<T>(
  rows: MaybeRef<T[] | null | undefined> | (() => T[] | null | undefined),
  accessors: SortAccessors<T>,
  initial?: { key?: string | null; dir?: SortDirection }
): TableSort<T> {
  const key = ref<string | null>(initial?.key ?? null);
  const dir = ref<SortDirection>(initial?.dir ?? 'asc');

  function toggle(k: string) {
    if (key.value === k) {
      if (dir.value === 'asc') {
        dir.value = 'desc';
      } else {
        // Third click clears sorting and restores the natural order
        key.value = initial?.key ?? null;
        dir.value = initial?.dir ?? 'asc';
        if (initial?.key === k) dir.value = 'asc';
      }
    } else {
      key.value = k;
      dir.value = 'asc';
    }
  }

  function compare(a: T, b: T): number {
    const k = key.value;
    if (!k || !accessors[k]) return 0;
    const va = accessors[k](a);
    const vb = accessors[k](b);
    const ea = isEmpty(va);
    const eb = isEmpty(vb);
    if (ea && eb) return 0;
    if (ea) return 1;
    if (eb) return -1;
    const c = compareSortValues(va, vb);
    return dir.value === 'asc' ? c : -c;
  }

  const sorted = computed<T[]>(() => {
    const list = (typeof rows === 'function' ? (rows as () => T[])() : unref(rows)) || [];
    if (!key.value || !accessors[key.value]) return list;
    return list
      .map((row, idx) => ({ row, idx }))
      .sort((x, y) => compare(x.row, y.row) || x.idx - y.idx)
      .map((x) => x.row);
  });

  return { key, dir, toggle, sorted, compare };
}

/**
 * Sort a flattened tree (parent/child hierarchy) while keeping children
 * grouped under their parent. Siblings are ordered by `compare`.
 */
export function sortHierarchy<T>(
  rows: T[],
  getId: (row: T) => string,
  getParentId: (row: T) => string | null | undefined,
  compare: (a: T, b: T) => number
): T[] {
  const ids = new Set(rows.map(getId));
  const children = new Map<string | null, T[]>();
  rows.forEach((r) => {
    const p = getParentId(r);
    const parent = p && ids.has(p) ? p : null;
    if (!children.has(parent)) children.set(parent, []);
    children.get(parent)!.push(r);
  });
  const out: T[] = [];
  const walk = (parent: string | null) => {
    const list = children.get(parent);
    if (!list) return;
    list
      .map((row, idx) => ({ row, idx }))
      .sort((x, y) => compare(x.row, y.row) || x.idx - y.idx)
      .forEach(({ row }) => {
        out.push(row);
        walk(getId(row));
      });
  };
  walk(null);
  return out;
}
