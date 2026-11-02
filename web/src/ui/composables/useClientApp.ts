import { computed } from 'vue';
import { usePwaInstall } from './usePwaInstall';

export interface ClientAppInfo {
  isNative: boolean;
  isPwa: boolean;
  label: string;
  version: string;
  build?: number;
}

export function useClientApp() {
  const { isStandalone } = usePwaInstall();

  const clientApp = computed<ClientAppInfo>(() => {
    if (typeof window === 'undefined') {
      return { isNative: false, isPwa: false, label: 'Web Client', version: '1.0.0' };
    }

    // 1. Android JS Interface NanAndroidNative
    if ((window as any).NanAndroidNative?.getVersionName) {
      const v = (window as any).NanAndroidNative.getVersionName();
      const b = (window as any).NanAndroidNative.getVersionCode();
      return { isNative: true, isPwa: false, label: `NaN Android App v${v} (b${b})`, version: v, build: Number(b) };
    }

    // 2. window.nanNativeApp
    if ((window as any).nanNativeApp?.version) {
      const v = (window as any).nanNativeApp.version;
      const b = (window as any).nanNativeApp.build;
      return { isNative: true, isPwa: false, label: `NaN Android App v${v}${b ? ` (b${b})` : ''}`, version: v, build: b ? Number(b) : undefined };
    }

    // 3. LocalStorage
    const lsVer = localStorage.getItem('nan_apk_version');
    const lsBuild = localStorage.getItem('nan_apk_build');
    if (lsVer) {
      return { isNative: true, isPwa: false, label: `NaN Android App v${lsVer}${lsBuild ? ` (b${lsBuild})` : ''}`, version: lsVer, build: lsBuild ? Number(lsBuild) : undefined };
    }

    // 4. User Agent matching (covers previous releases such as v1.5.0)
    const ua = navigator.userAgent || '';
    const uaMatch = ua.match(/NaN-NAS-Android\/([0-9.]+)(?:\s+\(Build\s+(\d+)\))?/i);
    if (uaMatch) {
      const v = uaMatch[1];
      const b = uaMatch[2];
      return { isNative: true, isPwa: false, label: `NaN Android App v${v}${b ? ` (b${b})` : ''}`, version: v, build: b ? Number(b) : undefined };
    }

    // 5. Standalone PWA vs Browser
    if (isStandalone.value) {
      return { isNative: false, isPwa: true, label: 'NaN PWA (Standalone Web)', version: 'PWA' };
    }

    return { isNative: false, isPwa: false, label: 'Web Browser Client', version: 'Web' };
  });

  return {
    clientApp,
  };
}
