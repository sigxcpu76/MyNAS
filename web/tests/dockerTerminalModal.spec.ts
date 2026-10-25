import { describe, expect, it, vi, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import DockerTerminalModal from '../src/ui/components/DockerTerminalModal.vue';

class MockWebSocket {
  static OPEN = 1;
  readyState = MockWebSocket.OPEN;
  binaryType = 'blob';
  onopen: (() => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  onmessage: ((ev: any) => void) | null = null;
  send = vi.fn();
  close = vi.fn();

  constructor(public url: string) {
    setTimeout(() => {
      this.onopen?.();
    }, 0);
  }
}

class MockResizeObserver {
  observe = vi.fn();
  unobserve = vi.fn();
  disconnect = vi.fn();
}

describe('DockerTerminalModal Component', () => {
  beforeEach(() => {
    vi.stubGlobal('WebSocket', MockWebSocket);
    vi.stubGlobal('ResizeObserver', MockResizeObserver);
    window.matchMedia = window.matchMedia || function() {
      return {
        matches: false,
        media: '',
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
      } as any;
    };
  });

  it('mounts properly and renders title and container options', async () => {
    const containers = [
      { id: 'c1', name: 'nginx-proxy-manager', serviceName: 'app', state: 'RUNNING' },
      { id: 'c2', name: 'db', serviceName: 'database', state: 'RUNNING' },
    ];

    const wrapper = mount(DockerTerminalModal, {
      props: {
        appTitle: 'Nginx Proxy Manager',
        containers,
        initialContainerId: 'c1',
      },
    });

    expect(wrapper.text()).toContain('Terminal:');
    expect(wrapper.text()).toContain('Nginx Proxy Manager');
    expect(wrapper.text()).toContain('nginx-proxy-manager');

    // Shell selector should be present with options
    const shellSelect = wrapper.find('select');
    expect(shellSelect.exists()).toBe(true);

    wrapper.unmount();
  });

  it('emits close event when close button is clicked', async () => {
    const containers = [
      { id: 'c1', name: 'nextcloud-app-1', serviceName: 'app', state: 'RUNNING' },
    ];

    const wrapper = mount(DockerTerminalModal, {
      props: {
        appTitle: 'Nextcloud',
        containers,
        initialContainerId: 'c1',
      },
    });

    const closeBtn = wrapper.find('button[title="Close Terminal"]');
    expect(closeBtn.exists()).toBe(true);

    await closeBtn.trigger('click');
    expect(wrapper.emitted('close')).toHaveLength(1);

    wrapper.unmount();
  });
});
