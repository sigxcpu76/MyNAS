import { ref } from 'vue';

export type ModalTone = 'danger' | 'warning' | 'info' | 'primary';

export interface ModalCheckboxOption {
  label: string;
  checked?: boolean;
  hint?: string;
}

export interface ModalPromptOption {
  label?: string;
  placeholder?: string;
  defaultValue?: string;
  required?: boolean;
}

export interface ModalOptions {
  title?: string;
  message: string;
  details?: string;
  confirmText?: string;
  cancelText?: string;
  tone?: ModalTone;
  icon?: string;
  checkbox?: ModalCheckboxOption;
  prompt?: ModalPromptOption;
  alertOnly?: boolean;
  /**
   * Type-to-confirm safety gate for destructive actions: the confirm button
   * stays disabled until the user types this exact text (usually the entity id/name).
   */
  confirmPhrase?: string;
}

export interface ModalResult {
  confirmed: boolean;
  checkboxChecked?: boolean;
  inputValue?: string;
}

export interface ActiveModalState extends ModalOptions {
  id: number;
  isOpen: boolean;
  checkboxValue: boolean;
  inputValue: string;
  confirmPhraseValue: string;
  resolve: (res: ModalResult) => void;
}

export const activeModal = ref<ActiveModalState | null>(null);

let modalSeq = 0;

export function askModal(options: ModalOptions | string): Promise<ModalResult> {
  const opts: ModalOptions = typeof options === 'string' ? { message: options } : { ...options };

  return new Promise((resolve) => {
    modalSeq++;
    activeModal.value = {
      ...opts,
      id: modalSeq,
      isOpen: true,
      tone: opts.tone || 'primary',
      confirmText: opts.confirmText || (opts.alertOnly ? 'OK' : 'Confirm'),
      cancelText: opts.cancelText || 'Cancel',
      checkboxValue: opts.checkbox?.checked ?? false,
      inputValue: opts.prompt?.defaultValue ?? '',
      confirmPhraseValue: '',
      resolve,
    };
  });
}

export async function confirmModal(options: ModalOptions | string): Promise<boolean> {
  const res = await askModal(options);
  return res.confirmed;
}

/**
 * Standard destructive-action confirmation that requires typing the entity name.
 * Returns the full ModalResult so callers can read optional checkbox state.
 */
export function confirmDelete(options: {
  kind: string;
  name: string;
  details?: string;
  confirmText?: string;
  checkbox?: ModalCheckboxOption;
}): Promise<ModalResult> {
  return askModal({
    title: `Delete ${options.kind}`,
    message: `This will permanently delete ${options.kind.toLowerCase()} "${options.name}".`,
    details: options.details,
    tone: 'danger',
    confirmText: options.confirmText || 'Delete',
    confirmPhrase: options.name,
    checkbox: options.checkbox,
  });
}

export async function alertModal(message: string, title = 'Notice'): Promise<void> {
  await askModal({
    title,
    message,
    alertOnly: true,
    tone: 'info',
    confirmText: 'OK',
  });
}

export function closeActiveModal(confirmed: boolean): void {
  if (!activeModal.value) return;
  const current = activeModal.value;
  activeModal.value = null;
  current.resolve({
    confirmed,
    checkboxChecked: current.checkboxValue,
    inputValue: current.inputValue,
  });
}
