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
      resolve,
    };
  });
}

export async function confirmModal(options: ModalOptions | string): Promise<boolean> {
  const res = await askModal(options);
  return res.confirmed;
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
