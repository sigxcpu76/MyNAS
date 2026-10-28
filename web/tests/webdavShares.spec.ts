import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import { nextTick } from 'vue';
import { mapWebdavShare } from '../src/adapters/http/httpAdapters';
import SharesView from '../src/ui/views/SharesView.vue';
import { nasState } from '../src/ui/store';

describe('WebDAV Shares & Port Isolation', () => {
  it('maps WebdavShare fields correctly', () => {
    const raw = {
      name: 'webdavShares/documents',
      share_name: 'documents',
      path: '/mnt/tank/documents',
      url_prefix: '/dav/documents',
      description: 'Personal Documents',
      read_only: true,
      allowed_users: ['alice'],
      allowed_groups: ['staff'],
      enabled: true,
    };
    const mapped = mapWebdavShare(raw);
    expect(mapped.name).toBe('webdavShares/documents');
    expect(mapped.path).toBe('/mnt/tank/documents');
    expect(mapped.urlPrefix).toBe('/dav/documents');
    expect(mapped.readOnly).toBe(true);
    expect(mapped.allowedUsers).toEqual(['alice']);
    expect(mapped.allowedGroups).toEqual(['staff']);
    expect(mapped.enabled).toBe(true);
  });

  it('renders WebDAV dedicated port 8081 mount URL and copy/connect buttons in wide table', async () => {
    nasState.webdavShares = [
      {
        name: 'webdavShares/media',
        shareName: 'media',
        path: '/mnt/tank/media',
        urlPrefix: '/dav/media',
        description: 'Media Store',
        readOnly: false,
        allowedUsers: ['alice'],
        allowedGroups: [],
        enabled: true,
      },
    ];

    const wrapper = mount(SharesView, {
      props: {
        activeSubtab: 'webdav',
      },
      global: {
        stubs: {
          MdiIcon: true,
          PathSelector: true,
          UserGroupSelector: true,
          SortableTh: {
            template: '<th><slot /></th>',
          },
        },
      },
    });

    await nextTick();

    // Verify endpoint cell renders port 8081 mount URL
    const endpointText = wrapper.find('.endpoint-url-text');
    expect(endpointText.exists()).toBe(true);
    expect(endpointText.text()).toContain(':8081/dav/media/');

    // Verify Read-Write badge
    expect(wrapper.text()).toContain('Read-Write');

    // Click on Add WebDAV Share to verify segmented Access Type selector
    const addBtn = wrapper.find('.card-header button.btn-primary');
    expect(addBtn.exists()).toBe(true);
    await addBtn.trigger('click');
    await nextTick();

    const accessCards = wrapper.findAll('.access-type-card');
    expect(accessCards.length).toBe(2);
    expect(accessCards[0].text()).toContain('Read-Write (Full Access)');
    expect(accessCards[1].text()).toContain('Read-Only');

    // Verify clicking Connect opens the Client Connect Guide modal
    const connectBtn = wrapper.find('button[title="Client Connect Guide"]');
    expect(connectBtn.exists()).toBe(true);
    await connectBtn.trigger('click');
    await nextTick();

    expect(wrapper.text()).toContain('Connect to WebDAV Share: media');
    expect(wrapper.text()).toContain('Dedicated WebDAV Port 8081 Mount URL');
    expect(wrapper.text()).toContain('Map Network Drive');
  });
});
