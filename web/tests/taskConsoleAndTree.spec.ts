import { describe, it, expect, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import {
  nasState,
  startTask,
  finishTask,
  appendTaskLog,
  clearTasks,
  runTrackedTask,
  selectResource,
  setTaskConsoleCollapsed,
  STORAGE_RESOURCE_KEY,
  STORAGE_TASK_COLLAPSED_KEY,
} from '../src/ui/store';
import TaskConsoleDock from '../src/ui/components/TaskConsoleDock.vue';
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

describe('Proxmox-Style Task Console & Resource System', () => {
  beforeEach(() => {
    nasState.tasks = [];
    nasState.selectedResourceId = 'node:local';
    nasState.isTaskConsoleCollapsed = false;
    localStorageMock.clear();
  });

  it('records, appends logs, and finishes tasks properly', () => {
    const task = startTask('VM "debian-server": Start', 'ID: 100', 'nas01', 'admin');
    expect(task.status).toBe('RUNNING');
    expect(task.description).toBe('VM "debian-server": Start');
    expect(task.node).toBe('nas01');
    expect(task.user).toBe('admin');
    expect(nasState.tasks.length).toBe(1);

    appendTaskLog(task.id, 'QEMU subprocess spawned (PID 4821)');
    expect(task.logs.some((l) => l.includes('PID 4821'))).toBe(true);

    finishTask(task.id, 'OK', 'Guest agent ready');
    expect(task.status).toBe('OK');
    expect(task.endTime).toBeDefined();
    expect(task.duration).toBeDefined();
    expect(task.logs.some((l) => l.includes('Guest agent ready'))).toBe(true);
  });

  it('runs async tracked tasks and reports OK / ERROR statuses', async () => {
    const result = await runTrackedTask('Pool "fast": Scrub', async () => {
      return 'scrub-started';
    });
    expect(result).toBe('scrub-started');
    expect(nasState.tasks[0].status).toBe('OK');

    await expect(
      runTrackedTask('Failing Operation', async () => {
        throw new Error('Device busy');
      })
    ).rejects.toThrow('Device busy');

    expect(nasState.tasks[0].status).toBe('ERROR');
    expect(nasState.tasks[0].logs.some((l) => l.includes('Device busy'))).toBe(true);
  });

  it('clears completed tasks and retains running ones', () => {
    const t1 = startTask('Task 1');
    finishTask(t1.id, 'OK');
    startTask('Running Task');

    expect(nasState.tasks.length).toBe(2);
    clearTasks();
    expect(nasState.tasks.length).toBe(1);
    expect(nasState.tasks[0].description).toBe('Running Task');
  });

  it('persists last active resource and task console collapsed state to localStorage', () => {
    selectResource('vm:debian-server');
    expect(nasState.selectedResourceId).toBe('vm:debian-server');
    expect(localStorageMock.getItem(STORAGE_RESOURCE_KEY)).toBe('vm:debian-server');

    setTaskConsoleCollapsed(true);
    expect(nasState.isTaskConsoleCollapsed).toBe(true);
    expect(localStorageMock.getItem(STORAGE_TASK_COLLAPSED_KEY)).toBe('true');
  });

  it('renders TaskConsoleDock with live status badges and filter controls', async () => {
    const t = startTask('VM "debian-server": Start');
    finishTask(t.id, 'OK');

    const wrapper = mount(TaskConsoleDock);
    expect(wrapper.text()).toContain('Task & Audit Log');
    expect(wrapper.text()).toContain('VM "debian-server": Start');
    expect(wrapper.text()).toContain('OK');

    // Test collapse toggle
    const toggleBtn = wrapper.find('.toggle-btn');
    await toggleBtn.trigger('click');
    expect(nasState.isTaskConsoleCollapsed).toBe(true);
  });

  it('renders ResourceTree with node hierarchy and status indicators', async () => {
    nasState.pools = [{ name: 'fast', poolName: 'fast', state: 'ONLINE', size: 1000, allocated: 500, free: 500 } as any];
    nasState.vms = [{ name: 'virtualMachines/debian-server', vmName: 'debian-server', state: 'RUNNING' } as any];
    nasState.lxcContainers = [{ name: 'lxcContainers/alpine', containerName: 'alpine', state: 'RUNNING' } as any];

    const wrapper = mount(ResourceTree);
    expect(wrapper.text()).toContain('Storage');
    expect(wrapper.text()).toContain('Virtual Machines');
    expect(wrapper.text()).toContain('LXC Containers');
    expect(wrapper.text()).toContain('fast');
    expect(wrapper.text()).toContain('debian-server');
    expect(wrapper.text()).toContain('alpine');
  });

  it('renders ActionRibbon with contextual actions for selected VM', async () => {
    nasState.selectedResourceId = 'vm:debian-server';
    nasState.vms = [
      {
        name: 'virtualMachines/debian-server',
        vmName: 'debian-server',
        state: 'RUNNING',
        guestCpuPercent: 5.5,
        guestMemoryUsedMib: 2048,
      } as any,
    ];

    const wrapper = mount(ActionRibbon);
    expect(wrapper.text()).toContain('debian-server');
    expect(wrapper.text()).toContain('RUNNING');
    expect(wrapper.text()).toContain('Shutdown');
    expect(wrapper.text()).toContain('Stop');
    expect(wrapper.text()).toContain('Console Pop-out');
  });
});
