<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { services } from '../../container';
import { initSession, nasState, notify } from '../store';
import type { VirtualMachineConsoleSession } from '../../domain/models';
import VncConsole from './VncConsole.vue';

const loading = ref(true);
const error = ref('');
const session = ref<VirtualMachineConsoleSession | null>(null);

const params = typeof window !== 'undefined' ? new URLSearchParams(window.location.search) : new URLSearchParams();
const vmName = params.get('vm') || '';
const initialToken = params.get('token') || '';
const port = params.get('port') ? Number(params.get('port')) : undefined;

async function loadSession() {
  if (!vmName) {
    error.value = 'No Virtual Machine specified. URL must include ?vm=<vmName>.';
    loading.value = false;
    return;
  }

  // Ensure session credentials are read from localStorage before API requests
  await initSession();

  // If token is already present in query parameters, use it immediately
  if (initialToken) {
    session.value = {
      vmName,
      token: initialToken,
      websocketUrl: `/ws/novnc/${encodeURIComponent(vmName)}?token=${encodeURIComponent(initialToken)}`,
      vncPort: port || 5900,
      resolution: '800x600',
      expiresAt: '',
    };
    loading.value = false;
    return;
  }

  // Otherwise request a fresh console session from backend
  try {
    loading.value = true;
    error.value = '';
    const s = await services.kvm.openConsole(vmName);
    session.value = s;
  } catch (err: any) {
    error.value = err?.message || 'Failed to open live QEMU console session';
    notify('error', error.value);
  } finally {
    loading.value = false;
  }
}

function handleClose() {
  if (typeof window !== 'undefined') {
    if (window.opener) {
      window.close();
    } else {
      window.location.href = '/kvm';
    }
  }
}

onMounted(() => {
  loadSession();
});
</script>

<template>
  <div class="standalone-vnc-container">
    <div v-if="loading" class="standalone-state-box">
      <span class="spinner" style="width: 28px; height: 28px; border-width: 3px"></span>
      <p style="margin-top: 14px; font-size: 14px; color: var(--text-secondary)">
        Connecting to console for <strong>{{ vmName || 'Virtual Machine' }}</strong>...
      </p>
    </div>

    <div v-else-if="error" class="card standalone-error-card">
      <div class="card-header">
        <h3 class="card-title" style="color: var(--danger-text)">Console Connection Error</h3>
      </div>
      <p style="margin: 12px 0; color: var(--text-secondary); font-size: 13.5px">
        {{ error }}
      </p>
      <div style="display: flex; gap: 10px; margin-top: 16px">
        <button class="btn btn-primary btn-sm" @click="loadSession">Retry Connection</button>
        <button class="btn btn-secondary btn-sm" @click="handleClose">Close Window</button>
      </div>
    </div>

    <VncConsole
      v-else-if="session"
      :vm-name="session.vmName"
      :token="session.token"
      :vnc-port="session.vncPort"
      :websocket-url="session.websocketUrl"
      :standalone="true"
      @close="handleClose"
    />
  </div>
</template>

<style scoped>
.standalone-vnc-container {
  width: 100vw;
  height: 100vh;
  box-sizing: border-box;
  background: var(--bg-primary, #0f172a);
  display: flex;
  flex-direction: column;
}

.standalone-state-box {
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  height: 100vh;
  color: var(--text-primary);
}

.standalone-error-card {
  max-width: 520px;
  margin: 80px auto;
  border-color: var(--danger-border);
}
</style>
