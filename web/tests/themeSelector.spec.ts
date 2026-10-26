import { beforeEach, describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import ThemeSelector from '../src/ui/components/ThemeSelector.vue';
import { activeThemeId, applyTheme } from '../src/ui/theme';

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

describe('ThemeSelector Component', () => {
  beforeEach(() => {
    localStorageMock.clear();
    applyTheme('tokyo-night');
  });

  it('renders current theme label and dark mode icon', () => {
    const wrapper = mount(ThemeSelector);
    expect(wrapper.find('.theme-active-name').text()).toBe('Tokyo Night');
    expect(wrapper.find('.theme-dropdown-menu').exists()).toBe(false);
  });

  it('toggles dropdown popover on button click', async () => {
    const wrapper = mount(ThemeSelector);
    const btn = wrapper.find('.theme-toggle-btn');

    await btn.trigger('click');
    expect(wrapper.find('.theme-dropdown-menu').exists()).toBe(true);

    // Verify sections for Dark and Light themes
    const options = wrapper.findAll('.theme-option');
    expect(options).toHaveLength(6);

    await btn.trigger('click');
    expect(wrapper.find('.theme-dropdown-menu').exists()).toBe(false);
  });

  it('switches to a light theme when clicked', async () => {
    const wrapper = mount(ThemeSelector);
    await wrapper.find('.theme-toggle-btn').trigger('click');

    const options = wrapper.findAll('.theme-option');
    const ghLight = options.find((opt) => opt.text().includes('GitHub Light'));
    expect(ghLight).toBeDefined();

    await ghLight!.trigger('click');

    expect(activeThemeId.value).toBe('github-light');
    expect(document.documentElement.getAttribute('data-theme')).toBe('github-light');
    expect(document.documentElement.getAttribute('data-mode')).toBe('light');
    expect(wrapper.find('.theme-dropdown-menu').exists()).toBe(false);
    expect(wrapper.find('.theme-active-name').text()).toBe('GitHub Light');
  });

  it('switches to Tokyo Night theme', async () => {
    applyTheme('github-light');
    const wrapper = mount(ThemeSelector);
    await wrapper.find('.theme-toggle-btn').trigger('click');

    const tokyoOption = wrapper.findAll('.theme-option').find((opt) => opt.text().includes('Tokyo Night'));
    expect(tokyoOption).toBeDefined();

    await tokyoOption!.trigger('click');

    expect(activeThemeId.value).toBe('tokyo-night');
    expect(document.documentElement.getAttribute('data-theme')).toBe('tokyo-night');
    expect(document.documentElement.getAttribute('data-mode')).toBe('dark');
  });
});
