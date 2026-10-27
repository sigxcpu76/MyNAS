import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import { nextTick } from 'vue';
import ConfirmModal from '../src/ui/components/ConfirmModal.vue';
import { activeModal, closeActiveModal, confirmDelete } from '../src/ui/modal';

describe('Type-to-confirm delete modal', () => {
  it('keeps the delete button disabled until the exact entity name is typed', async () => {
    const wrapper = mount(ConfirmModal, { attachTo: document.body });
    const promise = confirmDelete({ kind: 'Volume', name: 'tank/vms/arch' });
    await nextTick();

    expect(activeModal.value?.confirmPhrase).toBe('tank/vms/arch');
    expect(wrapper.text()).toContain('Delete Volume');

    const confirmBtn = () => wrapper.findAll('.confirm-modal-footer button').at(-1)!;
    expect(confirmBtn().attributes('disabled')).toBeDefined();

    const input = wrapper.find('#confirm-phrase-input');
    await input.setValue('tank/vms/arc');
    expect(confirmBtn().attributes('disabled')).toBeDefined();

    // Clicking while disabled / Enter with a wrong phrase must not confirm
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' }));
    await nextTick();
    expect(activeModal.value).not.toBeNull();

    await input.setValue('tank/vms/arch');
    expect(confirmBtn().attributes('disabled')).toBeUndefined();
    await confirmBtn().trigger('click');

    const res = await promise;
    expect(res.confirmed).toBe(true);
    wrapper.unmount();
  });

  it('can be cancelled without typing', async () => {
    const promise = confirmDelete({ kind: 'Dataset', name: 'tank/media' });
    closeActiveModal(false);
    expect((await promise).confirmed).toBe(false);
  });
});
