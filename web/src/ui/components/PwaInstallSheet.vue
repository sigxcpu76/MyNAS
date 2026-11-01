<script setup lang="ts">
import {
  mdiDownload,
  mdiExportVariant,
  mdiPlusBoxOutline,
  mdiDotsVertical,
  mdiCellphoneArrowDown,
  mdiCheckCircle,
} from '@mdi/js';
import MdiIcon from './MdiIcon.vue';
import BottomSheet from './BottomSheet.vue';
import { usePwaInstall } from '../composables/usePwaInstall';

const {
  hasNativePrompt,
  isIOS,
  showInstallSheet,
  triggerNativePrompt,
} = usePwaInstall();

async function handleInstallClick() {
  if (hasNativePrompt.value) {
    await triggerNativePrompt();
  }
}
</script>

<template>
  <BottomSheet
    v-model="showInstallSheet"
    title="Install NaN App"
    subtitle="Add to your home screen for quick full-screen access"
  >
    <div class="install-sheet-body">
      <!-- App Header Badge -->
      <div class="app-install-badge">
        <img src="/icon-192.png" alt="NaN Logo" class="app-icon" />
        <div class="app-info">
          <div class="app-name">NaN // Not Another NAS</div>
          <div class="app-tagline">Web Management & Hypervisor</div>
        </div>
      </div>

      <!-- Native Browser Install Action (If supported by browser) -->
      <div v-if="hasNativePrompt" class="native-install-section">
        <button class="btn btn-primary btn-block btn-install" @click="handleInstallClick">
          <MdiIcon :path="mdiCellphoneArrowDown" size="20" style="margin-right: 8px" />
          <span>Install NaN Now</span>
        </button>
      </div>

      <!-- iOS Safari Instructions -->
      <div v-else-if="isIOS" class="instructions-card">
        <div class="instructions-title">Install on iOS (Safari)</div>
        <div class="steps-list">
          <div class="step-item">
            <div class="step-num">1</div>
            <div class="step-content">
              <span>Tap the <strong>Share</strong> button in Safari's bottom toolbar.</span>
              <div class="step-icon-tag">
                <MdiIcon :path="mdiExportVariant" size="16" style="color: #38bdf8" />
                <span>Share</span>
              </div>
            </div>
          </div>

          <div class="step-item">
            <div class="step-num">2</div>
            <div class="step-content">
              <span>Scroll down and tap <strong>Add to Home Screen</strong>.</span>
              <div class="step-icon-tag">
                <MdiIcon :path="mdiPlusBoxOutline" size="16" style="color: #4ade80" />
                <span>Add to Home Screen</span>
              </div>
            </div>
          </div>

          <div class="step-item">
            <div class="step-num">3</div>
            <div class="step-content">
              <span>Tap <strong>Add</strong> in the top right corner.</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Android / Other Browser Instructions -->
      <div v-else class="instructions-card">
        <div class="instructions-title">Install on Android / Chrome</div>
        <div class="steps-list">
          <div class="step-item">
            <div class="step-num">1</div>
            <div class="step-content">
              <span>Tap the browser menu button in the top right corner.</span>
              <div class="step-icon-tag">
                <MdiIcon :path="mdiDotsVertical" size="16" style="color: #38bdf8" />
                <span>Menu</span>
              </div>
            </div>
          </div>

          <div class="step-item">
            <div class="step-num">2</div>
            <div class="step-content">
              <span>Select <strong>Install app</strong> or <strong>Add to Home screen</strong>.</span>
              <div class="step-icon-tag">
                <MdiIcon :path="mdiCellphoneArrowDown" size="16" style="color: #4ade80" />
                <span>Add to Home screen</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- PWA Features Summary -->
      <div class="features-summary">
        <div class="feature-item">
          <MdiIcon :path="mdiCheckCircle" size="14" style="color: #4ade80" />
          <span>Full-screen app view without browser bars</span>
        </div>
        <div class="feature-item">
          <MdiIcon :path="mdiCheckCircle" size="14" style="color: #4ade80" />
          <span>Instant home screen launch</span>
        </div>
        <div class="feature-item">
          <MdiIcon :path="mdiCheckCircle" size="14" style="color: #4ade80" />
          <span>Smooth mobile navigation & telemetry</span>
        </div>
      </div>
    </div>
  </BottomSheet>
</template>

<style scoped>
.install-sheet-body {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.app-install-badge {
  display: flex;
  align-items: center;
  gap: 14px;
  background: var(--bg-surface-elevated, #243248);
  border: 1px solid var(--border-color, #334155);
  border-radius: 14px;
  padding: 12px 14px;
}

.app-icon {
  width: 52px;
  height: 52px;
  border-radius: 12px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
}

.app-info {
  display: flex;
  flex-direction: column;
}

.app-name {
  font-size: 14px;
  font-weight: 800;
  color: var(--text-primary, #f8fafc);
}

.app-tagline {
  font-size: 12px;
  color: var(--text-secondary, #94a3b8);
  margin-top: 2px;
}

.btn-install {
  height: 44px;
  font-size: 14px;
  font-weight: 700;
  border-radius: 12px;
}

.instructions-card {
  background: var(--bg-surface-elevated, #243248);
  border: 1px solid var(--border-color, #334155);
  border-radius: 14px;
  padding: 14px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.instructions-title {
  font-size: 12px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: var(--accent-primary, #38bdf8);
}

.steps-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.step-item {
  display: flex;
  align-items: flex-start;
  gap: 12px;
}

.step-num {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--accent-primary, #38bdf8);
  color: #0f172a;
  font-size: 12px;
  font-weight: 800;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  margin-top: 2px;
}

.step-content {
  flex: 1;
  font-size: 13px;
  color: var(--text-primary, #f8fafc);
  line-height: 1.4;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.step-icon-tag {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: var(--bg-base, #0f172a);
  border: 1px solid var(--border-color, #334155);
  border-radius: 6px;
  padding: 3px 8px;
  font-size: 11px;
  font-weight: 600;
  color: var(--text-secondary, #94a3b8);
  align-self: flex-start;
  margin-top: 2px;
}

.features-summary {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 8px 4px;
}

.feature-item {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: var(--text-secondary, #94a3b8);
}
</style>
