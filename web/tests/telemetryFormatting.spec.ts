import { describe, it, expect } from 'vitest';
import { formatBytes, bytesToGib, gibToBytes, formatUptime } from '../src/domain/models';
import { nasState, handleWatchResourceChange } from '../src/ui/store';

describe('Telemetry & Byte Formatting', () => {
  describe('formatBytes helper', () => {
    it('formats numeric bytes correctly', () => {
      expect(formatBytes(0)).toBe('0 B');
      expect(formatBytes(1024)).toBe('1 KiB');
      expect(formatBytes(1024 * 1024)).toBe('1 MiB');
      expect(formatBytes(1024 * 1024 * 1024)).toBe('1 GiB');
      expect(formatBytes(34359738368)).toBe('32 GiB');
    });

    it('formats string representations of int64 safely without returning 0 B', () => {
      expect(formatBytes('34359738368')).toBe('32 GiB');
      expect(formatBytes('25769803776')).toBe('24 GiB');
      expect(formatBytes('1073741824')).toBe('1 GiB');
    });

    it('handles falsy or invalid inputs gracefully', () => {
      expect(formatBytes(null)).toBe('0 B');
      expect(formatBytes(undefined)).toBe('0 B');
      expect(formatBytes('')).toBe('0 B');
      expect(formatBytes(-100)).toBe('0 B');
      expect(formatBytes('invalid')).toBe('0 B');
    });
  });

  describe('bytesToGib and gibToBytes', () => {
    it('converts between GiB and bytes handling numbers and strings', () => {
      expect(gibToBytes(32)).toBe(34359738368);
      expect(gibToBytes('32')).toBe(34359738368);
      expect(bytesToGib(34359738368)).toBe(32);
      expect(bytesToGib('34359738368')).toBe(32);
      expect(bytesToGib(0)).toBe(0);
      expect(bytesToGib(null)).toBe(0);
    });
  });

  describe('formatUptime', () => {
    it('formats uptime seconds correctly from numbers and strings', () => {
      expect(formatUptime(3661)).toBe('1h 1m');
      expect(formatUptime('3661')).toBe('1h 1m');
      expect(formatUptime(86400 + 3600)).toBe('1d 1h 0m');
      expect(formatUptime(0)).toBe('0m');
      expect(formatUptime(null)).toBe('0m');
    });
  });

  describe('handleWatchResourceChange telemetry mapping', () => {
    it('maps int64 string fields from protojson telemetry events to numeric store state', () => {
      const protojsonPayload = JSON.stringify({
        cpu: {
          model: 'AMD Ryzen 9',
          physicalCores: 8,
          logicalCores: 16,
          usagePercent: 25.5,
        },
        memory: {
          totalBytes: '34359738368', // 32 GiB
          usedBytes: '25769803776',  // 24 GiB (~75%)
          freeBytes: '8589934592',   // 8 GiB
          availableBytes: '8589934592',
          zfsArcBytes: '8589934592', // 8 GiB
          servicesBytes: '17179869184', // 16 GiB
        },
      });

      handleWatchResourceChange({
        resourceType: 'telemetry',
        eventType: 'MODIFIED',
        resourceName: 'system/telemetry',
        jsonPayload: protojsonPayload,
      });

      expect(nasState.systemTelemetry).toBeDefined();
      expect(nasState.systemTelemetry?.memory.totalBytes).toBe(34359738368);
      expect(typeof nasState.systemTelemetry?.memory.totalBytes).toBe('number');
      expect(nasState.systemTelemetry?.memory.usedBytes).toBe(25769803776);
      expect(typeof nasState.systemTelemetry?.memory.usedBytes).toBe('number');

      // Verify that formatBytes on the mapped state renders readable units, NOT "0 B"
      const totalStr = formatBytes(nasState.systemTelemetry?.memory.totalBytes || 0);
      const usedStr = formatBytes(nasState.systemTelemetry?.memory.usedBytes || 0);
      expect(totalStr).toBe('32 GiB');
      expect(usedStr).toBe('24 GiB');

      const usedPct = Math.round(
        ((nasState.systemTelemetry?.memory.usedBytes || 0) /
          (nasState.systemTelemetry?.memory.totalBytes || 1)) *
          100
      );
      expect(usedPct).toBe(75);
    });
  });
});
