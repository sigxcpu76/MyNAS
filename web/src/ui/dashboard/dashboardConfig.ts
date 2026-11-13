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
  mdiViewDashboard,
} from '@mdi/js';

export interface DashboardWidgetConfig {
  id: string;
  type: string;
  lane: number; // -1 for top full-width ribbon, 0 = Col 1, 1 = Col 2, 2 = Col 3
  colSpan?: number; // legacy backward compatibility
  rowSpan?: number; // legacy backward compatibility
}

export interface WidgetCatalogItem {
  type: string;
  name: string;
  description: string;
  icon: string;
  defaultLane: number; // -1 for top ribbon, 0, 1, 2 for columns
  category: 'system' | 'storage' | 'network' | 'workloads';
}

export const WIDGET_CATALOG: WidgetCatalogItem[] = [
  {
    type: 'kpis',
    name: 'Overview KPIs',
    description: 'High-level summary cards for storage capacity, CPU load, RAM usage, and active workloads.',
    icon: mdiViewDashboard,
    defaultLane: -1,
    category: 'system',
  },
  {
    type: 'pools',
    name: 'ZFS Storage Pools',
    description: 'Capacity usage, fragmentation, dedup ratio, and scrub health for all active ZFS storage pools.',
    icon: mdiDatabase,
    defaultLane: 0,
    category: 'storage',
  },
  {
    type: 'disks',
    name: 'Physical Disks & Drives',
    description: 'Physical storage drive overview showing drive models, bus types (NVMe/SATA), capacity, and drive health.',
    icon: mdiHarddisk,
    defaultLane: 0,
    category: 'storage',
  },
  {
    type: 'disk_io',
    name: 'Disk I/O Throughput & IOPS',
    description: 'Live block device read/write throughput, IOPS, queue depth, and utilization from /proc/diskstats.',
    icon: mdiHarddisk,
    defaultLane: 1,
    category: 'storage',
  },
  {
    type: 'zpool_io',
    name: 'Zpool IOStat',
    description: 'Live ZFS pool read/write bandwidth, IOPS, and wait/active queues from /proc/spl/kstat/zfs.',
    icon: mdiDatabase,
    defaultLane: 0,
    category: 'storage',
  },
  {
    type: 'network',
    name: 'Network Throughput & Traffic',
    description: 'Active network adapter status, IP/MAC addresses, live transfer rates, and rolling throughput history graph.',
    icon: mdiLanConnect,
    defaultLane: 1,
    category: 'network',
  },
  {
    type: 'webapps',
    name: 'Web Applications',
    description: 'Quick-launch shortcuts for installed applications exposing Web UI ports with live status and one-click access.',
    icon: mdiOpenInNew,
    defaultLane: 0,
    category: 'workloads',
  },
  {
    type: 'apps',
    name: 'Apps & Containers',
    description: 'Docker Compose application stacks and LXC containers with live container counts and status chips.',
    icon: mdiDocker,
    defaultLane: 1,
    category: 'workloads',
  },
  {
    type: 'vms',
    name: 'Virtual Machines',
    description: 'KVM virtual machines with CPU allocation, RAM sizing, VNC display ports, and live running states.',
    icon: mdiServerNetwork,
    defaultLane: 1,
    category: 'workloads',
  },
  {
    type: 'shares',
    name: 'Network Shares',
    description: 'Samba (SMB) and WebDAV network storage shares with mount paths, access modes (RW/RO), and quick management.',
    icon: mdiFolderNetworkOutline,
    defaultLane: 1,
    category: 'workloads',
  },
  {
    type: 'workloads',
    name: 'Workloads (All-in-One)',
    description: 'Combined multi-panel card displaying Virtual Machines, Apps & Containers, and Network Shares together.',
    icon: mdiDocker,
    defaultLane: 1,
    category: 'workloads',
  },
  {
    type: 'sensors',
    name: 'Hardware Sensors & Power',
    description: 'System temperatures (CPU, GPU, PCI cards, storage), dedicated NVIDIA/AMD GPU metrics, fan speeds, and ACPI power meter.',
    icon: mdiThermometer,
    defaultLane: 2,
    category: 'system',
  },
  {
    type: 'system',
    name: 'Hardware & Platform Info',
    description: 'Specification sheet displaying hostname, motherboard, BIOS version, OS kernel, GPU, and system uptime.',
    icon: mdiServer,
    defaultLane: 2,
    category: 'system',
  },
  {
    type: 'cpu',
    name: 'Processor Telemetry',
    description: 'Real-time CPU utilization gauge, thermal temperature readout, clock speeds, and 1/5/15m load averages.',
    icon: mdiCpu64Bit,
    defaultLane: 2,
    category: 'system',
  },
  {
    type: 'memory',
    name: 'Memory (RAM) & ZFS ARC',
    description: 'RAM usage donut chart breakdown distinguishing container services, ZFS ARC cache, buffers, and free memory.',
    icon: mdiMemory,
    defaultLane: 2,
    category: 'system',
  },
];

export const DEFAULT_DASHBOARD_LAYOUT: DashboardWidgetConfig[] = [
  { id: 'widget-kpis', type: 'kpis', lane: -1 },
  { id: 'widget-pools', type: 'pools', lane: 0 },
  { id: 'widget-zpool-io', type: 'zpool_io', lane: 0 },
  { id: 'widget-network', type: 'network', lane: 1 },
  { id: 'widget-disk-io', type: 'disk_io', lane: 1 },
  { id: 'widget-sensors', type: 'sensors', lane: 2 },
];

export const BALANCED_DASHBOARD_LAYOUT: DashboardWidgetConfig[] = [
  { id: 'widget-kpis', type: 'kpis', lane: -1 },
  { id: 'widget-pools', type: 'pools', lane: 0 },
  { id: 'widget-zpool-io', type: 'zpool_io', lane: 0 },
  { id: 'widget-disks', type: 'disks', lane: 0 },
  { id: 'widget-network', type: 'network', lane: 1 },
  { id: 'widget-disk-io', type: 'disk_io', lane: 1 },
  { id: 'widget-apps', type: 'apps', lane: 1 },
  { id: 'widget-sensors', type: 'sensors', lane: 2 },
  { id: 'widget-system', type: 'system', lane: 2 },
];

export const STORAGE_KEY = 'nas_dashboard_layout_v4';
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

export function loadDashboardLayout(): DashboardWidgetConfig[] {
  try {
    if (typeof window !== 'undefined' && window.localStorage) {
      let raw = window.localStorage.getItem(STORAGE_KEY);
      // Migrate from v3, v2, or v1 if v4 is not yet saved
      if (!raw) {
        const prevRaw =
          window.localStorage.getItem('nas_dashboard_layout_v3') ||
          window.localStorage.getItem('nas_dashboard_layout_v2') ||
          window.localStorage.getItem('nas_dashboard_layout_v1');
        if (prevRaw) {
          const prevParsed = JSON.parse(prevRaw);
          if (Array.isArray(prevParsed) && prevParsed.length > 0) {
            let colCounter = 0;
            const migrated: DashboardWidgetConfig[] = prevParsed
              .filter(
                (item: any) =>
                  item &&
                  typeof item.id === 'string' &&
                  typeof item.type === 'string' &&
                  WIDGET_CATALOG.some((c) => c.type === item.type)
              )
              .map((w: any) => {
                const cat = WIDGET_CATALOG.find((c) => c.type === w.type);
                let assignedLane = typeof w.lane === 'number' ? w.lane : undefined;
                if (assignedLane === undefined) {
                  if (w.type === 'kpis') {
                    assignedLane = -1;
                  } else if (cat && typeof cat.defaultLane === 'number') {
                    assignedLane = cat.defaultLane;
                  } else {
                    assignedLane = colCounter % 3;
                    colCounter++;
                  }
                }
                return {
                  id: w.id,
                  type: w.type,
                  lane: assignedLane,
                };
              });

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
          let colCounter = 0;
          const valid = parsed
            .filter(
              (item: any) =>
                item &&
                typeof item.id === 'string' &&
                typeof item.type === 'string' &&
                WIDGET_CATALOG.some((c) => c.type === item.type)
            )
            .map((item: any) => {
              const cat = WIDGET_CATALOG.find((c) => c.type === item.type);
              let lane = typeof item.lane === 'number' ? item.lane : undefined;
              if (lane === undefined) {
                if (item.type === 'kpis') {
                  lane = -1;
                } else if (cat && typeof cat.defaultLane === 'number') {
                  lane = cat.defaultLane;
                } else {
                  lane = colCounter % 3;
                  colCounter++;
                }
              }
              return {
                id: item.id,
                type: item.type,
                lane,
              };
            });
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

export async function syncPreferencesFromBackend(services: any): Promise<{ layout: DashboardWidgetConfig[]; columnCount: number } | null> {
  try {
    if (!services?.auth?.isAuthenticated?.()) return null;
    const prefs = await services.auth.getUserPreferences();
    if (prefs?.dashboard) {
      const col = Number(prefs.dashboard.columnCount) || 3;
      const layout = prefs.dashboard.layout;
      if (Array.isArray(layout) && layout.length > 0) {
        const cleanLayout: DashboardWidgetConfig[] = layout
          .filter(
            (item: any) =>
              item &&
              typeof item.id === 'string' &&
              typeof item.type === 'string' &&
              WIDGET_CATALOG.some((c) => c.type === item.type)
          )
          .map((w: any) => ({
            id: String(w.id),
            type: String(w.type),
            lane: Number(w.lane),
          }));
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
  columnCount: number
): Promise<void> {
  // Always save locally first for instant cache & offline resilience
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
        })),
      },
    });
  } catch (err) {
    console.warn('Could not persist preferences to backend:', err);
  }
}
