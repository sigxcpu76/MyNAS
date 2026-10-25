import { describe, expect, it } from 'vitest';
import {
  activeModal,
  alertModal,
  askModal,
  closeActiveModal,
  confirmModal,
} from '../src/ui/modal';

describe('Modal Dialog System', () => {
  it('opens and confirms a confirmation modal', async () => {
    const promise = confirmModal({
      title: 'Destroy Pool',
      message: 'Are you sure?',
      tone: 'danger',
    });

    expect(activeModal.value).not.toBeNull();
    expect(activeModal.value?.isOpen).toBe(true);
    expect(activeModal.value?.title).toBe('Destroy Pool');
    expect(activeModal.value?.tone).toBe('danger');

    closeActiveModal(true);
    const confirmed = await promise;
    expect(confirmed).toBe(true);
    expect(activeModal.value).toBeNull();
  });

  it('opens and cancels a confirmation modal', async () => {
    const promise = confirmModal('Simple question?');

    expect(activeModal.value?.message).toBe('Simple question?');
    closeActiveModal(false);

    const confirmed = await promise;
    expect(confirmed).toBe(false);
    expect(activeModal.value).toBeNull();
  });

  it('supports questions with checkbox options (askModal)', async () => {
    const promise = askModal({
      title: 'Delete App',
      message: 'Delete Nextcloud?',
      tone: 'danger',
      checkbox: {
        label: 'Purge volumes',
        checked: false,
      },
    });

    expect(activeModal.value?.checkbox?.label).toBe('Purge volumes');
    expect(activeModal.value?.checkboxValue).toBe(false);

    // Toggle the checkbox
    if (activeModal.value) {
      activeModal.value.checkboxValue = true;
    }

    closeActiveModal(true);
    const result = await promise;

    expect(result.confirmed).toBe(true);
    expect(result.checkboxChecked).toBe(true);
  });

  it('supports alert-only modals (alertModal)', async () => {
    const promise = alertModal('System maintenance scheduled', 'Notice');

    expect(activeModal.value?.alertOnly).toBe(true);
    expect(activeModal.value?.title).toBe('Notice');
    expect(activeModal.value?.confirmText).toBe('OK');

    closeActiveModal(true);
    await promise;
    expect(activeModal.value).toBeNull();
  });
});
