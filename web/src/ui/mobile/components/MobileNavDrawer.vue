<script setup lang="ts">
import {
  mdiClose,
  mdiMonitor,
  mdiDocker,
  mdiDatabase,
  mdiLayers,
  mdiFolderNetworkOutline,
  mdiClockOutline,
  mdiAccountGroupOutline,
  mdiHarddisk,
  mdiLayersTripleOutline,
  mdiInformationOutline,
  mdiCheckCircleOutline,
  mdiPaletteOutline,
  mdiFormatSize,
  mdiConsole,
  mdiLogout,
  mdiLaptop,
  mdiCellphoneArrowDown,
  mdiUpdate,
} from '@mdi/js';
import MdiIcon from '../../components/MdiIcon.vue';
import NanLogo from '../../components/NanLogo.vue';
import { nasState } from '../../store';
import { navigateTo } from '../../router';
import { useDevice } from '../../composables/useDevice';
import { usePwaInstall } from '../../composables/usePwaInstall';
import { useClientApp } from '../../composables/useClientApp';
import ThemeSelector from '../../components/ThemeSelector.vue';
import FontSizeSelector from '../../components/FontSizeSelector.vue';

const props = defineProps<{
  modelValue: boolean;
}>();

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void;
  (e: 'open-terminal'): void;
  (e: 'open-system-details'): void;
  (e: 'open-prerequisites'): void;
  (e: 'open-update'): void;
  (e: 'logout'): void;
}>();

const { setViewMode } = useDevice();
const { isStandalone, promptInstall } = usePwaInstall();
const { clientApp } = useClientApp();

function close() {
  emit('update:modelValue', false);
}

function nav(tab: any) {
  close();
  navigateTo(tab);
}

function handleSwitchToDesktop() {
  setViewMode('desktop');
  close();
}
</script>

<template>
  <Teleport to="body">
    <Transition name="drawer-fade">
      <div v-if="modelValue" class="drawer-backdrop" @click.self="close">
        <div class="drawer-panel">
          <!-- Drawer Header -->
          <div class="drawer-header">
            <NanLogo :size="24" subtitle="Mobile" />
            <button class="btn btn-xs btn-ghost btn-icon" aria-label="Close menu" @click="close">
              <MdiIcon :path="mdiClose" size="18" />
            </button>
          </div>

          <!-- Drawer Navigation Items -->
          <div class="drawer-content">
            <!-- PWA Install Banner -->
            <div v-if="!isStandalone && !clientApp.isNative" class="pwa-drawer-tile" @click="close(); promptInstall()">
              <div class="pwa-drawer-icon">
                <MdiIcon :path="mdiCellphoneArrowDown" size="18" />
              </div>
              <div class="pwa-drawer-text">
                <span class="pwa-drawer-title">Install NaN App</span>
                <span class="pwa-drawer-sub">Add to Home Screen</span>
              </div>
            </div>
            <div class="nav-section-title">Core Management</div>
            <div class="nav-links-list">
              <button class="nav-link-btn" @click="nav('overview')">
                <MdiIcon :path="mdiMonitor" size="18" style="color: #38bdf8" />
                <span>Dashboard</span>
              </button>
              <button class="nav-link-btn" @click="nav('docker')">
                <MdiIcon :path="mdiDocker" size="18" style="color: #38bdf8" />
                <span>Docker Apps</span>
              </button>
              <button class="nav-link-btn" @click="nav('storage-pools')">
                <MdiIcon :path="mdiDatabase" size="18" style="color: #4ade80" />
                <span>Storage Pools</span>
              </button>
              <button class="nav-link-btn" @click="nav('kvm')">
                <MdiIcon :path="mdiLayers" size="18" style="color: #818cf8" />
                <span>VMs & LXC</span>
              </button>
            </div>

            <div class="nav-section-title">Storage & Files</div>
            <div class="nav-links-list">
              <button class="nav-link-btn" @click="nav('storage-disks')">
                <MdiIcon :path="mdiHarddisk" size="18" style="color: #4ade80" />
                <span>Physical Disks & SMART</span>
              </button>
              <button class="nav-link-btn" @click="nav('storage-datasets')">
                <MdiIcon :path="mdiLayersTripleOutline" size="18" style="color: #4ade80" />
                <span>ZFS Datasets</span>
              </button>
              <button class="nav-link-btn" @click="nav('storage-volumes')">
                <MdiIcon :path="mdiLayersTripleOutline" size="18" style="color: #38bdf8" />
                <span>ZFS Block Volumes (zvol)</span>
              </button>
              <button class="nav-link-btn" @click="nav('shares')">
                <MdiIcon :path="mdiFolderNetworkOutline" size="18" style="color: #fbbf24" />
                <span>Shares (SMB & WebDAV)</span>
              </button>
            </div>

            <div class="nav-section-title">Administration</div>
            <div class="nav-links-list">
              <button class="nav-link-btn" @click="close(); emit('open-update')">
                <MdiIcon :path="mdiUpdate" size="18" style="color: #fbbf24" />
                <span>Software Updates</span>
                <span v-if="nasState.systemUpdate?.updateAvailable" class="badge badge-warning" style="margin-left: auto; font-size: 10px; padding: 2px 6px">
                  {{ nasState.systemUpdate.latestVersion }}
                </span>
                <span v-else class="text-muted" style="margin-left: auto; font-size: 11px">
                  {{ nasState.systemUpdate?.currentVersion || 'v1.6.12' }}
                </span>
              </button>
              <div class="client-shell-tile" style="margin: 4px 0 6px 0; padding: 7px 10px; background: rgba(0, 0, 0, 0.25); border: 1px solid var(--border-subtle); border-radius: 8px; display: flex; align-items: center; justify-content: space-between; font-size: 11.5px">
                <span style="color: var(--text-secondary)">Client Shell:</span>
                <span class="mono" :style="{ color: clientApp.isNative ? '#4ade80' : 'var(--accent-primary)', fontWeight: 600 }">
                  {{ clientApp.label }}
                </span>
              </div>
              <button class="nav-link-btn" @click="nav('identity')">
                <MdiIcon :path="mdiAccountGroupOutline" size="18" style="color: #c084fc" />
                <span>Users & Groups</span>
              </button>
              <button class="nav-link-btn" @click="nav('cron')">
                <MdiIcon :path="mdiClockOutline" size="18" style="color: #f472b6" />
                <span>Scheduled Cron Jobs</span>
              </button>
              <button class="nav-link-btn" @click="close(); emit('open-terminal')">
                <MdiIcon :path="mdiConsole" size="18" style="color: #38bdf8" />
                <span>Host Root Shell</span>
              </button>
              <button class="nav-link-btn" @click="close(); emit('open-system-details')">
                <MdiIcon :path="mdiInformationOutline" size="18" style="color: #94a3b8" />
                <span>System Specs & Telemetry</span>
              </button>
              <button class="nav-link-btn" @click="close(); emit('open-prerequisites')">
                <MdiIcon :path="mdiCheckCircleOutline" size="18" style="color: #94a3b8" />
                <span>Prerequisites Inspector</span>
              </button>
            </div>

            <div class="nav-section-title">Preferences & Display</div>
            <div class="prefs-box">
              <div class="pref-row">
                <span class="pref-label">Theme</span>
                <ThemeSelector />
              </div>
              <div class="pref-row">
                <span class="pref-label">Font Scaling</span>
                <FontSizeSelector />
              </div>
              <button class="switch-desktop-btn" @click="handleSwitchToDesktop">
                <MdiIcon :path="mdiLaptop" size="16" />
                <span>Switch to Desktop Layout</span>
              </button>
            </div>
          </div>

          <!-- Drawer Footer -->
          <div v-if="nasState.authenticated" class="drawer-footer">
            <button class="btn btn-sm btn-ghost btn-block" @click="close(); emit('logout')">
              <MdiIcon :path="mdiLogout" size="16" style="margin-right: 6px" />
              <span>Sign Out ({{ nasState.currentUser?.username }})</span>
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.drawer-backdrop {
  position: fixed;
  inset: 0;
  z-index: 1250;
  background: rgba(0, 0, 0, 0.65);
  backdrop-filter: blur(4px);
  -webkit-backdrop-filter: blur(4px);
  display: flex;
  justify-content: flex-end;
}

.drawer-panel {
  width: 82%;
  max-width: 320px;
  height: 100%;
  background: var(--bg-surface, #1e293b);
  border-left: 1px solid var(--border-color, #334155);
  box-shadow: -8px 0 25px rgba(0, 0, 0, 0.5);
  display: flex;
  flex-direction: column;
  padding-top: env(safe-area-inset-top, 0px);
  padding-bottom: env(safe-area-inset-bottom, 0px);
}

.drawer-header {
  height: 52px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 16px;
  border-bottom: 1px solid var(--border-color, #334155);
  flex-shrink: 0;
}

.drawer-content {
  flex: 1;
  overflow-y: auto;
  -webkit-overflow-scrolling: touch;
  padding: 14px 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.pwa-drawer-tile {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border-radius: 12px;
  background: linear-gradient(135deg, rgba(56, 189, 248, 0.15) 0%, rgba(129, 140, 248, 0.15) 100%);
  border: 1px solid rgba(56, 189, 248, 0.35);
  cursor: pointer;
  touch-action: manipulation;
  transition: transform 0.1s ease;
}

.pwa-drawer-tile:active {
  transform: scale(0.98);
}

.pwa-drawer-icon {
  width: 32px;
  height: 32px;
  border-radius: 8px;
  background: rgba(56, 189, 248, 0.2);
  color: var(--accent-primary, #38bdf8);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.pwa-drawer-text {
  display: flex;
  flex-direction: column;
}

.pwa-drawer-title {
  font-size: 13px;
  font-weight: 700;
  color: var(--text-primary, #f8fafc);
}

.pwa-drawer-sub {
  font-size: 11px;
  color: var(--accent-primary, #38bdf8);
}

.nav-section-title {
  font-size: 10px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.6px;
  color: var(--text-secondary, #94a3b8);
  margin-top: 6px;
}

.nav-links-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.nav-link-btn {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border-radius: 10px;
  background: transparent;
  border: none;
  color: var(--text-primary, #f8fafc);
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  touch-action: manipulation;
  text-align: left;
  transition: background 0.15s ease;
}

.nav-link-btn:active {
  background: var(--bg-hover, #334155);
}

.prefs-box {
  background: var(--bg-surface-elevated, #243248);
  border: 1px solid var(--border-color, #334155);
  border-radius: 12px;
  padding: 10px 12px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.pref-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.pref-label {
  font-size: 12px;
  color: var(--text-secondary, #94a3b8);
}

.switch-desktop-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 8px;
  border-radius: 8px;
  background: var(--bg-base, #0f172a);
  border: 1px solid var(--border-color, #334155);
  color: var(--accent-primary, #38bdf8);
  font-size: 11px;
  font-weight: 600;
  cursor: pointer;
  margin-top: 4px;
}

.drawer-footer {
  padding: 12px 16px;
  border-top: 1px solid var(--border-color, #334155);
  flex-shrink: 0;
}

.btn-block {
  width: 100%;
}

.drawer-fade-enter-active,
.drawer-fade-leave-active {
  transition: opacity 0.25s ease;
}

.drawer-fade-enter-from,
.drawer-fade-leave-to {
  opacity: 0;
}

.drawer-fade-enter-active .drawer-panel {
  transition: transform 0.28s cubic-bezier(0.16, 1, 0.3, 1);
}

.drawer-fade-leave-active .drawer-panel {
  transition: transform 0.22s cubic-bezier(0.4, 0, 1, 1);
}

.drawer-fade-enter-from .drawer-panel,
.drawer-fade-leave-to .drawer-panel {
  transform: translateX(100%);
}
</style>
