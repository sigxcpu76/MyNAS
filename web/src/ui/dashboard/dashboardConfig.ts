import {
  mdiCpu64Bit,
  mdiDatabase,
  mdiDocker,
  mdiHarddisk,
  mdiLanConnect,
  mdiMemory,
  mdiServer,
  mdiViewDashboard,
} from '@mdi/js';

export interface DashboardWidgetConfig {
  id: string;
  type: string;
  colSpan: number; // 3, 4, 6, 8, 12
}

export interface WidgetCatalogItem {
  type: string;
  name: string;
  description: string;
  icon: string;
  defaultColSpan: number;
  allowedColSpans: number[];
  category: 'system' | 'storage' | 'network' | 'workloads';
}

export const WIDGET_CATALOG: WidgetCatalogItem[] = [
  {
    type: 'kpis',
    name: 'Overview KPIs',
    description: 'High-level summary cards for storage capacity, CPU load, RAM usage, and active workloads.',
    icon: mdiViewDashboard,
    defaultColSpan: 12,
    allowedColSpans: [6, 12],
    category: 'system',
  },
  {
    type: 'pools',
    name: 'ZFS Storage Pools',
    description: 'Capacity usage, fragmentation, dedup ratio, and scrub health for all active ZFS storage pools.',
    icon: mdiDatabase,
    defaultColSpan: 12,
    allowedColSpans: [6, 12],
    category: 'storage',
  },
  {
    type: 'cpu',
    name: 'Processor Telemetry',
    description: 'Real-time CPU utilization gauge, thermal temperature readout, clock speeds, and 1/5/15m load averages.',
    icon: mdiCpu64Bit,
    defaultColSpan: 6,
    allowedColSpans: [3, 4, 6, 8, 12],
    category: 'system',
  },
  {
    type: 'memory',
    name: 'Memory (RAM) & ZFS ARC',
    description: 'RAM usage donut chart breakdown distinguishing container services, ZFS ARC cache, buffers, and free memory.',
    icon: mdiMemory,
    defaultColSpan: 6,
    allowedColSpans: [4, 6, 8, 12],
    category: 'system',
  },
  {
    type: 'network',
    name: 'Network Throughput & Traffic',
    description: 'Active network adapter status, IP/MAC addresses, live transfer rates, and rolling throughput history graph.',
    icon: mdiLanConnect,
    defaultColSpan: 6,
    allowedColSpans: [4, 6, 8, 12],
    category: 'network',
  },
  {
    type: 'system',
    name: 'Hardware & Platform Info',
    description: 'Specification sheet displaying hostname, motherboard, BIOS version, OS kernel, GPU, and system uptime.',
    icon: mdiServer,
    defaultColSpan: 6,
    allowedColSpans: [4, 6, 8, 12],
    category: 'system',
  },
  {
    type: 'disks',
    name: 'Physical Disks & Drives',
    description: 'Physical storage drive overview showing drive models, bus types (NVMe/SATA), capacity, and drive health.',
    icon: mdiHarddisk,
    defaultColSpan: 6,
    allowedColSpans: [4, 6, 8, 12],
    category: 'storage',
  },
  {
    type: 'workloads',
    name: 'Workloads & Services',
    description: 'Quick health and count indicators for Docker containers, KVM virtual machines, LXC containers, and network shares.',
    icon: mdiDocker,
    defaultColSpan: 6,
    allowedColSpans: [3, 4, 6, 8, 12],
    category: 'workloads',
  },
];

export const DEFAULT_DASHBOARD_LAYOUT: DashboardWidgetConfig[] = [
  { id: 'widget-kpis', type: 'kpis', colSpan: 12 },
  { id: 'widget-pools', type: 'pools', colSpan: 12 },
  { id: 'widget-system', type: 'system', colSpan: 6 },
  { id: 'widget-network', type: 'network', colSpan: 6 },
];

export const STORAGE_KEY = 'nas_dashboard_layout_v2';

export function loadDashboardLayout(): DashboardWidgetConfig[] {
  try {
    if (typeof window !== 'undefined' && window.localStorage) {
      let raw = window.localStorage.getItem(STORAGE_KEY);
      // Migrate from v1 if v2 is not yet saved: filter out cpu and memory big cards
      if (!raw) {
        const v1Raw = window.localStorage.getItem('nas_dashboard_layout_v1');
        if (v1Raw) {
          const v1Parsed = JSON.parse(v1Raw);
          if (Array.isArray(v1Parsed)) {
            const migrated = v1Parsed.filter((w: any) => w && w.type !== 'cpu' && w.type !== 'memory');
            saveDashboardLayout(migrated);
            return migrated;
          }
        }
      }
      if (raw) {
        const parsed = JSON.parse(raw);
        if (Array.isArray(parsed) && parsed.length > 0) {
          // Sanitize items against known catalog types
          const valid = parsed.filter(
            (item: any) =>
              item &&
              typeof item.id === 'string' &&
              typeof item.type === 'string' &&
              WIDGET_CATALOG.some((c) => c.type === item.type)
          );
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

export function saveDashboardLayout(layout: DashboardWidgetConfig[]): void {
  try {
    if (typeof window !== 'undefined' && window.localStorage) {
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify(layout));
    }
  } catch (err) {
    console.error('Failed to save dashboard layout', err);
  }
}
