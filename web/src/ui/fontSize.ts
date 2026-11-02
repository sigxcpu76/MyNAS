import { computed, ref } from 'vue';

export type FontSizeId = 'compact' | 'default' | 'medium' | 'large' | 'xlarge';

export interface FontSizeOption {
  id: FontSizeId;
  label: string;
  percent: number;
  scale: number;
  description: string;
  tag?: string;
}

export const FONT_SIZE_OPTIONS: FontSizeOption[] = [
  {
    id: 'compact',
    label: 'Compact',
    percent: 90,
    scale: 0.9,
    description: 'Condensed typography for lower resolutions or high data density',
  },
  {
    id: 'default',
    label: 'Default',
    percent: 100,
    scale: 1.0,
    description: 'Standard balanced typography for 1080p displays',
  },
  {
    id: 'medium',
    label: 'Medium (+10%)',
    percent: 110,
    scale: 1.1,
    description: 'Comfortable reading for 1080p & 1440p displays',
    tag: 'Popular',
  },
  {
    id: 'large',
    label: 'Large (+20%)',
    percent: 120,
    scale: 1.2,
    description: 'Enhanced readability recommended for 1440p / 2K & HiDPI monitors',
    tag: 'HiDPI',
  },
  {
    id: 'xlarge',
    label: 'Extra Large (+30%)',
    percent: 130,
    scale: 1.3,
    description: 'Maximum visibility for 4K / UHD & high-resolution screens',
    tag: '4K / UHD',
  },
];

export const FONT_SIZE_STORAGE_KEY = 'nas_ui_font_size_preference_v1';

export const activeFontSizeId = ref<FontSizeId>('default');

export const currentFontOption = computed<FontSizeOption>(() => {
  return (
    FONT_SIZE_OPTIONS.find((opt) => opt.id === activeFontSizeId.value) ||
    FONT_SIZE_OPTIONS[1]
  );
});

export const effectiveFontScale = computed<number>(() => {
  return currentFontOption.value.scale;
});

export const activeFontLabel = computed<string>(() => {
  return `${currentFontOption.value.label} (${currentFontOption.value.percent}%)`;
});

export function detectHiDpiRecommendation(): FontSizeId {
  if (typeof window === 'undefined') return 'default';

  const dpr = window.devicePixelRatio || 1;
  const screenWidth = (window.screen && window.screen.width) || window.innerWidth || 1920;
  const screenHeight = (window.screen && window.screen.height) || window.innerHeight || 1080;

  if (dpr >= 2.0 || screenWidth >= 3840 || screenHeight >= 2160) {
    return 'xlarge';
  }
  if (dpr >= 1.5 || screenWidth >= 2560 || screenHeight >= 1440) {
    return 'large';
  }
  if (dpr >= 1.25) {
    return 'medium';
  }
  return 'default';
}

export function applyFontSize(id: FontSizeId): void {
  const option = FONT_SIZE_OPTIONS.find((opt) => opt.id === id) || FONT_SIZE_OPTIONS[1];
  activeFontSizeId.value = option.id;

  if (typeof document !== 'undefined') {
    const el = document.documentElement;
    el.setAttribute('data-font-size', option.id);
    el.style.setProperty('--fs-scale', String(option.scale));
    el.style.setProperty('--font-scale', String(option.scale));
  }

  try {
    if (typeof window !== 'undefined' && window.localStorage) {
      window.localStorage.setItem(FONT_SIZE_STORAGE_KEY, option.id);
    }
  } catch {
    // Ignore localStorage restrictions
  }

  // Trigger resize event so charts or terminals can re-measure if needed
  if (typeof window !== 'undefined' && typeof window.dispatchEvent === 'function') {
    setTimeout(() => {
      try {
        if (typeof window !== 'undefined' && typeof window.dispatchEvent === 'function') {
          window.dispatchEvent(new Event('resize'));
        }
      } catch {
        // ignore in test environments
      }
    }, 50);
  }
}

export function stepFontSize(direction: 'increase' | 'decrease'): void {
  const currentIndex = FONT_SIZE_OPTIONS.findIndex((opt) => opt.id === activeFontSizeId.value);
  const targetIndex =
    direction === 'increase'
      ? Math.min(currentIndex + 1, FONT_SIZE_OPTIONS.length - 1)
      : Math.max(currentIndex - 1, 0);

  if (targetIndex !== currentIndex) {
    applyFontSize(FONT_SIZE_OPTIONS[targetIndex].id);
  }
}

export function initFontSize(): void {
  let targetId: FontSizeId = 'default';

  try {
    if (typeof window !== 'undefined' && window.localStorage) {
      const stored = window.localStorage.getItem(FONT_SIZE_STORAGE_KEY);
      if (stored && FONT_SIZE_OPTIONS.some((opt) => opt.id === stored)) {
        targetId = stored as FontSizeId;
      }
    }
  } catch {
    // Ignore localStorage errors
  }

  applyFontSize(targetId);
}
