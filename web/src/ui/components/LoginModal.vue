<script setup lang="ts">
import { ref } from 'vue';
import { services } from '../../container';
import { nasState, notify, refreshAllData, startEventStream } from '../store';
import NanLogo from './NanLogo.vue';

const username = ref('sigxcpu');
const password = ref('admin123');
const submitting = ref(false);
const errorMsg = ref('');

async function handleLogin() {
  errorMsg.value = '';
  submitting.value = true;
  try {
    const session = await services.auth.login(username.value, password.value);
    nasState.currentUser = session.user;
    nasState.authenticated = true;
    notify('success', `Authenticated as Linux user ${session.user.username || username.value}`);
    await refreshAllData();
    startEventStream();
  } catch (err: any) {
    errorMsg.value = err?.message || 'Authentication failed';
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <div class="modal-backdrop">
    <div class="modal-card" style="max-width: 440px">
      <div style="display: flex; align-items: center; gap: 12px; margin-bottom: 18px">
        <NanLogo :size="42" :show-text="false" />
        <div>
          <h2 style="font-size: 18px; font-weight: 700">Linux System Login</h2>
          <p class="card-subtitle">Authenticating against Linux PAM & `/etc/passwd` (`/v1/auth:login`)</p>
        </div>
      </div>

      <div
        style="
          background: rgba(56, 189, 248, 0.08);
          border: 1px solid rgba(56, 189, 248, 0.25);
          border-radius: 8px;
          padding: 10px 12px;
          margin-bottom: 16px;
          font-size: 12.5px;
          color: var(--text-secondary);
        "
      >
        Sign in with your <strong>underlying Linux account</strong> (e.g. <code class="mono">sigxcpu</code> or <code class="mono">root</code>).
        Administrative rights are mapped directly from Linux groups (<code class="mono">wheel</code> / <code class="mono">sudo</code>).
      </div>

      <form @submit.prevent="handleLogin" style="display: flex; flex-direction: column; gap: 14px">
        <div class="form-group">
          <label>Linux System Username</label>
          <input
            v-model="username"
            type="text"
            class="input mono"
            placeholder="e.g. sigxcpu or root"
            autocomplete="username"
            required
          />
        </div>

        <div class="form-group">
          <label>Linux Password</label>
          <input
            v-model="password"
            type="password"
            class="input"
            placeholder="••••••••"
            autocomplete="current-password"
            required
          />
        </div>

        <div
          v-if="errorMsg"
          style="
            color: var(--danger-text);
            background: var(--danger-bg);
            border: 1px solid var(--danger-border);
            padding: 8px 10px;
            border-radius: 6px;
            font-size: 12.5px;
          "
        >
          {{ errorMsg }}
        </div>

        <button type="submit" class="btn btn-primary" :disabled="submitting" style="width: 100%; padding: 10px">
          {{ submitting ? 'Verifying Linux Credentials...' : 'Sign In with Linux Account' }}
        </button>
      </form>
    </div>
  </div>
</template>
