import { describe, expect, it, vi, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import HostTerminalModal from '../src/ui/components/HostTerminalModal.vue';
import { nasState } from '../src/ui/store';

let lastCreatedWsUrl = '';

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
    lastCreatedWsUrl = url;
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

describe('HostTerminalModal Component', () => {
  beforeEach(() => {
    lastCreatedWsUrl = '';
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
    nasState.currentUser = {
      id: 'sigxcpu',
      username: 'sigxcpu',
      isAdmin: true,
    } as any;
    nasState.systemTelemetry = {
      host: {
        hostname: 'mynas-host',
      },
    } as any;
  });

  it('mounts properly and renders title, host, and logged-in user', async () => {
    const wrapper = mount(HostTerminalModal);

    expect(wrapper.text()).toContain('Host Terminal');
    expect(wrapper.text()).toContain('mynas-host');
    expect(wrapper.text()).toContain('sigxcpu');
    expect(wrapper.text()).toContain('/etc/passwd');

    expect(lastCreatedWsUrl).toContain('/ws/host/terminal');

    wrapper.unmount();
  });

  it('emits close event when close button is clicked', async () => {
    const wrapper = mount(HostTerminalModal);

    const closeBtn = wrapper.find('button[aria-label="Close terminal"]');
    expect(closeBtn.exists()).toBe(true);

    await closeBtn.trigger('click');
    expect(wrapper.emitted('close')).toHaveLength(1);

    wrapper.unmount();
  });

  it('toggles fullscreen mode when fullscreen button is clicked', async () => {
    const wrapper = mount(HostTerminalModal);

    const fullscreenBtn = wrapper.find('button[aria-label="Fullscreen"]');
    expect(fullscreenBtn.exists()).toBe(true);

    await fullscreenBtn.trigger('click');
    expect(wrapper.find('.host-terminal-fullscreen').exists()).toBe(true);

    await fullscreenBtn.trigger('click');
    expect(wrapper.find('.host-terminal-fullscreen').exists()).toBe(false);

    wrapper.unmount();
  });

  it('triggers reconnect when refresh button is clicked', async () => {
    const wrapper = mount(HostTerminalModal);
    const initialUrl = lastCreatedWsUrl;
    expect(initialUrl).toContain('/ws/host/terminal');

    const reconnectBtn = wrapper.find('button[aria-label="Reconnect terminal session"]');
    expect(reconnectBtn.exists()).toBe(true);

    await reconnectBtn.trigger('click');
    expect(lastCreatedWsUrl).toContain('/ws/host/terminal');

    wrapper.unmount();
  });
});
