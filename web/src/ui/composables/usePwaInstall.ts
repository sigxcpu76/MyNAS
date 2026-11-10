import { ref } from 'vue';

interface BeforeInstallPromptEvent extends Event {
  readonly platforms: string[];
  readonly userChoice: Promise<{
    outcome: 'accepted' | 'dismissed';
    platform: string;
  }>;
  prompt(): Promise<void>;
}

const deferredPrompt = ref<BeforeInstallPromptEvent | null>(null);
const hasNativePrompt = ref(false);
const isStandalone = ref(false);
const isIOS = ref(false);
const showInstallSheet = ref(false);

let initialized = false;

function checkStandalone(): boolean {
  if (typeof window === 'undefined') return false;
  return Boolean(
    (typeof window.matchMedia === 'function' && window.matchMedia('(display-mode: standalone)').matches) ||
    (window.navigator as any)?.standalone === true ||
    (typeof document !== 'undefined' && document.referrer?.includes('android-app://'))
  );
}

function checkIsIOS(): boolean {
  if (typeof window === 'undefined' || !window.navigator) return false;
  const userAgent = window.navigator.userAgent.toLowerCase();
  const isIPadOS =
    window.navigator.platform === 'MacIntel' && window.navigator.maxTouchPoints > 1;
  return /iphone|ipad|ipod/.test(userAgent) || isIPadOS;
}

export function usePwaInstall() {
  if (!initialized && typeof window !== 'undefined') {
    initialized = true;
    isStandalone.value = checkStandalone();
    isIOS.value = checkIsIOS();

    window.addEventListener('beforeinstallprompt', (e: Event) => {
      e.preventDefault();
      deferredPrompt.value = e as BeforeInstallPromptEvent;
      hasNativePrompt.value = true;
    });

    window.addEventListener('appinstalled', () => {
      deferredPrompt.value = null;
      hasNativePrompt.value = false;
      isStandalone.value = true;
      showInstallSheet.value = false;
    });
  }

  async function triggerNativePrompt(): Promise<boolean> {
    const promptEvent = deferredPrompt.value;
    if (!promptEvent) return false;
    deferredPrompt.value = null;
    hasNativePrompt.value = false;
    try {
      await promptEvent.prompt();
      const choice = await promptEvent.userChoice;
      if (choice.outcome === 'accepted') {
        showInstallSheet.value = false;
        return true;
      }
    } catch (err) {
      console.warn('[PWA] Prompt failed:', err);
    }
    return false;
  }

  async function promptInstall(): Promise<void> {
    if (isStandalone.value) return;

    if (hasNativePrompt.value && deferredPrompt.value) {
      const ok = await triggerNativePrompt();
      if (!ok) {
        showInstallSheet.value = true;
      }
    } else {
      showInstallSheet.value = true;
    }
  }

  return {
    hasNativePrompt,
    isStandalone,
    isIOS,
    showInstallSheet,
    promptInstall,
    triggerNativePrompt,
  };
}
