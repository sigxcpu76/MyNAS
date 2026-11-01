import { beforeEach, describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import FontSizeSelector from '../src/ui/components/FontSizeSelector.vue';
import {
  activeFontSizeId,
  applyFontSize,
  FONT_SIZE_OPTIONS,
  FONT_SIZE_STORAGE_KEY,
  initFontSize,
  stepFontSize,
} from '../src/ui/fontSize';

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

describe('FontSizeSelector & typography scale', () => {
  beforeEach(() => {
    localStorageMock.clear();
    applyFontSize('default');
  });

  it('initializes with default font size (100%)', () => {
    expect(activeFontSizeId.value).toBe('default');
    expect(document.documentElement.getAttribute('data-font-size')).toBe('default');
    expect(document.documentElement.style.getPropertyValue('--fs-scale')).toBe('1');
  });

  it('toggles dropdown popover on button click', async () => {
    const wrapper = mount(FontSizeSelector);
    expect(wrapper.find('.font-size-dropdown-menu').exists()).toBe(false);

    const btn = wrapper.find('.font-size-toggle-btn');
    await btn.trigger('click');
    expect(wrapper.find('.font-size-dropdown-menu').exists()).toBe(true);

    const options = wrapper.findAll('.font-size-option');
    expect(options).toHaveLength(FONT_SIZE_OPTIONS.length);

    await btn.trigger('click');
    expect(wrapper.find('.font-size-dropdown-menu').exists()).toBe(false);
  });

  it('selects Large (+20%) and updates html attributes and css scale without zoom', async () => {
    const wrapper = mount(FontSizeSelector);
    await wrapper.find('.font-size-toggle-btn').trigger('click');

    const options = wrapper.findAll('.font-size-option');
    const largeOption = options.find((opt) => opt.text().includes('Large'));
    expect(largeOption).toBeDefined();

    await largeOption!.trigger('click');

    expect(activeFontSizeId.value).toBe('large');
    expect(document.documentElement.getAttribute('data-font-size')).toBe('large');
    expect(document.documentElement.style.getPropertyValue('--fs-scale')).toBe('1.2');
    expect(document.documentElement.style.getPropertyValue('--font-scale')).toBe('1.2');
    // Ensure document root zoom is never set
    expect((document.documentElement.style as any).zoom).toBeFalsy();
    expect(localStorageMock.getItem(FONT_SIZE_STORAGE_KEY)).toBe('large');
  });

  it('steps font size up and down using stepper buttons', async () => {
    const wrapper = mount(FontSizeSelector);
    await wrapper.find('.font-size-toggle-btn').trigger('click');

    const stepperBtns = wrapper.findAll('.stepper-btn');
    expect(stepperBtns).toHaveLength(2);
    const smallerBtn = stepperBtns[0];
    const largerBtn = stepperBtns[1];

    // From default to medium
    await largerBtn.trigger('click');
    expect(activeFontSizeId.value).toBe('medium');
    expect(document.documentElement.style.getPropertyValue('--fs-scale')).toBe('1.1');

    // From medium to large
    await largerBtn.trigger('click');
    expect(activeFontSizeId.value).toBe('large');
    expect(document.documentElement.style.getPropertyValue('--fs-scale')).toBe('1.2');

    // Step back down to medium
    await smallerBtn.trigger('click');
    expect(activeFontSizeId.value).toBe('medium');
    expect(document.documentElement.style.getPropertyValue('--fs-scale')).toBe('1.1');
  });

  it('restores stored font size preference on init', () => {
    localStorageMock.setItem(FONT_SIZE_STORAGE_KEY, 'xlarge');
    initFontSize();

    expect(activeFontSizeId.value).toBe('xlarge');
    expect(document.documentElement.getAttribute('data-font-size')).toBe('xlarge');
    expect(document.documentElement.style.getPropertyValue('--fs-scale')).toBe('1.3');
  });
});
