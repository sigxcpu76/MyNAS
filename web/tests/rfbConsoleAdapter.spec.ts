import { describe, it, expect, vi, beforeEach } from 'vitest';
import { RfbConsoleAdapter } from '../src/adapters/novnc/rfbConsoleAdapter';

// Mock @novnc/novnc RFB client
const mockRFBInstances: any[] = [];
vi.mock('@novnc/novnc', () => {
  return {
    default: vi.fn().mockImplementation((target: HTMLElement, url: string) => {
      const listeners: Record<string, Function[]> = {};
      const instance = {
        target,
        url,
        scaleViewport: false,
        clipViewport: false,
        resizeSession: false,
        focusOnClick: true,
        _display: {
          scale: 1.0,
        },
        addEventListener: vi.fn((event: string, cb: Function) => {
          listeners[event] = listeners[event] || [];
          listeners[event].push(cb);
        }),
        removeEventListener: vi.fn(),
        disconnect: vi.fn(),
        sendCtrlAltDel: vi.fn(),
        sendKey: vi.fn(),
        focus: vi.fn(),
        _trigger(event: string, detail?: any) {
          if (listeners[event]) {
            listeners[event].forEach((cb) => cb({ detail }));
          }
        },
      };
      mockRFBInstances.push(instance);
      return instance;
    }),
  };
});

describe('RfbConsoleAdapter', () => {
  let container: HTMLDivElement;

  beforeEach(() => {
    mockRFBInstances.length = 0;
    container = document.createElement('div');
    document.body.appendChild(container);
  });

  it('builds canonical WebSocket URL for noVNC endpoint', () => {
    const url = RfbConsoleAdapter.buildWebSocketUrl('debian-server', 'token-123');
    expect(url).toContain('/ws/novnc/debian-server?token=token-123');
  });

  it('initializes RFB and applies fit scale mode by default', () => {
    const statusFn = vi.fn();
    const adapter = new RfbConsoleAdapter({
      target: container,
      vmName: 'debian-server',
      token: 'tok-abc',
      onStatusChange: statusFn,
    });

    adapter.connect();

    expect(mockRFBInstances.length).toBe(1);
    const rfb = mockRFBInstances[0];
    expect(rfb.scaleViewport).toBe(true);

    // Simulate connect event
    rfb._trigger('connect');
    expect(statusFn).toHaveBeenCalledWith('CONNECTED', 'Connected to debian-server');
  });

  it('supports switching scaling modes between fit, 1:1, and custom zoom', () => {
    const adapter = new RfbConsoleAdapter({
      target: container,
      vmName: 'debian-server',
      token: 'tok-abc',
    });

    adapter.connect();
    const rfb = mockRFBInstances[0];

    // 1:1 native scale mode
    adapter.setScaleMode('1:1');
    expect(rfb.scaleViewport).toBe(false);
    expect(rfb._display.scale).toBe(1.0);

    // 150% zoom scale mode
    adapter.setScaleMode('150');
    expect(rfb.scaleViewport).toBe(false);
    expect(rfb._display.scale).toBe(1.5);

    // 200% zoom scale mode
    adapter.setScaleMode('200');
    expect(rfb.scaleViewport).toBe(false);
    expect(rfb._display.scale).toBe(2.0);

    // Fit scale mode
    adapter.setScaleMode('fit');
    expect(rfb.scaleViewport).toBe(true);
  });

  it('supports crisp vs smooth rendering toggling on canvas', () => {
    const canvas = document.createElement('canvas');
    container.appendChild(canvas);

    const adapter = new RfbConsoleAdapter({
      target: container,
      vmName: 'debian-server',
      token: 'tok-abc',
    });

    adapter.connect();
    const rfb = mockRFBInstances[0];
    rfb._trigger('connect');

    adapter.setCrisp(true);
    expect(canvas.style.imageRendering).toBe('pixelated');

    adapter.setCrisp(false);
    expect(canvas.style.imageRendering).toBe('auto');
  });

  it('sends Ctrl+Alt+Del and text to guest VM', () => {
    const adapter = new RfbConsoleAdapter({
      target: container,
      vmName: 'debian-server',
      token: 'tok-abc',
    });

    adapter.connect();
    const rfb = mockRFBInstances[0];

    adapter.sendCtrlAltDel();
    expect(rfb.sendCtrlAltDel).toHaveBeenCalled();

    adapter.sendText('ls -la');
    // Each char has keydown and keyup
    expect(rfb.sendKey).toHaveBeenCalledTimes(12);

    adapter.sendEnter();
    expect(rfb.sendKey).toHaveBeenCalledWith(0xff0d, 'Enter', true);
    expect(rfb.sendKey).toHaveBeenCalledWith(0xff0d, 'Enter', false);
  });
});
