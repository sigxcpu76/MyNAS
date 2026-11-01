import { computed, ref } from 'vue';

export type UiScaleMode = 'auto' | '100' | '110' | '120' | '130' | '140';

export interface UiScaleOption {
  id: UiScaleMode;
  label: string;
  factor: number;
  description: string;
}

export const UI_SCALE_OPTIONS: UiScaleOption[] = [
  {
    id: 'auto',
    label: 'Auto',
    factor: 0,
    description: 'Detects screen density & display resolution automatically',
  },
  {
    id: '100',
    label: '100%',
    factor: 1.0,
    description: 'Standard scale (Compact)',
  },
  {
    id: '110',
    label: '110%',
    factor: 1.1,
    description: 'Slightly enlarged text & controls (+10%)',
  },
  {
    id: '120',
    label: '120%',
    factor: 1.2,
    description: 'Comfortable reading for 1440p / 2K displays (+20%)',
  },
  {
    id: '130',
    label: '130%',
    factor: 1.3,
    description: 'Large scale for 4K / UHD & HiDPI monitors (+30%)',
  },
  {
    id: '140',
    label: '140%',
    factor: 1.4,
    description: 'Maximum readability & accessibility (+40%)',
  },
];

export const SCALE_STORAGE_KEY = 'nas_ui_scale_mode_v1';

export const activeScaleMode = ref<UiScaleMode>('auto');
export const detectedFactor = ref<number>(1.0);

export const effectiveScaleFactor = computed<number>(() => {
  if (activeScaleMode.value === 'auto') {
    return detectedFactor.value;
  }
  const found = UI_SCALE_OPTIONS.find((o) => o.id === activeScaleMode.value);
  return found ? found.factor : 1.0;
});

export const activeScaleLabel = computed<string>(() => {
  const percent = Math.round(effectiveScaleFactor.value * 100);
  if (activeScaleMode.value === 'auto') {
    return `Auto (${percent}%)`;
  }
  return `${percent}%`;
});

export function detectHiDpiFactor(): number {
  if (typeof window === 'undefined') return 1.0;

  const dpr = window.devicePixelRatio || 1.0;
  const screenWidth = (window.screen && window.screen.width) || window.innerWidth || 1920;
  const screenHeight = (window.screen && window.screen.height) || window.innerHeight || 1080;

  // Ultra-high DPI or 4K/UHD monitor (e.g. 3840x2160)
  if (dpr >= 2.0 || screenWidth >= 3840 || screenHeight >= 2160) {
    return 1.3;
  }
  // High DPI (150%) or 1440p / 2K monitor (e.g. 2560x1440)
  if (dpr >= 1.5 || screenWidth >= 2560 || screenHeight >= 1440) {
    return 1.2;
  }
  // Moderate HiDPI (125% fractional scaling)
  if (dpr >= 1.25) {
    return 1.15;
  }

  return 1.0;
}

export function applyScale(mode: UiScaleMode): void {
  activeScaleMode.value = mode;
  detectedFactor.value = detectHiDpiFactor();
  const factor = effectiveScaleFactor.value;

  if (typeof document !== 'undefined') {
    const el = document.documentElement;
    el.setAttribute('data-ui-scale', mode);
    el.setAttribute('data-ui-scale-factor', String(factor));
    el.style.setProperty('--ui-scale', String(factor));
    (el.style as any).zoom = factor === 1.0 ? '' : String(factor);
  }

  try {
    if (typeof window !== 'undefined' && window.localStorage) {
      window.localStorage.setItem(SCALE_STORAGE_KEY, mode);
    }
  } catch {
    // Ignore localStorage restrictions
  }

  // Trigger window resize event so charts, xterm terminals, and noVNC adapt immediately
  if (typeof window !== 'undefined') {
    setTimeout(() => {
      window.dispatchEvent(new Event('resize'));
    }, 50);
  }
}

export function initScale(): void {
  let initialMode: UiScaleMode = 'auto';
  try {
    if (typeof window !== 'undefined' && window.localStorage) {
      const stored = window.localStorage.getItem(SCALE_STORAGE_KEY);
      if (stored && UI_SCALE_OPTIONS.some((o) => o.id === stored)) {
        initialMode = stored as UiScaleMode;
      }
    }
  } catch {
    // Ignore localStorage errors
  }

  applyScale(initialMode);

  if (typeof window !== 'undefined') {
    window.addEventListener('resize', () => {
      if (activeScaleMode.value === 'auto') {
        const newFactor = detectHiDpiFactor();
        if (newFactor !== detectedFactor.value) {
          applyScale('auto');
        }
      }
    });

    try {
      if (window.matchMedia) {
        const mq = window.matchMedia(`(resolution: ${window.devicePixelRatio}dppx)`);
        mq.addEventListener?.('change', () => {
          if (activeScaleMode.value === 'auto') {
            applyScale('auto');
          }
        });
      }
    } catch {
      // Ignore matchMedia listener issues
    }
  }
}
