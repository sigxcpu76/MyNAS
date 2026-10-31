import { describe, it, expect, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import {
  nasState,
  selectResource,
} from '../src/ui/store';
import VmDetailView from '../src/ui/views/VmDetailView.vue';
import LxcDetailView from '../src/ui/views/LxcDetailView.vue';
import DockerDetailView from '../src/ui/views/DockerDetailView.vue';
import ResourceTree from '../src/ui/components/ResourceTree.vue';
import ActionRibbon from '../src/ui/components/ActionRibbon.vue';

const localStorageMock = (() => {
  let store: Record<string, string> = {};
  return {
    getItem: (key: string) => store[key] || null,
    setItem: (key: string, value: string) => {
      store[key] = value.toString();
    },
    removeItem: (key: string) => {
      delete store[key];
    },
    clear: () => {
      store = {};
    },
  };
})();

Object.defineProperty(window, 'localStorage', {
  value: localStorageMock,
  writable: true,
});

describe('Proxmox-Style Resource Detail Views (VM, LXC, Docker)', () => {
  beforeEach(() => {
    localStorageMock.clear();
    nasState.vms = [
      {
        name: 'virtualMachines/debian-server',
        vmName: 'debian-server',
        uuid: 'uuid-1',
        state: 'RUNNING',
        vcpus: 4,
        memoryMib: 8192,
        diskPath: '/dev/zvol/tank/vm-disk-1',
        diskSizeGib: 32,
        isoImage: '',
        networkBridge: 'br0',
        vncPort: 5900,
        vncDisplay: ':0',
        osVariant: 'debian12',
        description: '',
        guestCpuPercent: 12.5,
        guestMemoryUsedMib: 1536,
        autostart: true,
        agentRunning: true,
        guestOsName: 'Debian 12 Bookworm',
        guestIpAddresses: ['192.168.27.150'],
        disks: [
          {
            id: 'scsi0',
            bus: 'scsi',
            diskPath: '/dev/zvol/tank/vm-disk-1',
            sizeBytes: 34359738368, // 32 GiB
            cacheMode: 'none',
          },
        ],
        networkInterfaces: [
          {
            id: 'net0',
            model: 'virtio',
            bridge: 'br0',
            macAddress: '52:54:00:12:34:56',
          },
        ],
      },
    ];

    nasState.lxcContainers = [
      {
        name: 'lxcContainers/alpine-app',
        containerName: 'alpine-app',
        state: 'RUNNING',
        distribution: 'alpine',
        release: '3.19',
        architecture: 'amd64',
        ipv4Address: '192.168.27.180',
        autostart: true,
        unprivileged: true,
        rootfsPath: '/var/lib/lxc/alpine-app/rootfs',
        memoryLimitMib: 2048,
        cpuCores: 2,
        cpuPercent: 3.4,
        memoryUsedMib: 256,
        mountPoints: [
          {
            hostPath: '/mnt/tank/media',
            containerPath: '/media',
            readOnly: false,
          },
        ],
      },
    ];

    nasState.dockerApps = [
      {
        name: 'dockerApps/nextcloud',
        appId: 'nextcloud',
        title: 'Nextcloud Hub',
        state: 'RUNNING',
        statusText: 'All services healthy',
        autoStart: true,
        composeYaml: 'services:\n  app:\n    image: nextcloud:latest\n',
        composeFilePath: '/etc/docker/compose/nextcloud/docker-compose.yml',
        services: ['app', 'db'],
        portMappings: ['8080:80'],
        volumes: ['nextcloud-data:/var/www/html'],
        webUiUrl: 'http://192.168.27.25:8080',
        totalCpuPercent: 8.2,
        totalMemoryMib: 512,
        createdAt: '2026-10-30T00:00:00Z',
        updatedAt: '2026-10-30T00:00:00Z',
        containers: [
          {
            containerId: 'c1234567890',
            name: 'nextcloud-app-1',
            serviceName: 'app',
            image: 'nextcloud:latest',
            state: 'running',
            status: 'Up 2 hours',
            cpuPercent: 5.1,
            memoryMib: 320,
            ports: ['8080:80'],
            createdAt: '2026-10-30T00:00:00Z',
          },
        ],
      },
    ];
  });

  it('renders VmDetailView with real telemetry and hides configured resource allocations', () => {
    nasState.selectedResourceId = 'vm:debian-server';
    const ribbon = mount(ActionRibbon);
    expect(ribbon.text()).toContain('debian-server');
    expect(ribbon.text()).toContain('VM');
    expect(ribbon.text()).toContain('RUNNING');
    expect(ribbon.text()).toContain('12.5%');
    expect(ribbon.text()).toContain('1.50 GiB');
    expect(ribbon.text()).toContain('Shutdown');
    expect(ribbon.text()).toContain('Stop');
    expect(ribbon.text()).toContain('Reboot');
    expect(ribbon.text()).toContain('Console Pop-out');

    const wrapper = mount(VmDetailView, {
      props: {
        vmName: 'debian-server',
      },
    });

    expect(wrapper.text()).toContain('12.5%'); // Real CPU
    expect(wrapper.text()).toContain('1.50 GiB'); // Real Memory (1536 MiB)
    expect(wrapper.text()).toContain('RUNNING');
    expect(wrapper.text()).toContain('Debian 12 Bookworm');
    // Ensure configured limits (4 vCPUs, 8192 MiB) are not shown in the telemetry card
    expect(wrapper.text()).not.toContain('8192 MiB');
  });

  it('renders LxcDetailView with real telemetry, mounts, and subtabs', () => {
    nasState.selectedResourceId = 'lxc:alpine-app';
    const ribbon = mount(ActionRibbon);
    expect(ribbon.text()).toContain('alpine-app');
    expect(ribbon.text()).toContain('LXC');
    expect(ribbon.text()).toContain('RUNNING');
    expect(ribbon.text()).toContain('3.4%');
    expect(ribbon.text()).toContain('256 MiB');
    expect(ribbon.text()).toContain('Stop');
    expect(ribbon.text()).toContain('Reboot');
    expect(ribbon.text()).toContain('Freeze');
    expect(ribbon.text()).toContain('Terminal');

    const wrapper = mount(LxcDetailView, {
      props: {
        containerName: 'alpine-app',
      },
    });

    expect(wrapper.text()).toContain('3.4%'); // Real CPU
    expect(wrapper.text()).toContain('256 MiB'); // Real Memory
    expect(wrapper.text()).toContain('192.168.27.180');
    expect(wrapper.text()).toContain('/mnt/tank/media');
    expect(wrapper.text()).toContain('/media');
    // Ensure configured limits (2048 MiB) are not shown in telemetry
    expect(wrapper.text()).not.toContain('2048 MiB');
  });

  it('renders DockerDetailView with containers, compose specification, and actions', () => {
    nasState.selectedResourceId = 'docker:nextcloud';
    const ribbon = mount(ActionRibbon);
    expect(ribbon.text()).toContain('Nextcloud Hub');
    expect(ribbon.text()).toContain('APP');
    expect(ribbon.text()).toContain('RUNNING');
    expect(ribbon.text()).toContain('8.2%');
    expect(ribbon.text()).toContain('512 MiB');
    expect(ribbon.text()).toContain('Stop');
    expect(ribbon.text()).toContain('Restart');
    expect(ribbon.text()).toContain('Web UI');
    expect(ribbon.text()).toContain('Upgrade');
    expect(ribbon.text()).toContain('Delete');

    const wrapper = mount(DockerDetailView, {
      props: {
        appName: 'nextcloud',
      },
    });

    expect(wrapper.text()).toContain('8.2%'); // Total Real CPU
    expect(wrapper.text()).toContain('512 MiB'); // Total Real Memory
    expect(wrapper.text()).toContain('8080:80');
    expect(wrapper.text()).toContain('nextcloud-app-1');
  });

  it('ResourceTree includes Overview nodes for VMs, Containers, and Docker Apps without dockerApps prefix', () => {
    const wrapper = mount(ResourceTree);
    expect(wrapper.text()).toContain('VMs Overview');
    expect(wrapper.text()).toContain('Containers Overview');
    expect(wrapper.text()).toContain('Apps Overview');
    expect(wrapper.text()).toContain('debian-server');
    expect(wrapper.text()).toContain('alpine-app');
    expect(wrapper.text()).toContain('Nextcloud Hub');
    expect(wrapper.text()).not.toContain('dockerApps/');
  });
});
