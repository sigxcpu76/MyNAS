import { watch } from 'vue';
import { nasState, type NavTab } from './store';

export const TAB_ROUTES: Record<NavTab, string> = {
  overview: '/overview',
  prerequisites: '/prerequisites',
  'storage-disks': '/storage/disks',
  'storage-pools': '/storage/pools',
  'storage-datasets': '/storage/datasets',
  'storage-volumes': '/storage/volumes',
  zfs: '/zfs',
  kvm: '/kvm',
  docker: '/apps',
  lxc: '/lxc',
  shares: '/shares',
  'shares-cifs': '/shares/cifs',
  'shares-webdav': '/shares/webdav',
  identity: '/users',
  'vnc-console': '/vnc-console',
};

export const ROUTE_ALIASES: Record<string, NavTab> = {
  '': 'overview',
  '/': 'overview',
  '/overview': 'overview',
  '/prerequisites': 'prerequisites',
  '/prereqs': 'prerequisites',
  '/storage': 'storage-disks',
  '/storage/disks': 'storage-disks',
  '/disks': 'storage-disks',
  '/storage/pools': 'storage-pools',
  '/pools': 'storage-pools',
  '/storage/datasets': 'storage-datasets',
  '/datasets': 'storage-datasets',
  '/storage/volumes': 'storage-volumes',
  '/volumes': 'storage-volumes',
  '/zvols': 'storage-volumes',
  '/zfs': 'zfs',
  '/kvm': 'kvm',
  '/vms': 'kvm',
  '/vm': 'kvm',
  '/vnc-console': 'vnc-console',
  '/console/vnc': 'vnc-console',
  '/kvm/console': 'vnc-console',
  '/apps': 'docker',
  '/docker': 'docker',
  '/lxc': 'lxc',
  '/containers': 'lxc',
  '/shares': 'shares',
  '/cifs': 'shares',
  '/shares/cifs': 'shares-cifs',
  '/shares/smb': 'shares-cifs',
  '/smb': 'shares-cifs',
  '/shares/webdav': 'shares-webdav',
  '/webdav': 'shares-webdav',
  '/dav': 'shares-webdav',
  '/users': 'identity',
  '/identity': 'identity',
  '/groups': 'identity',
};

export function getPathFromTab(tab: NavTab): string {
  return TAB_ROUTES[tab] || '/overview';
}

export function getTabFromPath(path: string): NavTab {
  const clean = path.trim().toLowerCase().replace(/\/+$/, '') || '/';
  if (ROUTE_ALIASES[clean]) {
    return ROUTE_ALIASES[clean];
  }
  for (const [route, tab] of Object.entries(ROUTE_ALIASES)) {
    if (route !== '/' && route !== '' && clean.startsWith(route)) {
      return tab;
    }
  }
  return 'overview';
}

let isNavigatingFromPopState = false;

export function navigateTo(tab: NavTab, replace = false): void {
  const currentTab = nasState.activeTab;
  const targetPath = getPathFromTab(tab);

  if (currentTab === tab && typeof window !== 'undefined' && window.location.pathname === targetPath && !replace) {
    return;
  }

  nasState.activeTab = tab;

  if (typeof window !== 'undefined' && window.history) {
    if (replace) {
      window.history.replaceState({ tab }, '', targetPath);
    } else {
      window.history.pushState({ tab }, '', targetPath);
    }
  }
}

export function initRouter(): void {
  if (typeof window === 'undefined') return;

  // 1. Resolve initial route from browser URL
  const currentPath = window.location.pathname;
  const initialTab = getTabFromPath(currentPath);
  nasState.activeTab = initialTab;

  // Set initial canonical path if at root or non-canonical
  const search = window.location.search || '';
  const hash = window.location.hash || '';
  const canonicalPath = (currentPath === '/' ? '/' : getPathFromTab(initialTab)) + search + hash;
  if (window.history && window.history.replaceState) {
    window.history.replaceState({ tab: initialTab }, '', canonicalPath);
  }

  // 2. Listen to browser Back / Forward (popstate)
  window.addEventListener('popstate', (e: PopStateEvent) => {
    const targetTab = e.state?.tab || getTabFromPath(window.location.pathname);
    if (targetTab && nasState.activeTab !== targetTab) {
      isNavigatingFromPopState = true;
      nasState.activeTab = targetTab;
    }
  });

  // 3. Keep URL in sync if state changed via direct assignment in any view
  watch(
    () => nasState.activeTab,
    (newTab) => {
      if (isNavigatingFromPopState) {
        isNavigatingFromPopState = false;
        return;
      }
      const targetPath = getPathFromTab(newTab);
      if (window.location.pathname !== targetPath) {
        window.history.pushState({ tab: newTab }, '', targetPath);
      }
    }
  );
}
