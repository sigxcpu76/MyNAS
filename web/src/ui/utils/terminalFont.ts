/**
 * Shared Powerline & Nerd Font stack and font-loading helper for xterm.js terminals
 * (LXC, Docker, Host, Standalone, and Mobile consoles).
 */

export const TERMINAL_FONT_FAMILY =
  '"JetBrainsMono Nerd Font Mono", "JetBrainsMono Nerd Font", "MesloLGS NF", "MesloLGS Nerd Font Mono", "FiraCode Nerd Font Mono", "Hack Nerd Font Mono", "DejaVu Sans Mono for Powerline", "Meslo LG S for Powerline", "Roboto Mono for Powerline", "Source Code Pro for Powerline", "Ubuntu Mono derivative Powerline", "Symbols Nerd Font Mono", "JetBrains Mono", ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace';

/**
 * Triggers load of the bundled Powerline / Nerd Font webfonts via the CSS Font Loading API
 * (when available in the current browser environment) and invokes `onReady` once loaded so
 * xterm.js can re-measure cell metrics and re-fit the terminal canvas.
 */
export function ensureTerminalFontsLoaded(onReady: () => void): void {
  if (typeof document === 'undefined' || !('fonts' in document) || !document.fonts) {
    return;
  }

  try {
    const fontSet = document.fonts;
    const loads: Promise<unknown>[] = [];
    if (typeof fontSet.load === 'function') {
      loads.push(
        fontSet.load('400 13px "JetBrainsMono Nerd Font Mono"'),
        fontSet.load('700 13px "JetBrainsMono Nerd Font Mono"'),
        fontSet.load('400 13px "Symbols Nerd Font Mono"', '\ue0b0')
      );
    }
    if (fontSet.ready) {
      loads.push(fontSet.ready);
    }
    if (loads.length > 0) {
      Promise.allSettled(loads).then(() => {
        onReady();
      });
    }
  } catch {
    // Ignore errors in headless/non-standard DOM environments (e.g. jsdom)
  }
}
