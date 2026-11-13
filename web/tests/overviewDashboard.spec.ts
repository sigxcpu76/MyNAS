import { beforeEach, describe, expect, it, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import OverviewView from '../src/ui/views/OverviewView.vue';
import WidgetCpu from '../src/ui/dashboard/widgets/WidgetCpu.vue';
import WidgetMemory from '../src/ui/dashboard/widgets/WidgetMemory.vue';
import WidgetSystem from '../src/ui/dashboard/widgets/WidgetSystem.vue';
import WidgetSensors from '../src/ui/dashboard/widgets/WidgetSensors.vue';
import WidgetVms from '../src/ui/dashboard/widgets/WidgetVms.vue';
import WidgetApps from '../src/ui/dashboard/widgets/WidgetApps.vue';
import WidgetShares from '../src/ui/dashboard/widgets/WidgetShares.vue';
import WidgetWorkloads from '../src/ui/dashboard/widgets/WidgetWorkloads.vue';
import WidgetDiskIo from '../src/ui/dashboard/widgets/WidgetDiskIo.vue';
import WidgetZpoolIo from '../src/ui/dashboard/widgets/WidgetZpoolIo.vue';
import SystemDetailsTopBarButton from '../src/ui/components/SystemDetailsTopBarButton.vue';
import SystemDetailsModal from '../src/ui/components/SystemDetailsModal.vue';
import { nasState } from '../src/ui/store';
import { formatUptime, type ZfsPool } from '../src/domain/models';

const store = new Map<string, string>();
const localStorageMock = {
  getItem: (key: string) => store.get(key) ?? null,
  setItem: (key: string, val: string) => store.set(key, String(val)),
  removeItem: (key: string) => store.delete(key),
  clear: () => store.clear(),
};

Object.defineProperty(window, 'localStorage', {
  value: localStorageMock,
  writable: true,
});

describe('Overview Dashboard & Telemetry', () => {
  beforeEach(() => {
    localStorageMock.clear();

    // Populate mock state
    nasState.pools = [
      {
        name: 'zfsPools/tank',
        poolName: 'tank',
        state: 'ONLINE',
        sizeBytes: 4000000000000,
        allocatedBytes: 1500000000000,
        freeBytes: 2500000000000,
        fragmentationPercent: 12,
        capacityPercent: 37,
        dedupRatio: 1.25,
        vdevLayout: 'raidz1',
        mountpoint: '/nas/tank',
        compression: 'lz4',
        lastScrubStatus: 'Finished with 0 errors',
      } as ZfsPool,
      {
        name: 'zfsPools/backup',
        poolName: 'backup',
        state: 'ONLINE',
        sizeBytes: 8000000000000,
        allocatedBytes: 6000000000000,
        freeBytes: 2000000000000,
        fragmentationPercent: 5,
        capacityPercent: 75,
        dedupRatio: 1.0,
        vdevLayout: 'mirror',
        mountpoint: '/nas/backup',
        compression: 'zstd',
        lastScrubStatus: 'Finished with 0 errors',
      } as ZfsPool,
    ];

    nasState.systemTelemetry = {
      cpu: {
        model: 'AMD Ryzen 9 9950X 16-Core Processor',
        physicalCores: 16,
        logicalCores: 32,
        usagePercent: 24.5,
        load1: 0.65,
        load5: 0.82,
        load15: 0.95,
        temperatureCelsius: 48.5,
      },
      memory: {
        totalBytes: 68719476736, // 64 GiB
        freeBytes: 17179869184, // 16 GiB
        availableBytes: 42949672960,
        usedBytes: 25769803776, // 24 GiB
        zfsArcBytes: 12884901888, // 12 GiB
        zfsArcMaxBytes: 34359738368,
        buffersBytes: 4294967296,
        cachedBytes: 17179869184,
        servicesBytes: 12884901888, // 12 GiB
      },
      host: {
        hostname: 'titan-nas',
        osName: 'Ubuntu 24.04 LTS',
        osVersion: '24.04',
        kernelVersion: '6.8.0-45-generic',
        architecture: 'x86_64',
        uptimeSeconds: 185420, // ~2 days 3h 30m
        motherboardVendor: 'ASUS',
        motherboardName: 'ROG STRIX X870-E',
        motherboardVersion: 'Rev 1.0',
        biosVendor: 'American Megatrends Inc.',
        biosVersion: '0805',
        biosDate: '09/12/2024',
        gpus: ['NVIDIA Corporation GA102GL [RTX A5000]', 'AMD Radeon RX 6600'],
      },
      network: [
        {
          name: 'enp15s0',
          ipAddresses: ['192.168.1.100', 'fe80::1'],
          macAddress: '00:11:22:33:44:55',
          isUp: true,
          speedMbps: 2500,
          rxBytes: 524288000,
          txBytes: 1048576000,
          rxRateBytesPerSec: 1048576, // 1 MiB/s
          txRateBytesPerSec: 2097152, // 2 MiB/s
          isDefault: true,
        },
        {
          name: 'docker0',
          ipAddresses: ['172.17.0.1'],
          macAddress: '02:42:0a:11:22:33',
          isUp: true,
          speedMbps: 0,
          rxBytes: 12000,
          txBytes: 34000,
          rxRateBytesPerSec: 0,
          txRateBytesPerSec: 0,
          isDefault: false,
        },
      ],
      diskIo: [
        {
          name: 'nvme0n1',
          readBytes: 1073741824,
          writeBytes: 2147483648,
          readRateBytesPerSec: 52428800,
          writeRateBytesPerSec: 26214400,
          readIops: 450,
          writeIops: 220,
          ioInProgress: 2,
          utilizationPercent: 34.5,
        },
      ],
      zpoolIo: [
        {
          poolName: 'tank',
          readBytes: 4294967296,
          writeBytes: 8589934592,
          readRateBytesPerSec: 104857600,
          writeRateBytesPerSec: 52428800,
          readIops: 800,
          writeIops: 400,
          waitQueue: 1,
          activeQueue: 4,
        },
      ],
    };
  });

  it('does NOT display prerequisites anywhere on the overview dashboard', () => {
    const wrapper = mount(OverviewView);
    const html = wrapper.html().toLowerCase();

    // Verify prerequisites section, tables and headers are not displayed
    expect(html).not.toContain('system prerequisites health');
    expect(html).not.toContain('inspect prerequisites');
    expect(html).not.toContain('prerequisites/prerequisite');
  });

  it('renders one card per ZFS storage pool with usage and fragmentation sections', () => {
    const wrapper = mount(OverviewView);
    const poolCards = wrapper.findAll('.pool-telemetry-card');

    expect(poolCards).toHaveLength(2);

    // Verify Tank pool
    const tankCard = poolCards[0];
    expect(tankCard.text()).toContain('tank');
    expect(tankCard.text()).toContain('raidz1');
    expect(tankCard.text()).toContain('Usage');
    expect(tankCard.text()).toContain('Frag');
    expect(tankCard.text()).toContain('12%'); // Frag pct
    expect(tankCard.text()).toContain('1.25x'); // Dedup

    // Verify Backup pool
    const backupCard = poolCards[1];
    expect(backupCard.text()).toContain('backup');
    expect(backupCard.text()).toContain('mirror');
    expect(backupCard.text()).toContain('5%');
  });

  it('renders CPU telemetry with load averages, core counts and temperature', () => {
    const wrapper = mount(OverviewView);

    expect(wrapper.text()).toContain('Processor & Load');
    expect(wrapper.text()).toContain('32 Logical Threads (16 Cores)');
    expect(wrapper.text()).toContain('48.5 °C');
    expect(wrapper.text()).toContain('0.65'); // load1
    expect(wrapper.text()).toContain('0.82'); // load5
  });

  it('renders RAM information and formatted values in overview KPI', () => {
    const wrapper = mount(OverviewView);

    expect(wrapper.text()).toContain('Memory (RAM)');
    expect(wrapper.text()).toContain('Services: 12 GiB');
    expect(wrapper.text()).toContain('ARC: 12 GiB');
    expect(wrapper.text()).toContain('Free: 16 GiB');
  });

  it('renders catalog WidgetMemory and WidgetCpu with chart breakdowns when added', () => {
    const memWrapper = mount(WidgetMemory);
    expect(memWrapper.text()).toContain('Memory');
    expect(memWrapper.text()).toContain('Services & Apps');
    expect(memWrapper.text()).toContain('ZFS ARC Cache');
    expect(memWrapper.text()).toContain('Buffers / Cache');
    expect(memWrapper.text()).toContain('Free RAM');

    const cpuWrapper = mount(WidgetCpu);
    expect(cpuWrapper.text()).toContain('Processor');
    expect(cpuWrapper.text()).toContain('AMD Ryzen 9 9950X 16-Core Processor');
    expect(cpuWrapper.text()).toContain('16 Physical Cores · 32 Logical Threads');
  });

  it('renders fastfetch / neofetch hardware specification card when WidgetSystem is used', () => {
    const wrapper = mount(WidgetSystem);

    const fastfetch = wrapper.find('.fastfetch-card');
    expect(fastfetch.exists()).toBe(true);
    expect(fastfetch.text()).toContain('titan-nas');
    expect(fastfetch.text()).toContain('Ubuntu 24.04 LTS');
    expect(fastfetch.text()).toContain('6.8.0-45-generic');
    expect(fastfetch.text()).toContain('ROG STRIX X870-E');
    expect(fastfetch.text()).toContain('American Megatrends Inc.');
    expect(fastfetch.text()).toContain('RTX A5000');
    expect(fastfetch.text()).toContain('AMD Radeon RX 6600');
    expect(fastfetch.find('.ascii-art').exists()).toBe(false);
  });

  it('renders system details top bar button and opens modal with detailed specs', async () => {
    const btnWrapper = mount(SystemDetailsTopBarButton);
    expect(btnWrapper.text()).toContain('titan-nas');
    expect(btnWrapper.text()).toContain('2d 3h 30m');

    await btnWrapper.trigger('click');
    expect(btnWrapper.emitted('click')).toBeTruthy();

    const modalWrapper = mount(SystemDetailsModal);
    expect(modalWrapper.text()).toContain('System Information');
    expect(modalWrapper.text()).toContain('ROG STRIX X870-E');
    expect(modalWrapper.text()).toContain('AMD Ryzen 9 9950X 16-Core Processor');
    expect(modalWrapper.text()).toContain('American Megatrends Inc.');
    expect(modalWrapper.text()).toContain('enp15s0');
  });

  it('renders system network interface with IPs, MAC, and live in/out throughput graph', () => {
    const wrapper = mount(OverviewView);

    const netCard = wrapper.find('.system-network-card');
    expect(netCard.exists()).toBe(true);

    // Displays system interface only
    expect(netCard.text()).toContain('enp15s0');
    expect(netCard.text()).not.toContain('Primary / System');
    expect(netCard.text()).toContain('ONLINE');
    expect(netCard.text()).toContain('192.168.1.100');
    expect(netCard.text()).toContain('00:11:22:33:44:55');
    expect(netCard.text()).toContain('2500 Mbps');

    // Does NOT render secondary virtual bridge (docker0)
    expect(netCard.text()).not.toContain('docker0');
    expect(netCard.text()).not.toContain('172.17.0.1');

    // Inbound and Outbound throughput rates & graph
    expect(netCard.text()).toContain('Inbound (RX)');
    expect(netCard.text()).toContain('Outbound (TX)');
    expect(netCard.text()).toContain('In / Out Throughput History');
    expect(netCard.find('.traffic-chart-wrapper').exists()).toBe(true);
  });

  it('formats uptime correctly', () => {
    expect(formatUptime(0)).toBe('0m');
    expect(formatUptime(120)).toBe('2m');
    expect(formatUptime(3665)).toBe('1h 1m');
    expect(formatUptime(90060)).toBe('1d 1h 1m');
  });

  describe('Dashboard Customization, Fluid Waterfall Lanes, and Reordering', () => {
    it('enters and exits edit mode when clicking customize dashboard', async () => {
      const wrapper = mount(OverviewView);
      expect(wrapper.find('.dashboard-edit-banner').exists()).toBe(false);

      // Click Customize Dashboard
      const editBtn = wrapper.find('.edit-dashboard-btn');
      expect(editBtn.exists()).toBe(true);
      await editBtn.trigger('click');

      // Edit banner should be visible
      expect(wrapper.find('.dashboard-edit-banner').exists()).toBe(true);
      expect(wrapper.find('.widget-lane-toolbar').exists()).toBe(true);

      // Click Done Editing
      const doneBtn = wrapper.find('.done-edit-btn');
      expect(doneBtn.exists()).toBe(true);
      await doneBtn.trigger('click');

      expect(wrapper.find('.dashboard-edit-banner').exists()).toBe(false);
      expect(wrapper.find('.widget-lane-toolbar').exists()).toBe(false);
    });

    it('renders top ribbon for KPIs and fluid waterfall columns for cards', () => {
      const wrapper = mount(OverviewView);
      expect(wrapper.find('.dashboard-top-ribbon').exists()).toBe(true);
      expect(wrapper.find('.dashboard-lanes').exists()).toBe(true);

      const lanes = wrapper.findAll('.dashboard-lane');
      expect(lanes.length).toBe(3);
    });

    it('allows shifting a widget to another column using move left/right buttons', async () => {
      const wrapper = mount(OverviewView);
      await wrapper.find('.edit-dashboard-btn').trigger('click');

      const lanes = wrapper.findAll('.dashboard-lane');
      const lane0Widgets = lanes[0].findAll('.lane-widget-card');
      expect(lane0Widgets.length).toBeGreaterThanOrEqual(1);

      // Move widget from Column 1 to Column 2 (move-right-btn)
      const moveRightBtn = lane0Widgets[0].find('.move-right-btn');
      expect(moveRightBtn.exists()).toBe(true);
      await moveRightBtn.trigger('click');

      // Check that layout persisted to localStorage with new lane
      expect(store.get('nas_dashboard_layout_v4')).toContain('"lane":1');
    });

    it('allows reordering a widget within its column using move up/down buttons', async () => {
      const wrapper = mount(OverviewView);
      await wrapper.find('.edit-dashboard-btn').trigger('click');

      // Add a second widget to Column 1 (Disks)
      await wrapper.find('.add-widget-btn').trigger('click');
      const catalogCards = wrapper.findAll('.catalog-card');
      const disksCard = catalogCards.find((c) => c.text().includes('Physical Disks'));
      await disksCard!.find('button.btn').trigger('click');
      await wrapper.find('.modal-footer button').trigger('click');

      // Column 1 now has 3 widgets
      const lanes = wrapper.findAll('.dashboard-lane');
      const lane0Widgets = lanes[0].findAll('.lane-widget-card');
      expect(lane0Widgets.length).toBe(3);

      const firstLabel = lane0Widgets[0].find('.widget-type-name').text();
      const secondLabel = lane0Widgets[1].find('.widget-type-name').text();

      // Move the second widget up
      const moveUpBtn = lane0Widgets[1].find('.move-up-btn');
      await moveUpBtn.trigger('click');

      const updatedWidgets = wrapper.findAll('.dashboard-lane')[0].findAll('.lane-widget-card');
      expect(updatedWidgets[0].find('.widget-type-name').text()).toBe(secondLabel);
      expect(updatedWidgets[1].find('.widget-type-name').text()).toBe(firstLabel);
    });

    it('allows removing a widget from the dashboard', async () => {
      const wrapper = mount(OverviewView);
      await wrapper.find('.edit-dashboard-btn').trigger('click');

      const initialCount = wrapper.findAll('.widget-wrapper').length;
      expect(initialCount).toBe(6);

      // Remove the first lane widget
      const removeBtn = wrapper.find('.remove-btn');
      await removeBtn.trigger('click');

      expect(wrapper.findAll('.widget-wrapper').length).toBe(initialCount - 1);
      expect(store.get('nas_dashboard_layout_v4')).toBeDefined();
    });

    it('allows adding a new widget from the widget catalog modal', async () => {
      const wrapper = mount(OverviewView);
      await wrapper.find('.edit-dashboard-btn').trigger('click');

      const initialCount = wrapper.findAll('.widget-wrapper').length;

      // Open Add Widget Modal
      await wrapper.find('.add-widget-btn').trigger('click');
      expect(wrapper.find('.modal-dialog').exists()).toBe(true);
      expect(wrapper.text()).toContain('Add Widget to Dashboard');

      // Find Disks widget card in catalog
      const catalogCards = wrapper.findAll('.catalog-card');
      const disksCard = catalogCards.find((c) => c.text().includes('Physical Disks'));
      expect(disksCard).toBeDefined();

      const addBtn = disksCard!.find('button.btn');
      await addBtn.trigger('click');

      // Close modal
      await wrapper.find('.modal-footer button').trigger('click');
      expect(wrapper.find('.modal-dialog').exists()).toBe(false);

      // Should have 1 more widget and include disks
      const widgets = wrapper.findAll('.widget-wrapper');
      expect(widgets.length).toBe(initialCount + 1);
      expect(wrapper.text()).toContain('Physical Disks');
    });

    it('resets layout to default when clicking reset default', async () => {
      const { closeActiveModal } = await import('../src/ui/modal');
      const wrapper = mount(OverviewView);
      await wrapper.find('.edit-dashboard-btn').trigger('click');

      // Remove a widget
      await wrapper.find('.remove-btn').trigger('click');
      expect(wrapper.findAll('.widget-wrapper').length).toBe(5);

      // Click Reset Default and confirm modal
      const clickPromise = wrapper.find('.reset-layout-btn').trigger('click');
      closeActiveModal(true);
      await clickPromise;
      await wrapper.vm.$nextTick();
      expect(wrapper.findAll('.widget-wrapper').length).toBe(6);
    });

    it('allows switching between 2 Columns and 3 Columns in edit mode', async () => {
      const wrapper = mount(OverviewView);
      await wrapper.find('.edit-dashboard-btn').trigger('click');

      const toggleButtons = wrapper.findAll('.col-toggle-btn');
      expect(toggleButtons.length).toBe(2);

      // Click 2 Columns
      const twoColBtn = toggleButtons.find((b) => b.text().includes('2 Columns'));
      expect(twoColBtn).toBeDefined();
      await twoColBtn!.trigger('click');

      expect(wrapper.find('.columns-2').exists()).toBe(true);
    });

    it('applies balanced layout setup when clicking balanced layout button', async () => {
      const { closeActiveModal } = await import('../src/ui/modal');
      const wrapper = mount(OverviewView);
      await wrapper.find('.edit-dashboard-btn').trigger('click');

      const balancedBtn = wrapper.find('.balanced-layout-btn');
      expect(balancedBtn.exists()).toBe(true);
      const clickPromise = balancedBtn.trigger('click');
      closeActiveModal(true);
      await clickPromise;
      await wrapper.vm.$nextTick();

      const widgets = wrapper.findAll('.widget-wrapper');
      expect(widgets.length).toBe(9);

      expect(store.get('nas_dashboard_layout_v4')).toContain('widget-pools');
      expect(store.get('nas_dashboard_layout_v4')).toContain('widget-disks');
      expect(store.get('nas_dashboard_layout_v4')).toContain('widget-sensors');
    });

    it('migrates legacy saved layout (v3/v2/v1) and distributes widgets into lanes', () => {
      const legacyLayout = [
        { id: 'widget-kpis', type: 'kpis', colSpan: 12 },
        { id: 'widget-pools', type: 'pools', colSpan: 6 },
        { id: 'widget-sensors', type: 'sensors', colSpan: 6 },
      ];
      store.set('nas_dashboard_layout_v3', JSON.stringify(legacyLayout));

      const wrapper = mount(OverviewView);
      const widgets = wrapper.findAll('.widget-wrapper');
      expect(widgets.length).toBe(3);

      // KPIs in top ribbon
      expect(wrapper.find('.dashboard-top-ribbon').text()).toContain('Storage');
      // Pools in Column 1
      expect(wrapper.findAll('.dashboard-lane')[0].text()).toContain('tank');
    });
  });

  describe('WidgetSensors (Hardware Sensors, Power & Temperatures)', () => {
    it('renders power meter, GPU metrics, and temperature items', async () => {
      nasState.systemTelemetry = {
        cpu: {
          model: 'Intel Xeon E5-2680 v4',
          physicalCores: 28,
          logicalCores: 56,
          usagePercent: 12.5,
          load1: 1.2,
          load5: 0.8,
          load15: 0.5,
          temperatureCelsius: 41.0,
          coreTemperatures: [30, 32],
          packageTemperatures: [35, 41],
        },
        memory: {
          totalBytes: 67108864000,
          freeBytes: 30000000000,
          availableBytes: 40000000000,
          usedBytes: 27108864000,
          zfsArcBytes: 15000000000,
          zfsArcMaxBytes: 30000000000,
          buffersBytes: 100000000,
          cachedBytes: 5000000000,
          servicesBytes: 12108864000,
        },
        host: {
          hostname: 'nasulica',
          osName: 'Debian GNU/Linux',
          osVersion: '12',
          kernelVersion: '6.12.0',
          architecture: 'x86_64',
          uptimeSeconds: 86400,
          motherboardVendor: 'HP',
          motherboardName: 'ProLiant DL380 Gen9',
          motherboardVersion: '1.0',
          biosVendor: 'HP',
          biosVersion: 'P89',
          biosDate: '2020-05-21',
          gpus: ['NVIDIA GeForce RTX 4060 Ti'],
        },
        network: [],
        sensors: {
          lmSensorsInstalled: true,
          powerMeter: {
            deviceName: 'power_meter-acpi-0',
            adapter: 'ACPI interface',
            powerWatts: 152.0,
            intervalSeconds: 300.0,
          },
          gpus: [
            {
              index: 0,
              name: 'NVIDIA GeForce RTX 4060 Ti',
              driverVersion: '615.78.08',
              temperatureCelsius: 30.0,
              utilizationPercent: 15.0,
              memoryUsedBytes: 8311013376,
              memoryTotalBytes: 17179869184,
              powerWatts: 12.7,
              fanSpeedPercent: 0,
              vendor: 'NVIDIA',
            },
          ],
          temperatures: [
            {
              chip: 'coretemp-isa-0000',
              adapter: 'ISA adapter',
              label: 'Package id 0',
              category: 'CPU',
              currentValue: 35.0,
              averageValue: 31.5,
              unit: '°C',
              highThreshold: 93.0,
              critThreshold: 103.0,
              alarm: false,
              cores: [
                { label: 'Core 0', currentValue: 30.0 },
                { label: 'Core 1', currentValue: 33.0 },
              ],
            },
            {
              chip: 'tg3-pci-0200',
              adapter: 'PCI adapter',
              label: 'tg3-pci-0200: temp1',
              category: 'PCI',
              currentValue: 52.0,
              unit: '°C',
              critThreshold: 110.0,
              alarm: false,
            },
          ],
          fans: [
            {
              chip: 'it8696-isa-0a40',
              adapter: 'ISA adapter',
              label: 'fan1',
              category: 'FAN',
              currentValue: 1073.0,
              unit: 'RPM',
              highThreshold: 0,
              critThreshold: 0,
              alarm: false,
            },
          ],
        },
      };

      const wrapper = mount(WidgetSensors);
      expect(wrapper.text()).toContain('Hardware Sensors & Power');
      expect(wrapper.text()).toContain('152.0 W');
      expect(wrapper.text()).toContain('power_meter-acpi-0');
      expect(wrapper.text()).toContain('NVIDIA GeForce RTX 4060 Ti');
      expect(wrapper.text()).toContain('30.0°C');
      expect(wrapper.text()).toContain('CPU 0');
      expect(wrapper.text()).toContain('Avg: 31.5°C');
      expect(wrapper.text()).toContain('2 Cores');
      expect(wrapper.text()).toContain('tg3-pci-0200: temp1');
      expect(wrapper.text()).toContain('1073 RPM');

      // Click Show Cores toggle to expand cores breakdown
      const toggleBtn = wrapper.find('.cores-btn-toggle');
      expect(toggleBtn.exists()).toBe(true);
      expect(toggleBtn.text()).toContain('Show Cores');
      await toggleBtn.trigger('click');
      expect(wrapper.text()).toContain('Hide Cores');
      expect(wrapper.find('.cores-breakdown-grid').exists()).toBe(true);
      expect(wrapper.text()).toContain('C0');
      expect(wrapper.text()).toContain('C1');
    });

    it('renders dedicated WidgetVms with status chips and clean metadata without text clipping', () => {
      nasState.vms = [
        {
          name: 'vms/debian-server',
          vmName: 'debian-server',
          vcpus: 2,
          memoryMib: 4096,
          vncPort: 0,
          state: 'RUNNING',
        } as any,
        {
          name: 'vms/windows-10',
          vmName: 'windows-10',
          vcpus: 8,
          memoryMib: 16384,
          vncPort: 0,
          state: 'STOPPED',
        } as any,
      ];

      const wrapper = mount(WidgetVms);
      expect(wrapper.text()).toContain('Virtual Machines');
      expect(wrapper.text()).toContain('1/2'); // running / total
      expect(wrapper.text()).toContain('debian-server');
      expect(wrapper.text()).toContain('2 vCPU · 4096 MiB · VNC :0');
      expect(wrapper.text()).toContain('Running');
      expect(wrapper.text()).toContain('windows-10');
      expect(wrapper.text()).toContain('Stopped');
    });

    it('renders dedicated WidgetApps with Docker Compose and LXC containers', () => {
      nasState.dockerApps = [
        {
          name: 'dockerApps/clickhouse',
          appId: 'clickhouse',
          title: 'Clickhouse',
          state: 'RUNNING',
          statusText: '1/1 running',
          containers: [{ id: 'c1', name: 'ch', state: 'running' }],
        } as any,
      ];
      nasState.lxcContainers = [
        {
          name: 'lxcContainers/alpine',
          containerName: 'Alpine Linux',
          distribution: 'alpine',
          ipv4Address: '192.168.1.50',
          state: 'STOPPED',
        } as any,
      ];

      const wrapper = mount(WidgetApps);
      expect(wrapper.text()).toContain('Apps & Containers');
      expect(wrapper.text()).toContain('1/2');
      expect(wrapper.text()).toContain('Clickhouse');
      expect(wrapper.text()).toContain('APP');
      expect(wrapper.text()).toContain('Running');
      expect(wrapper.text()).toContain('Alpine Linux');
      expect(wrapper.text()).toContain('LXC');
      expect(wrapper.text()).toContain('Stopped');
    });

    it('renders dedicated WidgetShares with truncated paths and isolated RW/RO badges without overlap', () => {
      nasState.cifsShares = [
        {
          name: 'cifsShares/habackups',
          shareName: 'HABackups',
          path: '/mnt/storage/library/ha-backups',
          readOnly: false,
          enabled: true,
        } as any,
        {
          name: 'cifsShares/software',
          shareName: 'Software',
          path: '/mnt/raid0/torrents/downloads/Software',
          readOnly: false,
          enabled: true,
        } as any,
      ];
      nasState.webdavShares = [
        {
          name: 'webdavShares/movies',
          urlPrefix: '/dav/movies',
          path: '/mnt/raid0/torrents/downloads/Movies',
          readOnly: true,
          enabled: true,
        } as any,
      ];

      const wrapper = mount(WidgetShares);
      expect(wrapper.text()).toContain('Network Shares');
      expect(wrapper.text()).toContain('3/3');
      expect(wrapper.text()).toContain('//titan-nas/HABackups');
      expect(wrapper.text()).toContain('/mnt/storage/library/ha-backups');
      expect(wrapper.text()).toContain('//titan-nas/Software');
      expect(wrapper.text()).toContain('/mnt/raid0/torrents/downloads/Software');
      expect(wrapper.text()).toContain('/dav/movies');
      expect(wrapper.text()).toContain('SMB');
      expect(wrapper.text()).toContain('WEBDAV');

      const permBadges = wrapper.findAll('.perm-badge');
      expect(permBadges.length).toBe(3);
      expect(permBadges[0].classes()).toContain('perm-rw');
      expect(permBadges[0].text()).toBe('RW');
      expect(permBadges[2].classes()).toContain('perm-ro');
      expect(permBadges[2].text()).toBe('RO');
    });

    it('renders WidgetWorkloads unified wrapper displaying all three sub-widgets', () => {
      const wrapper = mount(WidgetWorkloads);
      expect(wrapper.findComponent(WidgetVms).exists()).toBe(true);
      expect(wrapper.findComponent(WidgetApps).exists()).toBe(true);
      expect(wrapper.findComponent(WidgetShares).exists()).toBe(true);
    });

    it('renders WidgetDiskIo with /proc/diskstats throughput, IOPS, and device table', () => {
      const wrapper = mount(WidgetDiskIo);
      expect(wrapper.text()).toContain('Disk I/O');
      expect(wrapper.text()).toContain('nvme0n1');
      expect(wrapper.text()).toContain('50 MiB/s');
      expect(wrapper.text()).toContain('25 MiB/s');
      expect(wrapper.text()).toContain('450 IOPS');
      expect(wrapper.text()).toContain('220 IOPS');
    });

    it('renders WidgetZpoolIo with zpool kstat iostat throughput, IOPS, and queues', () => {
      const wrapper = mount(WidgetZpoolIo);
      expect(wrapper.text()).toContain('Zpool IOStat');
      expect(wrapper.text()).toContain('tank');
      expect(wrapper.text()).toContain('100 MiB/s');
      expect(wrapper.text()).toContain('50 MiB/s');
      expect(wrapper.text()).toContain('800 IOPS');
      expect(wrapper.text()).toContain('400 IOPS');
      expect(wrapper.text()).toContain('Q 1/4');
    });
  });
});
