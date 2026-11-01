import { describe, expect, it } from 'vitest';
import { useAutoScroll } from '../src/ui/composables/useAutoScroll';

describe('useAutoScroll composable', () => {
  it('initializes with autoScroll true and scrolls to bottom on demand', async () => {
    const { containerRef, autoScroll, scrollToBottom, onScroll, toggleAutoScroll } = useAutoScroll();
    expect(autoScroll.value).toBe(true);

    const mockEl = {
      scrollTop: 0,
      scrollHeight: 1000,
      clientHeight: 400,
    } as unknown as HTMLElement;

    containerRef.value = mockEl;

    await scrollToBottom();
    expect(mockEl.scrollTop).toBe(1000);

    // Simulate user scrolling up
    mockEl.scrollTop = 200;
    onScroll();
    expect(autoScroll.value).toBe(false);

    // Calling scrollToBottom while autoScroll is false shouldn't force scroll unless force=true
    mockEl.scrollTop = 200;
    await scrollToBottom(false);
    expect(mockEl.scrollTop).toBe(200);

    // Toggling autoScroll back on should force scroll to bottom
    toggleAutoScroll();
    expect(autoScroll.value).toBe(true);
    await scrollToBottom();
    expect(mockEl.scrollTop).toBe(1000);

    // Simulate scrolling back within threshold
    mockEl.scrollTop = 580; // 1000 - 580 - 400 = 20 <= 40
    onScroll();
    expect(autoScroll.value).toBe(true);
  });
});
