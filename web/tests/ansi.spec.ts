import { describe, expect, it } from 'vitest';
import { ansiToHtml, stripAnsi } from '../src/ui/utils/ansi';

describe('ANSI Escape Codes to HTML Converter', () => {
  it('converts basic 16 ANSI colors into styled spans', () => {
    const input = '\x1b[31mRed Text\x1b[0m normal';
    const html = ansiToHtml(input);
    expect(html).toContain('style="color: #ef4444"');
    expect(html).toContain('Red Text</span>');
    expect(html).toContain(' normal');
  });

  it('converts bold, dim, and italic styles', () => {
    const input = '\x1b[1mBold\x1b[0m \x1b[2mDim\x1b[0m \x1b[3mItalic\x1b[0m';
    const html = ansiToHtml(input);
    expect(html).toContain('font-weight: 700');
    expect(html).toContain('opacity: 0.65');
    expect(html).toContain('font-style: italic');
  });

  it('escapes raw HTML to prevent XSS injection', () => {
    const input = '\x1b[31m<script>alert("xss")</script>\x1b[0m & <b>test</b>';
    const html = ansiToHtml(input);
    expect(html).not.toContain('<script>');
    expect(html).toContain('&lt;script&gt;');
    expect(html).toContain('&amp;');
    expect(html).toContain('&lt;b&gt;');
  });

  it('accurately parses the user Docker playit log snippet with mixed codes', () => {
    // Exact structure from user's screenshot:
    // \x1b[2m2026-11-02T18:51:29.311407Z\x1b[0m \x1b[31mERROR\x1b[0m \x1b[2mplayit_api_client::http_client\x1b[0m: API call failed
    const userLog =
      'playit-1  | \x1b[2m2026-11-02T18:51:29.311407Z\x1b[0m \x1b[31mERROR\x1b[0m \x1b[2mplayit_api_client::http_client\x1b[0m\x1b[2m:\x1b[0m API call failed \x1b[3merror\x1b[0m=ParseError';

    const html = ansiToHtml(userLog);
    // Does not leak raw escape codes or arrows
    expect(html).not.toContain('\x1b');
    expect(html).not.toContain('[2m');
    expect(html).not.toContain('[31m');
    expect(html).not.toContain('[0m');

    // Has styled spans for dim timestamp, red ERROR, and italic error
    expect(html).toContain('<span style="opacity: 0.65">2026-11-02T18:51:29.311407Z</span>');
    expect(html).toContain('<span style="color: #ef4444">ERROR</span>');
    expect(html).toContain('<span style="font-style: italic">error</span>');
  });

  it('converts 256 colors and 24-bit TrueColor', () => {
    const input256 = '\x1b[38;5;196mRed256\x1b[0m';
    const html256 = ansiToHtml(input256);
    expect(html256).toContain('color: rgb(255, 0, 0)');

    const inputTrueColor = '\x1b[38;2;120;200;80mTrueColor\x1b[0m';
    const htmlTrueColor = ansiToHtml(inputTrueColor);
    expect(htmlTrueColor).toContain('color: rgb(120, 200, 80)');
  });

  it('strips ANSI codes cleanly for clipboard copying', () => {
    const input = 'playit-1  | \x1b[2m2026-11-02T18:51:29Z\x1b[0m \x1b[31mERROR\x1b[0m failed';
    const plain = stripAnsi(input);
    expect(plain).toBe('playit-1  | 2026-11-02T18:51:29Z ERROR failed');
  });
});
