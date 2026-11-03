import { describe, it, expect, vi, beforeEach } from 'vitest';
import { mapUserPreferences } from '../src/adapters/http/httpAdapters';
import {
  loadDashboardLayout,
  loadDashboardColumnCount,
  syncPreferencesFromBackend,
  persistPreferencesToBackend,
  STORAGE_KEY,
  STORAGE_KEY_COLUMNS,
} from '../src/ui/dashboard/dashboardConfig';

const store = new Map<string, string>();
const localStorageMock = {
  getItem: (key: string) => store.get(key) ?? null,
  setItem: (key: string, val: string) => store.set(key, String(val)),
  removeItem: (key: string) => store.delete(key),
  clear: () => store.clear(),
};

Object.defineProperty(window, 'localStorage', {
  value: localStorageMock,
  writable: true,
});

describe('User Preferences & Dashboard Layout Sync', () => {
  beforeEach(() => {
    localStorageMock.clear();
    vi.clearAllMocks();
  });

  it('maps raw preferences payload from backend', () => {
    const raw = {
      dashboard: {
        columnCount: 2,
        layout: [
          { id: 'widget-kpis', type: 'kpis', lane: -1 },
          { id: 'widget-pools', type: 'pools', lane: 0 },
        ],
      },
      custom: {
        theme: 'dark',
      },
    };

    const mapped = mapUserPreferences(raw);
    expect(mapped.dashboard.columnCount).toBe(2);
    expect(mapped.dashboard.layout).toHaveLength(2);
    expect(mapped.dashboard.layout[0]).toEqual({
      id: 'widget-kpis',
      type: 'kpis',
      lane: -1,
    });
    expect(mapped.custom?.theme).toBe('dark');
  });

  it('defaults columnCount to 3 when missing or invalid in payload', () => {
    const mapped = mapUserPreferences({});
    expect(mapped.dashboard.columnCount).toBe(3);
    expect(mapped.dashboard.layout).toEqual([]);
  });

  it('persists preferences locally and calls backend auth port', async () => {
    const mockAuth = {
      isAuthenticated: vi.fn().mockReturnValue(true),
      updateUserPreferences: vi.fn().mockResolvedValue({
        dashboard: {
          columnCount: 2,
          layout: [{ id: 'widget-kpis', type: 'kpis', lane: -1 }],
        },
      }),
    };

    const layout = [{ id: 'widget-kpis', type: 'kpis', lane: -1 }];
    await persistPreferencesToBackend({ auth: mockAuth }, layout, 2);

    // Verify localStorage updated
    expect(localStorageMock.getItem(STORAGE_KEY_COLUMNS)).toBe('2');
    expect(JSON.parse(localStorageMock.getItem(STORAGE_KEY)!)).toEqual(layout);

    // Verify backend port called
    expect(mockAuth.updateUserPreferences).toHaveBeenCalledWith({
      dashboard: {
        columnCount: 2,
        layout: [{ id: 'widget-kpis', type: 'kpis', lane: -1 }],
      },
    });
  });

  it('syncs preferences from backend and caches in localStorage', async () => {
    const mockAuth = {
      isAuthenticated: vi.fn().mockReturnValue(true),
      getUserPreferences: vi.fn().mockResolvedValue({
        dashboard: {
          columnCount: 2,
          layout: [
            { id: 'widget-kpis', type: 'kpis', lane: -1 },
            { id: 'widget-pools', type: 'pools', lane: 0 },
            { id: 'widget-network', type: 'network', lane: 1 },
          ],
        },
      }),
    };

    const result = await syncPreferencesFromBackend({ auth: mockAuth });
    expect(result).not.toBeNull();
    expect(result?.columnCount).toBe(2);
    expect(result?.layout).toHaveLength(3);

    // Verify loaded through local getters
    expect(loadDashboardColumnCount()).toBe(2);
    const localLayout = loadDashboardLayout();
    expect(localLayout).toHaveLength(3);
  });
});
