import type { DockerApp, LxcContainer, CatalogApp, LxcCatalogItem, VirtualMachine } from '../../domain/models';
import { nasState } from '../store';
import { getAppSlug, normalizeAppIconUrl } from '../dashboard/webAppsHelper';

export const LXC_DISTRO_ICONS: Record<string, string> = {
  debian: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/debian.png',
  ubuntu: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/ubuntu.png',
  alpine: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/alpine-linux.png',
  arch: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/arch-linux.png',
  archlinux: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/arch-linux.png',
  fedora: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/fedora.png',
  rocky: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/rocky-linux.png',
  rockylinux: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/rocky-linux.png',
  almalinux: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/almalinux.png',
};

export interface DockerContainerIconSource {
  image?: string;
  name?: string;
  serviceName?: string;
  containerName?: string;
  iconUrl?: string;
}

/**
 * Returns the best available icon URL for a Docker container (from an app stack or raw container).
 */
export function getDockerContainerIcon(
  container: DockerContainerIconSource,
  catalogApps?: CatalogApp[]
): string {
  const serviceName = (container.serviceName || '').toLowerCase().trim();
  const rawName = (container.name || container.containerName || '').toLowerCase().trim();
  const containerName = rawName.replace(/^dockercontainers\//, '');
  const nameParts = containerName.split(/[-_.]/).filter(Boolean);

  const normalizedExplicit = normalizeAppIconUrl(container.iconUrl, serviceName || containerName);
  if (normalizedExplicit) {
    return normalizedExplicit;
  }

  const catalog = catalogApps || nasState.catalogApps || [];

  // 1. Try matching against catalog (by service name, full container name, or individual name parts)
  const matched = catalog.find((c) => {
    const id = c.appId.toLowerCase();
    const title = c.title.toLowerCase();
    return (
      (serviceName && (id === serviceName || title === serviceName)) ||
      (containerName && (id === containerName || title === containerName)) ||
      nameParts.includes(id) ||
      nameParts.includes(title)
    );
  });

  const matchedIcon = normalizeAppIconUrl(matched?.iconUrl, matched?.appId);
  if (matchedIcon) {
    return matchedIcon;
  }

  // 2. Extract identity from container image
  const rawImage = container.image || '';
  if (rawImage) {
    const imageNoDigest = rawImage.split('@')[0];
    const imageNoTag = imageNoDigest.replace(/:[^/:]+$/, '');
    const parts = imageNoTag.split('/');
    const lastPart = (parts[parts.length - 1] || '').toLowerCase();
    const secondLastPart = parts.length > 1 ? (parts[parts.length - 2] || '').toLowerCase() : '';

    // Match image against catalog
    const matchedImageCatalog = catalog.find((c) => {
      const id = c.appId.toLowerCase();
      const title = c.title.toLowerCase();
      return (
        id === lastPart ||
        title === lastPart ||
        (secondLastPart && !secondLastPart.includes('.') && (id === secondLastPart || title === secondLastPart))
      );
    });
    const matchedImgIcon = normalizeAppIconUrl(matchedImageCatalog?.iconUrl, matchedImageCatalog?.appId);
    if (matchedImgIcon) {
      return matchedImgIcon;
    }

    // Identify candidate slug
    const genericNames = ['server', 'app', 'core', 'web', 'backend', 'frontend', 'daemon', 'client', 'image', 'container', 'service', 'base'];
    let candidate = lastPart;
    if (genericNames.includes(candidate) && secondLastPart && !secondLastPart.includes('.')) {
      candidate = secondLastPart;
    }

    if (!genericNames.includes(candidate)) {
      candidate = candidate
        .replace(/^docker-/, '')
        .replace(/-ce$/, '')
        .replace(/-server$/, '')
        .replace(/^pms-docker$/, 'plex');

      const slug = getAppSlug(candidate);
      if (slug && !genericNames.includes(slug)) {
        return `https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/${slug}.png`;
      }
    }
  }

  // 3. Fallback to serviceName or containerName slug
  const fallbackCandidate = serviceName || (nameParts.length > 0 ? nameParts[0] : '');
  const genericTokens = ['app', 'web', 'service', 'container', 'db', 'server'];
  if (fallbackCandidate && !genericTokens.includes(fallbackCandidate)) {
    const slug = getAppSlug(fallbackCandidate);
    if (slug && !genericTokens.includes(slug)) {
      return `https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/${slug}.png`;
    }
  }

  return '';
}

/**
 * Returns the best available icon URL for a Docker application.
 */
export function getDockerAppIcon(app: DockerApp, catalogApps?: CatalogApp[]): string {
  const cleanAppId = app.appId || (app.name ? app.name.replace(/^dockerApps\//, '') : '');
  const normalizedAppIcon = normalizeAppIconUrl(app.iconUrl, app.catalogAppId || cleanAppId);
  if (normalizedAppIcon) {
    return normalizedAppIcon;
  }

  const catalog = catalogApps || nasState.catalogApps || [];
  const matched = catalog.find(
    (c) =>
      (app.catalogAppId && c.appId === app.catalogAppId) ||
      (app.appId && c.appId.toLowerCase() === app.appId.toLowerCase()) ||
      (app.title && c.title.toLowerCase() === app.title.toLowerCase())
  );

  const normalizedCatIcon = normalizeAppIconUrl(matched?.iconUrl, matched?.appId || cleanAppId);
  if (normalizedCatIcon) {
    return normalizedCatIcon;
  }

  const slug = getAppSlug(app.appId || app.title || '');
  if (slug) {
    return `https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/${slug}.png`;
  }

  if (app.containers && app.containers.length > 0) {
    for (const c of app.containers) {
      const cIcon = getDockerContainerIcon(c, catalog);
      if (cIcon) return cIcon;
    }
  }

  return '';
}

/**
 * Returns the best available icon URL for an LXC container.
 */
export function getLxcContainerIcon(lx: LxcContainer, lxcCatalogItems?: LxcCatalogItem[]): string {
  if (lx.iconUrl?.trim()) {
    return lx.iconUrl.trim();
  }

  const catalog = lxcCatalogItems || nasState.lxcCatalogItems || [];
  const cName = (lx.containerName || '').toLowerCase();
  const cleanId = (lx.name.replace(/^lxcContainers\//, '') || '').toLowerCase();
  const recipe = (lx.installRecipe || '').toLowerCase();

  const matched = catalog.find((c) => {
    const id = c.id.toLowerCase();
    const title = c.title.toLowerCase();
    return (
      id === cName ||
      id === cleanId ||
      title === cName ||
      (recipe && (id === recipe || c.installRecipe?.toLowerCase() === recipe))
    );
  });

  if (matched?.iconUrl?.trim()) {
    return matched.iconUrl.trim();
  }

  const distroKey = (lx.distribution || '').toLowerCase().trim();
  const cleanName = (lx.containerName || '').toLowerCase().replace(/^(ct|lxc)[-_]/, '');
  const isDistroNamed =
    distroKey &&
    (cleanName.includes(distroKey) || ['debian', 'ubuntu', 'alpine', 'arch', 'archlinux', 'fedora', 'rocky', 'rockylinux', 'almalinux'].some((d) => cleanName.startsWith(d)));

  // If container name explicitly indicates a distro container (e.g. ct-debian, debian-12)
  if (isDistroNamed && LXC_DISTRO_ICONS[distroKey]) {
    return LXC_DISTRO_ICONS[distroKey];
  }

  // Check if container name matches a known service (e.g. pihole, plex, adguard, nextcloud)
  const slug = getAppSlug(cleanName || lx.containerName || '');
  const isGeneric = ['container', 'debian', 'ubuntu', 'alpine', 'arch', 'archlinux', 'fedora', 'rocky', 'rockylinux', 'almalinux', 'ct', 'lxc'].includes(slug);
  if (slug && !isGeneric) {
    return `https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/${slug}.png`;
  }

  // Fallback to distribution icon
  if (distroKey && LXC_DISTRO_ICONS[distroKey]) {
    return LXC_DISTRO_ICONS[distroKey];
  }

  return '';
}

export const VM_OS_ICONS: Record<string, string> = {
  debian: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/debian.png',
  ubuntu: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/ubuntu.png',
  alpine: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/alpine-linux.png',
  arch: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/arch-linux.png',
  fedora: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/fedora.png',
  rocky: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/rocky-linux.png',
  rockylinux: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/rocky-linux.png',
  almalinux: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/almalinux.png',
  redhat: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/redhat.png',
  freebsd: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/freebsd.png',
  opnsense: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/opnsense.png',
  pfsense: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/pfsense.png',
  windows: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/windows.png',
  proxmox: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/proxmox.png',
  truenas: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/truenas.png',
  homeassistant: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/home-assistant.png',
  talos: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/talos.png',
  nixos: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/nixos.png',
  linux: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/linux.png',
};

function matchOsTokenToIcon(raw: string): string {
  const s = (raw || '').toLowerCase();
  if (!s) return '';
  if (s.includes('opnsense')) return VM_OS_ICONS.opnsense;
  if (s.includes('pfsense')) return VM_OS_ICONS.pfsense;
  if (s.includes('truenas')) return VM_OS_ICONS.truenas;
  if (s.includes('proxmox') || s.includes('pve')) return VM_OS_ICONS.proxmox;
  if (s.includes('homeassistant') || s.includes('home-assistant') || s.includes('haos')) return VM_OS_ICONS.homeassistant;
  if (s.includes('talos')) return VM_OS_ICONS.talos;
  if (s.includes('nixos') || s.includes('nix')) return VM_OS_ICONS.nixos;
  if (s.includes('windows') || s.includes('win2k') || s.includes('win10') || s.includes('win11') || /\bwin\b/.test(s)) {
    return VM_OS_ICONS.windows;
  }
  if (s.includes('ubuntu')) return VM_OS_ICONS.ubuntu;
  if (s.includes('debian') || s.includes('mynas')) return VM_OS_ICONS.debian;
  if (s.includes('rocky')) return VM_OS_ICONS.rocky;
  if (s.includes('alma')) return VM_OS_ICONS.almalinux;
  if (s.includes('rhel') || s.includes('redhat') || s.includes('centos')) return VM_OS_ICONS.redhat;
  if (s.includes('freebsd') || s.includes('bsd')) return VM_OS_ICONS.freebsd;
  if (s.includes('arch')) return VM_OS_ICONS.arch;
  if (s.includes('fedora')) return VM_OS_ICONS.fedora;
  if (s.includes('alpine')) return VM_OS_ICONS.alpine;
  if (s === 'generic' || s.includes('linux')) return VM_OS_ICONS.linux;
  return '';
}

/**
 * Returns the best available OS / appliance icon URL for a KVM Virtual Machine.
 */
export function getVmIcon(vm: Partial<VirtualMachine>): string {
  // 1. Check specific appliance/service keywords in VM name or ISO first (e.g. OPNsense on FreeBSD, Home Assistant, TrueNAS)
  const nameAndDesc = `${vm.vmName || ''} ${vm.name || ''} ${vm.description || ''}`;
  const isoPaths = [
    vm.cdromIsoImage || '',
    vm.isoImage || '',
    ...(vm.cdroms || []).map((c) => c.isoPath || ''),
  ]
    .filter((p) => !p.toLowerCase().includes('virtio-win'))
    .join(' ');

  for (const special of ['opnsense', 'pfsense', 'truenas', 'proxmox', 'homeassistant', 'home-assistant', 'haos', 'talos', 'nixos']) {
    if (nameAndDesc.toLowerCase().includes(special) || isoPaths.toLowerCase().includes(special)) {
      const icon = matchOsTokenToIcon(special);
      if (icon) return icon;
    }
  }

  // 2. Declared osVariant
  if (vm.osVariant) {
    const byVariant = matchOsTokenToIcon(vm.osVariant);
    if (byVariant) return byVariant;
  }

  // 3. Live Guest Agent OS name
  if (vm.guestOsName) {
    const byGuest = matchOsTokenToIcon(vm.guestOsName);
    if (byGuest) return byGuest;
  }

  // 4. Attached ISO image filenames & VM name/description
  const byIso = matchOsTokenToIcon(isoPaths);
  if (byIso) return byIso;

  const byName = matchOsTokenToIcon(nameAndDesc);
  if (byName) return byName;

  return '';
}

