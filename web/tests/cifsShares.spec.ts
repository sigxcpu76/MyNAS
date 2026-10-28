import { describe, expect, it } from 'vitest';
import { mapCifsGlobalConfig, mapCifsShare, mapUser } from '../src/adapters/http/httpAdapters';

describe('CIFS Shares & Global Config Mappers', () => {
  it('maps CifsGlobalConfig correctly with defaults', () => {
    const raw = {
      workgroup: 'HOMELAB',
      server_string: 'Homelab Samba Server',
      fruit_aapl: true,
      wsdd_enabled: true,
      disable_spoolss: true,
      max_log_size: 100,
    };
    const mapped = mapCifsGlobalConfig(raw);
    expect(mapped.workgroup).toBe('HOMELAB');
    expect(mapped.serverString).toBe('Homelab Samba Server');
    expect(mapped.fruitAapl).toBe(true);
    expect(mapped.wsddEnabled).toBe(true);
    expect(mapped.disableSpoolss).toBe(true);
    expect(mapped.maxLogSize).toBe(100);
  });

  it('maps extended CifsShare fields correctly including VFS flags and ACLs', () => {
    const raw = {
      name: 'cifsShares/tank-media',
      share_name: 'tank-media',
      path: '/tank/media',
      comment: '4K Movies',
      read_only: false,
      guest_ok: true,
      browseable: false,
      valid_users: ['alice', 'bob'],
      valid_groups: ['family'],
      read_list: ['guest'],
      write_list: ['alice'],
      force_user: 'alice',
      force_group: 'family',
      guest_account: 'guestuser',
      create_mask: '0664',
      directory_mask: '0775',
      force_create_mode: '0664',
      force_directory_mode: '0775',
      shadow_copy: true,
      time_machine: true,
      recycle_bin: true,
    };
    const share = mapCifsShare(raw);
    expect(share.shareName).toBe('tank-media');
    expect(share.browseable).toBe(false);
    expect(share.guestOk).toBe(true);
    expect(share.validUsers).toEqual(['alice', 'bob']);
    expect(share.validGroups).toEqual(['family']);
    expect(share.readList).toEqual(['guest']);
    expect(share.writeList).toEqual(['alice']);
    expect(share.forceUser).toBe('alice');
    expect(share.forceGroup).toBe('family');
    expect(share.guestAccount).toBe('guestuser');
    expect(share.createMask).toBe('0664');
    expect(share.directoryMask).toBe('0775');
    expect(share.shadowCopy).toBe(true);
    expect(share.timeMachine).toBe(true);
    expect(share.recycleBin).toBe(true);
  });

  it('maps User with smbEnabled flag', () => {
    const raw = {
      name: 'users/nasuser',
      username: 'nasuser',
      uid: 1001,
      smb_enabled: true,
    };
    const user = mapUser(raw);
    expect(user.username).toBe('nasuser');
    expect(user.smbEnabled).toBe(true);
  });

  it('sends correct unnested payload when updating CifsGlobalConfig', async () => {
    let capturedUrl = '';
    let capturedBody: any = null;
    const mockClient = {
      patch: async (url: string, body: any) => {
        capturedUrl = url;
        capturedBody = body;
        return body;
      },
    } as any;

    const { HttpShareAdapter } = await import('../src/adapters/http/httpAdapters');
    const adapter = new HttpShareAdapter(mockClient);
    await adapter.updateCifsGlobalConfig({
      workgroup: 'MYGROUP',
      serverString: 'My Server',
      mapToGuest: 'Bad User',
      guestAccount: 'nobody',
      maxLogSize: 50,
      disableSpoolss: true,
      fruitAapl: true,
      wsddEnabled: true,
    });

    expect(capturedUrl).toBe('cifsGlobalConfig');
    expect(capturedBody).not.toHaveProperty('globalConfig');
    expect(capturedBody.workgroup).toBe('MYGROUP');
    expect(capturedBody.serverString).toBe('My Server');
  });

  it('renders PathSelector for Samba and WebDAV share creation with folder browsers', async () => {
    const { mount } = await import('@vue/test-utils');
    const { default: SharesView } = await import('../src/ui/views/SharesView.vue');
    const { default: PathSelector } = await import('../src/ui/components/PathSelector.vue');
    const { nasState } = await import('../src/ui/store');

    nasState.systemConfig = {
      ...(nasState.systemConfig as any),
      enabledModules: ['cifs', 'webdav'],
    };

    const wrapper = mount(SharesView);
    const addSmbBtn = wrapper.findAll('button').find((b) => b.text().includes('Add SMB Share'));
    if (addSmbBtn) {
      await addSmbBtn.trigger('click');
    }

    const pathSelectors = wrapper.findAllComponents(PathSelector);
    expect(pathSelectors.length).toBeGreaterThanOrEqual(1);

    const browseButtons = wrapper.findAll('button[title="Browse Folders..."]');
    expect(browseButtons.length).toBeGreaterThanOrEqual(1);
  });

  it('renders edit and delete icon buttons for Samba and WebDAV shares', async () => {
    const { mount } = await import('@vue/test-utils');
    const { default: SharesView } = await import('../src/ui/views/SharesView.vue');
    const { default: MdiIcon } = await import('../src/ui/components/MdiIcon.vue');
    const { nasState } = await import('../src/ui/store');

    nasState.cifsShares = [
      {
        name: 'cifsShares/test-share',
        shareName: 'test-share',
        path: '/tank/test',
        comment: 'Test share',
        readOnly: false,
        browseable: true,
        guestOk: false,
      } as any,
    ];
    nasState.webdavShares = [
      {
        name: 'webdavShares/test-webdav',
        urlPrefix: '/webdav/test',
        path: '/tank/test',
        description: 'Test WebDAV',
        readOnly: false,
        enabled: true,
      } as any,
    ];

    const wrapper = mount(SharesView);

    // Check CIFS action buttons
    const editCifsBtn = wrapper.find('button[title="Edit SMB Share"]');
    expect(editCifsBtn.exists()).toBe(true);
    expect(editCifsBtn.classes()).toContain('btn-icon');
    expect(editCifsBtn.findComponent(MdiIcon).exists()).toBe(true);

    const deleteCifsBtn = wrapper.find('button[title="Delete SMB Share"]');
    expect(deleteCifsBtn.exists()).toBe(true);
    expect(deleteCifsBtn.classes()).toContain('btn-icon');
    expect(deleteCifsBtn.classes()).toContain('btn-danger');
    expect(deleteCifsBtn.findComponent(MdiIcon).exists()).toBe(true);

    // Check WebDAV action buttons
    const editWebdavBtn = wrapper.find('button[title="Edit WebDAV Share"]');
    expect(editWebdavBtn.exists()).toBe(true);
    expect(editWebdavBtn.classes()).toContain('btn-icon');
    expect(editWebdavBtn.findComponent(MdiIcon).exists()).toBe(true);

    const deleteWebdavBtn = wrapper.find('button[title="Delete WebDAV Share"]');
    expect(deleteWebdavBtn.exists()).toBe(true);
    expect(deleteWebdavBtn.classes()).toContain('btn-icon');
    expect(deleteWebdavBtn.classes()).toContain('btn-danger');
    expect(deleteWebdavBtn.findComponent(MdiIcon).exists()).toBe(true);
  });

  it('renders nested subtab navigation and switches between CIFS and WebDAV wide tables', async () => {
    const { mount } = await import('@vue/test-utils');
    const { default: SharesView } = await import('../src/ui/views/SharesView.vue');
    const { nasState } = await import('../src/ui/store');

    // Mount with activeSubtab = 'cifs'
    nasState.activeTab = 'shares-cifs';
    const wrapper = mount(SharesView, {
      props: {
        activeSubtab: 'cifs',
      },
    });

    expect(wrapper.find('.shares-subnav').exists()).toBe(true);
    const subnavPills = wrapper.findAll('.subnav-pill');
    expect(subnavPills.length).toBe(2);
    expect(subnavPills[0].text()).toContain('Windows (SMB / CIFS)');
    expect(subnavPills[1].text()).toContain('WebDAV');

    // Only CIFS card should be present
    expect(wrapper.text()).toContain('SMB / CIFS Shares');
    expect(wrapper.text()).not.toContain('WebDAV Endpoints');

    // Re-mount or update prop to 'webdav'
    await wrapper.setProps({ activeSubtab: 'webdav' });
    expect(wrapper.text()).toContain('WebDAV Endpoints');
    expect(wrapper.text()).not.toContain('SMB / CIFS Shares');
  });
});
