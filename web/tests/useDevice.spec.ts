import { describe, it, expect, beforeEach } from 'vitest';
import { useDevice } from '../src/ui/composables/useDevice';

const store = new Map<string, string>();
const localStorageMock = {
  getItem: (key: string) => store.get(key) ?? null,
  setItem: (key: string, val: string) => store.set(key, String(val)),
  removeItem: (key: string) => store.delete(key),
  clear: () => store.clear(),
};

Object.defineProperty(window, 'localStorage', {
  value: localStorageMock,
  writable: true,
});

describe('useDevice composable', () => {
  beforeEach(() => {
    localStorageMock.clear();
  });

  it('allows setting and reading view mode', () => {
    const { isMobile, viewMode, setViewMode } = useDevice();

    setViewMode('mobile');
    expect(viewMode.value).toBe('mobile');
    expect(isMobile.value).toBe(true);
    expect(window.localStorage.getItem('nas_device_view_mode')).toBe('mobile');

    setViewMode('desktop');
    expect(viewMode.value).toBe('desktop');
    expect(isMobile.value).toBe(false);
    expect(window.localStorage.getItem('nas_device_view_mode')).toBe('desktop');

    setViewMode('auto');
    expect(viewMode.value).toBe('auto');
  });
});
