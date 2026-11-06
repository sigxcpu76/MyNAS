import type { DockerApp, LxcContainer, CatalogApp, LxcCatalogItem } from '../../domain/models';
import { nasState } from '../store';
import { getAppSlug } from '../dashboard/webAppsHelper';

export const LXC_DISTRO_ICONS: Record<string, string> = {
  debian: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/debian.png',
  ubuntu: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/ubuntu.png',
  alpine: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/alpine-linux.png',
  arch: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/arch-linux.png',
  archlinux: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/arch-linux.png',
  fedora: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/fedora.png',
  rocky: 'https://raw.githubusercontent.com/walkxcode/dashboard-icons/main/png/rocky-linux.png',
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
  if (container.iconUrl?.trim()) {
    return container.iconUrl.trim();
  }

  const catalog = catalogApps || nasState.catalogApps || [];

  const serviceName = (container.serviceName || '').toLowerCase().trim();
  const rawName = (container.name || container.containerName || '').toLowerCase().trim();
  const containerName = rawName.replace(/^dockercontainers\//, '');
  const nameParts = containerName.split(/[-_.]/).filter(Boolean);

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

  if (matched?.iconUrl?.trim()) {
    return matched.iconUrl.trim();
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
    if (matchedImageCatalog?.iconUrl?.trim()) {
      return matchedImageCatalog.iconUrl.trim();
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
  if (app.iconUrl?.trim()) {
    return app.iconUrl.trim();
  }

  const catalog = catalogApps || nasState.catalogApps || [];
  const matched = catalog.find(
    (c) =>
      (app.catalogAppId && c.appId === app.catalogAppId) ||
      (app.appId && c.appId.toLowerCase() === app.appId.toLowerCase()) ||
      (app.title && c.title.toLowerCase() === app.title.toLowerCase())
  );

  if (matched?.iconUrl?.trim()) {
    return matched.iconUrl.trim();
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
    (cleanName.includes(distroKey) || ['debian', 'ubuntu', 'alpine', 'arch', 'archlinux', 'fedora', 'rocky', 'almalinux'].some((d) => cleanName.startsWith(d)));

  // If container name explicitly indicates a distro container (e.g. ct-debian, debian-12)
  if (isDistroNamed && LXC_DISTRO_ICONS[distroKey]) {
    return LXC_DISTRO_ICONS[distroKey];
  }

  // Check if container name matches a known service (e.g. pihole, plex, adguard, nextcloud)
  const slug = getAppSlug(cleanName || lx.containerName || '');
  const isGeneric = ['container', 'debian', 'ubuntu', 'alpine', 'arch', 'archlinux', 'fedora', 'rocky', 'almalinux', 'ct', 'lxc'].includes(slug);
  if (slug && !isGeneric) {
    return `https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/${slug}.png`;
  }

  // Fallback to distribution icon
  if (distroKey && LXC_DISTRO_ICONS[distroKey]) {
    return LXC_DISTRO_ICONS[distroKey];
  }

  return '';
}

