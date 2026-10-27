import { computed } from 'vue';
import { isDarkMode } from '../theme';

export function useChartPalette() {
  return computed(() => {
    const dark = isDarkMode.value;
    return {
      textColor: dark ? '#c0caf5' : '#1f2328',
      subtextColor: dark ? '#a9b1d6' : '#57606a',
      borderColor: dark ? '#24283b' : '#d0d7de',
      tooltipBg: dark ? '#1f2335' : '#ffffff',
      tooltipBorder: dark ? '#3b4261' : '#d0d7de',

      // ZFS Pool Usage
      allocated: dark ? '#7aa2f7' : '#0969da',
      free: dark ? '#292e42' : '#eaeef2',

      // ZFS Pool Frag
      fragHigh: dark ? '#f7768e' : '#cf222e',
      fragMed: dark ? '#e0af68' : '#bf8700',
      fragLow: dark ? '#7dcfff' : '#0969da',
      contiguous: dark ? '#9ece6a' : '#1a7f37',

      // RAM Slices
      ramServices: dark ? '#7aa2f7' : '#0969da',
      ramArc: dark ? '#7dcfff' : '#0550ae',
      ramBuffers: dark ? '#e0af68' : '#9a6700',
      ramFree: dark ? '#9ece6a' : '#1a7f37',

      // CPU Gauge
      cpuLow: dark ? '#9ece6a' : '#1a7f37',
      cpuMed: dark ? '#e0af68' : '#bf8700',
      cpuHigh: dark ? '#f7768e' : '#cf222e',
      gaugeBg: dark ? '#292e42' : '#eaeef2',

      // Network Traffic Graph
      netIn: dark ? '#9ece6a' : '#1a7f37',
      netInArea: dark ? 'rgba(158, 206, 106, 0.22)' : 'rgba(26, 127, 55, 0.16)',
      netOut: dark ? '#7aa2f7' : '#0969da',
      netOutArea: dark ? 'rgba(122, 162, 247, 0.22)' : 'rgba(9, 105, 218, 0.16)',
      gridBorder: dark ? 'rgba(255, 255, 255, 0.08)' : 'rgba(0, 0, 0, 0.08)',
    };
  });
}
