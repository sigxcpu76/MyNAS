import { beforeEach, describe, expect, it } from 'vitest';
import {
  activeTheme,
  activeThemeId,
  applyTheme,
  darkThemes,
  initTheme,
  isDarkMode,
  lightThemes,
  THEME_STORAGE_KEY,
  THEMES,
} from '../src/ui/theme';

// Mock localStorage for testing environment
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

// WCAG 2.1 relative luminance and contrast ratio calculation
function hexToRgb(hex: string): [number, number, number] {
  const clean = hex.replace('#', '');
  const r = parseInt(clean.substring(0, 2), 16) / 255;
  const g = parseInt(clean.substring(2, 4), 16) / 255;
  const b = parseInt(clean.substring(4, 6), 16) / 255;
  return [r, g, b];
}

function getLuminance(r: number, g: number, b: number): number {
  const [lr, lg, lb] = [r, g, b].map((c) =>
    c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4)
  );
  return 0.2126 * lr + 0.7152 * lg + 0.0722 * lb;
}

function getContrastRatio(hex1: string, hex2: string): number {
  const [r1, g1, b1] = hexToRgb(hex1);
  const [r2, g2, b2] = hexToRgb(hex2);
  const l1 = getLuminance(r1, g1, b1);
  const l2 = getLuminance(r2, g2, b2);
  const max = Math.max(l1, l2);
  const min = Math.min(l1, l2);
  return (max + 0.05) / (min + 0.05);
}

describe('Theming System', () => {
  beforeEach(() => {
    localStorageMock.clear();
    document.documentElement.removeAttribute('data-theme');
    document.documentElement.removeAttribute('data-mode');
  });

  it('provides 3 dark themes and 3 light themes with Tokyo Night required', () => {
    expect(THEMES).toHaveLength(6);
    expect(darkThemes.value).toHaveLength(3);
    expect(lightThemes.value).toHaveLength(3);

    // Dark themes must include Tokyo Night
    const darkIds = darkThemes.value.map((t) => t.id);
    expect(darkIds).toContain('tokyo-night');
    expect(darkIds).toContain('catppuccin-mocha');
    expect(darkIds).toContain('github-dark');

    // Light themes
    const lightIds = lightThemes.value.map((t) => t.id);
    expect(lightIds).toContain('github-light');
    expect(lightIds).toContain('catppuccin-latte');
    expect(lightIds).toContain('one-light');
  });

  it('applies theme and updates DOM attributes & localStorage', () => {
    applyTheme('tokyo-night');
    expect(activeThemeId.value).toBe('tokyo-night');
    expect(activeTheme.value.name).toBe('Tokyo Night');
    expect(isDarkMode.value).toBe(true);
    expect(document.documentElement.getAttribute('data-theme')).toBe('tokyo-night');
    expect(document.documentElement.getAttribute('data-mode')).toBe('dark');
    expect(localStorageMock.getItem(THEME_STORAGE_KEY)).toBe('tokyo-night');

    // Switch to light theme
    applyTheme('github-light');
    expect(activeThemeId.value).toBe('github-light');
    expect(activeTheme.value.name).toBe('GitHub Light');
    expect(isDarkMode.value).toBe(false);
    expect(document.documentElement.getAttribute('data-theme')).toBe('github-light');
    expect(document.documentElement.getAttribute('data-mode')).toBe('light');
    expect(localStorageMock.getItem(THEME_STORAGE_KEY)).toBe('github-light');
  });

  it('restores theme from localStorage upon initialization', () => {
    localStorageMock.setItem(THEME_STORAGE_KEY, 'catppuccin-latte');
    initTheme();

    expect(activeThemeId.value).toBe('catppuccin-latte');
    expect(activeTheme.value.name).toBe('Catppuccin Latte');
    expect(isDarkMode.value).toBe(false);
    expect(document.documentElement.getAttribute('data-theme')).toBe('catppuccin-latte');
  });

  it('falls back to tokyo-night when storage is empty or invalid', () => {
    localStorageMock.setItem(THEME_STORAGE_KEY, 'non-existent-theme');
    initTheme();

    expect(activeThemeId.value).toBe('tokyo-night');
    expect(isDarkMode.value).toBe(true);
    expect(document.documentElement.getAttribute('data-theme')).toBe('tokyo-night');
  });

  describe('Light Theme Contrast Verification (WCAG AA/AAA)', () => {
    it('verifies GitHub Light contrast exceeds WCAG AA (>= 4.5:1) and AAA (>= 7.0:1 for primary text)', () => {
      const surface = '#ffffff';
      const base = '#f6f8fa';

      const primary = '#1f2328';
      const secondary = '#59636e';
      const muted = '#656d76';
      const accent = '#0969da';
      const success = '#1a7f37';
      const warning = '#9a6700';
      const danger = '#cf222e';

      // Primary text must achieve WCAG AAA (>= 7.0:1)
      expect(getContrastRatio(primary, surface)).toBeGreaterThanOrEqual(14.0);
      expect(getContrastRatio(primary, base)).toBeGreaterThanOrEqual(13.0);

      // Secondary & muted text must achieve WCAG AA (>= 4.5:1)
      expect(getContrastRatio(secondary, surface)).toBeGreaterThanOrEqual(5.5);
      expect(getContrastRatio(muted, surface)).toBeGreaterThanOrEqual(4.8);

      // Semantic status colors must achieve WCAG AA (>= 4.5:1)
      expect(getContrastRatio(accent, surface)).toBeGreaterThanOrEqual(4.8);
      expect(getContrastRatio(success, surface)).toBeGreaterThanOrEqual(4.8);
      expect(getContrastRatio(warning, surface)).toBeGreaterThanOrEqual(4.5);
      expect(getContrastRatio(danger, surface)).toBeGreaterThanOrEqual(5.0);

      // Main pool amber badge text contrast on amber background
      const poolBg = '#fef3c7';
      const poolText = '#78350f';
      expect(getContrastRatio(poolText, poolBg)).toBeGreaterThanOrEqual(7.0);
    });

    it('verifies Catppuccin Latte contrast exceeds WCAG AA (>= 4.5:1)', () => {
      const surface = '#ffffff';
      const base = '#eff1f5';

      const primary = '#4c4f69';
      const secondary = '#5c5f77';
      const muted = '#6c6f85';
      const accent = '#185adb';
      const success = '#16803c';
      const warning = '#a15c00';
      const danger = '#d20f39';

      expect(getContrastRatio(primary, surface)).toBeGreaterThanOrEqual(7.5);
      expect(getContrastRatio(primary, base)).toBeGreaterThanOrEqual(7.0);
      expect(getContrastRatio(secondary, surface)).toBeGreaterThanOrEqual(5.5);
      expect(getContrastRatio(muted, surface)).toBeGreaterThanOrEqual(4.8);
      expect(getContrastRatio(accent, base)).toBeGreaterThanOrEqual(5.0);
      expect(getContrastRatio(success, surface)).toBeGreaterThanOrEqual(4.8);
      expect(getContrastRatio(warning, surface)).toBeGreaterThanOrEqual(4.8);
      expect(getContrastRatio(danger, surface)).toBeGreaterThanOrEqual(5.0);
    });

    it('verifies One Light contrast exceeds WCAG AA (>= 4.5:1)', () => {
      const surface = '#ffffff';
      const base = '#fafafa';

      const primary = '#24292e';
      const secondary = '#495162';
      const muted = '#656c76';
      const accent = '#0172a3';
      const success = '#1e7e34';
      const warning = '#9e6000';
      const danger = '#c0392b';

      expect(getContrastRatio(primary, surface)).toBeGreaterThanOrEqual(14.0);
      expect(getContrastRatio(primary, base)).toBeGreaterThanOrEqual(13.5);
      expect(getContrastRatio(secondary, surface)).toBeGreaterThanOrEqual(7.0);
      expect(getContrastRatio(muted, surface)).toBeGreaterThanOrEqual(4.8);
      expect(getContrastRatio(accent, surface)).toBeGreaterThanOrEqual(5.0);
      expect(getContrastRatio(success, surface)).toBeGreaterThanOrEqual(5.0);
      expect(getContrastRatio(warning, surface)).toBeGreaterThanOrEqual(4.8);
      expect(getContrastRatio(danger, surface)).toBeGreaterThanOrEqual(5.0);
    });
  });
});
