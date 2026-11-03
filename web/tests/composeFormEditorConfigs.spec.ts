import { describe, it, expect, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import ComposeFormEditor from '../src/ui/components/ComposeFormEditor.vue';

describe('ComposeFormEditor: Inline Configs Directive and Volume Mapping', () => {
  it('renders Configs & Files top tab and allows adding top-level inline configs', async () => {
    const wrapper = mount(ComposeFormEditor, {
      props: {
        initialAppId: 'my-app',
        initialTitle: 'My Test App',
        initialComposeYaml: '',
      },
    });

    // Top tab button for configs
    const buttons = wrapper.findAll('button');
    const configsTabBtn = buttons.find((b) => b.text().includes('Configs & Files'));
    expect(configsTabBtn).toBeDefined();

    // Click Configs & Files tab
    await configsTabBtn!.trigger('click');

    // Should display empty placeholder
    expect(wrapper.text()).toContain('Inline Configuration Files (Compose Configs)');
    expect(wrapper.text()).toContain('No inline config files declared yet');

    // Click "Add Config File" button
    const addBtn = wrapper.findAll('button').find((b) => b.text().includes('Add Config File'));
    expect(addBtn).toBeDefined();
    await addBtn!.trigger('click');

    // Should now render a config card
    expect(wrapper.text()).toContain('Config Name:');
    const nameInput = wrapper.find('input[placeholder="e.g. nginx_conf, app_settings"]');
    expect(nameInput.exists()).toBe(true);

    const textarea = wrapper.find('textarea[placeholder="Paste entire file contents here..."]');
    expect(textarea.exists()).toBe(true);

    // Type content
    await textarea.setValue('server {\n  listen 80;\n}\n');
    expect((textarea.element as HTMLTextAreaElement).value).toContain('listen 80;');
  });

  it('allows mapping an inline config directly inside a service volume mount', async () => {
    const wrapper = mount(ComposeFormEditor, {
      props: {
        initialAppId: 'nginx-app',
        initialTitle: 'Nginx App',
        initialComposeYaml: `services:
  web:
    image: nginx:alpine
`,
      },
    });

    // Go to Storage & Volumes sub-tab
    const buttons = wrapper.findAll('button');
    const volumesSubTab = buttons.find((b) => b.text().includes('Storage & Volumes'));
    expect(volumesSubTab).toBeDefined();
    await volumesSubTab!.trigger('click');

    // Click Add Storage Mount
    const addMountBtn = wrapper.findAll('button').find((b) => b.text().includes('Add Storage Mount'));
    expect(addMountBtn).toBeDefined();
    await addMountBtn!.trigger('click');

    // Switch Volume Type to "Inline Config File"
    const inlineConfigTypeBtn = wrapper.findAll('button').find((b) => b.text().includes('Inline Config File'));
    expect(inlineConfigTypeBtn).toBeDefined();
    await inlineConfigTypeBtn!.trigger('click');

    // Should show "INLINE CONFIG FILE" label
    expect(wrapper.text()).toContain('INLINE CONFIG FILE');
    expect(wrapper.text()).toContain('Read-Only (File)');

    // Should render inline content editor directly in the volume card
    const inlineTextarea = wrapper.find('textarea[placeholder*="Paste file content here"]');
    expect(inlineTextarea.exists()).toBe(true);

    // Enter content
    await inlineTextarea.setValue('daemon off;\nworker_processes 1;\n');

    // Set container destination path
    const destInput = wrapper.find('input[placeholder*="/etc/nginx/nginx.conf"]');
    expect(destInput.exists()).toBe(true);
    await destInput.setValue('/etc/nginx/nginx.conf');

    // Click Save and verify emitted YAML
    const saveBtn = wrapper.findAll('button').find((b) => b.text().includes('Deploy Application') || b.text().includes('Save & Redeploy'));
    expect(saveBtn).toBeDefined();
    await saveBtn!.trigger('click');

    const emitted = wrapper.emitted('save');
    expect(emitted).toBeDefined();
    expect(emitted![0]).toBeDefined();
    const payload = emitted![0][0] as { appId: string; title: string; composeYaml: string };

    expect(payload.composeYaml).toContain('configs:');
    expect(payload.composeYaml).toContain('/etc/nginx/nginx.conf');
    expect(payload.composeYaml).toContain('daemon off;');
    expect(payload.composeYaml).toContain('worker_processes 1;');
  });

  it('synchronizes raw YAML with configs when switching editor mode', async () => {
    const rawYaml = `services:
  app:
    image: alpine:latest
    configs:
      - source: app_json
        target: /app/config.json

configs:
  app_json:
    content: |
      {"debug": true}
`;
    const wrapper = mount(ComposeFormEditor, {
      props: {
        initialAppId: 'test-sync',
        initialTitle: 'Test Sync',
        initialComposeYaml: rawYaml,
      },
    });

    // Check Configs & Files top tab count
    expect(wrapper.text()).toContain('Configs & Files (1)');

    // Switch to Configs & Files tab
    const configsTabBtn = wrapper.findAll('button').find((b) => b.text().includes('Configs & Files'));
    await configsTabBtn!.trigger('click');

    // Verify parsed name and content
    const nameInput = wrapper.find('input[placeholder="e.g. nginx_conf, app_settings"]');
    expect((nameInput.element as HTMLInputElement).value).toBe('app_json');

    const textarea = wrapper.find('textarea[placeholder="Paste entire file contents here..."]');
    expect((textarea.element as HTMLTextAreaElement).value).toContain('{"debug": true}');
  });

  it('uses monospace font for config editor textarea and allows opening bigger editor modal', async () => {
    const rawYaml = `services:
  web:
    image: nginx:alpine
    configs:
      - source: nginx_conf
        target: /etc/nginx/nginx.conf

configs:
  nginx_conf:
    content: |
      events {}
      http {
        server {
          listen 80;
        }
      }
`;
    const wrapper = mount(ComposeFormEditor, {
      props: {
        initialAppId: 'nginx-modal-test',
        initialTitle: 'Nginx Modal Test',
        initialComposeYaml: rawYaml,
      },
    });

    // 1. Check monospace styling on the inline config in Configs tab
    const configsTabBtn = wrapper.findAll('button').find((b) => b.text().includes('Configs & Files'));
    await configsTabBtn!.trigger('click');

    const textarea = wrapper.find('textarea[placeholder="Paste entire file contents here..."]');
    expect(textarea.exists()).toBe(true);
    expect(textarea.classes()).toContain('mono');
    expect(textarea.classes()).toContain('config-editor-textarea');
    expect(textarea.attributes('style')).toContain('font-family: var(--font-mono)');

    // 2. Modal is initially closed
    expect(wrapper.find('.config-editor-modal').exists()).toBe(false);

    // 3. Find and click "Bigger Editor" button
    const biggerEditorBtn = wrapper.findAll('button').find((b) => b.text().includes('Bigger Editor'));
    expect(biggerEditorBtn).toBeDefined();
    await biggerEditorBtn!.trigger('click');

    // 4. Modal should now be open
    const modal = wrapper.find('.config-editor-modal');
    expect(modal.exists()).toBe(true);
    expect(modal.text()).toContain('Config Editor');
    expect(modal.text()).toContain('nginx_conf');

    // 5. Close the modal via "Done Editing"
    const doneBtn = modal.findAll('button').find((b) => b.text().includes('Done Editing'));
    expect(doneBtn).toBeDefined();
    await doneBtn!.trigger('click');

    // 6. Modal should be closed
    expect(wrapper.find('.config-editor-modal').exists()).toBe(false);
  });

  it('allows opening bigger editor modal directly from service volume mount row', async () => {
    const rawYaml = `services:
  web:
    image: nginx:alpine
    configs:
      - source: custom_config
        target: /app/config.ini

configs:
  custom_config:
    content: "key=value"
`;
    const wrapper = mount(ComposeFormEditor, {
      props: {
        initialAppId: 'app-vol-modal',
        initialTitle: 'App Vol Modal',
        initialComposeYaml: rawYaml,
      },
    });

    // Navigate to Storage & Volumes
    const volumesSubTab = wrapper.findAll('button').find((b) => b.text().includes('Storage & Volumes'));
    await volumesSubTab!.trigger('click');

    // Find the inline textarea and verify mono styling
    const inlineTextarea = wrapper.find('textarea.config-editor-textarea');
    expect(inlineTextarea.exists()).toBe(true);
    expect(inlineTextarea.classes()).toContain('mono');
    expect(inlineTextarea.attributes('style')).toContain('font-family: var(--font-mono)');

    // Click "Bigger Editor" from the volume row
    const biggerBtn = wrapper.findAll('button').find((b) => b.text().includes('Bigger Editor'));
    expect(biggerBtn).toBeDefined();
    await biggerBtn!.trigger('click');

    // Verify modal is open with custom_config
    const modal = wrapper.find('.config-editor-modal');
    expect(modal.exists()).toBe(true);
    expect(modal.text()).toContain('custom_config');

    // Close modal via close button
    const closeBtn = modal.find('.modal-close-btn');
    expect(closeBtn.exists()).toBe(true);
    await closeBtn.trigger('click');

    expect(wrapper.find('.config-editor-modal').exists()).toBe(false);
  });
});
