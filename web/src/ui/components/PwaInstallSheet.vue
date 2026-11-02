<script setup lang="ts">
import { computed } from 'vue';
import {
  mdiDownload,
  mdiExportVariant,
  mdiPlusBoxOutline,
  mdiDotsVertical,
  mdiCellphoneArrowDown,
  mdiCheckCircle,
  mdiLockAlertOutline,
  mdiCertificateOutline,
  mdiOpenInNew,
  mdiAndroid,
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

const isHttps = computed(() => {
  if (typeof window === 'undefined') return true;
  return window.location.protocol === 'https:';
});

const httpsUrl = computed(() => {
  if (typeof window === 'undefined') return '#';
  return `https://${window.location.hostname}:8443${window.location.pathname}${window.location.search}`;
});

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

      <!-- HTTPS Recommended Notice if accessing over HTTP -->
      <div v-if="!isHttps" class="https-prompt-card">
        <div class="https-prompt-header">
          <MdiIcon :path="mdiLockAlertOutline" size="20" style="color: #f59e0b" />
          <span class="https-prompt-title">HTTPS Recommended for PWA</span>
        </div>
        <p class="https-prompt-desc">
          Mobile browsers (Chrome / Safari) strictly require a secure HTTPS connection to install standalone PWAs and register Service Workers.
        </p>
        <a :href="httpsUrl" class="btn btn-warning btn-block btn-switch-https">
          <span>Switch to HTTPS (Port 8443)</span>
          <MdiIcon :path="mdiOpenInNew" size="16" style="margin-left: 6px" />
        </a>
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

      <!-- Direct Native APK Download (Available whenever not on iOS) -->
      <div v-if="!isIOS" class="native-apk-box">
        <div class="apk-box-header">
          <MdiIcon :path="mdiAndroid" size="18" style="color: #4ade80" />
          <span class="apk-box-title">Or Get the Native Android App (.apk)</span>
        </div>
        <p class="apk-box-desc">
          Prefer a standalone APK? Includes built-in self-signed SSL trust, server switcher, and offline persistence:
        </p>
        <a href="/nan-nas.apk" download="nan-nas.apk" class="btn btn-success btn-block btn-apk-download">
          <MdiIcon :path="mdiDownload" size="18" style="margin-right: 6px;" />
          <span>Download NaN App (.apk)</span>
        </a>
      </div>

      <!-- Root Certificate Authority Card for 1-Tap Trust -->
      <div class="ca-card">
        <div class="ca-card-header">
          <MdiIcon :path="mdiCertificateOutline" size="18" style="color: #38bdf8" />
          <span class="ca-card-title">1-Tap Certificate Trust</span>
        </div>
        <p class="ca-card-desc">
          If your browser warns about the self-signed SSL certificate on port 8443, download and install the NAS Root CA certificate once:
        </p>
        <a href="/ca.crt" download="nan-nas-ca.crt" class="btn btn-secondary btn-block btn-ca-download">
          <MdiIcon :path="mdiDownload" size="18" style="margin-right: 6px; color: #38bdf8;" />
          <span>Download Root CA (nan-nas-ca.crt)</span>
        </a>
        <details class="ca-details">
          <summary class="ca-summary">How to trust on your phone / PC</summary>
          <div class="ca-steps">
            <div><strong>Android:</strong> Settings &rarr; Security &amp; privacy &rarr; More security &rarr; Install from storage &rarr; <em>CA certificate</em> &rarr; select downloaded <code>nan-nas-ca.crt</code>.</div>
            <div><strong>iOS / Safari:</strong> Download cert &rarr; Allow &rarr; Settings &rarr; Profile Downloaded &rarr; Install &rarr; Settings &rarr; General &rarr; About &rarr; <em>Certificate Trust Settings</em> &rarr; Enable full trust.</div>
            <div><strong>Windows / Mac:</strong> Double-click cert file &rarr; Install into <em>Trusted Root Certification Authorities</em>.</div>
          </div>
        </details>
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

.https-prompt-card {
  background: rgba(245, 158, 11, 0.08);
  border: 1px solid rgba(245, 158, 11, 0.35);
  border-radius: 14px;
  padding: 14px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.https-prompt-header {
  display: flex;
  align-items: center;
  gap: 8px;
}

.https-prompt-title {
  font-size: 13px;
  font-weight: 700;
  color: #fbbf24;
}

.https-prompt-desc {
  font-size: 12px;
  color: var(--text-secondary, #cbd5e1);
  line-height: 1.4;
  margin: 0;
}

.btn-switch-https {
  background: #f59e0b;
  color: #0f172a;
  font-weight: 700;
  border: none;
  padding: 10px 14px;
  border-radius: 10px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  text-decoration: none;
  font-size: 13px;
  cursor: pointer;
}

.btn-switch-https:hover {
  background: #d97706;
}

.ca-card {
  background: var(--bg-surface-elevated, #243248);
  border: 1px solid var(--border-color, #334155);
  border-radius: 14px;
  padding: 14px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.ca-card-header {
  display: flex;
  align-items: center;
  gap: 8px;
}

.ca-card-title {
  font-size: 12px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: var(--accent-primary, #38bdf8);
}

.ca-card-desc {
  font-size: 12px;
  color: var(--text-secondary, #94a3b8);
  line-height: 1.4;
  margin: 0;
}

.btn-ca-download {
  background: rgba(56, 189, 248, 0.12);
  color: #38bdf8;
  border: 1px solid rgba(56, 189, 248, 0.3);
  padding: 10px 14px;
  border-radius: 10px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 13px;
  font-weight: 600;
  text-decoration: none;
  cursor: pointer;
}

.btn-ca-download:hover {
  background: rgba(56, 189, 248, 0.2);
}

.ca-details {
  font-size: 12px;
  color: var(--text-secondary, #94a3b8);
}

.ca-summary {
  cursor: pointer;
  font-weight: 600;
  color: var(--text-muted, #64748b);
  margin-bottom: 6px;
}

.ca-summary:hover {
  color: var(--text-primary, #f8fafc);
}

.ca-steps {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 8px 10px;
  background: var(--bg-base, #0f172a);
  border-radius: 8px;
  font-size: 11.5px;
  line-height: 1.4;
  color: var(--text-secondary, #cbd5e1);
}

.native-apk-box {
  margin-top: 14px;
  padding-top: 14px;
  border-top: 1px dashed var(--border-color, #334155);
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.apk-box-header {
  display: flex;
  align-items: center;
  gap: 8px;
}

.apk-box-title {
  font-size: 12.5px;
  font-weight: 700;
  color: #4ade80;
}

.apk-box-desc {
  font-size: 12px;
  color: var(--text-secondary, #94a3b8);
  line-height: 1.4;
  margin: 0;
}

.btn-apk-download {
  background: rgba(34, 197, 94, 0.15);
  color: #4ade80;
  border: 1px solid rgba(34, 197, 94, 0.35);
  padding: 10px 14px;
  border-radius: 10px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 13px;
  font-weight: 700;
  text-decoration: none;
  cursor: pointer;
}

.btn-apk-download:hover {
  background: rgba(34, 197, 94, 0.25);
}
</style>
