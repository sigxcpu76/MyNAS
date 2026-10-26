<script setup lang="ts">
import type { VirtualMachineConsoleSession } from '../../domain/models';
import VncConsole from './VncConsole.vue';

const props = defineProps<{
  session: VirtualMachineConsoleSession;
}>();

const emit = defineEmits<{
  (e: 'close'): void;
}>();

function handleDetach() {
  const url = `/vnc-console?vm=${encodeURIComponent(props.session.vmName)}&token=${encodeURIComponent(props.session.token)}&port=${props.session.vncPort || ''}`;
  const w = 1120;
  const h = 820;
  const left = typeof window !== 'undefined' ? Math.max(0, (window.screen.width - w) / 2) : 50;
  const top = typeof window !== 'undefined' ? Math.max(0, (window.screen.height - h) / 2) : 50;
  const win = window.open(
    url,
    `nas_vnc_${props.session.vmName}`,
    `width=${w},height=${h},left=${left},top=${top},menubar=no,toolbar=no,location=no,status=no,resizable=yes,scrollbars=yes`
  );
  if (win) {
    win.focus();
    emit('close');
  }
}
</script>

<template>
  <div class="modal-backdrop" @click.self="emit('close')">
    <div class="modal-card vnc-modal-card">
      <VncConsole
        :vm-name="session.vmName"
        :token="session.token"
        :vnc-port="session.vncPort"
        :websocket-url="session.websocketUrl"
        :standalone="false"
        @close="emit('close')"
        @detach="handleDetach"
      />
    </div>
  </div>
</template>

<style scoped>
.vnc-modal-card {
  max-width: 1200px;
  width: 95vw;
  max-height: 94vh;
  display: flex;
  flex-direction: column;
}
</style>
