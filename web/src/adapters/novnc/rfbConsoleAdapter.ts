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

export type ScaleMode = 'fit' | '1:1' | '125' | '150' | '200';

export interface RfbConsoleOptions {
  target: HTMLElement;
  vmName: string;
  token: string;
  websocketUrl?: string;
  onStatusChange?: ConsoleStatusCallback;
  scaleMode?: ScaleMode;
  crisp?: boolean;
}

export class RfbConsoleAdapter {
  private rfb: RFB | null = null;
  private readonly target: HTMLElement;
  private readonly vmName: string;
  private token: string;
  private websocketUrl?: string;
  private readonly onStatusChange?: ConsoleStatusCallback;
  private scaleMode: ScaleMode = 'fit';
  private crisp: boolean = false;

  constructor(options: RfbConsoleOptions) {
    this.target = options.target;
    this.vmName = options.vmName;
    this.token = options.token;
    this.websocketUrl = options.websocketUrl;
    this.onStatusChange = options.onStatusChange;
    if (options.scaleMode) {
      this.scaleMode = options.scaleMode;
    }
    if (options.crisp !== undefined) {
      this.crisp = options.crisp;
    }
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

      this.rfb.resizeSession = false;
      this.rfb.focusOnClick = true;

      this.applyScaleMode();

      this.rfb.addEventListener('connect', () => {
        this.onStatusChange?.('CONNECTED', `Connected to ${this.vmName}`);
        this.applyScaleMode();
        this.setCrisp(this.crisp);
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

      if (typeof window !== 'undefined') {
        window.addEventListener('resize', this.handleWindowResize);
      }
    } catch (err: any) {
      this.onStatusChange?.('ERROR', err?.message || 'Failed to initialize noVNC client');
    }
  }

  private handleWindowResize = (): void => {
    if (this.scaleMode !== 'fit') {
      this.applyScaleMode();
    }
  };

  setScaleMode(mode: ScaleMode): void {
    this.scaleMode = mode;
    this.applyScaleMode();
  }

  updateCredentials(token: string, websocketUrl?: string): void {
    this.token = token;
    this.websocketUrl = websocketUrl;
  }

  setCrisp(crisp: boolean): void {
    this.crisp = crisp;
    const canvas = this.target.querySelector('canvas');
    if (canvas) {
      if (crisp) {
        canvas.style.imageRendering = 'pixelated';
      } else {
        canvas.style.imageRendering = 'auto';
      }
    }
  }

  private applyScaleMode(): void {
    if (!this.rfb) return;

    if (this.scaleMode === 'fit') {
      this.rfb.scaleViewport = true;
      this.rfb.clipViewport = false;
    } else {
      this.rfb.scaleViewport = false;
      this.rfb.clipViewport = false;
      const factor =
        this.scaleMode === '125' ? 1.25 :
        this.scaleMode === '150' ? 1.5 :
        this.scaleMode === '200' ? 2.0 : 1.0;

      const rfbAny = this.rfb as any;
      if (rfbAny._display) {
        rfbAny._display.scale = factor;
      }
    }
  }

  disconnect(): void {
    if (typeof window !== 'undefined') {
      window.removeEventListener('resize', this.handleWindowResize);
    }
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

  sendAltFx(fKeyNum: number): void {
    if (!this.rfb) return;
    const fKeysym = 0xffbd + fKeyNum;
    const fCode = `F${fKeyNum}`;
    this.rfb.sendKey(0xffe9, 'AltLeft', true);
    this.rfb.sendKey(fKeysym, fCode, true);
    this.rfb.sendKey(fKeysym, fCode, false);
    this.rfb.sendKey(0xffe9, 'AltLeft', false);
  }

  sendCtrlAltFx(fKeyNum: number): void {
    if (!this.rfb) return;
    const fKeysym = 0xffbd + fKeyNum;
    const fCode = `F${fKeyNum}`;
    this.rfb.sendKey(0xffe3, 'ControlLeft', true);
    this.rfb.sendKey(0xffe9, 'AltLeft', true);
    this.rfb.sendKey(fKeysym, fCode, true);
    this.rfb.sendKey(fKeysym, fCode, false);
    this.rfb.sendKey(0xffe9, 'AltLeft', false);
    this.rfb.sendKey(0xffe3, 'ControlLeft', false);
  }

  sendLinuxShortcut(
    shortcut:
      | 'alt-f1'
      | 'alt-f2'
      | 'alt-f3'
      | 'ctrl-alt-f1'
      | 'ctrl-alt-f2'
      | 'ctrl-alt-f3'
  ): void {
    switch (shortcut) {
      case 'alt-f1':
        this.sendAltFx(1);
        break;
      case 'alt-f2':
        this.sendAltFx(2);
        break;
      case 'alt-f3':
        this.sendAltFx(3);
        break;
      case 'ctrl-alt-f1':
        this.sendCtrlAltFx(1);
        break;
      case 'ctrl-alt-f2':
        this.sendCtrlAltFx(2);
        break;
      case 'ctrl-alt-f3':
        this.sendCtrlAltFx(3);
        break;
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
