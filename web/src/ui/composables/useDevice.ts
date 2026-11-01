import { ref } from 'vue';

export type DeviceViewMode = 'auto' | 'mobile' | 'desktop';

const STORAGE_KEY = 'nas_device_view_mode';

function getStoredMode(): DeviceViewMode {
  if (typeof window === 'undefined' || !window.localStorage) return 'auto';
  const val = window.localStorage.getItem(STORAGE_KEY);
  if (val === 'mobile' || val === 'desktop' || val === 'auto') return val;
  return 'auto';
}

const viewMode = ref<DeviceViewMode>(getStoredMode());
const isMobile = ref<boolean>(false);
let initialized = false;

function updateDeviceState() {
  if (typeof window === 'undefined') return;

  const widthMatches = window.innerWidth <= 768 || (window.matchMedia && window.matchMedia('(max-width: 768px)').matches);

  if (viewMode.value === 'mobile') {
    isMobile.value = true;
  } else if (viewMode.value === 'desktop') {
    isMobile.value = false;
  } else {
    isMobile.value = widthMatches;
  }
}

export function useDevice() {
  if (!initialized && typeof window !== 'undefined') {
    initialized = true;
    updateDeviceState();
    window.addEventListener('resize', updateDeviceState);
    if (window.matchMedia) {
      try {
        window.matchMedia('(max-width: 768px)').addEventListener('change', updateDeviceState);
      } catch {
        // Fallback for older browsers
      }
    }
  }

  function setViewMode(mode: DeviceViewMode) {
    viewMode.value = mode;
    if (typeof window !== 'undefined' && window.localStorage) {
      window.localStorage.setItem(STORAGE_KEY, mode);
    }
    updateDeviceState();
  }

  return {
    isMobile,
    viewMode,
    setViewMode,
  };
}
