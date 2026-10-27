import { beforeEach, describe, expect, it, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import OverviewView from '../src/ui/views/OverviewView.vue';
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

    expect(wrapper.text()).toContain('AMD Ryzen 9 9950X 16-Core Processor');
    expect(wrapper.text()).toContain('16 Physical Cores · 32 Logical Threads');
    expect(wrapper.text()).toContain('48.5 °C');
    expect(wrapper.text()).toContain('0.65'); // load1
    expect(wrapper.text()).toContain('0.82'); // load5
    expect(wrapper.text()).toContain('0.95'); // load15
  });

  it('renders RAM information with pie chart breakdown and formatted values', () => {
    const wrapper = mount(OverviewView);

    expect(wrapper.text()).toContain('Memory');
    expect(wrapper.text()).toContain('Services & Apps');
    expect(wrapper.text()).toContain('ZFS ARC Cache');
    expect(wrapper.text()).toContain('Buffers / Cache');
    expect(wrapper.text()).toContain('Free RAM');
  });

  it('renders fastfetch / neofetch hardware specification card', () => {
    const wrapper = mount(OverviewView);

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

  describe('Dashboard Customization, Drag-and-Drop, and Resizing', () => {
    it('enters and exits edit mode when clicking customize dashboard', async () => {
      const wrapper = mount(OverviewView);
      expect(wrapper.find('.dashboard-edit-banner').exists()).toBe(false);

      // Click Customize Dashboard
      const editBtn = wrapper.find('.edit-dashboard-btn');
      expect(editBtn.exists()).toBe(true);
      await editBtn.trigger('click');

      // Edit banner should be visible
      expect(wrapper.find('.dashboard-edit-banner').exists()).toBe(true);
      expect(wrapper.find('.widget-edit-toolbar').exists()).toBe(true);

      // Click Done Editing
      const doneBtn = wrapper.find('.done-edit-btn');
      expect(doneBtn.exists()).toBe(true);
      await doneBtn.trigger('click');

      expect(wrapper.find('.dashboard-edit-banner').exists()).toBe(false);
      expect(wrapper.find('.widget-edit-toolbar').exists()).toBe(false);
    });

    it('allows resizing a widget using column span pills in edit mode', async () => {
      const wrapper = mount(OverviewView);
      await wrapper.find('.edit-dashboard-btn').trigger('click');

      const firstWidget = wrapper.find('.widget-wrapper');
      expect(firstWidget.classes()).toContain('col-span-12');

      // Find span buttons for the first widget (kpis allowed [6, 12])
      const spanButtons = firstWidget.findAll('.span-btn');
      expect(spanButtons.length).toBeGreaterThanOrEqual(2);

      // Click the 1/2 span button
      const halfBtn = spanButtons.find((btn) => btn.text().includes('1/2'));
      expect(halfBtn).toBeDefined();
      await halfBtn!.trigger('click');

      // Should now have class col-span-6
      expect(firstWidget.classes()).toContain('col-span-6');
      expect(store.get('nas_dashboard_layout_v1')).toContain('"colSpan":6');
    });

    it('allows removing a widget from the dashboard', async () => {
      const wrapper = mount(OverviewView);
      await wrapper.find('.edit-dashboard-btn').trigger('click');

      const initialCount = wrapper.findAll('.widget-wrapper').length;
      expect(initialCount).toBe(6);

      // Remove the first widget
      const removeBtn = wrapper.find('.remove-btn');
      await removeBtn.trigger('click');

      expect(wrapper.findAll('.widget-wrapper').length).toBe(initialCount - 1);
      expect(store.get('nas_dashboard_layout_v1')).toBeDefined();
    });

    it('allows reordering widgets using move left/right buttons', async () => {
      const wrapper = mount(OverviewView);
      await wrapper.find('.edit-dashboard-btn').trigger('click');

      let widgets = wrapper.findAll('.widget-wrapper');
      const firstLabel = widgets[0].find('.widget-type-label').text();
      const secondLabel = widgets[1].find('.widget-type-label').text();
      expect(firstLabel).not.toEqual(secondLabel);

      // Move first widget right
      const moveRightBtn = widgets[0].find('.move-right-btn');
      await moveRightBtn.trigger('click');

      widgets = wrapper.findAll('.widget-wrapper');
      expect(widgets[1].find('.widget-type-label').text()).toEqual(firstLabel);
      expect(widgets[0].find('.widget-type-label').text()).toEqual(secondLabel);
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
      const wrapper = mount(OverviewView);
      await wrapper.find('.edit-dashboard-btn').trigger('click');

      // Remove a couple widgets
      await wrapper.find('.remove-btn').trigger('click');
      await wrapper.find('.remove-btn').trigger('click');
      expect(wrapper.findAll('.widget-wrapper').length).toBe(4);

      // Click Reset Default
      await wrapper.find('.reset-layout-btn').trigger('click');
      expect(wrapper.findAll('.widget-wrapper').length).toBe(6);
    });

    it('reorders widgets via drag and drop', async () => {
      const wrapper = mount(OverviewView);
      await wrapper.find('.edit-dashboard-btn').trigger('click');

      let widgets = wrapper.findAll('.widget-wrapper');
      const firstLabel = widgets[0].find('.widget-type-label').text();
      const secondLabel = widgets[1].find('.widget-type-label').text();

      // Drag first widget over second and drop
      const dataTransfer = {
        setData: vi.fn(),
        getData: vi.fn(),
        effectAllowed: '',
      };

      await widgets[0].trigger('dragstart', { dataTransfer });
      expect(widgets[0].classes()).toContain('is-dragging');

      await widgets[1].trigger('dragover');
      expect(widgets[1].classes()).toContain('is-drag-over');

      await widgets[1].trigger('drop');
      await widgets[0].trigger('dragend');

      widgets = wrapper.findAll('.widget-wrapper');
      expect(widgets[0].find('.widget-type-label').text()).toBe(secondLabel);
      expect(widgets[1].find('.widget-type-label').text()).toBe(firstLabel);
    });
  });
});
