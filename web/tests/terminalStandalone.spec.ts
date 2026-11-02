import { describe, expect, it, vi, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import TerminalStandalone from '../src/ui/components/TerminalStandalone.vue';

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

describe('TerminalStandalone Component', () => {
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
  });

  it('routes to host terminal when no parameters are present', async () => {
    window.history.replaceState(null, '', '/terminal');

    const wrapper = mount(TerminalStandalone);
    await new Promise((r) => setTimeout(r, 10));

    expect(wrapper.text()).toContain('Host Shell (root)');
    expect(lastCreatedWsUrl).toContain('/ws/host/terminal');
  });

  it('routes to LXC container terminal when ?lxc=100 is given', async () => {
    window.history.replaceState(null, '', '/terminal?lxc=100');

    const wrapper = mount(TerminalStandalone);
    await new Promise((r) => setTimeout(r, 10));

    expect(wrapper.text()).toContain('LXC Terminal — 100');
    expect(lastCreatedWsUrl).toContain('/ws/lxc/terminal/100');
    expect(lastCreatedWsUrl).not.toContain('/ws/host/terminal');
  });

  it('routes to Docker container terminal when ?container=web is given', async () => {
    window.history.replaceState(null, '', '/terminal?container=web');

    const wrapper = mount(TerminalStandalone);
    await new Promise((r) => setTimeout(r, 10));

    expect(wrapper.text()).toContain('Docker Terminal — web');
    expect(lastCreatedWsUrl).toContain('/ws/docker/terminal/web');
    expect(lastCreatedWsUrl).not.toContain('/ws/host/terminal');
  });
});
