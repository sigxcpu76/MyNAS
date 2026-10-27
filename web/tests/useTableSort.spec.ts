import { describe, it, expect } from 'vitest';
import { ref } from 'vue';
import { useTableSort, sortHierarchy } from '../src/ui/composables/useTableSort';

interface Row {
  name: string;
  size: number | null;
}

describe('useTableSort', () => {
  const rows = ref<Row[]>([
    { name: 'disk10', size: 30 },
    { name: 'disk2', size: null },
    { name: 'Disk1', size: 10 },
  ]);

  it('keeps natural order when no key is selected', () => {
    const s = useTableSort(rows, { name: (r) => r.name, size: (r) => r.size });
    expect(s.sorted.value.map((r) => r.name)).toEqual(['disk10', 'disk2', 'Disk1']);
  });

  it('sorts strings naturally and case-insensitively, cycling asc -> desc -> off', () => {
    const s = useTableSort(rows, { name: (r) => r.name });
    s.toggle('name');
    expect(s.sorted.value.map((r) => r.name)).toEqual(['Disk1', 'disk2', 'disk10']);
    s.toggle('name');
    expect(s.sorted.value.map((r) => r.name)).toEqual(['disk10', 'disk2', 'Disk1']);
    s.toggle('name');
    expect(s.key.value).toBeNull();
  });

  it('sorts numbers and always places empty values last', () => {
    const s = useTableSort(rows, { size: (r) => r.size });
    s.toggle('size');
    expect(s.sorted.value.map((r) => r.size)).toEqual([10, 30, null]);
    s.toggle('size');
    expect(s.sorted.value.map((r) => r.size)).toEqual([30, 10, null]);
  });

  it('reacts to source changes', () => {
    const src = ref<Row[]>([{ name: 'b', size: 1 }]);
    const s = useTableSort(src, { name: (r) => r.name }, { key: 'name' });
    src.value = [...src.value, { name: 'a', size: 2 }];
    expect(s.sorted.value.map((r) => r.name)).toEqual(['a', 'b']);
  });
});

describe('sortHierarchy', () => {
  it('sorts siblings while keeping children under their parent', () => {
    const items = [
      { id: 'pool', parent: null, n: 'pool' },
      { id: 'pool/b', parent: 'pool', n: 'b' },
      { id: 'pool/b/x', parent: 'pool/b', n: 'x' },
      { id: 'pool/a', parent: 'pool', n: 'a' },
    ];
    const out = sortHierarchy(
      items,
      (i) => i.id,
      (i) => i.parent,
      (a, b) => a.n.localeCompare(b.n)
    );
    expect(out.map((i) => i.id)).toEqual(['pool', 'pool/a', 'pool/b', 'pool/b/x']);
  });
});
