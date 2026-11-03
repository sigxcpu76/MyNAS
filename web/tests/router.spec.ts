import { describe, it, expect, beforeEach } from 'vitest';
import { getTabFromPath, getPathFromTab, navigateTo, initRouter, resolveRouteFromPath, getCurrentPathFromState } from '../src/ui/router';
import { nasState, selectResource } from '../src/ui/store';

describe('SPA History Router', () => {
  beforeEach(() => {
    nasState.activeTab = 'overview';
    nasState.selectedResourceId = '';
    window.history.replaceState(null, '', '/');
  });

  it('maps paths to correct tabs', () => {
    expect(getTabFromPath('/apps')).toBe('docker');
    expect(getTabFromPath('/docker')).toBe('docker');
    expect(getTabFromPath('/zfs')).toBe('zfs');
    expect(getTabFromPath('/kvm')).toBe('kvm');
    expect(getTabFromPath('/vms')).toBe('kvm');
    expect(getTabFromPath('/vnc-console')).toBe('vnc-console');
    expect(getTabFromPath('/console/vnc')).toBe('vnc-console');
    expect(getTabFromPath('/kvm/console')).toBe('vnc-console');
    expect(getTabFromPath('/prerequisites')).toBe('prerequisites');
    expect(getTabFromPath('/prereqs')).toBe('prerequisites');
    expect(getTabFromPath('/shares')).toBe('shares');
    expect(getTabFromPath('/cifs')).toBe('shares');
    expect(getTabFromPath('/users')).toBe('identity');
    expect(getTabFromPath('/identity')).toBe('identity');
    expect(getTabFromPath('/lxc')).toBe('lxc');
    expect(getTabFromPath('/overview')).toBe('overview');
    expect(getTabFromPath('/')).toBe('overview');
    expect(getTabFromPath('')).toBe('overview');
    expect(getTabFromPath('/unknown-path')).toBe('overview');
  });

  it('resolves parameterized deep routes for resources and subtabs', () => {
    expect(resolveRouteFromPath('/kvm/win11/hardware')).toEqual({
      tab: 'kvm',
      resourceId: 'vm:win11',
      subTab: 'hardware',
    });
    expect(resolveRouteFromPath('/storage/pools/tank/topology')).toEqual({
      tab: 'storage-pools',
      resourceId: 'pool:tank',
      subTab: 'topology',
    });
    expect(resolveRouteFromPath('/apps/jellyfin/logs')).toEqual({
      tab: 'docker',
      resourceId: 'docker:jellyfin',
      subTab: 'logs',
    });
    expect(resolveRouteFromPath('/lxc/alpine01/terminal')).toEqual({
      tab: 'lxc',
      resourceId: 'lxc:alpine01',
      subTab: 'terminal',
    });
  });

  it('formats hierarchical URLs from state', () => {
    nasState.activeTab = 'kvm';
    nasState.selectedResourceId = 'vm:win11';
    nasState.vmSubTab = 'snapshots';
    expect(getCurrentPathFromState()).toBe('/kvm/win11/snapshots');

    nasState.activeTab = 'storage-pools';
    nasState.selectedResourceId = 'pool:tank';
    nasState.poolSubTab = 'datasets';
    expect(getCurrentPathFromState()).toBe('/storage/pools/tank/datasets');
  });

  it('maps tabs to canonical URL paths', () => {
    expect(getPathFromTab('docker')).toBe('/apps');
    expect(getPathFromTab('zfs')).toBe('/zfs');
    expect(getPathFromTab('kvm')).toBe('/kvm');
    expect(getPathFromTab('lxc')).toBe('/lxc');
    expect(getPathFromTab('shares')).toBe('/shares');
    expect(getPathFromTab('identity')).toBe('/users');
    expect(getPathFromTab('vnc-console')).toBe('/vnc-console');
    expect(getPathFromTab('prerequisites')).toBe('/prerequisites');
    expect(getPathFromTab('overview')).toBe('/overview');
  });

  it('navigates to apps tab and updates browser history', () => {
    initRouter();
    navigateTo('docker');

    expect(nasState.activeTab).toBe('docker');
    expect(window.location.pathname).toBe('/apps');
    expect(window.history.state).toEqual({ tab: 'docker' });
  });

  it('handles popstate browser back from VM detail to KVM list view', () => {
    window.history.replaceState(null, '', '/kvm/win11');
    initRouter();

    expect(nasState.activeTab).toBe('kvm');
    expect(nasState.selectedResourceId).toBe('vm:win11');

    // Simulate browser Back button to /kvm (list view)
    window.history.replaceState({ tab: 'kvm' }, '', '/kvm');
    window.dispatchEvent(new PopStateEvent('popstate', { state: { tab: 'kvm' } }));

    expect(nasState.activeTab).toBe('kvm');
    expect(nasState.selectedResourceId).toBe('node:vms');
  });

  it('handles popstate browser back/forward navigation', () => {
    initRouter();

    navigateTo('docker');
    expect(nasState.activeTab).toBe('docker');
    expect(window.location.pathname).toBe('/apps');

    // Simulate browser Back button to /zfs
    window.history.replaceState({ tab: 'zfs' }, '', '/zfs');
    window.dispatchEvent(new PopStateEvent('popstate', { state: { tab: 'zfs' } }));

    expect(nasState.activeTab).toBe('zfs');
  });

  it('preserves query parameters on initial route initialization for detached consoles', () => {
    window.history.replaceState(null, '', '/vnc-console?vm=debian-server&token=secret-token');
    initRouter();

    expect(nasState.activeTab).toBe('vnc-console');
    expect(window.location.pathname).toBe('/vnc-console');
    expect(window.location.search).toBe('?vm=debian-server&token=secret-token');
  });

  it('navigates directly to an app resource and updates URL and active state', () => {
    initRouter();
    navigateTo('docker', false, 'docker:clickhouse');

    expect(nasState.activeTab).toBe('docker');
    expect(nasState.selectedResourceId).toBe('docker:clickhouse');
    expect(window.location.pathname).toBe('/apps/clickhouse');
  });

  it('preserves selected child resource if selectResource is called before navigateTo', () => {
    initRouter();
    selectResource('docker:clickhouse');
    navigateTo('docker');

    expect(nasState.activeTab).toBe('docker');
    expect(nasState.selectedResourceId).toBe('docker:clickhouse');
    expect(window.location.pathname).toBe('/apps/clickhouse');
  });

  it('clears previous mismatched child resource when navigating to a new module without target', () => {
    initRouter();
    selectResource('vm:win11');
    expect(nasState.selectedResourceId).toBe('vm:win11');

    navigateTo('docker');
    expect(nasState.activeTab).toBe('docker');
    expect(nasState.selectedResourceId).toBe('node:docker');
    expect(window.location.pathname).toBe('/apps');
  });

  it('navigates back to top-level list view when explicitly requested with node resource', () => {
    initRouter();
    navigateTo('docker', false, 'docker:clickhouse');
    expect(window.location.pathname).toBe('/apps/clickhouse');

    navigateTo('docker', false, 'node:docker');
    expect(nasState.activeTab).toBe('docker');
    expect(nasState.selectedResourceId).toBe('node:docker');
    expect(window.location.pathname).toBe('/apps');
  });
});
