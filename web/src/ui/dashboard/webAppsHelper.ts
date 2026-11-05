import type { DockerApp, CatalogApp } from '../../domain/models';

export interface WebPortalLink {
  name: string;
  url: string;
  port: string;
  protocol: string;
  isPrimary: boolean;
}

export interface WebAppLauncherItem {
  appId: string;
  appName: string; // AIP resource name e.g. "dockerApps/nextcloud"
  title: string;
  iconUrl?: string;
  fallbackIconUrl?: string;
  state: string;
  isRunning: boolean;
  statusText: string;
  primaryUrl: string;
  primaryPort: string;
  portals: WebPortalLink[];
  displayPorts: string;
  tooltip: string;
}

export const WELL_KNOWN_WEB_PORTS = new Set([
  '80',
  '81',
  '443',
  '3000',
  '3001',
  '3002',
  '3210',
  '5000',
  '5001',
  '7860',
  '8000',
  '8080',
  '8081',
  '8082',
  '8088',
  '8090',
  '8096',
  '8123',
  '8181',
  '8384',
  '8443',
  '8989',
  '9000',
  '9091',
  '9443',
  '30020',
  '30024',
]);

export const NON_WEB_PORTS = new Set([
  '22',
  '53',
  '137',
  '138',
  '139',
  '445',
  '1883',
  '3306',
  '5432',
  '6379',
  '9100',
  '27017',
  '28015',
]);

/**
 * Resolves localhost / 0.0.0.0 / 127.0.0.1 in URLs to current window hostname.
 */
export function resolveWebUrl(rawUrl: string): string {
  if (!rawUrl) return '';
  const currentHost =
    typeof window !== 'undefined' && window.location && window.location.hostname
      ? window.location.hostname
      : 'localhost';

  return rawUrl.replace(/:\/\/(?:localhost|127\.0\.0\.1|0\.0\.0\.0)(?::|\/|$)/, (match) => {
    return match.replace(/localhost|127\.0\.0\.1|0\.0\.0\.0/, currentHost);
  });
}

/**
 * Extracts port number string from a URL.
 */
export function extractPortFromUrl(url: string): string {
  if (!url) return '';
  // Check explicit colon port first
  const match = url.match(/:(\d+)(?:\/|$)/);
  if (match) return match[1];

  if (url.startsWith('https://')) return '443';
  if (url.startsWith('http://')) return '80';
  return '';
}

interface ParsedPortEntry {
  hostPort: string;
  containerPort: string;
  protocol: string;
}

/**
 * Parses raw port strings like "8080:80/tcp", "0.0.0.0:8080->80/tcp", etc.
 */
export function parsePortString(raw: string): ParsedPortEntry | null {
  if (!raw || typeof raw !== 'string') return null;
  let clean = raw.trim();

  // Handle Go stringified map: "map[mode:ingress protocol:tcp published:30020 target:81]"
  if (clean.startsWith('map[') && clean.endsWith(']')) {
    const inner = clean.slice(4, -1);
    const fields = inner.split(/\s+/);
    const m: Record<string, string> = {};
    for (const f of fields) {
      const idx = f.indexOf(':');
      if (idx !== -1) {
        m[f.slice(0, idx)] = f.slice(idx + 1);
      }
    }
    const pub = m['published'];
    const tgt = m['target'];
    const pr = (m['protocol'] || 'tcp').toLowerCase();
    if (pub && tgt) {
      return { hostPort: pub, containerPort: tgt, protocol: pr };
    } else if (tgt) {
      return { hostPort: tgt, containerPort: tgt, protocol: pr };
    }
    return null;
  }

  let protocol = 'tcp';
  if (clean.includes('/')) {
    const parts = clean.split('/');
    clean = parts[0];
    protocol = (parts[1] || 'tcp').toLowerCase();
  }

  if (protocol === 'udp') return null;

  // format 0.0.0.0:8080->80
  if (clean.includes('->')) {
    const parts = clean.split('->');
    const hostPart = parts[0].replace(/^(0\.0\.0\.0|\[::\]|:::):/, '');
    const contPart = parts[1];
    if (hostPart && contPart) {
      return { hostPort: hostPart, containerPort: contPart, protocol };
    }
  }

  // format host:container or container only
  const colons = clean.split(':');
  if (colons.length === 2) {
    return { hostPort: colons[0], containerPort: colons[1], protocol };
  } else if (colons.length === 3) {
    return { hostPort: colons[1], containerPort: colons[2], protocol };
  } else if (colons.length === 1 && colons[0] && !isNaN(Number(colons[0]))) {
    return { hostPort: colons[0], containerPort: colons[0], protocol };
  }

  return null;
}

/**
 * Builds a clean slug for looking up community app icons (e.g. Nextcloud -> nextcloud).
 */
export function getAppSlug(titleOrId: string): string {
  return titleOrId
    .toLowerCase()
    .replace(/[^a-z0-9_-]/g, '-')
    .replace(/-+/g, '-')
    .replace(/^-|-$/g, '');
}

/**
 * Collects Web UI ports and portal links for a single DockerApp.
 */
export function extractAppWebUis(
  app: DockerApp,
  catalogApps: CatalogApp[] = []
): WebAppLauncherItem | null {
  const currentHost =
    typeof window !== 'undefined' && window.location && window.location.hostname
      ? window.location.hostname
      : 'localhost';

  const portals: WebPortalLink[] = [];
  const seenUrls = new Set<string>();
  const seenPorts = new Set<string>();

  // 1. Explicit portals from app descriptor (e.g. x-portals)
  if (Array.isArray(app.portals)) {
    for (const p of app.portals) {
      if (p.url) {
        const resolved = resolveWebUrl(p.url);
        const portStr = p.port ? String(p.port) : extractPortFromUrl(resolved);
        if (!seenUrls.has(resolved)) {
          seenUrls.add(resolved);
          if (portStr) seenPorts.add(portStr);
          portals.push({
            name: p.name || 'Web UI',
            url: resolved,
            port: portStr,
            protocol: p.protocol || 'http',
            isPrimary: portals.length === 0,
          });
        }
      }
    }
  }

  // 2. Explicit app.webUiUrl
  if (app.webUiUrl) {
    const resolved = resolveWebUrl(app.webUiUrl);
    const portStr = extractPortFromUrl(resolved);
    if (!seenUrls.has(resolved)) {
      seenUrls.add(resolved);
      if (portStr) seenPorts.add(portStr);
      // Insert as primary at the start if no primary yet
      portals.unshift({
        name: 'Web UI',
        url: resolved,
        port: portStr,
        protocol: resolved.startsWith('https') ? 'https' : 'http',
        isPrimary: true,
      });
    }
  }

  // 3. Port mappings from Compose / live containers
  const rawPorts: string[] = [
    ...(app.portMappings || []),
    ...((app.containers || []).flatMap((c) => c.ports || [])),
  ];

  const parsedPorts: ParsedPortEntry[] = [];
  for (const raw of rawPorts) {
    const parsed = parsePortString(raw);
    if (parsed) parsedPorts.push(parsed);
  }

  // Match defaultPort from catalog if available
  const matchingCat = catalogApps.find(
    (c) =>
      (app.catalogAppId && c.appId === app.catalogAppId) ||
      c.appId.toLowerCase() === app.appId.toLowerCase() ||
      c.title.toLowerCase() === app.title.toLowerCase()
  );

  const catDefaultPort = matchingCat?.defaultPort ? String(matchingCat.defaultPort) : '';

  // Filter for Web ports
  for (const p of parsedPorts) {
    const isExplicitNonWeb =
      NON_WEB_PORTS.has(p.containerPort) || NON_WEB_PORTS.has(p.hostPort);
    const isWeb =
      !isExplicitNonWeb &&
      (WELL_KNOWN_WEB_PORTS.has(p.containerPort) ||
        WELL_KNOWN_WEB_PORTS.has(p.hostPort) ||
        (catDefaultPort && (p.containerPort === catDefaultPort || p.hostPort === catDefaultPort)));

    if (isWeb && p.hostPort && !seenPorts.has(p.hostPort)) {
      seenPorts.add(p.hostPort);
      const proto = p.hostPort === '443' || p.containerPort === '443' ? 'https' : 'http';
      const constructedUrl = `${proto}://${currentHost}:${p.hostPort}`;
      if (!seenUrls.has(constructedUrl)) {
        seenUrls.add(constructedUrl);
        portals.push({
          name: `Port ${p.hostPort}`,
          url: constructedUrl,
          port: p.hostPort,
          protocol: proto,
          isPrimary: portals.length === 0,
        });
      }
    }
  }

  // If no web portals or ports were detected, this app has no Web UI
  if (portals.length === 0) {
    return null;
  }

  // Ensure exactly one primary
  let primaryPortal = portals.find((p) => p.isPrimary);
  if (!primaryPortal) {
    portals[0].isPrimary = true;
    primaryPortal = portals[0];
  }

  // Icon resolution
  let iconUrl = app.iconUrl?.trim() || '';
  if (!iconUrl && matchingCat?.iconUrl) {
    iconUrl = matchingCat.iconUrl.trim();
  }

  const slug = getAppSlug(app.appId || app.title);
  const fallbackIconUrl = `https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/${slug}.png`;

  const cleanAppId = app.appId || app.name.replace(/^dockerApps\//, '');
  const title = app.title || cleanAppId;
  const isRunning = app.state === 'RUNNING' || app.state === 'running';

  const portsSummary = portals
    .map((p) => (p.port ? `:${p.port}` : ''))
    .filter(Boolean)
    .join(', ');

  const displayPorts = portsSummary || (primaryPortal.port ? `:${primaryPortal.port}` : 'Web UI');

  const stateLabel = isRunning
    ? 'Running'
    : app.state
    ? app.state.charAt(0).toUpperCase() + app.state.slice(1).toLowerCase()
    : 'Stopped';
  const tooltip = `${title} · ${displayPorts} (${stateLabel}) · Click to open Web UI`;

  return {
    appId: cleanAppId,
    appName: app.name,
    title,
    iconUrl: iconUrl || fallbackIconUrl,
    fallbackIconUrl,
    state: app.state || 'STOPPED',
    isRunning,
    statusText: app.statusText || '',
    primaryUrl: primaryPortal.url,
    primaryPort: primaryPortal.port,
    portals,
    displayPorts,
    tooltip,
  };
}

/**
 * Collects and normalizes Web UI applications across all installed Docker apps.
 */
export function collectWebApps(
  dockerApps: DockerApp[] = [],
  catalogApps: CatalogApp[] = []
): WebAppLauncherItem[] {
  const result: WebAppLauncherItem[] = [];

  for (const app of dockerApps) {
    const item = extractAppWebUis(app, catalogApps);
    if (item) {
      result.push(item);
    }
  }

  // Sort: Running apps first, then alphabetical by title
  result.sort((a, b) => {
    if (a.isRunning && !b.isRunning) return -1;
    if (!a.isRunning && b.isRunning) return 1;
    return a.title.localeCompare(b.title);
  });

  return result;
}

/**
 * Safely opens a Web UI URL in a new browser tab.
 */
export function openWebUiPortal(url: string): void {
  if (!url) return;
  const resolved = resolveWebUrl(url);
  if (typeof window !== 'undefined') {
    window.open(resolved, '_blank', 'noopener,noreferrer');
  }
}
