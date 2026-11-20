import {
  mdiCpu64Bit,
  mdiDatabase,
  mdiDocker,
  mdiFolderNetworkOutline,
  mdiHarddisk,
  mdiLanConnect,
  mdiMemory,
  mdiOpenInNew,
  mdiServer,
  mdiServerNetwork,
  mdiThermometer,
  mdiVideo3dVariant,
  mdiViewDashboard,
} from '@mdi/js';
import {
  GRID_COLS,
  clampRect,
  compactLayout,
  findOpenSlot,
} from './gridEngine';

export interface DashboardWidgetConfig {
  id: string;
  type: string;
  x: number;
  y: number;
  w: number;
  h: number;
  lane: number; // kept for backward compatibility with legacy backend/storage
  colSpan?: number;
  rowSpan?: number;
}

export interface WidgetCatalogItem {
  type: string;
  name: string;
  description: string;
  icon: string;
  defaultLane: number;
  defaultW: number;
  defaultH: number;
  minW: number;
  minH: number;
  category: 'system' | 'storage' | 'network' | 'workloads';
}

export const WIDGET_CATALOG: WidgetCatalogItem[] = [
  {
    type: 'kpis',
    name: 'Overview KPIs',
    description: 'High-level summary cards for storage capacity, CPU load, RAM usage, and active workloads.',
    icon: mdiViewDashboard,
    defaultLane: -1,
    defaultW: 24,
    defaultH: 3,
    minW: 6,
    minH: 2,
    category: 'system',
  },
  {
    type: 'gpu',
    name: 'GPU Monitor (nvtop)',
    description: 'nvtop-style real-time GPU telemetry with rolling utilization & VRAM charts, clocks, power, thermals, and active GPU processes.',
    icon: mdiVideo3dVariant,
    defaultLane: 2,
    defaultW: 12,
    defaultH: 7,
    minW: 6,
    minH: 5,
    category: 'system',
  },
  {
    type: 'pools',
    name: 'ZFS Storage Pools',
    description: 'Capacity usage, fragmentation, dedup ratio, and scrub health for all active ZFS storage pools.',
    icon: mdiDatabase,
    defaultLane: 0,
    defaultW: 8,
    defaultH: 7,
    minW: 6,
    minH: 4,
    category: 'storage',
  },
  {
    type: 'disks',
    name: 'Physical Disks & Drives',
    description: 'Physical storage drive overview showing drive models, bus types (NVMe/SATA), capacity, and drive health.',
    icon: mdiHarddisk,
    defaultLane: 0,
    defaultW: 8,
    defaultH: 7,
    minW: 6,
    minH: 4,
    category: 'storage',
  },
  {
    type: 'disk_io',
    name: 'Disk I/O Throughput & IOPS',
    description: 'Live block device read/write throughput, IOPS, queue depth, and utilization from /proc/diskstats.',
    icon: mdiHarddisk,
    defaultLane: 1,
    defaultW: 8,
    defaultH: 7,
    minW: 6,
    minH: 5,
    category: 'storage',
  },
  {
    type: 'zpool_io',
    name: 'Zpool IOStat',
    description: 'Live ZFS pool read/write bandwidth, IOPS, and wait/active queues from /proc/spl/kstat/zfs.',
    icon: mdiDatabase,
    defaultLane: 0,
    defaultW: 8,
    defaultH: 7,
    minW: 6,
    minH: 5,
    category: 'storage',
  },
  {
    type: 'network',
    name: 'Network Throughput & Traffic',
    description: 'Active network adapter status, IP/MAC addresses, live transfer rates, and rolling throughput history graph.',
    icon: mdiLanConnect,
    defaultLane: 1,
    defaultW: 8,
    defaultH: 7,
    minW: 6,
    minH: 5,
    category: 'network',
  },
  {
    type: 'webapps',
    name: 'Web Applications',
    description: 'Quick-launch shortcuts for installed applications exposing Web UI ports with live status and one-click access.',
    icon: mdiOpenInNew,
    defaultLane: 0,
    defaultW: 8,
    defaultH: 6,
    minW: 6,
    minH: 4,
    category: 'workloads',
  },
  {
    type: 'apps',
    name: 'Apps & Containers',
    description: 'Docker Compose application stacks and LXC containers with live container counts and status chips.',
    icon: mdiDocker,
    defaultLane: 1,
    defaultW: 8,
    defaultH: 6,
    minW: 6,
    minH: 4,
    category: 'workloads',
  },
  {
    type: 'vms',
    name: 'Virtual Machines',
    description: 'KVM virtual machines with CPU allocation, RAM sizing, VNC display ports, and live running states.',
    icon: mdiServerNetwork,
    defaultLane: 1,
    defaultW: 8,
    defaultH: 6,
    minW: 6,
    minH: 4,
    category: 'workloads',
  },
  {
    type: 'shares',
    name: 'Network Shares',
    description: 'Samba (SMB) and WebDAV network storage shares with mount paths, access modes (RW/RO), and quick management.',
    icon: mdiFolderNetworkOutline,
    defaultLane: 1,
    defaultW: 8,
    defaultH: 6,
    minW: 6,
    minH: 4,
    category: 'workloads',
  },
  {
    type: 'workloads',
    name: 'Workloads (All-in-One)',
    description: 'Combined multi-panel card displaying Virtual Machines, Apps & Containers, and Network Shares together.',
    icon: mdiDocker,
    defaultLane: 1,
    defaultW: 24,
    defaultH: 7,
    minW: 8,
    minH: 5,
    category: 'workloads',
  },
  {
    type: 'sensors',
    name: 'Hardware Sensors & Power',
    description: 'System temperatures (CPU, GPU, PCI cards, storage), dedicated NVIDIA/AMD GPU metrics, fan speeds, and ACPI power meter.',
    icon: mdiThermometer,
    defaultLane: 2,
    defaultW: 8,
    defaultH: 10,
    minW: 6,
    minH: 5,
    category: 'system',
  },
  {
    type: 'system',
    name: 'Hardware & Platform Info',
    description: 'Specification sheet displaying hostname, motherboard, BIOS version, OS kernel, GPU, and system uptime.',
    icon: mdiServer,
    defaultLane: 2,
    defaultW: 8,
    defaultH: 7,
    minW: 6,
    minH: 4,
    category: 'system',
  },
  {
    type: 'cpu',
    name: 'Processor Telemetry',
    description: 'Real-time CPU utilization gauge, thermal temperature readout, clock speeds, and 1/5/15m load averages.',
    icon: mdiCpu64Bit,
    defaultLane: 2,
    defaultW: 8,
    defaultH: 6,
    minW: 5,
    minH: 5,
    category: 'system',
  },
  {
    type: 'memory',
    name: 'Memory (RAM) & ZFS ARC',
    description: 'RAM usage donut chart breakdown distinguishing container services, ZFS ARC cache, buffers, and free memory.',
    icon: mdiMemory,
    defaultLane: 2,
    defaultW: 8,
    defaultH: 6,
    minW: 5,
    minH: 5,
    category: 'system',
  },
];

export function getWidgetCatalogItem(type: string): WidgetCatalogItem | undefined {
  return WIDGET_CATALOG.find((c) => c.type === type);
}

export const DEFAULT_DASHBOARD_LAYOUT: DashboardWidgetConfig[] = [
  { id: 'widget-kpis', type: 'kpis', x: 0, y: 0, w: 24, h: 3, lane: -1 },
  { id: 'widget-pools', type: 'pools', x: 0, y: 3, w: 8, h: 7, lane: 0 },
  { id: 'widget-network', type: 'network', x: 8, y: 3, w: 8, h: 7, lane: 1 },
  { id: 'widget-sensors', type: 'sensors', x: 16, y: 3, w: 8, h: 14, lane: 2 },
  { id: 'widget-zpool-io', type: 'zpool_io', x: 0, y: 10, w: 8, h: 7, lane: 0 },
  { id: 'widget-disk-io', type: 'disk_io', x: 8, y: 10, w: 8, h: 7, lane: 1 },
  { id: 'widget-gpu', type: 'gpu', x: 0, y: 17, w: 12, h: 7, lane: 2 },
];

export const BALANCED_DASHBOARD_LAYOUT: DashboardWidgetConfig[] = [
  { id: 'widget-kpis', type: 'kpis', x: 0, y: 0, w: 24, h: 3, lane: -1 },
  { id: 'widget-gpu', type: 'gpu', x: 0, y: 3, w: 12, h: 7, lane: 0 },
  { id: 'widget-network', type: 'network', x: 12, y: 3, w: 12, h: 7, lane: 1 },
  { id: 'widget-pools', type: 'pools', x: 0, y: 10, w: 8, h: 7, lane: 0 },
  { id: 'widget-zpool-io', type: 'zpool_io', x: 8, y: 10, w: 8, h: 7, lane: 1 },
  { id: 'widget-disk-io', type: 'disk_io', x: 16, y: 10, w: 8, h: 7, lane: 2 },
  { id: 'widget-sensors', type: 'sensors', x: 0, y: 17, w: 8, h: 9, lane: 0 },
  { id: 'widget-apps', type: 'apps', x: 8, y: 17, w: 8, h: 9, lane: 1 },
  { id: 'widget-system', type: 'system', x: 16, y: 17, w: 8, h: 9, lane: 2 },
  { id: 'widget-disks', type: 'disks', x: 0, y: 26, w: 12, h: 7, lane: 0 },
];

export const STORAGE_KEY = 'nas_dashboard_layout_v5';
export const STORAGE_KEY_COLUMNS = 'nas_dashboard_column_count_v4';

export function loadDashboardColumnCount(): number {
  try {
    if (typeof window !== 'undefined' && window.localStorage) {
      const raw = window.localStorage.getItem(STORAGE_KEY_COLUMNS);
      if (raw) {
        const val = parseInt(raw, 10);
        if (val === 2 || val === 3) return val;
      }
    }
  } catch {
    // fallback
  }
  return 3;
}

export function saveDashboardColumnCount(count: number): void {
  try {
    if (typeof window !== 'undefined' && window.localStorage) {
      window.localStorage.setItem(STORAGE_KEY_COLUMNS, String(count));
    }
  } catch {
    // ignore
  }
}

/**
 * Normalizes raw widget items (including legacy lane-only layouts) into valid 24-column Grafana grid configs.
 */
export function normalizeToGridLayout(rawItems: any[], legacyCols = 3): DashboardWidgetConfig[] {
  const validRaw = rawItems.filter(
    (item: any) =>
      item &&
      typeof item.id === 'string' &&
      typeof item.type === 'string' &&
      WIDGET_CATALOG.some((c) => c.type === item.type)
  );
  if (validRaw.length === 0) return [];

  const hasExplicitGrid = validRaw.some(
    (item: any) => typeof item.w === 'number' && item.w > 0 && typeof item.h === 'number' && item.h > 0
  );

  if (hasExplicitGrid) {
    const placed: DashboardWidgetConfig[] = [];
    for (const item of validRaw) {
      const cat = getWidgetCatalogItem(item.type);
      const minW = cat?.minW ?? 4;
      const minH = cat?.minH ?? 3;
      const w = typeof item.w === 'number' && item.w > 0 ? item.w : cat?.defaultW ?? 8;
      const h = typeof item.h === 'number' && item.h > 0 ? item.h : cat?.defaultH ?? 6;
      const hasPos = typeof item.x === 'number' && typeof item.y === 'number' && typeof item.w === 'number' && item.w > 0;
      const pos = hasPos ? { x: item.x, y: item.y } : findOpenSlot(placed, w, h, GRID_COLS);
      const clamped = clampRect(
        {
          id: String(item.id),
          type: String(item.type),
          x: pos.x,
          y: pos.y,
          w,
          h,
          lane: typeof item.lane === 'number' ? item.lane : cat?.defaultLane ?? 0,
        },
        minW,
        minH,
        GRID_COLS
      );
      placed.push(clamped);
    }
    return compactLayout(placed, GRID_COLS);
  }

  // Migrate legacy lane-based layout into 24-column Grafana grid
  const cols = legacyCols === 2 ? 2 : 3;
  const colW = Math.floor(GRID_COLS / cols);
  const laneY = new Array(cols).fill(0);
  const result: DashboardWidgetConfig[] = [];

  // Place top-ribbon (lane === -1 or kpis) first at full width
  let topY = 0;
  const ribbonItems = validRaw.filter((w: any) => w.lane === -1 || (w.lane === undefined && w.type === 'kpis'));
  const bodyItems = validRaw.filter((w: any) => !(w.lane === -1 || (w.lane === undefined && w.type === 'kpis')));

  for (const w of ribbonItems) {
    const cat = getWidgetCatalogItem(w.type);
    const h = cat?.defaultH ?? 3;
    result.push({
      id: String(w.id),
      type: String(w.type),
      x: 0,
      y: topY,
      w: GRID_COLS,
      h,
      lane: -1,
    });
    topY += h;
  }

  for (let i = 0; i < cols; i++) {
    laneY[i] = topY;
  }

  let fallbackCol = 0;
  for (const w of bodyItems) {
    const cat = getWidgetCatalogItem(w.type);
    let lane = typeof w.lane === 'number' && w.lane >= 0 ? w.lane : cat?.defaultLane ?? fallbackCol++;
    if (lane < 0) lane = 0;
    if (lane >= cols) lane = cols - 1;

    const h = cat?.defaultH ?? 7;
    result.push({
      id: String(w.id),
      type: String(w.type),
      x: lane * colW,
      y: laneY[lane],
      w: colW,
      h,
      lane,
    });
    laneY[lane] += h;
  }

  return compactLayout(result, GRID_COLS);
}

export function loadDashboardLayout(): DashboardWidgetConfig[] {
  try {
    if (typeof window !== 'undefined' && window.localStorage) {
      let raw = window.localStorage.getItem(STORAGE_KEY);
      if (!raw) {
        const prevRaw =
          window.localStorage.getItem('nas_dashboard_layout_v4') ||
          window.localStorage.getItem('nas_dashboard_layout_v3') ||
          window.localStorage.getItem('nas_dashboard_layout_v2') ||
          window.localStorage.getItem('nas_dashboard_layout_v1');
        if (prevRaw) {
          const prevParsed = JSON.parse(prevRaw);
          if (Array.isArray(prevParsed) && prevParsed.length > 0) {
            const migrated = normalizeToGridLayout(prevParsed, loadDashboardColumnCount());
            if (migrated.length > 0) {
              saveDashboardLayout(migrated);
              return migrated;
            }
          }
        }
      }

      if (raw) {
        const parsed = JSON.parse(raw);
        if (Array.isArray(parsed) && parsed.length > 0) {
          const valid = normalizeToGridLayout(parsed, loadDashboardColumnCount());
          if (valid.length > 0) {
            return valid;
          }
        }
      }
    }
  } catch (err) {
    console.warn('Failed to load dashboard layout, using default', err);
  }
  return JSON.parse(JSON.stringify(DEFAULT_DASHBOARD_LAYOUT));
}

export function saveDashboardLayout(layout: DashboardWidgetConfig[], columnCount?: number): void {
  try {
    if (typeof window !== 'undefined' && window.localStorage) {
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify(layout));
      if (columnCount !== undefined) {
        window.localStorage.setItem(STORAGE_KEY_COLUMNS, String(columnCount));
      }
    }
  } catch (err) {
    console.error('Failed to save dashboard layout', err);
  }
}

export async function syncPreferencesFromBackend(
  services: any
): Promise<{ layout: DashboardWidgetConfig[]; columnCount: number } | null> {
  try {
    if (!services?.auth?.isAuthenticated?.()) return null;
    const prefs = await services.auth.getUserPreferences();
    if (prefs?.dashboard) {
      const col = Number(prefs.dashboard.columnCount) || 3;
      const layout = prefs.dashboard.layout;
      if (Array.isArray(layout) && layout.length > 0) {
        const cleanLayout = normalizeToGridLayout(layout, col);
        if (cleanLayout.length > 0) {
          saveDashboardLayout(cleanLayout, col);
          return { layout: cleanLayout, columnCount: col };
        }
      }
    }
  } catch (err) {
    console.warn('Could not sync preferences from backend:', err);
  }
  return null;
}

export async function persistPreferencesToBackend(
  services: any,
  layout: DashboardWidgetConfig[],
  columnCount = 3
): Promise<void> {
  saveDashboardLayout(layout, columnCount);
  try {
    if (!services?.auth?.isAuthenticated?.()) return;
    await services.auth.updateUserPreferences({
      dashboard: {
        columnCount,
        layout: layout.map((w) => ({
          id: w.id,
          type: w.type,
          lane: w.lane,
          x: w.x,
          y: w.y,
          w: w.w,
          h: w.h,
        })),
      },
    });
  } catch (err) {
    console.warn('Could not persist preferences to backend:', err);
  }
}
