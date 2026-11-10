<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue';
import {
  mdiAccountMultiple,
  mdiClockOutline,
  mdiClose,
  mdiCogOutline,
  mdiConsole,
  mdiCubeOutline,
  mdiDatabase,
  mdiFileDocumentOutline,
  mdiFolderNetwork,
  mdiInformationOutline,
  mdiLayers,
  mdiMagnify,
  mdiMonitor,
  mdiOpenInNew,
  mdiPlay,
  mdiPower,
  mdiRefresh,
  mdiRestart,
  mdiServer,
  mdiShieldAlertOutline,
  mdiStop,
  mdiUpdate,
} from '@mdi/js';
import MdiIcon from './MdiIcon.vue';
import { confirmModal, nasState, notify, refreshAllData, runTrackedTask, selectResource } from '../store';
import { navigateTo } from '../router';
import { services } from '../../container';

const props = defineProps<{
  modelValue: boolean;
}>();

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void;
  (e: 'open-host-terminal'): void;
  (e: 'open-system-details'): void;
  (e: 'open-prerequisites'): void;
}>();

const searchQuery = ref('');
const selectedIndex = ref(0);
const searchInput = ref<HTMLInputElement | null>(null);

interface CommandItem {
  id: string;
  category: 'Resources' | 'Actions' | 'Navigation' | 'System';
  title: string;
  subtitle: string;
  icon: string;
  badge?: string;
  keywords?: string[];
  action: () => void | Promise<void>;
}

function close() {
  emit('update:modelValue', false);
}

async function runAsyncAction(title: string, fn: () => Promise<unknown>) {
  close();
  try {
    await runTrackedTask(title, fn);
    notify('success', `${title} completed`);
    await refreshAllData();
  } catch (err: any) {
    notify('error', `${title} failed: ${err.message}`);
  }
}

const allItems = computed<CommandItem[]>(() => {
  const items: CommandItem[] = [];

  // =========================================================================
  // 1. Virtual Machines (Resources & Contextual Actions)
  // =========================================================================
  for (const vm of nasState.vms) {
    const vmName = vm.vmName;
    const cleanId = vm.name.replace(/^virtualMachines\//, '');
    const isRunning = vm.state === 'RUNNING';

    // VM Resource
    items.push({
      id: `vm:${vmName}`,
      category: 'Resources',
      title: vmName,
      subtitle: `Virtual Machine · ${vm.state || 'STOPPED'} · ${vm.vcpus} vCPUs · ${vm.memoryMib} MiB`,
      icon: mdiMonitor,
      badge: vm.state,
      keywords: ['vm', 'virtual', 'machine', 'kvm', 'qemu', vmName.toLowerCase()],
      action: () => {
        selectResource(`vm:${vmName}`);
        nasState.activeTab = 'kvm';
        navigateTo('kvm');
        close();
      },
    });

    // VM Action: Console / noVNC
    items.push({
      id: `vm:${vmName}:console`,
      category: 'Actions',
      title: `${vmName}: Console`,
      subtitle: `Open noVNC graphical display and serial console for ${vmName}`,
      icon: mdiConsole,
      badge: 'Console',
      keywords: [
        vmName.toLowerCase(),
        'console',
        'con',
        'term',
        'terminal',
        'vnc',
        'novnc',
        'screen',
        'display',
        'monitor',
        'serial',
      ],
      action: () => {
        selectResource(`vm:${vmName}`);
        nasState.vmSubTab = 'console';
        nasState.activeTab = 'kvm';
        navigateTo('kvm');
        close();
      },
    });

    // VM Action: Pop-out Console
    items.push({
      id: `vm:${vmName}:popout`,
      category: 'Actions',
      title: `${vmName}: Pop-out Console`,
      subtitle: `Open detached noVNC viewer in separate popup window`,
      icon: mdiOpenInNew,
      badge: 'Popout',
      keywords: [
        vmName.toLowerCase(),
        'pop',
        'popout',
        'window',
        'external',
        'vnc',
        'novnc',
        'console',
        'con',
        'display',
      ],
      action: () => {
        const url = `/vnc-console?vm=${encodeURIComponent(cleanId)}`;
        window.open(
          url,
          `vnc_${cleanId}`,
          'width=1024,height=768,location=no,toolbar=no,menubar=no,status=no,resizable=yes'
        );
        close();
      },
    });

    // VM Action: Hardware Config
    items.push({
      id: `vm:${vmName}:hardware`,
      category: 'Actions',
      title: `${vmName}: Hardware Config`,
      subtitle: `Configure vCPUs, memory, disks, and network interfaces for ${vmName}`,
      icon: mdiCogOutline,
      badge: 'Hardware',
      keywords: [
        vmName.toLowerCase(),
        'hardware',
        'config',
        'conf',
        'vcpu',
        'ram',
        'memory',
        'disk',
        'nic',
        'net',
        'network',
      ],
      action: () => {
        selectResource(`vm:${vmName}`);
        nasState.vmSubTab = 'hardware';
        nasState.activeTab = 'kvm';
        navigateTo('kvm');
        close();
      },
    });

    // VM Action: Lifecycle (Start / Shutdown / Reboot)
    if (isRunning) {
      items.push({
        id: `vm:${vmName}:shutdown`,
        category: 'Actions',
        title: `${vmName}: ACPI Shutdown`,
        subtitle: `Send graceful ACPI shutdown signal to ${vmName}`,
        icon: mdiPower,
        badge: 'Shutdown',
        keywords: [vmName.toLowerCase(), 'shutdown', 'stop', 'halt', 'acpi', 'poweroff'],
        action: async () => {
          close();
          const ok = await confirmModal({
            title: `Shutdown VM "${vmName}"?`,
            message: `Send an ACPI shutdown signal to virtual machine "${vmName}"?`,
            tone: 'warning',
            confirmText: 'Shutdown VM',
          });
          if (!ok) return;
          await runAsyncAction(`VM "${vmName}": ACPI Shutdown`, () => services.kvm.stopVirtualMachine(vm.name, false));
        },
      });
      items.push({
        id: `vm:${vmName}:reboot`,
        category: 'Actions',
        title: `${vmName}: Reboot`,
        subtitle: `Trigger graceful ACPI reboot of ${vmName}`,
        icon: mdiRestart,
        badge: 'Reboot',
        keywords: [vmName.toLowerCase(), 'reboot', 'restart', 'reset'],
        action: async () => {
          close();
          const ok = await confirmModal({
            title: `Reboot VM "${vmName}"?`,
            message: `Send an ACPI reboot signal to virtual machine "${vmName}"?`,
            tone: 'warning',
            confirmText: 'Reboot VM',
          });
          if (!ok) return;
          await runAsyncAction(`VM "${vmName}": Reboot`, () => services.kvm.rebootVirtualMachine(vm.name, false));
        },
      });
    } else {
      items.push({
        id: `vm:${vmName}:start`,
        category: 'Actions',
        title: `${vmName}: Start`,
        subtitle: `Power on virtual machine ${vmName}`,
        icon: mdiPlay,
        badge: 'Start',
        keywords: [vmName.toLowerCase(), 'start', 'run', 'up', 'launch', 'power', 'on', 'boot'],
        action: () => runAsyncAction(`VM "${vmName}": Start`, () => services.kvm.startVirtualMachine(vm.name)),
      });
    }
  }

  // =========================================================================
  // 2. LXC Containers (Resources & Contextual Actions)
  // =========================================================================
  for (const ct of nasState.lxcContainers) {
    const name = ct.containerName || ct.name.replace(/^lxcContainers\//, '');
    const cleanId = ct.name.replace(/^lxcContainers\//, '');
    const isRunning = ct.state === 'RUNNING';

    // LXC Resource
    items.push({
      id: `lxc:${name}`,
      category: 'Resources',
      title: name,
      subtitle: `LXC Container · ${ct.state || 'STOPPED'} · IP: ${ct.ipv4Address || 'None'}`,
      icon: mdiCubeOutline,
      badge: ct.state,
      keywords: ['lxc', 'ct', 'container', 'linux', name.toLowerCase()],
      action: () => {
        selectResource(`lxc:${name}`);
        nasState.activeTab = 'lxc';
        navigateTo('lxc');
        close();
      },
    });

    // LXC Action: Terminal / Console
    items.push({
      id: `lxc:${name}:console`,
      category: 'Actions',
      title: `${name}: Terminal / Console`,
      subtitle: `Open interactive root shell inside container ${name}`,
      icon: mdiConsole,
      badge: 'Terminal',
      keywords: [
        name.toLowerCase(),
        'console',
        'con',
        'term',
        'terminal',
        'sh',
        'bash',
        'shell',
        'cli',
        'root',
        'exec',
      ],
      action: () => {
        selectResource(`lxc:${name}`);
        nasState.lxcSubTab = 'console';
        nasState.activeTab = 'lxc';
        navigateTo('lxc');
        close();
      },
    });

    // LXC Action: Pop-out Terminal
    items.push({
      id: `lxc:${name}:popout`,
      category: 'Actions',
      title: `${name}: Pop-out Terminal`,
      subtitle: `Launch detached LXC terminal in separate window`,
      icon: mdiOpenInNew,
      badge: 'Popout',
      keywords: [
        name.toLowerCase(),
        'pop',
        'popout',
        'window',
        'external',
        'terminal',
        'shell',
        'term',
        'con',
      ],
      action: () => {
        const url = `/terminal?lxc=${encodeURIComponent(cleanId)}`;
        window.open(
          url,
          `lxc_terminal_${cleanId}`,
          'width=1000,height=650,location=no,toolbar=no,menubar=no,status=no,resizable=yes'
        );
        close();
      },
    });

    // LXC Action: Configuration & Mounts
    items.push({
      id: `lxc:${name}:config`,
      category: 'Actions',
      title: `${name}: Configuration`,
      subtitle: `Configure CPU, memory, mounts, and network for ${name}`,
      icon: mdiCogOutline,
      badge: 'Config',
      keywords: [name.toLowerCase(), 'config', 'conf', 'mounts', 'network', 'rootfs', 'settings'],
      action: () => {
        selectResource(`lxc:${name}`);
        nasState.lxcSubTab = 'configuration';
        nasState.activeTab = 'lxc';
        navigateTo('lxc');
        close();
      },
    });

    // LXC Lifecycle (Start / Shutdown / Reboot)
    if (isRunning) {
      items.push({
        id: `lxc:${name}:shutdown`,
        category: 'Actions',
        title: `${name}: Shutdown`,
        subtitle: `Gracefully stop LXC container ${name}`,
        icon: mdiPower,
        badge: 'Shutdown',
        keywords: [name.toLowerCase(), 'stop', 'shutdown', 'halt', 'poweroff'],
        action: async () => {
          close();
          const ok = await confirmModal({
            title: `Stop Container "${name}"?`,
            message: `Gracefully stop LXC container "${name}"?`,
            tone: 'warning',
            confirmText: 'Stop Container',
          });
          if (!ok) return;
          await runAsyncAction(`LXC "${name}": Shutdown`, () => services.lxc.stopContainer(ct.name));
        },
      });
      items.push({
        id: `lxc:${name}:reboot`,
        category: 'Actions',
        title: `${name}: Reboot`,
        subtitle: `Reboot LXC container ${name}`,
        icon: mdiRestart,
        badge: 'Reboot',
        keywords: [name.toLowerCase(), 'reboot', 'restart', 'reset'],
        action: async () => {
          close();
          const ok = await confirmModal({
            title: `Reboot Container "${name}"?`,
            message: `Restart LXC container "${name}"?`,
            tone: 'warning',
            confirmText: 'Reboot Container',
          });
          if (!ok) return;
          await runAsyncAction(`LXC "${name}": Reboot`, () => services.lxc.restartContainer(ct.name));
        },
      });
    } else {
      items.push({
        id: `lxc:${name}:start`,
        category: 'Actions',
        title: `${name}: Start`,
        subtitle: `Start LXC container ${name}`,
        icon: mdiPlay,
        badge: 'Start',
        keywords: [name.toLowerCase(), 'start', 'run', 'up', 'launch', 'boot'],
        action: () => runAsyncAction(`LXC "${name}": Start`, () => services.lxc.startContainer(ct.name)),
      });
    }
  }

  // =========================================================================
  // 3. Docker Applications (Resources & Contextual Actions)
  // =========================================================================
  for (const app of nasState.dockerApps) {
    const title = app.title || app.appId || app.name.replace(/^dockerApps\//, '');
    const rawId = app.appId || title;
    const isRunning = app.state === 'RUNNING';

    // Docker Resource
    items.push({
      id: `docker:${rawId}`,
      category: 'Resources',
      title,
      subtitle: `Docker Application · ${app.state || 'STOPPED'} · ${app.containers?.length || 0} containers`,
      icon: mdiLayers,
      badge: app.state,
      keywords: ['docker', 'app', 'application', 'compose', 'stack', title.toLowerCase(), rawId.toLowerCase()],
      action: () => {
        selectResource(`docker:${rawId}`);
        nasState.activeTab = 'docker';
        navigateTo('docker');
        close();
      },
    });

    // Docker Action: Console / Terminal
    items.push({
      id: `docker:${rawId}:console`,
      category: 'Actions',
      title: `${title}: Console / Terminal`,
      subtitle: `Open interactive container terminal for ${title}`,
      icon: mdiConsole,
      badge: 'Console',
      keywords: [
        title.toLowerCase(),
        rawId.toLowerCase(),
        'console',
        'con',
        'term',
        'terminal',
        'sh',
        'bash',
        'shell',
        'cli',
        'exec',
      ],
      action: () => {
        selectResource(`docker:${rawId}`);
        nasState.dockerSubTab = 'console';
        nasState.activeTab = 'docker';
        navigateTo('docker');
        close();
      },
    });

    // Docker Action: Logs
    items.push({
      id: `docker:${rawId}:logs`,
      category: 'Actions',
      title: `${title}: Logs`,
      subtitle: `View stdout and stderr log stream for ${title}`,
      icon: mdiFileDocumentOutline,
      badge: 'Logs',
      keywords: [
        title.toLowerCase(),
        rawId.toLowerCase(),
        'log',
        'logs',
        'output',
        'stdout',
        'stderr',
        'tail',
      ],
      action: () => {
        selectResource(`docker:${rawId}`);
        nasState.dockerSubTab = 'logs';
        nasState.activeTab = 'docker';
        navigateTo('docker');
        close();
      },
    });

    // Docker Action: Configuration / Compose
    items.push({
      id: `docker:${rawId}:config`,
      category: 'Actions',
      title: `${title}: Configuration`,
      subtitle: `Edit Docker Compose specification and environment variables for ${title}`,
      icon: mdiCogOutline,
      badge: 'Config',
      keywords: [
        title.toLowerCase(),
        rawId.toLowerCase(),
        'config',
        'conf',
        'configuration',
        'compose',
        'yaml',
        'env',
        'edit',
        'settings',
      ],
      action: () => {
        selectResource(`docker:${rawId}`);
        nasState.dockerSubTab = 'configuration';
        nasState.activeTab = 'docker';
        navigateTo('docker');
        close();
      },
    });

    // Docker Action: Containers List
    items.push({
      id: `docker:${rawId}:containers`,
      category: 'Actions',
      title: `${title}: Containers`,
      subtitle: `Inspect individual containers and port bindings for ${title}`,
      icon: mdiLayers,
      badge: 'Containers',
      keywords: [
        title.toLowerCase(),
        rawId.toLowerCase(),
        'container',
        'containers',
        'ports',
        'ps',
        'inspect',
      ],
      action: () => {
        selectResource(`docker:${rawId}`);
        nasState.dockerSubTab = 'containers';
        nasState.activeTab = 'docker';
        navigateTo('docker');
        close();
      },
    });

    // Docker Lifecycle (Start / Stop / Restart)
    if (isRunning) {
      items.push({
        id: `docker:${rawId}:restart`,
        category: 'Actions',
        title: `${title}: Restart`,
        subtitle: `Restart all containers in stack ${title}`,
        icon: mdiRestart,
        badge: 'Restart',
        keywords: [title.toLowerCase(), rawId.toLowerCase(), 'restart', 'reboot', 'reload'],
        action: async () => {
          close();
          const ok = await confirmModal({
            title: `Restart App "${title}"?`,
            message: `Restart all containers in Docker application "${title}"?`,
            tone: 'warning',
            confirmText: 'Restart App',
          });
          if (!ok) return;
          await runAsyncAction(`App "${title}": Restart`, () => services.apps.restartApp(app.name));
        },
      });
      items.push({
        id: `docker:${rawId}:stop`,
        category: 'Actions',
        title: `${title}: Stop`,
        subtitle: `Stop all containers in stack ${title}`,
        icon: mdiStop,
        badge: 'Stop',
        keywords: [title.toLowerCase(), rawId.toLowerCase(), 'stop', 'halt', 'down'],
        action: async () => {
          close();
          const ok = await confirmModal({
            title: `Stop App "${title}"?`,
            message: `Stop all containers in Docker application "${title}"?`,
            tone: 'warning',
            confirmText: 'Stop App',
          });
          if (!ok) return;
          await runAsyncAction(`App "${title}": Stop`, () => services.apps.stopApp(app.name));
        },
      });
    } else {
      items.push({
        id: `docker:${rawId}:start`,
        category: 'Actions',
        title: `${title}: Start`,
        subtitle: `Start all containers in stack ${title}`,
        icon: mdiPlay,
        badge: 'Start',
        keywords: [title.toLowerCase(), rawId.toLowerCase(), 'start', 'run', 'up', 'launch'],
        action: () => runAsyncAction(`App "${title}": Start`, () => services.apps.startApp(app.name)),
      });
    }
  }

  // =========================================================================
  // 4. Storage Pools (Resources & Actions)
  // =========================================================================
  for (const pool of nasState.pools) {
    const name = pool.poolName || pool.name.replace(/^zfsPools\//, '');
    items.push({
      id: `pool:${name}`,
      category: 'Resources',
      title: name,
      subtitle: `ZFS Pool · ${pool.state || 'ONLINE'} · Capacity: ${pool.capacityPercent ?? 0}%`,
      icon: mdiDatabase,
      badge: pool.state,
      keywords: ['pool', 'zfs', 'storage', 'vdev', name.toLowerCase()],
      action: () => {
        selectResource(`pool:${name}`);
        nasState.activeTab = 'storage-pools';
        navigateTo('storage-pools');
        close();
      },
    });

    // Pool Action: Scrub
    items.push({
      id: `pool:${name}:scrub`,
      category: 'Actions',
      title: `${name}: Scrub Pool`,
      subtitle: `Run OpenZFS data integrity verification scrub on ${name}`,
      icon: mdiRefresh,
      badge: 'Scrub',
      keywords: [name.toLowerCase(), 'scrub', 'verify', 'repair', 'check', 'zfs'],
      action: () => runAsyncAction(`ZFS Pool "${name}": Start Scrub`, () => services.zfs.scrubPool(pool.name)),
    });

    // Pool Action: Datasets & Volumes
    items.push({
      id: `pool:${name}:datasets`,
      category: 'Actions',
      title: `${name}: Datasets & Volumes`,
      subtitle: `Browse datasets, snapshots, and filesystem volumes in ${name}`,
      icon: mdiDatabase,
      badge: 'Datasets',
      keywords: [name.toLowerCase(), 'dataset', 'datasets', 'volumes', 'fs', 'snapshots'],
      action: () => {
        selectResource(`pool:${name}`);
        nasState.poolSubTab = 'datasets';
        nasState.activeTab = 'storage-pools';
        navigateTo('storage-pools');
        close();
      },
    });
  }

  // =========================================================================
  // 5. System Navigation Sections
  // =========================================================================
  items.push(
    {
      id: 'nav:overview',
      category: 'Navigation',
      title: 'Dashboard Overview',
      subtitle: 'System telemetry, KPI gauges, CPU/RAM/Network meters',
      icon: mdiServer,
      keywords: ['dashboard', 'overview', 'telemetry', 'kpi', 'cpu', 'ram', 'memory', 'meters', 'stats'],
      action: () => {
        nasState.activeTab = 'overview';
        selectResource('node:local');
        navigateTo('overview');
        close();
      },
    },
    {
      id: 'nav:vms',
      category: 'Navigation',
      title: 'Virtual Machines',
      subtitle: 'KVM hypervisor, noVNC consoles, and guest disks',
      icon: mdiMonitor,
      keywords: ['vms', 'virtual', 'machines', 'kvm', 'qemu', 'hypervisor'],
      action: () => {
        nasState.activeTab = 'kvm';
        selectResource('node:vms');
        navigateTo('kvm');
        close();
      },
    },
    {
      id: 'nav:lxc',
      category: 'Navigation',
      title: 'LXC Containers',
      subtitle: 'System containers, OS templates, and mount points',
      icon: mdiCubeOutline,
      keywords: ['lxc', 'containers', 'linux', 'templates', 'mounts'],
      action: () => {
        nasState.activeTab = 'lxc';
        selectResource('node:lxc');
        navigateTo('lxc');
        close();
      },
    },
    {
      id: 'nav:docker',
      category: 'Navigation',
      title: 'Docker Applications',
      subtitle: 'Compose stacks, App Store catalog, container logs',
      icon: mdiLayers,
      keywords: ['docker', 'apps', 'applications', 'compose', 'catalog', 'stacks'],
      action: () => {
        nasState.activeTab = 'docker';
        selectResource('node:docker');
        navigateTo('docker');
        close();
      },
    },
    {
      id: 'nav:storage-pools',
      category: 'Navigation',
      title: 'Storage Pools',
      subtitle: 'OpenZFS pool health, VDEV topologies, and scrub status',
      icon: mdiDatabase,
      keywords: ['storage', 'pools', 'zfs', 'vdev', 'scrub'],
      action: () => {
        nasState.activeTab = 'storage-pools';
        selectResource('node:pools');
        navigateTo('storage-pools');
        close();
      },
    },
    {
      id: 'nav:storage-disks',
      category: 'Navigation',
      title: 'Physical Disks',
      subtitle: 'NVMe, SATA drives, S.M.A.R.T. health, and temperature',
      icon: mdiDatabase,
      keywords: ['disks', 'physical', 'smart', 'nvme', 'sata', 'drives', 'hdd', 'ssd'],
      action: () => {
        nasState.activeTab = 'storage-disks';
        selectResource('node:disks');
        navigateTo('storage-disks');
        close();
      },
    },
    {
      id: 'nav:shares-cifs',
      category: 'Navigation',
      title: 'Samba / CIFS Shares',
      subtitle: 'Windows file shares and network access control',
      icon: mdiFolderNetwork,
      keywords: ['samba', 'smb', 'cifs', 'shares', 'network', 'windows'],
      action: () => {
        nasState.activeTab = 'shares-cifs';
        selectResource('node:shares');
        navigateTo('shares-cifs');
        close();
      },
    },
    {
      id: 'nav:shares-webdav',
      category: 'Navigation',
      title: 'WebDAV Shares',
      subtitle: 'HTTP-based file shares and remote storage',
      icon: mdiFolderNetwork,
      keywords: ['webdav', 'dav', 'http', 'shares', 'remote'],
      action: () => {
        nasState.activeTab = 'shares-webdav';
        selectResource('node:shares');
        navigateTo('shares-webdav');
        close();
      },
    },
    {
      id: 'nav:identity',
      category: 'Navigation',
      title: 'Users & Identity',
      subtitle: 'POSIX user accounts, groups, and SSH keys',
      icon: mdiAccountMultiple,
      keywords: ['users', 'groups', 'identity', 'auth', 'accounts', 'ssh'],
      action: () => {
        nasState.activeTab = 'identity';
        selectResource('node:identity');
        navigateTo('identity');
        close();
      },
    },
    {
      id: 'nav:cron',
      category: 'Navigation',
      title: 'Scheduled Tasks & Cron Jobs',
      subtitle: 'Periodic maintenance, automation scripts, and timers',
      icon: mdiClockOutline,
      keywords: ['cron', 'jobs', 'scheduled', 'tasks', 'automation', 'timers'],
      action: () => {
        nasState.activeTab = 'cron';
        selectResource('node:local');
        navigateTo('cron');
        close();
      },
    }
  );

  // =========================================================================
  // 6. Global System Actions
  // =========================================================================
  items.push(
    {
      id: 'act:terminal',
      category: 'System',
      title: 'Open Host Web Terminal',
      subtitle: 'Launch detached interactive root bash terminal',
      icon: mdiConsole,
      badge: 'Terminal',
      keywords: ['host', 'terminal', 'term', 'con', 'console', 'bash', 'root', 'shell', 'ssh'],
      action: () => {
        emit('open-host-terminal');
        close();
      },
    },
    {
      id: 'act:refresh',
      category: 'System',
      title: 'Refresh All System Telemetry',
      subtitle: 'Re-fetch pools, containers, VMs, and system metrics',
      icon: mdiRefresh,
      keywords: ['refresh', 'reload', 'sync', 'update', 'telemetry'],
      action: () => {
        refreshAllData();
        close();
      },
    },
    {
      id: 'act:prereq',
      category: 'System',
      title: 'System Prerequisites & Health Status',
      subtitle: 'Audit kernel modules, OpenZFS, Docker, and QEMU-KVM',
      icon: mdiInformationOutline,
      keywords: ['prereq', 'prerequisites', 'health', 'audit', 'check', 'kernel', 'modules'],
      action: () => {
        emit('open-prerequisites');
        close();
      },
    },
    {
      id: 'act:system-details',
      category: 'System',
      title: 'System Hardware Specs & Host Details',
      subtitle: 'View host CPU architecture, motherboard, and OS version',
      icon: mdiServer,
      keywords: ['system', 'specs', 'hardware', 'motherboard', 'cpu', 'details', 'host', 'os'],
      action: () => {
        emit('open-system-details');
        close();
      },
    }
  );

  return items;
});

function isWordBoundary(str: string, idx: number): boolean {
  if (idx === 0) return true;
  const prev = str[idx - 1];
  return /[^a-zA-Z0-9]/.test(prev);
}

function fuzzyScore(pattern: string, target: string): { matched: boolean; score: number } {
  pattern = pattern.toLowerCase();
  target = target.toLowerCase();

  if (!pattern) return { matched: true, score: 0 };
  if (pattern.length > target.length) return { matched: false, score: 0 };

  if (target === pattern) return { matched: true, score: 400 };
  if (target.startsWith(pattern)) return { matched: true, score: 250 + pattern.length * 5 };

  let pIdx = 0;
  let tIdx = 0;
  let score = 0;
  let prevMatchIdx = -1;
  let consecutiveMatches = 0;

  while (pIdx < pattern.length && tIdx < target.length) {
    const pChar = pattern[pIdx];
    let bestTIdx = -1;

    // Check if immediate next character is consecutive match
    if (target[tIdx] === pChar) {
      bestTIdx = tIdx;
    } else {
      // Look ahead for matching character, preferring word boundary
      for (let i = tIdx; i < target.length; i++) {
        if (target[i] === pChar) {
          if (bestTIdx === -1) bestTIdx = i;
          if (isWordBoundary(target, i)) {
            bestTIdx = i;
            break;
          }
        }
      }
    }

    if (bestTIdx === -1) {
      return { matched: false, score: 0 };
    }

    tIdx = bestTIdx;
    let charScore = 10;

    if (isWordBoundary(target, tIdx)) {
      charScore += 35;
    }

    if (prevMatchIdx !== -1 && tIdx === prevMatchIdx + 1) {
      consecutiveMatches++;
      charScore += 25 * consecutiveMatches;
    } else {
      consecutiveMatches = 0;
      if (prevMatchIdx !== -1) {
        const gap = tIdx - prevMatchIdx - 1;
        charScore -= Math.min(gap * 2, 20);
      }
    }

    score += charScore;
    prevMatchIdx = tIdx;
    pIdx++;
    tIdx++;
  }

  if (pIdx < pattern.length) {
    return { matched: false, score: 0 };
  }

  return { matched: true, score };
}

const filteredItems = computed<CommandItem[]>(() => {
  const query = searchQuery.value.trim().toLowerCase();
  if (!query) {
    return allItems.value;
  }

  const tokens = query.split(/\s+/).filter(Boolean);
  const combinedNoSpaces = query.replace(/\s+/g, '');

  const matched: { item: CommandItem; score: number }[] = [];

  for (const item of allItems.value) {
    const title = item.title.toLowerCase();
    const badge = (item.badge || '').toLowerCase();
    const keywords = (item.keywords || []).map((k) => k.toLowerCase()).join(' ');
    const composite = `${title} ${badge} ${keywords}`;

    let score = 0;
    let matches = false;

    // 1. Direct or fuzzy match of combined string across title or composite
    // (e.g. "qualo" matches "Quasar: Logs", "quaterm" matches "Quasar: Console / Terminal")
    const titleFuzzy = fuzzyScore(combinedNoSpaces, title);
    const compositeFuzzy = fuzzyScore(combinedNoSpaces, composite);

    if (titleFuzzy.matched) {
      matches = true;
      score = Math.max(score, titleFuzzy.score * 1.5);
    } else if (compositeFuzzy.matched) {
      matches = true;
      score = Math.max(score, compositeFuzzy.score);
    }

    // 2. Multi-token match if spaces present (e.g. "qua lo" or "term qua")
    if (tokens.length > 1) {
      let tokensScore = 0;
      let allTokensMatch = true;

      for (const token of tokens) {
        const tFuzzyTitle = fuzzyScore(token, title);
        const tFuzzyComp = fuzzyScore(token, composite);
        if (tFuzzyTitle.matched) {
          tokensScore += tFuzzyTitle.score * 1.5;
        } else if (tFuzzyComp.matched) {
          tokensScore += tFuzzyComp.score;
        } else {
          allTokensMatch = false;
          break;
        }
      }

      if (allTokensMatch) {
        matches = true;
        score = Math.max(score, tokensScore);
      }
    }

    // Bonus for exact word prefix in title
    if (title.startsWith(query)) {
      score += 100;
    }

    // Single token boost for base Resources (e.g. "qua" -> Quasar over actions)
    if (tokens.length === 1 && item.category === 'Resources' && title.startsWith(query)) {
      score += 80;
    }

    if (matches && score > 0) {
      matched.push({ item, score });
    }
  }

  // Sort descending by score
  matched.sort((a, b) => b.score - a.score);

  return matched.map((m) => m.item);
});

watch(filteredItems, () => {
  selectedIndex.value = 0;
});

function handleKeydown(e: KeyboardEvent) {
  if (e.key === 'ArrowDown') {
    e.preventDefault();
    if (filteredItems.value.length > 0) {
      selectedIndex.value = (selectedIndex.value + 1) % filteredItems.value.length;
      scrollToSelected();
    }
  } else if (e.key === 'ArrowUp') {
    e.preventDefault();
    if (filteredItems.value.length > 0) {
      selectedIndex.value =
        (selectedIndex.value - 1 + filteredItems.value.length) % filteredItems.value.length;
      scrollToSelected();
    }
  } else if (e.key === 'Enter') {
    e.preventDefault();
    const item = filteredItems.value[selectedIndex.value];
    if (item) {
      item.action();
    }
  } else if (e.key === 'Escape') {
    e.preventDefault();
    close();
  }
}

function scrollToSelected() {
  nextTick(() => {
    const el = document.querySelector(`.palette-item.active`);
    if (el) {
      el.scrollIntoView({ block: 'nearest' });
    }
  });
}

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      searchQuery.value = '';
      selectedIndex.value = 0;
      nextTick(() => {
        searchInput.value?.focus();
      });
    }
  }
);
</script>

<template>
  <div v-if="modelValue" class="palette-overlay" @click.self="close" @keydown="handleKeydown">
    <div class="palette-container">
      <!-- Search Input Bar -->
      <div class="palette-search-bar">
        <MdiIcon :path="mdiMagnify" size="20" style="color: var(--text-muted); flex-shrink: 0" />
        <input
          ref="searchInput"
          v-model="searchQuery"
          type="text"
          class="palette-input"
          placeholder="Fuzzy search resources & actions (e.g. 'qualo', 'quaterm', 'quacon', 'wincon')..."
          spellcheck="false"
        />
        <kbd class="palette-kbd">ESC</kbd>
      </div>

      <!-- Results List -->
      <div class="palette-results">
        <div v-if="filteredItems.length === 0" class="palette-empty">
          <span>No resources or actions matching "{{ searchQuery }}"</span>
        </div>
        <div
          v-for="(item, idx) in filteredItems"
          :key="item.id"
          class="palette-item"
          :class="{ active: idx === selectedIndex }"
          @click="item.action"
          @mouseenter="selectedIndex = idx"
        >
          <div class="item-icon-box">
            <MdiIcon :path="item.icon" size="18" />
          </div>
          <div class="item-text">
            <div class="item-title-row">
              <span class="item-title">{{ item.title }}</span>
              <span
                v-if="item.badge"
                class="badge badge-sm"
                :class="{
                  'badge-success': item.badge === 'RUNNING' || item.badge === 'ONLINE' || item.badge === 'Start',
                  'badge-warning': item.badge === 'PAUSED' || item.badge === 'PARTIALLY_RUNNING' || item.badge === 'Reboot',
                  'badge-danger': item.badge === 'Shutdown' || item.badge === 'Stop',
                  'badge-secondary': item.badge === 'STOPPED',
                  'badge-info': item.badge === 'Console' || item.badge === 'Terminal' || item.badge === 'Logs' || item.badge === 'Config' || item.badge === 'Popout' || item.badge === 'Hardware' || item.badge === 'Datasets' || item.badge === 'Scrub'
                }"
              >
                {{ item.badge }}
              </span>
            </div>
            <span class="item-subtitle">{{ item.subtitle }}</span>
          </div>
          <span class="item-category">{{ item.category }}</span>
        </div>
      </div>

      <!-- Footer Quick Tips -->
      <div class="palette-footer">
        <div class="footer-tip">
          <kbd>↑</kbd><kbd>↓</kbd> <span>Navigate</span>
        </div>
        <div class="footer-tip">
          <kbd>↵</kbd> <span>Select & Run</span>
        </div>
        <div class="footer-tip">
          <kbd>ESC</kbd> <span>Close</span>
        </div>
        <div class="footer-tip hide-mobile" style="margin-left: auto; color: var(--text-muted)">
          <span>Fuzzy: <code style="color: var(--accent-primary)">qualo</code> · <code style="color: var(--accent-primary)">quacon</code> · <code style="color: var(--accent-primary)">quaterm</code> · <code style="color: var(--accent-primary)">wincon</code></span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.palette-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.65);
  backdrop-filter: blur(4px);
  z-index: 9999;
  display: flex;
  align-items: flex-start;
  justify-content: center;
  padding-top: 12vh;
}

.palette-container {
  width: 100%;
  max-width: 650px;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: 10px;
  box-shadow: 0 16px 40px rgba(0, 0, 0, 0.45);
  overflow: hidden;
  display: flex;
  flex-direction: column;
  animation: palette-slide-in 0.14s ease-out;
}

@keyframes palette-slide-in {
  from {
    opacity: 0;
    transform: translateY(-12px) scale(0.98);
  }
  to {
    opacity: 1;
    transform: translateY(0) scale(1);
  }
}

.palette-search-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 18px;
  border-bottom: 1px solid var(--border-subtle);
  background: var(--bg-surface);
}

.palette-input {
  flex: 1;
  background: transparent;
  border: none;
  outline: none;
  font-size: 15px;
  color: var(--text-primary);
  font-family: inherit;
}

.palette-input::placeholder {
  color: var(--text-muted);
}

.palette-kbd {
  font-size: 10px;
  font-weight: 600;
  padding: 2px 6px;
  border-radius: 4px;
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  color: var(--text-secondary);
}

.palette-results {
  max-height: 400px;
  overflow-y: auto;
  padding: 6px 0;
}

.palette-empty {
  padding: 32px;
  text-align: center;
  color: var(--text-muted);
  font-size: 13px;
}

.palette-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 18px;
  cursor: pointer;
  transition: all 0.08s ease;
  border-left: 3px solid transparent;
}

.palette-item:hover,
.palette-item.active {
  background: var(--bg-hover);
  border-left-color: var(--accent-primary);
}

.item-icon-box {
  width: 32px;
  height: 32px;
  border-radius: 6px;
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-secondary);
  flex-shrink: 0;
}

.palette-item.active .item-icon-box {
  color: var(--accent-primary);
  border-color: var(--accent-primary);
}

.item-text {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.item-title-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.item-title {
  font-weight: 600;
  font-size: 13.5px;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.item-subtitle {
  font-size: 11.5px;
  color: var(--text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.item-category {
  font-size: 10px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: var(--text-muted);
  flex-shrink: 0;
}

.badge-info {
  background: rgba(59, 130, 246, 0.15);
  color: #60a5fa;
  border: 1px solid rgba(59, 130, 246, 0.3);
}

.palette-footer {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 8px 18px;
  background: var(--bg-base);
  border-top: 1px solid var(--border-subtle);
  font-size: 11.5px;
  color: var(--text-muted);
}

.footer-tip {
  display: flex;
  align-items: center;
  gap: 4px;
}

.footer-tip kbd {
  font-size: 10px;
  padding: 1px 4px;
  border-radius: 3px;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  color: var(--text-secondary);
}

@media (max-width: 600px) {
  .palette-overlay {
    padding: 0;
    align-items: stretch;
  }
  .palette-container {
    height: 100vh;
    max-height: 100vh;
    border-radius: 0;
  }
  .palette-results {
    max-height: none;
    flex: 1;
  }
}
</style>
