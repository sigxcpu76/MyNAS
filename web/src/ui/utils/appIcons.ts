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

  // Check if container name matches a known service (e.g. pihole, plex, adguard, nextcloud)
  const slug = getAppSlug(lx.containerName || '');
  const isGeneric = ['container', 'debian', 'ubuntu', 'alpine', 'arch', 'fedora', 'rocky', 'almalinux'].includes(slug);
  if (slug && !isGeneric) {
    return `https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/${slug}.png`;
  }

  // Distribution icon
  const distroKey = (lx.distribution || '').toLowerCase().trim();
  if (distroKey && LXC_DISTRO_ICONS[distroKey]) {
    return LXC_DISTRO_ICONS[distroKey];
  }

  return '';
}
