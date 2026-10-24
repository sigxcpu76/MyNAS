import { describe, it, expect, beforeEach } from 'vitest';
import { getTabFromPath, getPathFromTab, navigateTo, initRouter } from '../src/ui/router';
import { nasState } from '../src/ui/store';

describe('SPA History Router', () => {
  beforeEach(() => {
    nasState.activeTab = 'overview';
    window.history.replaceState(null, '', '/');
  });

  it('maps paths to correct tabs', () => {
    expect(getTabFromPath('/apps')).toBe('docker');
    expect(getTabFromPath('/docker')).toBe('docker');
    expect(getTabFromPath('/zfs')).toBe('zfs');
    expect(getTabFromPath('/kvm')).toBe('kvm');
    expect(getTabFromPath('/vms')).toBe('kvm');
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

  it('maps tabs to canonical URL paths', () => {
    expect(getPathFromTab('docker')).toBe('/apps');
    expect(getPathFromTab('zfs')).toBe('/zfs');
    expect(getPathFromTab('kvm')).toBe('/kvm');
    expect(getPathFromTab('lxc')).toBe('/lxc');
    expect(getPathFromTab('shares')).toBe('/shares');
    expect(getPathFromTab('identity')).toBe('/users');
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
});
