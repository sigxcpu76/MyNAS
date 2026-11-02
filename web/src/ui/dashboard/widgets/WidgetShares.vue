<script setup lang="ts">
import { computed } from 'vue';
import { mdiFolderNetworkOutline, mdiChevronRight } from '@mdi/js';
import { nasState } from '../../store';
import { navigateTo } from '../../router';
import SkeletonBox from '../../components/SkeletonBox.vue';

const cifsShares = computed(() => nasState.cifsShares || []);
const webdavShares = computed(() => nasState.webdavShares || []);

const totalCount = computed(() => cifsShares.value.length + webdavShares.value.length);
const activeCount = computed(() => {
  const cActive = cifsShares.value.filter((s) => s.enabled).length;
  const wActive = webdavShares.value.filter((s) => s.enabled).length;
  return cActive + wActive;
});
</script>

<template>
  <div class="card workload-card">
    <div class="card-header workload-header">
      <div class="header-left">
        <div class="header-icon-box">
          <svg viewBox="0 0 24 24" class="header-icon"><path :d="mdiFolderNetworkOutline" /></svg>
        </div>
        <h3 class="card-title">Network Shares</h3>
        <span class="badge badge-neutral mono count-badge">
          {{ activeCount }}/{{ totalCount }}
        </span>
      </div>
      <button class="header-action-btn" @click="navigateTo('shares')" title="Manage Network Shares">
        <span>Manage</span>
        <svg viewBox="0 0 24 24" class="chevron-svg"><path :d="mdiChevronRight" /></svg>
      </button>
    </div>

    <!-- Loading Skeleton -->
    <div v-if="nasState.loading && totalCount === 0" class="workload-list">
      <div v-for="s in 3" :key="s" class="workload-item skeleton-item">
        <div class="item-meta">
          <SkeletonBox width="140px" height="14px" />
          <SkeletonBox width="180px" height="11px" />
        </div>
        <SkeletonBox width="34px" height="20px" rounded="4px" />
      </div>
    </div>

    <!-- Empty State -->
    <div v-else-if="totalCount === 0" class="empty-state">
      <span>No network shares configured.</span>
      <button class="btn btn-xs btn-secondary" @click="navigateTo('shares')">Add Share</button>
    </div>

    <!-- Shares List -->
    <div v-else class="workload-list">
      <!-- SMB / CIFS Shares -->
      <div
        v-for="cs in cifsShares"
        :key="cs.name"
        class="workload-item"
        @click="navigateTo('shares')"
        role="button"
        tabindex="0"
      >
        <div class="item-meta">
          <div class="item-row-top">
            <span class="type-pill pill-smb">SMB</span>
            <span class="item-title" :title="`//nas/${cs.shareName}`">
              //nas/{{ cs.shareName }}
            </span>
          </div>
          <div class="item-subtitle mono share-path" :title="cs.path">
            {{ cs.path }}
          </div>
        </div>

        <span class="perm-badge" :class="cs.readOnly ? 'perm-ro' : 'perm-rw'" :title="cs.readOnly ? 'Read-Only' : 'Read-Write'">
          {{ cs.readOnly ? 'RO' : 'RW' }}
        </span>
      </div>

      <!-- WebDAV Shares -->
      <div
        v-for="ws in webdavShares"
        :key="ws.name"
        class="workload-item"
        @click="navigateTo('shares')"
        role="button"
        tabindex="0"
      >
        <div class="item-meta">
          <div class="item-row-top">
            <span class="type-pill pill-webdav">WEBDAV</span>
            <span class="item-title" :title="ws.urlPrefix">
              {{ ws.urlPrefix }}
            </span>
          </div>
          <div class="item-subtitle mono share-path" :title="ws.path">
            {{ ws.path }}
          </div>
        </div>

        <span class="perm-badge" :class="ws.readOnly ? 'perm-ro' : 'perm-rw'" :title="ws.readOnly ? 'Read-Only' : 'Read-Write'">
          {{ ws.readOnly ? 'RO' : 'RW' }}
        </span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.workload-card {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.workload-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding-bottom: 8px;
  margin-bottom: 10px;
  border-bottom: 1px solid var(--border-subtle);
}

.header-left {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.header-icon-box {
  width: 26px;
  height: 26px;
  border-radius: 6px;
  background: var(--accent-bg-subtle, rgba(56, 189, 248, 0.1));
  border: 1px solid var(--border-subtle);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.header-icon {
  width: 15px;
  height: 15px;
  fill: var(--accent-primary, #38bdf8);
}

.card-title {
  margin: 0;
  font-size: 13.5px;
  font-weight: 600;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.count-badge {
  font-size: 10px;
  padding: 1px 6px;
  flex-shrink: 0;
}

.header-action-btn {
  background: transparent;
  border: 1px solid var(--border-subtle);
  border-radius: 5px;
  padding: 2px 7px;
  font-size: 11px;
  font-weight: 500;
  color: var(--text-secondary);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 2px;
  flex-shrink: 0;
  transition: all 0.15s ease;
}

.header-action-btn:hover {
  color: var(--text-primary);
  border-color: var(--border-strong);
  background: var(--bg-hover, rgba(255, 255, 255, 0.04));
}

.chevron-svg {
  width: 14px;
  height: 14px;
  fill: currentColor;
  transition: transform 0.15s ease;
}

.header-action-btn:hover .chevron-svg {
  transform: translateX(1px);
}

.workload-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  overflow-y: auto;
  max-height: 260px;
  padding-right: 2px;
}

.workload-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 6px 8px;
  border-radius: 6px;
  background: var(--bg-surface-elevated, rgba(255, 255, 255, 0.02));
  border: 1px solid var(--border-subtle);
  cursor: pointer;
  transition: all 0.15s ease;
}

.workload-item:hover {
  background: var(--bg-hover, rgba(255, 255, 255, 0.05));
  border-color: var(--border-strong);
}

.workload-item.skeleton-item {
  cursor: default;
}

.item-meta {
  min-width: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.item-row-top {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.type-pill {
  font-size: 9px;
  font-weight: 700;
  padding: 1px 4px;
  border-radius: 3px;
  font-family: var(--font-mono);
  flex-shrink: 0;
  letter-spacing: 0.3px;
}

.pill-smb {
  background: rgba(59, 130, 246, 0.14);
  color: #60a5fa;
  border: 1px solid rgba(59, 130, 246, 0.28);
}

.pill-webdav {
  background: rgba(168, 85, 247, 0.14);
  color: #c084fc;
  border: 1px solid rgba(168, 85, 247, 0.28);
}

.item-title {
  font-size: 12.5px;
  font-weight: 600;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.share-path {
  font-size: 11px;
  color: var(--text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 100%;
}

/* Permission Badges */
.perm-badge {
  flex-shrink: 0;
  min-width: 30px;
  height: 20px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 10px;
  font-weight: 700;
  border-radius: 4px;
  font-family: var(--font-mono);
  letter-spacing: 0.5px;
}

.perm-rw {
  background: transparent;
  color: var(--text-muted);
  border: 1px solid var(--border-subtle);
  opacity: 0.65;
}

.perm-ro {
  background: rgba(245, 158, 11, 0.12);
  color: #f59e0b;
  border: 1px solid rgba(245, 158, 11, 0.25);
}

.empty-state {
  padding: 24px 12px;
  text-align: center;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  color: var(--text-muted);
  font-size: 12px;
}
</style>