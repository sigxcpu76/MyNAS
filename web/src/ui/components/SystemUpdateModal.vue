<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import {
  mdiAlertCircle,
  mdiCheckCircle,
  mdiClose,
  mdiDownload,
  mdiGithub,
  mdiOpenInNew,
  mdiRefresh,
  mdiRocketLaunch,
  mdiServer,
  mdiUpdate,
} from '@mdi/js';
import MdiIcon from './MdiIcon.vue';
import { checkForUpdate, nasState } from '../store';
import { services } from '../../container';
import { formatBytes } from '../../domain/models';

const emit = defineEmits<{
  (e: 'close'): void;
}>();

const updateInfo = computed(() => nasState.systemUpdate);
const isChecking = computed(() => nasState.isCheckingUpdate);
const isApplying = computed(() => nasState.isApplyingUpdate);

const updateStep = ref<'idle' | 'updating' | 'restarting' | 'error'>('idle');
const errorMessage = ref('');
const reconnectCountdown = ref(10);
let reconnectTimer: any = null;

onMounted(() => {
  if (!updateInfo.value) {
    handleCheck(false);
  }
});

async function handleCheck(force = true) {
  errorMessage.value = '';
  try {
    await checkForUpdate(force);
  } catch (err: any) {
    errorMessage.value = err?.message || 'Failed to check for updates';
  }
}

async function handleApply() {
  if (!updateInfo.value || isApplying.value) return;
  errorMessage.value = '';
  updateStep.value = 'updating';

  try {
    const res = await services.system.applyUpdate(updateInfo.value.latestVersion, true);
    if (res.serviceRestarting) {
      updateStep.value = 'restarting';
      startReconnectPolling();
    } else {
      updateStep.value = 'idle';
      await handleCheck(true);
    }
  } catch (err: any) {
    updateStep.value = 'error';
    errorMessage.value = err?.message || 'Failed to apply update';
  }
}

function startReconnectPolling() {
  reconnectCountdown.value = 8;
  if (reconnectTimer) clearInterval(reconnectTimer);

  reconnectTimer = setInterval(async () => {
    reconnectCountdown.value--;
    if (reconnectCountdown.value <= 3) {
      try {
        await services.system.getTelemetry();
        // Daemon is back up!
        clearInterval(reconnectTimer);
        reconnectTimer = null;
        window.location.reload();
      } catch {
        // Keep waiting
      }
    }

    if (reconnectCountdown.value <= 0) {
      // Periodic check until response returns
      try {
        await services.system.getTelemetry();
        clearInterval(reconnectTimer);
        reconnectTimer = null;
        window.location.reload();
      } catch {
        reconnectCountdown.value = 2; // continue polling
      }
    }
  }, 1000);
}

function handleClose() {
  if (updateStep.value === 'restarting') return;
  emit('close');
}
</script>

<template>
  <div class="modal-backdrop" @click.self="handleClose">
    <div class="modal-card system-update-modal" style="max-width: 680px">
      <!-- Modal Header -->
      <div class="modal-header">
        <div style="display: flex; align-items: center; gap: 10px">
          <div class="icon-avatar">
            <MdiIcon :path="mdiUpdate" size="20" />
          </div>
          <div>
            <h2 style="font-size: 17px; font-weight: 700; margin: 0">Software Updates (OTA)</h2>
            <p style="font-size: 12px; color: var(--text-secondary); margin: 2px 0 0 0">
              Over-The-Air single binary daemon and web UI update pipeline
            </p>
          </div>
        </div>
        <button
          class="btn btn-xs btn-icon btn-ghost"
          :disabled="updateStep === 'restarting' || updateStep === 'updating'"
          title="Close modal"
          @click="handleClose"
        >
          <MdiIcon :path="mdiClose" size="16" />
        </button>
      </div>

      <!-- Modal Body -->
      <div class="modal-body" style="padding: 16px 20px; display: flex; flex-direction: column; gap: 16px">
        <!-- Reconnection Overlay -->
        <div v-if="updateStep === 'restarting'" class="restarting-box">
          <div class="spinner-lg" style="margin-bottom: 12px"></div>
          <h3 style="font-size: 16px; font-weight: 700; margin: 0 0 6px 0">Restarting nasctl Service</h3>
          <p style="font-size: 13px; color: var(--text-secondary); margin: 0 0 12px 0">
            The updated binary has been installed. Reconnecting in {{ reconnectCountdown }}s...
          </p>
          <span class="badge badge-info" style="font-family: monospace">
            Upgrading to {{ updateInfo?.latestVersion || 'new version' }}
          </span>
        </div>

        <template v-else>
          <!-- Error banner -->
          <div v-if="errorMessage" class="alert alert-danger" style="margin: 0; font-size: 12.5px">
            <MdiIcon :path="mdiAlertCircle" size="16" />
            <span>{{ errorMessage }}</span>
          </div>

          <!-- Version Comparison Card -->
          <div class="version-banner">
            <div class="version-col">
              <span class="version-label">Current Running Version</span>
              <div class="version-val">
                <span class="mono font-semibold">{{ updateInfo?.currentVersion || 'Checking...' }}</span>
                <span v-if="updateInfo?.currentCommit" class="text-muted" style="font-size: 11px">
                  ({{ updateInfo.currentCommit.slice(0, 7) }})
                </span>
              </div>
            </div>

            <div class="version-divider">→</div>

            <div class="version-col">
              <span class="version-label">Latest Upstream Release</span>
              <div class="version-val">
                <span class="mono font-semibold" style="color: var(--accent-primary)">
                  {{ updateInfo?.latestVersion || 'Checking...' }}
                </span>
                <span
                  v-if="updateInfo?.updateAvailable"
                  class="badge badge-warning"
                  style="font-size: 10px; margin-left: 6px"
                >
                  Update Available
                </span>
                <span
                  v-else-if="updateInfo && !updateInfo.updateAvailable"
                  class="badge badge-success"
                  style="font-size: 10px; margin-left: 6px"
                >
                  Up to Date
                </span>
              </div>
            </div>
          </div>

          <!-- Metadata spec table -->
          <div class="details-card">
            <div class="spec-row">
              <span class="spec-label">Target Binary Path:</span>
              <span class="spec-val mono">{{ updateInfo?.binaryPath || '/usr/local/bin/nasctl' }}</span>
            </div>
            <div v-if="updateInfo?.publishedAt" class="spec-row">
              <span class="spec-label">Release Date:</span>
              <span class="spec-val">{{ new Date(updateInfo.publishedAt).toLocaleString() }}</span>
            </div>
            <div v-if="updateInfo?.binarySizeBytes" class="spec-row">
              <span class="spec-label">Package Size:</span>
              <span class="spec-val mono">{{ formatBytes(updateInfo.binarySizeBytes) }}</span>
            </div>
            <div v-if="updateInfo?.expectedSha256" class="spec-row">
              <span class="spec-label">SHA256 Hash:</span>
              <span class="spec-val mono text-truncate" style="max-width: 380px" :title="updateInfo.expectedSha256">
                {{ updateInfo.expectedSha256 }}
              </span>
            </div>
          </div>

          <!-- Release Notes -->
          <div v-if="updateInfo?.releaseNotes" class="release-notes-box">
            <div class="release-notes-header">
              <span style="font-weight: 600; font-size: 12.5px">Release Notes / Changelog</span>
              <a
                v-if="updateInfo.releaseUrl"
                :href="updateInfo.releaseUrl"
                target="_blank"
                rel="noopener"
                class="release-link"
              >
                <span>View on GitHub</span>
                <MdiIcon :path="mdiOpenInNew" size="12" />
              </a>
            </div>
            <div class="release-notes-content">
              <pre>{{ updateInfo.releaseNotes.trim() }}</pre>
            </div>
          </div>

          <!-- Up to date confirmation -->
          <div
            v-if="updateInfo && !updateInfo.updateAvailable"
            class="alert alert-success"
            style="margin: 0; font-size: 12.5px"
          >
            <MdiIcon :path="mdiCheckCircle" size="16" />
            <span>nasctl is running the latest version! No updates needed.</span>
          </div>
        </template>
      </div>

      <!-- Modal Footer -->
      <div
        v-if="updateStep !== 'restarting'"
        class="modal-footer"
        style="display: flex; justify-content: space-between; align-items: center; padding: 12px 20px"
      >
        <button
          class="btn btn-sm btn-secondary"
          :disabled="isChecking || isApplying"
          @click="handleCheck(true)"
        >
          <MdiIcon :path="mdiRefresh" size="14" :class="{ 'spin-icon': isChecking }" />
          <span>{{ isChecking ? 'Checking GitHub...' : 'Check for Updates' }}</span>
        </button>

        <div style="display: flex; gap: 8px">
          <button class="btn btn-sm btn-ghost" :disabled="isApplying" @click="handleClose">
            Close
          </button>
          <button
            v-if="updateInfo?.updateAvailable"
            class="btn btn-sm btn-primary"
            :disabled="isApplying || isChecking"
            @click="handleApply"
          >
            <MdiIcon :path="mdiRocketLaunch" size="14" :class="{ 'spin-icon': isApplying }" />
            <span>{{ isApplying ? 'Downloading & Updating...' : `Update Now to ${updateInfo.latestVersion}` }}</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.system-update-modal {
  border: 1px solid var(--border-color);
  box-shadow: 0 16px 40px rgba(0, 0, 0, 0.4);
}

.icon-avatar {
  width: 36px;
  height: 36px;
  border-radius: 8px;
  background: rgba(56, 189, 248, 0.12);
  color: var(--accent-primary);
  display: flex;
  align-items: center;
  justify-content: center;
}

.version-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 18px;
  border-radius: 8px;
  background: var(--bg-surface-elevated, rgba(255, 255, 255, 0.04));
  border: 1px solid var(--border-color);
}

.version-col {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.version-label {
  font-size: 11px;
  color: var(--text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.version-val {
  display: flex;
  align-items: center;
  font-size: 14px;
}

.version-divider {
  font-size: 18px;
  color: var(--text-secondary);
  font-weight: 300;
}

.details-card {
  padding: 10px 14px;
  border-radius: 8px;
  background: rgba(0, 0, 0, 0.15);
  border: 1px solid var(--border-color);
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.spec-row {
  display: flex;
  justify-content: space-between;
  font-size: 12px;
}

.spec-label {
  color: var(--text-secondary);
}

.spec-val {
  color: var(--text-primary);
}

.release-notes-box {
  border: 1px solid var(--border-color);
  border-radius: 8px;
  overflow: hidden;
  background: var(--bg-surface);
}

.release-notes-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 12px;
  background: rgba(255, 255, 255, 0.03);
  border-bottom: 1px solid var(--border-color);
}

.release-link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  color: var(--accent-primary);
  text-decoration: none;
}

.release-link:hover {
  text-decoration: underline;
}

.release-notes-content {
  padding: 12px;
  max-height: 200px;
  overflow-y: auto;
  font-size: 12px;
  line-height: 1.5;
}

.release-notes-content pre {
  margin: 0;
  font-family: inherit;
  white-space: pre-wrap;
  word-break: break-word;
}

.restarting-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 40px 20px;
  text-align: center;
}

.spinner-lg {
  width: 44px;
  height: 44px;
  border: 3px solid rgba(56, 189, 248, 0.2);
  border-top-color: var(--accent-primary);
  border-radius: 50%;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
