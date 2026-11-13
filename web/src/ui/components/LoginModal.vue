<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { mdiEyeOffOutline, mdiEyeOutline } from '@mdi/js';
import { services } from '../../container';
import { nasState, notify, refreshAllData, startEventStream, startWatchStream } from '../store';
import MdiIcon from './MdiIcon.vue';
import NanLogo from './NanLogo.vue';

const username = ref('');
const password = ref('');
const showPassword = ref(false);
const submitting = ref(false);
const errorMsg = ref('');
const usernameInputRef = ref<HTMLInputElement | null>(null);

onMounted(() => {
  usernameInputRef.value?.focus();
});

async function handleLogin() {
  errorMsg.value = '';
  submitting.value = true;
  try {
    const session = await services.auth.login(username.value, password.value);
    nasState.currentUser = session.user;
    nasState.authenticated = true;
    notify('success', `Authenticated as Linux user ${session.user.username || username.value}`);
    await refreshAllData();
    startWatchStream();
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
          <h2 style="font-size: 18px; font-weight: 700">Sign In</h2>
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
        Sign in with a Linux host administrator account (<code>root</code> or member of <code>sudo</code> / <code>wheel</code>).
      </div>

      <form
        method="post"
        action="#"
        @submit.prevent="handleLogin"
        style="display: flex; flex-direction: column; gap: 14px"
      >
        <div class="form-group">
          <label for="login-username">Username</label>
          <input
            id="login-username"
            ref="usernameInputRef"
            name="username"
            v-model="username"
            type="text"
            class="input mono"
            placeholder="e.g. root or your user"
            autocomplete="username"
            autofocus
            required
          />
        </div>

        <div class="form-group">
          <label for="login-password">Password</label>
          <div style="position: relative; display: flex; align-items: center">
            <input
              id="login-password"
              name="password"
              v-model="password"
              :type="showPassword ? 'text' : 'password'"
              class="input"
              style="padding-right: 36px; width: 100%"
              placeholder="••••••••"
              autocomplete="current-password"
              required
            />
            <button
              type="button"
              class="btn btn-xs btn-ghost"
              :aria-label="showPassword ? 'Hide password' : 'Show password'"
              :title="showPassword ? 'Hide password' : 'Show password'"
              style="position: absolute; right: 6px; padding: 3px 5px; color: var(--text-secondary)"
              @click="showPassword = !showPassword"
            >
              <MdiIcon :path="showPassword ? mdiEyeOffOutline : mdiEyeOutline" size="15" />
            </button>
          </div>
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
          {{ submitting ? 'Signing In...' : 'Sign In' }}
        </button>
      </form>
    </div>
  </div>
</template>
