import { watch } from 'vue';
import { nasState, selectResource, type NavTab } from './store';

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
  cron: '/cron',
  identity: '/users',
  terminal: '/terminal',
  'vnc-console': '/vnc-console',
};

export const ROUTE_ALIASES: Record<string, NavTab> = {
  '': 'overview',
  '/': 'overview',
  '/overview': 'overview',
  '/prerequisites': 'prerequisites',
  '/prereqs': 'prerequisites',
  '/terminal': 'terminal',
  '/shell': 'terminal',
  '/host/terminal': 'terminal',
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
  '/cron': 'cron',
  '/tasks': 'cron',
  '/scheduled': 'cron',
};

export function getPathFromTab(tab: NavTab): string {
  return TAB_ROUTES[tab] || '/overview';
}

export interface ResolvedRoute {
  tab: NavTab;
  resourceId?: string;
  subTab?: string;
}

export function resolveRouteFromPath(path: string): ResolvedRoute {
  const clean = path.trim().replace(/\/+$/, '') || '/';
  const cleanLower = clean.toLowerCase();

  // 1. Storage Pools: /storage/pools/:pool(/:subtab)? or /pools/:pool(/:subtab)?
  const poolMatch = clean.match(/^\/(?:storage\/pools|pools)\/([^/]+)(?:\/([^/]+))?$/i);
  if (poolMatch) {
    const pool = decodeURIComponent(poolMatch[1]);
    const subTab = poolMatch[2]?.toLowerCase();
    return {
      tab: 'storage-pools',
      resourceId: `pool:${pool}`,
      subTab,
    };
  }

  // 2. KVM: /kvm/:vm(/:subtab)? or /vms/:vm(/:subtab)? (exempting /kvm/console alias)
  if (!cleanLower.startsWith('/kvm/console') && !cleanLower.startsWith('/console/vnc')) {
    const kvmMatch = clean.match(/^\/(?:kvm|vms|vm)\/([^/]+)(?:\/([^/]+))?$/i);
    if (kvmMatch) {
      const vm = decodeURIComponent(kvmMatch[1]);
      const subTab = kvmMatch[2]?.toLowerCase();
      return {
        tab: 'kvm',
        resourceId: `vm:${vm}`,
        subTab,
      };
    }
  }

  // 3. Docker / Apps: /apps/:app(/:subtab)? or /docker/:app(/:subtab)?
  const dockerMatch = clean.match(/^\/(?:apps|docker)\/([^/]+)(?:\/([^/]+))?$/i);
  if (dockerMatch) {
    const app = decodeURIComponent(dockerMatch[1]);
    const subTab = dockerMatch[2]?.toLowerCase();
    return {
      tab: 'docker',
      resourceId: `docker:${app}`,
      subTab,
    };
  }

  // 4. LXC: /lxc/:ct(/:subtab)? or /containers/:ct(/:subtab)?
  const lxcMatch = clean.match(/^\/(?:lxc|containers)\/([^/]+)(?:\/([^/]+))?$/i);
  if (lxcMatch) {
    const ct = decodeURIComponent(lxcMatch[1]);
    const subTab = lxcMatch[2]?.toLowerCase();
    return {
      tab: 'lxc',
      resourceId: `lxc:${ct}`,
      subTab,
    };
  }

  const tab = getTabFromPath(path);
  return { tab };
}

export function getCurrentPathFromState(): string {
  const tab = nasState.activeTab;
  const res = nasState.selectedResourceId;

  if ((tab === 'storage-pools' || tab === 'zfs') && res && res.startsWith('pool:')) {
    const pool = res.replace(/^pool:/, '');
    const sub = nasState.poolSubTab && nasState.poolSubTab !== 'summary' ? `/${nasState.poolSubTab}` : '';
    return `/storage/pools/${encodeURIComponent(pool)}${sub}`;
  }

  if (tab === 'kvm' && res && res.startsWith('vm:')) {
    const vm = res.replace(/^vm:/, '').replace(/^virtualMachines\//, '');
    const sub = nasState.vmSubTab && nasState.vmSubTab !== 'summary' ? `/${nasState.vmSubTab}` : '';
    return `/kvm/${encodeURIComponent(vm)}${sub}`;
  }

  if (tab === 'docker' && res && res.startsWith('docker:')) {
    const app = res.replace(/^docker:/, '').replace(/^dockerApps\//, '');
    const sub = nasState.dockerSubTab && nasState.dockerSubTab !== 'summary' ? `/${nasState.dockerSubTab}` : '';
    return `/apps/${encodeURIComponent(app)}${sub}`;
  }

  if (tab === 'lxc' && res && res.startsWith('lxc:')) {
    const ct = res.replace(/^lxc:/, '').replace(/^lxcContainers\//, '');
    const sub = nasState.lxcSubTab && nasState.lxcSubTab !== 'summary' ? `/${nasState.lxcSubTab}` : '';
    return `/lxc/${encodeURIComponent(ct)}${sub}`;
  }

  return getPathFromTab(tab);
}

export function applyResolvedRoute(route: ResolvedRoute): void {
  nasState.activeTab = route.tab;
  if (route.resourceId) {
    nasState.selectedResourceId = route.resourceId;
    if (route.subTab) {
      if (route.tab === 'storage-pools' || route.tab === 'zfs') {
        nasState.poolSubTab = route.subTab as any;
      } else if (route.tab === 'kvm') {
        nasState.vmSubTab = route.subTab as any;
      } else if (route.tab === 'docker') {
        nasState.dockerSubTab = route.subTab as any;
      } else if (route.tab === 'lxc') {
        nasState.lxcSubTab = route.subTab as any;
      }
    }
  } else {
    // When navigating to a top-level view without a child resource, reset selectedResourceId to match the destination tab
    if (route.tab === 'kvm') {
      nasState.selectedResourceId = 'node:vms';
    } else if (route.tab === 'docker') {
      nasState.selectedResourceId = 'node:docker';
    } else if (route.tab === 'lxc') {
      nasState.selectedResourceId = 'node:lxc';
    } else if (route.tab === 'storage-pools' || route.tab === 'zfs') {
      nasState.selectedResourceId = 'node:pools';
    } else if (route.tab === 'storage-datasets') {
      nasState.selectedResourceId = 'node:datasets';
    } else if (route.tab === 'storage-volumes') {
      nasState.selectedResourceId = 'node:volumes';
    } else if (route.tab === 'storage-disks') {
      nasState.selectedResourceId = 'node:disks';
    } else if (route.tab === 'shares' || route.tab === 'shares-cifs' || route.tab === 'shares-webdav') {
      nasState.selectedResourceId = 'node:shares';
    } else if (route.tab === 'cron') {
      nasState.selectedResourceId = 'node:cron';
    } else if (route.tab === 'identity') {
      nasState.selectedResourceId = 'node:identity';
    } else {
      nasState.selectedResourceId = 'node:local';
    }
  }
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

export function navigateTo(tab: NavTab, replace = false, targetResourceId?: string): void {
  if (targetResourceId !== undefined) {
    selectResource(targetResourceId);
  } else {
    // If navigating to top-level tab without a specific target resource,
    // only reset to list view if current selection does NOT match the destination tab.
    // (If the caller already selected a child resource for this tab, preserve it.)
    if (tab === 'kvm' && !nasState.selectedResourceId?.startsWith('vm:')) {
      nasState.selectedResourceId = 'node:vms';
    } else if (tab === 'docker' && !nasState.selectedResourceId?.startsWith('docker:')) {
      nasState.selectedResourceId = 'node:docker';
    } else if (tab === 'lxc' && !nasState.selectedResourceId?.startsWith('lxc:')) {
      nasState.selectedResourceId = 'node:lxc';
    } else if ((tab === 'storage-pools' || tab === 'zfs') && !nasState.selectedResourceId?.startsWith('pool:')) {
      nasState.selectedResourceId = 'node:pools';
    } else if (tab === 'storage-datasets') {
      nasState.selectedResourceId = 'node:datasets';
    } else if (tab === 'storage-volumes') {
      nasState.selectedResourceId = 'node:volumes';
    } else if (tab === 'storage-disks') {
      nasState.selectedResourceId = 'node:disks';
    } else if (tab === 'shares' || tab === 'shares-cifs' || tab === 'shares-webdav') {
      nasState.selectedResourceId = 'node:shares';
    } else if (tab === 'cron') {
      nasState.selectedResourceId = 'node:cron';
    } else if (tab === 'identity') {
      nasState.selectedResourceId = 'node:identity';
    } else if (tab === 'overview') {
      nasState.selectedResourceId = 'node:local';
    }
  }

  nasState.activeTab = tab;
  const targetPath = getCurrentPathFromState();

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
  const initialRoute = resolveRouteFromPath(currentPath);
  applyResolvedRoute(initialRoute);

  // Set initial canonical path if at root or non-canonical
  const search = window.location.search || '';
  const hash = window.location.hash || '';
  const canonicalPath = (currentPath === '/' ? '/' : getCurrentPathFromState()) + search + hash;
  if (window.history && window.history.replaceState) {
    window.history.replaceState(
      { tab: initialRoute.tab, resourceId: initialRoute.resourceId, subTab: initialRoute.subTab },
      '',
      canonicalPath
    );
  }

  // 2. Listen to browser Back / Forward (popstate)
  window.addEventListener('popstate', () => {
    isNavigatingFromPopState = true;
    const route = resolveRouteFromPath(window.location.pathname);
    applyResolvedRoute(route);
    Promise.resolve().then(() => {
      isNavigatingFromPopState = false;
    });
  });

  // 3. Keep URL in sync if state changed via direct assignment or selectResource
  watch(
    [
      () => nasState.activeTab,
      () => nasState.selectedResourceId,
      () => nasState.vmSubTab,
      () => nasState.lxcSubTab,
      () => nasState.dockerSubTab,
      () => nasState.poolSubTab,
    ],
    () => {
      if (isNavigatingFromPopState) {
        isNavigatingFromPopState = false;
        return;
      }
      const targetPath = getCurrentPathFromState();
      if (window.location.pathname !== targetPath) {
        window.history.pushState(
          {
            tab: nasState.activeTab,
            resourceId: nasState.selectedResourceId,
          },
          '',
          targetPath
        );
      }
    }
  );
}
