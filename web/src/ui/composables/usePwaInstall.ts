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
  return (
    window.matchMedia('(display-mode: standalone)').matches ||
    (window.navigator as any).standalone === true ||
    (typeof document !== 'undefined' && document.referrer.includes('android-app://'))
  );
}

function checkIsIOS(): boolean {
  if (typeof window === 'undefined' || !window.navigator) return false;
  const userAgent = window.navigator.userAgent.toLowerCase();
  return /iphone|ipad|ipod/.test(userAgent);
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
    if (!deferredPrompt.value) return false;
    try {
      await deferredPrompt.value.prompt();
      const choice = await deferredPrompt.value.userChoice;
      if (choice.outcome === 'accepted') {
        deferredPrompt.value = null;
        hasNativePrompt.value = false;
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
