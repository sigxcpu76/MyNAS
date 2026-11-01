import { beforeEach, describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import {
  activeScaleLabel,
  activeScaleMode,
  applyScale,
  detectedFactor,
  detectHiDpiFactor,
  effectiveScaleFactor,
  initScale,
  SCALE_STORAGE_KEY,
  UI_SCALE_OPTIONS,
} from '../src/ui/scale';
import UiScaleSelector from '../src/ui/components/UiScaleSelector.vue';

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

describe('UI Scale & HiDPI Detection System', () => {
  beforeEach(() => {
    localStorageMock.clear();
    document.documentElement.removeAttribute('data-ui-scale');
    document.documentElement.removeAttribute('data-ui-scale-factor');
    (document.documentElement.style as any).zoom = '';
    applyScale('100');
  });

  it('provides scale options including Auto and various zoom factors', () => {
    expect(UI_SCALE_OPTIONS.length).toBeGreaterThanOrEqual(5);
    const ids = UI_SCALE_OPTIONS.map((o) => o.id);
    expect(ids).toContain('auto');
    expect(ids).toContain('100');
    expect(ids).toContain('110');
    expect(ids).toContain('120');
    expect(ids).toContain('130');
    expect(ids).toContain('140');
  });

  it('applies exact manual scale mode and sets DOM attributes', () => {
    applyScale('120');
    expect(activeScaleMode.value).toBe('120');
    expect(effectiveScaleFactor.value).toBe(1.2);
    expect(activeScaleLabel.value).toBe('120%');
    expect(document.documentElement.getAttribute('data-ui-scale')).toBe('120');
    expect(document.documentElement.getAttribute('data-ui-scale-factor')).toBe('1.2');
    expect((document.documentElement.style as any).zoom).toBe('1.2');
    expect(localStorageMock.getItem(SCALE_STORAGE_KEY)).toBe('120');
  });

  it('removes zoom style property when at standard 100% scale', () => {
    applyScale('100');
    expect(activeScaleMode.value).toBe('100');
    expect(effectiveScaleFactor.value).toBe(1.0);
    expect((document.documentElement.style as any).zoom).toBe('');
  });

  it('detects HiDPI scale factor based on devicePixelRatio', () => {
    const originalDpr = window.devicePixelRatio;

    try {
      Object.defineProperty(window, 'devicePixelRatio', { value: 2.0, configurable: true });
      expect(detectHiDpiFactor()).toBe(1.3);

      Object.defineProperty(window, 'devicePixelRatio', { value: 1.5, configurable: true });
      expect(detectHiDpiFactor()).toBe(1.2);

      Object.defineProperty(window, 'devicePixelRatio', { value: 1.25, configurable: true });
      expect(detectHiDpiFactor()).toBe(1.15);

      Object.defineProperty(window, 'devicePixelRatio', { value: 1.0, configurable: true });
      expect(detectHiDpiFactor()).toBe(1.0);
    } finally {
      Object.defineProperty(window, 'devicePixelRatio', { value: originalDpr, configurable: true });
    }
  });

  it('restores stored scale setting on initScale', () => {
    localStorageMock.setItem(SCALE_STORAGE_KEY, '130');
    initScale();
    expect(activeScaleMode.value).toBe('130');
    expect(effectiveScaleFactor.value).toBe(1.3);
    expect((document.documentElement.style as any).zoom).toBe('1.3');
  });

  it('renders UiScaleSelector component and toggles dropdown', async () => {
    applyScale('120');
    const wrapper = mount(UiScaleSelector);

    expect(wrapper.find('.scale-toggle-btn').exists()).toBe(true);
    expect(wrapper.find('.scale-dropdown-menu').exists()).toBe(false);

    await wrapper.find('.scale-toggle-btn').trigger('click');
    expect(wrapper.find('.scale-dropdown-menu').exists()).toBe(true);

    const options = wrapper.findAll('.scale-option');
    expect(options.length).toBe(UI_SCALE_OPTIONS.length);

    // Switch to 130%
    const opt130 = options.find((opt) => opt.text().includes('130%'));
    expect(opt130).toBeDefined();
    await opt130!.trigger('click');

    expect(activeScaleMode.value).toBe('130');
    expect(effectiveScaleFactor.value).toBe(1.3);
    expect(wrapper.find('.scale-dropdown-menu').exists()).toBe(false);
  });
});
