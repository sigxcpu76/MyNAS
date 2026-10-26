/**
 * Real WebSocket noVNC Console Client Adapter (`rfbConsoleAdapter.ts`).
 * Connects to `ws(s)://<host>/ws/novnc/<vmName>?token=<token>` using the
 * real @novnc/novnc RFB client, rendering the live QEMU framebuffer onto
 * the target DOM container with full keyboard, mouse, and display support.
 */
import RFB from '@novnc/novnc';

export type ConsoleStatusCallback = (
  status: 'CONNECTING' | 'CONNECTED' | 'DISCONNECTED' | 'ERROR',
  detail?: string
) => void;

export interface RfbConsoleOptions {
  target: HTMLElement;
  vmName: string;
  token: string;
  websocketUrl?: string;
  onStatusChange?: ConsoleStatusCallback;
}

export class RfbConsoleAdapter {
  private rfb: RFB | null = null;
  private readonly target: HTMLElement;
  private readonly vmName: string;
  private readonly token: string;
  private readonly websocketUrl?: string;
  private readonly onStatusChange?: ConsoleStatusCallback;

  constructor(options: RfbConsoleOptions) {
    this.target = options.target;
    this.vmName = options.vmName;
    this.token = options.token;
    this.websocketUrl = options.websocketUrl;
    this.onStatusChange = options.onStatusChange;
  }

  static buildWebSocketUrl(vmName: string, token: string, explicitUrl?: string): string {
    if (explicitUrl && explicitUrl.startsWith('ws')) {
      const hasToken = explicitUrl.includes('token=');
      if (hasToken) return explicitUrl;
      const sep = explicitUrl.includes('?') ? '&' : '?';
      return `${explicitUrl}${sep}token=${encodeURIComponent(token)}`;
    }
    const proto =
      typeof window !== 'undefined' && window.location.protocol === 'https:'
        ? 'wss:'
        : 'ws:';
    const host =
      typeof window !== 'undefined' && window.location.host
        ? window.location.host
        : 'localhost:8080';
    const cleanPath =
      explicitUrl && explicitUrl.startsWith('/')
        ? explicitUrl.split('?')[0]
        : `/ws/novnc/${encodeURIComponent(vmName)}`;
    return `${proto}//${host}${cleanPath}?token=${encodeURIComponent(token)}`;
  }

  connect(): void {
    this.disconnect();
    this.onStatusChange?.('CONNECTING', 'Connecting to live QEMU VNC server...');

    const url = RfbConsoleAdapter.buildWebSocketUrl(
      this.vmName,
      this.token,
      this.websocketUrl
    );

    try {
      this.rfb = new RFB(this.target, url, {
        wsProtocols: ['binary'],
      });

      this.rfb.scaleViewport = true;
      this.rfb.resizeSession = false;
      this.rfb.focusOnClick = true;

      this.rfb.addEventListener('connect', () => {
        this.onStatusChange?.('CONNECTED', `Connected to ${this.vmName}`);
      });

      this.rfb.addEventListener('disconnect', (e: any) => {
        const clean = e?.detail?.clean;
        this.onStatusChange?.(
          'DISCONNECTED',
          clean ? 'Session closed' : 'Disconnected from VNC server'
        );
      });

      this.rfb.addEventListener('securityfailure', (e: any) => {
        this.onStatusChange?.(
          'ERROR',
          `Security negotiation failed: ${e?.detail?.reason || 'unauthorized'}`
        );
      });

      this.rfb.addEventListener('credentialsrequired', () => {
        this.onStatusChange?.('ERROR', 'VNC password required');
      });
    } catch (err: any) {
      this.onStatusChange?.('ERROR', err?.message || 'Failed to initialize noVNC client');
    }
  }

  disconnect(): void {
    if (this.rfb) {
      try {
        this.rfb.disconnect();
      } catch {
        // ignore
      }
      this.rfb = null;
    }
  }

  sendCtrlAltDel(): void {
    if (this.rfb) {
      this.rfb.sendCtrlAltDel();
    }
  }

  sendText(text: string): void {
    if (!this.rfb || !text) return;
    for (const char of text) {
      const code = char.charCodeAt(0);
      this.rfb.sendKey(code, char, true);
      this.rfb.sendKey(code, char, false);
    }
  }

  sendEnter(): void {
    if (!this.rfb) return;
    this.rfb.sendKey(0xff0d, 'Enter', true);
    this.rfb.sendKey(0xff0d, 'Enter', false);
  }

  focus(): void {
    this.rfb?.focus();
  }
}
