import { describe, expect, it, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import FileDropzone from '../src/ui/components/FileDropzone.vue';

describe('FileDropzone Component', () => {
  it('renders idle state with label, sublabel, and accept attribute', () => {
    const wrapper = mount(FileDropzone, {
      props: {
        accept: '.iso,.img',
        label: 'Drag & drop ISO file here',
        sublabel: 'Supports .iso and .img',
      },
    });

    expect(wrapper.text()).toContain('Drag & drop ISO file here');
    expect(wrapper.text()).toContain('Supports .iso and .img');

    const input = wrapper.find<HTMLInputElement>('input[type="file"]');
    expect(input.exists()).toBe(true);
    expect(input.attributes('accept')).toBe('.iso,.img');
  });

  it('updates classes and label on dragenter and dragleave', async () => {
    const wrapper = mount(FileDropzone, {
      props: {
        accept: '.iso,.img',
        label: 'Drag & drop ISO file here',
      },
    });

    const dropzone = wrapper.find('.file-dropzone');
    expect(dropzone.classes()).not.toContain('drag-over');

    await dropzone.trigger('dragenter');
    expect(dropzone.classes()).toContain('drag-over');
    expect(wrapper.text()).toContain('Drop file to upload immediately');

    await dropzone.trigger('dragleave');
    expect(dropzone.classes()).not.toContain('drag-over');
    expect(wrapper.text()).toContain('Drag & drop ISO file here');
  });

  it('emits file-selected when a valid file is dropped', async () => {
    const wrapper = mount(FileDropzone, {
      props: {
        accept: '.iso,.img',
      },
    });

    const file = new File(['dummy iso content'], 'fedora.iso', { type: 'application/x-iso9660-image' });
    const dropEvent = {
      preventDefault: vi.fn(),
      dataTransfer: {
        files: [file],
      },
    };

    await wrapper.find('.file-dropzone').trigger('drop', dropEvent);

    expect(wrapper.emitted('file-selected')).toBeTruthy();
    expect(wrapper.emitted('file-selected')![0][0]).toBe(file);
    expect(wrapper.emitted('invalid-file')).toBeFalsy();
  });

  it('emits invalid-file when a file with unsupported extension is dropped', async () => {
    const wrapper = mount(FileDropzone, {
      props: {
        accept: '.iso,.img',
      },
    });

    const file = new File(['text content'], 'readme.txt', { type: 'text/plain' });
    const dropEvent = {
      preventDefault: vi.fn(),
      dataTransfer: {
        files: [file],
      },
    };

    await wrapper.find('.file-dropzone').trigger('drop', dropEvent);

    expect(wrapper.emitted('file-selected')).toBeFalsy();
    expect(wrapper.emitted('invalid-file')).toBeTruthy();
    expect(wrapper.emitted('invalid-file')![0][0]).toBe('readme.txt');
  });

  it('supports compound extensions like .tar.gz and .tar.xz for LXC templates', async () => {
    const wrapper = mount(FileDropzone, {
      props: {
        accept: '.tar.gz,.tar.xz,.tar.zst,.tar,.tgz',
      },
    });

    const validFile = new File(['archive'], 'debian-12.tar.xz');
    await wrapper.find('.file-dropzone').trigger('drop', {
      preventDefault: vi.fn(),
      dataTransfer: { files: [validFile] },
    });
    expect(wrapper.emitted('file-selected')).toBeTruthy();

    const invalidFile = new File(['archive'], 'debian-12.zip');
    await wrapper.find('.file-dropzone').trigger('drop', {
      preventDefault: vi.fn(),
      dataTransfer: { files: [invalidFile] },
    });
    expect(wrapper.emitted('invalid-file')).toBeTruthy();
  });

  it('renders rich progress state when isUploading is true', () => {
    const wrapper = mount(FileDropzone, {
      props: {
        isUploading: true,
        progress: {
          filename: 'ubuntu-server-24.04.iso',
          percent: 45,
          loaded: 450 * 1024 * 1024,
          total: 1000 * 1024 * 1024,
        },
      },
    });

    expect(wrapper.find('.file-dropzone').classes()).toContain('is-uploading');
    expect(wrapper.text()).toContain('ubuntu-server-24.04.iso');
    expect(wrapper.text()).toContain('Uploading...');
    expect(wrapper.text()).toContain('45%');

    const fill = wrapper.find('.dropzone-progress-fill');
    expect(fill.exists()).toBe(true);
    expect(fill.attributes('style')).toContain('width: 45%');
  });

  it('exposes openFileDialog to programmatically open the file picker', () => {
    const wrapper = mount(FileDropzone);
    const input = wrapper.find<HTMLInputElement>('input[type="file"]').element;
    const clickSpy = vi.spyOn(input, 'click');

    wrapper.vm.openFileDialog();
    expect(clickSpy).toHaveBeenCalled();
  });
});
