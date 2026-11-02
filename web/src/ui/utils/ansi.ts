/**
 * ANSI Escape Code to HTML Converter & Sanitizer
 * Converts terminal ANSI colors and formatting into clean, safe HTML spans.
 */

const ANSI_COLORS_STANDARD: Record<number, string> = {
  30: '#64748b', // black / dark slate
  31: '#ef4444', // red
  32: '#10b981', // green
  33: '#f59e0b', // yellow
  34: '#3b82f6', // blue
  35: '#d946ef', // magenta
  36: '#06b6d4', // cyan
  37: '#f8fafc', // white
};

const ANSI_COLORS_BRIGHT: Record<number, string> = {
  90: '#94a3b8', // bright black / gray
  91: '#f87171', // bright red
  92: '#34d399', // bright green
  93: '#fbbf24', // bright yellow
  94: '#60a5fa', // bright blue
  95: '#e879f9', // bright magenta
  96: '#67e8f9', // bright cyan
  97: '#ffffff', // bright white
};

const ANSI_BG_STANDARD: Record<number, string> = {
  40: '#1e293b',
  41: '#7f1d1d',
  42: '#064e3b',
  43: '#78350f',
  44: '#1e3a8a',
  45: '#701a75',
  46: '#164e63',
  47: '#f1f5f9',
};

const ANSI_BG_BRIGHT: Record<number, string> = {
  100: '#334155',
  101: '#991b1b',
  102: '#065f46',
  103: '#92400e',
  104: '#1e40af',
  105: '#86198f',
  106: '#155e75',
  107: '#ffffff',
};

// 6x6x6 color cube steps
const COLOR_CUBE_STEPS = [0, 95, 135, 175, 215, 255];

function get256Color(code: number): string {
  if (code < 8) {
    return ANSI_COLORS_STANDARD[30 + code] || '#ffffff';
  }
  if (code < 16) {
    return ANSI_COLORS_BRIGHT[90 + (code - 8)] || '#ffffff';
  }
  if (code >= 16 && code <= 231) {
    const c = code - 16;
    const r = COLOR_CUBE_STEPS[Math.floor(c / 36)];
    const g = COLOR_CUBE_STEPS[Math.floor((c % 36) / 6)];
    const b = COLOR_CUBE_STEPS[c % 6];
    return `rgb(${r}, ${g}, ${b})`;
  }
  if (code >= 232 && code <= 255) {
    const gray = 8 + (code - 232) * 10;
    return `rgb(${gray}, ${gray}, ${gray})`;
  }
  return '#ffffff';
}

function escapeHtml(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

/**
 * Strips all ANSI escape sequences and non-printable control characters from text.
 */
export function stripAnsi(text: string): string {
  if (!text) return '';
  return text
    // Strip CSI sequences: \x1b[ ... letter
    .replace(/\x1b\[[0-9;?]*[A-Za-z]/g, '')
    // Strip OSC sequences: \x1b] ... \x07 or \x1b\
    .replace(/\x1b\][^\x07\x1b]*(\x07|\x1b\\)/g, '')
    // Strip standalone escape characters or arrows from raw terminal dumps
    .replace(/\x1b/g, '');
}

interface AnsiStyleState {
  color?: string;
  bgColor?: string;
  bold?: boolean;
  dim?: boolean;
  italic?: boolean;
  underline?: boolean;
}

function buildStyleString(state: AnsiStyleState): string {
  const styles: string[] = [];
  if (state.color) styles.push(`color: ${state.color}`);
  if (state.bgColor) styles.push(`background-color: ${state.bgColor}`);
  if (state.bold) styles.push('font-weight: 700');
  if (state.dim) styles.push('opacity: 0.65');
  if (state.italic) styles.push('font-style: italic');
  if (state.underline) styles.push('text-decoration: underline');
  return styles.join('; ');
}

/**
 * Converts terminal output containing ANSI escape codes into safe, colored HTML spans.
 */
export function ansiToHtml(raw: string): string {
  if (!raw) return '';

  // Standardize carriage returns
  const text = raw.replace(/\r\n/g, '\n').replace(/\r/g, '\n');

  // Regex matching either an SGR escape sequence, non-SGR escape sequence, or normal text chunk
  const tokenRegex = /(\x1b\[[0-9;]*m)|(\x1b\[[0-9;?]*[A-Za-z])|([^\x1b]+)|(\x1b)/g;

  let html = '';
  let state: AnsiStyleState = {};
  let inSpan = false;

  let match: RegExpExecArray | null;
  while ((match = tokenRegex.exec(text)) !== null) {
    const [full, sgr, nonSgr, plainText] = match;

    if (sgr) {
      // Parse SGR codes e.g. "\x1b[31;1m" -> ["31", "1"]
      const codeStr = sgr.slice(2, -1);
      const codes = codeStr.length === 0 ? [0] : codeStr.split(';').map((c) => parseInt(c, 10) || 0);

      let i = 0;
      while (i < codes.length) {
        const code = codes[i];

        if (code === 0) {
          // Reset
          state = {};
        } else if (code === 1) {
          state.bold = true;
        } else if (code === 2) {
          state.dim = true;
        } else if (code === 3) {
          state.italic = true;
        } else if (code === 4) {
          state.underline = true;
        } else if (code === 22) {
          state.bold = false;
          state.dim = false;
        } else if (code === 23) {
          state.italic = false;
        } else if (code === 24) {
          state.underline = false;
        } else if (code >= 30 && code <= 37) {
          state.color = ANSI_COLORS_STANDARD[code];
        } else if (code === 39) {
          delete state.color;
        } else if (code >= 40 && code <= 47) {
          state.bgColor = ANSI_BG_STANDARD[code];
        } else if (code === 49) {
          delete state.bgColor;
        } else if (code >= 90 && code <= 97) {
          state.color = ANSI_COLORS_BRIGHT[code];
        } else if (code >= 100 && code <= 107) {
          state.bgColor = ANSI_BG_BRIGHT[code];
        } else if (code === 38) {
          // Extended foreground color: 38;5;n or 38;2;r;g;b
          if (codes[i + 1] === 5 && codes[i + 2] !== undefined) {
            state.color = get256Color(codes[i + 2]);
            i += 2;
          } else if (codes[i + 1] === 2 && codes[i + 4] !== undefined) {
            state.color = `rgb(${codes[i + 2]}, ${codes[i + 3]}, ${codes[i + 4]})`;
            i += 4;
          }
        } else if (code === 48) {
          // Extended background color: 48;5;n or 48;2;r;g;b
          if (codes[i + 1] === 5 && codes[i + 2] !== undefined) {
            state.bgColor = get256Color(codes[i + 2]);
            i += 2;
          } else if (codes[i + 1] === 2 && codes[i + 4] !== undefined) {
            state.bgColor = `rgb(${codes[i + 2]}, ${codes[i + 3]}, ${codes[i + 4]})`;
            i += 4;
          }
        }

        i++;
      }

      if (inSpan) {
        html += '</span>';
        inSpan = false;
      }

      const styleStr = buildStyleString(state);
      if (styleStr) {
        html += `<span style="${styleStr}">`;
        inSpan = true;
      }
    } else if (nonSgr) {
      // Discard non-SGR sequences (e.g. cursor moves, clear line)
      continue;
    } else if (plainText) {
      html += escapeHtml(plainText);
    }
  }

  if (inSpan) {
    html += '</span>';
  }

  return html;
}
