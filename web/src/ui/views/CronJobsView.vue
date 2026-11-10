<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue';
import {
  mdiAlertCircle,
  mdiCalendarClock,
  mdiCheck,
  mdiCheckCircle,
  mdiChevronDoubleDown,
  mdiClockOutline,
  mdiClose,
  mdiContentCopy,
  mdiDelete,
  mdiFileDocumentOutline,
  mdiFolderOutline,
  mdiInformationOutline,
  mdiMagnify,
  mdiPause,
  mdiPencil,
  mdiPlay,
  mdiPlus,
  mdiRefresh,
  mdiStop,
} from '@mdi/js';
import MdiIcon from '../components/MdiIcon.vue';
import SkeletonBox from '../components/SkeletonBox.vue';
import SortableTh from '../components/SortableTh.vue';
import PathSelector from '../components/PathSelector.vue';
import { useTableSort } from '../composables/useTableSort';
import { useAutoScroll } from '../composables/useAutoScroll';
import { getModuleAvailability, nasState, notify, refreshCronJobs } from '../store';
import { confirmDelete } from '../modal';
import type { CronEnvVar, CronJob, CronVolumeMount } from '../../domain/models';
import { services } from '../../container';

const searchQuery = ref('');
const isRunningJob = reactive<Record<string, boolean>>({});

function isJobActive(job: CronJob): boolean {
  return job.state === 'RUNNING' || job.lastRunStatus === 'RUNNING' || !!isRunningJob[job.id];
}

const currentTime = ref(Date.now());
let clockInterval: any = null;
let watcherInterval: any = null;

onMounted(() => {
  refreshCronJobs();
  clockInterval = setInterval(() => {
    currentTime.value = Date.now();
  }, 1000);
  watcherInterval = setInterval(() => {
    if (nasState.cronJobs.some((j) => isJobActive(j))) {
      refreshCronJobs();
    }
  }, 2500);
});

onUnmounted(() => {
  if (clockInterval) clearInterval(clockInterval);
  if (watcherInterval) clearInterval(watcherInterval);
  stopLogPolling();
});

function formatElapsed(startTime?: string): string {
  if (!startTime) return '0s';
  const start = new Date(startTime).getTime();
  if (isNaN(start)) return '0s';
  const diffSec = Math.max(0, Math.floor((currentTime.value - start) / 1000));
  if (diffSec < 60) return `${diffSec}s`;
  const mins = Math.floor(diffSec / 60);
  const secs = diffSec % 60;
  if (mins < 60) return `${mins}m ${secs < 10 ? '0' : ''}${secs}s`;
  const hours = Math.floor(mins / 60);
  const remMins = mins % 60;
  return `${hours}h ${remMins < 10 ? '0' : ''}${remMins}m ${secs < 10 ? '0' : ''}${secs}s`;
}

// Filter & Sort
const filteredJobs = computed(() => {
  const q = searchQuery.value.trim().toLowerCase();
  if (!q) return nasState.cronJobs;
  return nasState.cronJobs.filter(
    (j) =>
      j.id.toLowerCase().includes(q) ||
      (j.title && j.title.toLowerCase().includes(q)) ||
      (j.description && j.description.toLowerCase().includes(q)) ||
      j.schedule.toLowerCase().includes(q) ||
      j.image.toLowerCase().includes(q)
  );
});

const jobSort = useTableSort(filteredJobs, {
  name: (j) => j.title || j.id,
  schedule: (j) => j.schedule,
  nextRun: (j) => (j.nextScheduleTime ? new Date(j.nextScheduleTime).getTime() : 0),
  lastRun: (j) => (j.lastScheduleTime ? new Date(j.lastScheduleTime).getTime() : 0),
  status: (j) => isJobActive(j) ? 'RUNNING' : (j.lastRunStatus || j.state),
});

// Humanized schedule description
function humanizeCron(expr: string): string {
  const clean = expr.trim();
  if (clean === '0 23 * * *') return 'Daily at 23:00';
  if (clean === '0 0 * * *' || clean === '@daily' || clean === '@midnight') return 'Daily at midnight (00:00)';
  if (clean === '0 * * * *' || clean === '@hourly') return 'Every hour on the hour';
  if (clean === '0 0 * * 0' || clean === '@weekly') return 'Every Sunday at midnight';
  if (clean === '0 0 1 * *' || clean === '@monthly') return 'First of every month';
  if (clean === '0 0 1 1 *' || clean === '@yearly' || clean === '@annually') return 'Annually on Jan 1st at midnight';

  const parts = clean.split(/\s+/);
  if (parts.length === 5) {
    const [min, hour, dom, mon, dow] = parts;
    const isNum = (s: string) => /^\d+$/.test(s);
    const dowNames: Record<string, string> = {
      '0': 'Sun', '1': 'Mon', '2': 'Tue', '3': 'Wed', '4': 'Thu', '5': 'Fri', '6': 'Sat', '7': 'Sun',
      '1-5': 'Mon–Fri', '0,6': 'Weekends',
    };
    const dayStr = dowNames[dow] || (dow !== '*' ? `on ${dow}` : '');

    if (min.startsWith('*/') && hour === '*' && dom === '*' && mon === '*' && dow === '*') {
      return `Every ${min.slice(2)} minutes`;
    }
    if (min.startsWith('*/') && hour !== '*' && dom === '*' && mon === '*' && dow === '*') {
      return `Every ${min.slice(2)} min (hours ${hour})`;
    }
    if (min === '0' && hour.startsWith('*/') && dom === '*' && mon === '*' && dow === '*') {
      return `Every ${hour.slice(2)} hours`;
    }
    if (isNum(min) && isNum(hour) && dom === '*' && mon === '*') {
      const timeStr = `${hour.padStart(2, '0')}:${min.padStart(2, '0')}`;
      return dayStr ? `Every ${dayStr} at ${timeStr}` : `Daily at ${timeStr}`;
    }
  }

  return clean;
}

function formatDate(iso?: string): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (isNaN(d.getTime())) return iso;
  return d.toLocaleString([], {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
}

function formatRelativeNext(iso?: string): string {
  if (!iso) return 'Not scheduled';
  const target = new Date(iso).getTime();
  const diffMs = target - Date.now();
  if (diffMs <= 0) return 'Due now';
  const mins = Math.floor(diffMs / 60000);
  if (mins < 60) return `in ${mins}m`;
  const hours = Math.floor(mins / 60);
  const remMins = mins % 60;
  if (hours < 24) return `in ${hours}h ${remMins}m`;
  const days = Math.floor(hours / 24);
  return `in ${days}d ${hours % 24}h`;
}

function formatTimeout(seconds?: number): string {
  if (!seconds || seconds <= 0) return '';
  if (seconds < 60) return `${seconds}s`;
  const mins = Math.floor(seconds / 60);
  const remSecs = seconds % 60;
  if (mins < 60) {
    return remSecs > 0 ? `${mins}m ${remSecs}s` : `${mins}m`;
  }
  const hours = Math.floor(mins / 60);
  const remMins = mins % 60;
  return remMins > 0 ? `${hours}h ${remMins}m` : `${hours}h`;
}

// -----------------------------------------------------------------------------
// Run Job Now
// -----------------------------------------------------------------------------
async function handleRunNow(job: CronJob) {
  if (isJobActive(job)) return;
  isRunningJob[job.id] = true;
  job.state = 'RUNNING';
  job.lastRunStatus = 'RUNNING';
  job.lastScheduleTime = new Date().toISOString();
  job.lastRunDuration = '';
  job.lastRunError = '';

  try {
    await services.cron.runCronJob(job.name);
    notify('success', `Scheduled task "${job.title || job.id}" started`);
    await refreshCronJobs();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to trigger cronjob execution');
    await refreshCronJobs();
  } finally {
    isRunningJob[job.id] = false;
  }
}

// -----------------------------------------------------------------------------
// Stop / Abort Running Job
// -----------------------------------------------------------------------------
async function handleStopJob(job: CronJob) {
  if (!isJobActive(job)) return;
  try {
    // Toggling enabled to false signals the backend execution engine to abort the active container
    await services.cron.updateCronJob(job.name, { enabled: false }, ['enabled']);
    if (job.enabled) {
      await services.cron.updateCronJob(job.name, { enabled: true }, ['enabled']);
    }
    notify('info', `Stopped execution of "${job.title || job.id}"`);
    await refreshCronJobs();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to stop cron job');
  }
}

// -----------------------------------------------------------------------------
// Toggle Enable / Disable
// -----------------------------------------------------------------------------
async function handleToggleEnabled(job: CronJob) {
  try {
    const updated = await services.cron.updateCronJob(job.name, { enabled: !job.enabled }, ['enabled']);
    job.enabled = updated.enabled;
    notify('success', `CronJob "${job.title || job.id}" ${updated.enabled ? 'enabled' : 'disabled'}`);
    await refreshCronJobs();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to update cronjob status');
  }
}

// -----------------------------------------------------------------------------
// Delete Job
// -----------------------------------------------------------------------------
async function handleDelete(job: CronJob) {
  const res = await confirmDelete({
    kind: 'CronJob',
    name: job.title || job.id,
    details: 'This scheduled task and its execution history logs will be permanently deleted.',
    confirmText: 'Delete CronJob',
  });
  if (!res.confirmed) return;

  try {
    await services.cron.deleteCronJob(job.name);
    notify('info', `Deleted CronJob "${job.title || job.id}"`);
    await refreshCronJobs();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to delete cronjob');
  }
}

// -----------------------------------------------------------------------------
// Log Viewer Modal
// -----------------------------------------------------------------------------
const isLogModalOpen = ref(false);
const logJob = ref<CronJob | null>(null);
const logContent = ref('');
const logStatus = ref('');
const logRunId = ref('');
const logExitCode = ref<number | undefined>(undefined);
const isLoadingLogs = ref(false);
let logPollTimer: any = null;

const {
  containerRef: logContainerRef,
  autoScroll,
  scrollToBottom,
  onScroll: onLogScroll,
  toggleAutoScroll,
} = useAutoScroll();

async function openLogModal(job: CronJob) {
  logJob.value = job;
  autoScroll.value = true;
  isLogModalOpen.value = true;
  await refreshLogs();
  scrollToBottom(true);
  startLogPolling();
}

function closeLogModal() {
  isLogModalOpen.value = false;
  stopLogPolling();
}

function startLogPolling() {
  stopLogPolling();
  logPollTimer = setInterval(() => {
    if (!isLogModalOpen.value) {
      stopLogPolling();
      return;
    }
    const current = logJob.value ? nasState.cronJobs.find((j) => j.id === logJob.value!.id) : null;
    const isStillRunning = logStatus.value === 'RUNNING' || (current && isJobActive(current));
    if (isStillRunning) {
      refreshLogs(true);
    }
  }, 2000);
}

function stopLogPolling() {
  if (logPollTimer) {
    clearInterval(logPollTimer);
    logPollTimer = null;
  }
}

async function refreshLogs(silent = false) {
  if (!logJob.value) return;
  if (!silent) isLoadingLogs.value = true;
  try {
    const res = await services.cron.getCronJobLogs(logJob.value.name, undefined, 500);
    logContent.value = res.logs || 'No log output recorded for this run.';
    logRunId.value = res.runId || '';
    logStatus.value = res.status || '';
    logExitCode.value = res.exitCode;
    scrollToBottom();
  } catch (err: any) {
    if (!silent) logContent.value = `Failed to load logs: ${err?.message || 'Unknown error'}`;
  } finally {
    if (!silent) {
      isLoadingLogs.value = false;
      scrollToBottom();
    }
  }
}

function copyLogs() {
  if (!logContent.value) return;
  navigator.clipboard.writeText(logContent.value);
  notify('info', 'Logs copied to clipboard');
}

// -----------------------------------------------------------------------------
// Create / Edit Modal
// -----------------------------------------------------------------------------
const isEditModalOpen = ref(false);
const isEditing = ref(false);
const isSaving = ref(false);

const form = reactive({
  id: '',
  title: '',
  description: '',
  schedulePreset: '0 23 * * *',
  schedule: '0 23 * * *',
  image: '',
  commandStr: '',
  argsStr: '',
  enabled: true,
  concurrencyPolicy: 'Forbid',
  restartPolicy: 'Never',
  timeoutPreset: '0',
  timeoutCustomMinutes: 60,
  volumes: [] as CronVolumeMount[],
  env: [] as CronEnvVar[],
});

const TIMEOUT_PRESETS = [
  { label: 'No timeout (Unlimited)', value: '0' },
  { label: '5 minutes', value: '300' },
  { label: '15 minutes', value: '900' },
  { label: '30 minutes', value: '1800' },
  { label: '1 hour', value: '3600' },
  { label: '2 hours', value: '7200' },
  { label: '4 hours', value: '14400' },
  { label: '6 hours', value: '21600' },
  { label: '12 hours', value: '43200' },
  { label: '24 hours', value: '86400' },
  { label: 'Custom (minutes)...', value: 'custom' },
];

const SCHEDULE_PRESETS = [
  { label: 'Daily at 23:00 (Nightly Backup)', value: '0 23 * * *' },
  { label: 'Daily at Midnight (00:00)', value: '0 0 * * *' },
  { label: 'Every Hour', value: '0 * * * *' },
  { label: 'Every 15 Minutes', value: '*/15 * * * *' },
  { label: 'Every Sunday at Midnight', value: '0 0 * * 0' },
  { label: 'First of Every Month', value: '0 0 1 * *' },
  { label: 'Custom Expression...', value: 'custom' },
];

function onPresetChange() {
  if (form.schedulePreset !== 'custom') {
    form.schedule = form.schedulePreset;
  }
}

watch(
  () => form.schedule,
  (val) => {
    const match = SCHEDULE_PRESETS.find((p) => p.value === val);
    form.schedulePreset = match ? match.value : 'custom';
  }
);

function addVolumeMount() {
  form.volumes.push({ hostPath: '', mountPath: '', readOnly: false });
}

function removeVolumeMount(index: number) {
  form.volumes.splice(index, 1);
}

function addEnvVar() {
  form.env.push({ name: '', value: '' });
}

function removeEnvVar(index: number) {
  form.env.splice(index, 1);
}

function applyTemplate(type: 'rclone' | 'shell' | 'empty') {
  if (type === 'rclone') {
    const mainPool = (nasState.systemConfig?.mainPool || 'tank').replace(/^zfsPools\//, '');
    form.image = 'rclone/rclone:1';
    form.commandStr = '';
    form.argsStr = 'sync\n/data/source\nremote:/backup/target';
    form.volumes = [
      { hostPath: `/mnt/${mainPool}/data`, mountPath: '/data/source', readOnly: true },
      { hostPath: `/mnt/${mainPool}/.nas-system/rclone`, mountPath: '/config/rclone', readOnly: false },
    ];
  } else if (type === 'shell') {
    form.image = 'alpine:latest';
    form.commandStr = '/bin/sh -c';
    form.argsStr = 'echo "Running scheduled maintenance task"\ndate\necho "Task finished"';
    form.volumes = [];
  } else {
    form.image = '';
    form.commandStr = '';
    form.argsStr = '';
    form.volumes = [];
  }
}

function openCreateModal() {
  isEditing.value = false;
  form.id = '';
  form.title = '';
  form.description = '';
  form.schedulePreset = '0 23 * * *';
  form.schedule = '0 23 * * *';
  applyTemplate('empty');
  form.enabled = true;
  form.concurrencyPolicy = 'Forbid';
  form.restartPolicy = 'no';
  form.timeoutPreset = '0';
  form.timeoutCustomMinutes = 60;
  form.env = [];
  isEditModalOpen.value = true;
}

function formatCommandStr(command?: string[]): string {
  return (command || [])
    .map((arg) => (/\s/.test(arg) ? JSON.stringify(arg) : arg))
    .join(' ');
}

function parseCommandStr(input: string): string[] {
  const trimmed = input.trim();
  if (!trimmed) return [];
  const matches = trimmed.match(/(?:[^\s"']+|"[^"]*"|'[^']*')+/g) || [];
  return matches.map((s) => {
    if ((s.startsWith('"') && s.endsWith('"')) || (s.startsWith("'") && s.endsWith("'"))) {
      return s.slice(1, -1);
    }
    return s;
  });
}

function openEditModal(job: CronJob) {
  isEditing.value = true;
  form.id = job.id;
  form.title = job.title;
  form.description = job.description;
  form.schedule = job.schedule;
  const matchPreset = SCHEDULE_PRESETS.find((p) => p.value === job.schedule);
  form.schedulePreset = matchPreset ? matchPreset.value : 'custom';
  form.image = job.image;
  form.commandStr = formatCommandStr(job.command);
  form.argsStr = (job.args || []).join('\n');
  form.enabled = job.enabled;
  form.concurrencyPolicy = job.concurrencyPolicy || 'Forbid';
  form.restartPolicy = job.restartPolicy || 'no';
  const timeoutSec = job.timeoutSeconds || 0;
  const matchTimeout = TIMEOUT_PRESETS.find((p) => p.value === String(timeoutSec));
  if (matchTimeout) {
    form.timeoutPreset = matchTimeout.value;
    form.timeoutCustomMinutes = timeoutSec > 0 ? Math.max(1, Math.round(timeoutSec / 60)) : 60;
  } else {
    form.timeoutPreset = 'custom';
    form.timeoutCustomMinutes = Math.max(1, Math.round(timeoutSec / 60));
  }
  form.volumes = (job.volumes || []).map((v) => ({ ...v }));
  form.env = (job.env || []).map((e) => ({ ...e }));
  isEditModalOpen.value = true;
}

function tokenizeArgs(input: string): string[] {
  const result: string[] = [];
  const lines = input.split('\n');
  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed) continue;
    const matches = trimmed.match(/[^\s"']+|"([^"]*)"|'([^']*)'/g);
    if (matches) {
      for (const m of matches) {
        if ((m.startsWith('"') && m.endsWith('"')) || (m.startsWith("'") && m.endsWith("'"))) {
          result.push(m.slice(1, -1));
        } else {
          result.push(m);
        }
      }
    }
  }
  return result;
}

async function saveForm() {
  if (!form.id.trim() && !isEditing.value) {
    notify('error', 'Job ID is required');
    return;
  }
  if (!form.schedule.trim()) {
    notify('error', 'Cron schedule is required');
    return;
  }
  if (!form.image.trim()) {
    notify('error', 'Container image is required');
    return;
  }

  isSaving.value = true;
  try {
    const command = parseCommandStr(form.commandStr);
    const args = tokenizeArgs(form.argsStr);
    const timeoutSeconds =
      form.timeoutPreset === 'custom'
        ? Math.max(1, Math.floor(Number(form.timeoutCustomMinutes) || 0)) * 60
        : Math.max(0, parseInt(form.timeoutPreset, 10) || 0);

    const payload: Partial<CronJob> = {
      title: form.title.trim() || form.id.trim(),
      description: form.description.trim(),
      schedule: form.schedule.trim(),
      image: form.image.trim(),
      command,
      args,
      enabled: form.enabled,
      concurrencyPolicy: form.concurrencyPolicy,
      restartPolicy: form.restartPolicy,
      timeoutSeconds,
      volumes: form.volumes.filter((v) => v.hostPath && v.mountPath),
      env: form.env.filter((e) => e.name),
    };

    if (isEditing.value) {
      await services.cron.updateCronJob(`cronJobs/${form.id}`, payload);
      notify('success', `Updated CronJob "${form.title || form.id}"`);
    } else {
      await services.cron.createCronJob(form.id.trim(), payload);
      notify('success', `Created CronJob "${form.title || form.id}"`);
    }

    isEditModalOpen.value = false;
    await refreshCronJobs();
  } catch (err: any) {
    notify('error', err?.message || 'Failed to save CronJob');
  } finally {
    isSaving.value = false;
  }
}
</script>

<template>
  <div class="view-container">
    <!-- Single Control Row -->
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; flex-wrap: wrap; gap: 12px">
      <div style="display: flex; align-items: center; gap: 10px">
        <h2 style="font-size: 20px; font-weight: 700; color: var(--text-primary); margin: 0; display: flex; align-items: center; gap: 8px">
          <MdiIcon :path="mdiCalendarClock" size="22" style="color: var(--accent-primary)" />
          Scheduled Tasks
        </h2>
        <span
          v-if="nasState.cronJobs?.length"
          style="font-size: 11.5px; font-weight: 600; color: var(--text-secondary); background: var(--bg-surface); padding: 2px 8px; border-radius: 12px; border: 1px solid var(--border-subtle)"
        >
          {{ nasState.cronJobs.length }}
        </span>
      </div>

      <div style="display: flex; align-items: center; gap: 8px; flex: 1; justify-content: flex-end; max-width: 560px">
        <div style="position: relative; flex: 1; max-width: 280px">
          <input
            v-model="searchQuery"
            type="text"
            class="form-control"
            placeholder="Filter scheduled tasks..."
            style="padding-left: 30px; font-size: 12.5px; height: 32px"
          />
          <MdiIcon
            :path="mdiMagnify"
            size="15"
            style="position: absolute; left: 9px; top: 50%; transform: translateY(-50%); color: var(--text-muted); pointer-events: none"
          />
        </div>

        <button class="btn btn-secondary btn-sm" :disabled="nasState.loading" @click="refreshCronJobs" style="height: 32px">
          <MdiIcon :path="mdiRefresh" size="14" :class="{ 'spin-icon': nasState.loading }" />
          <span class="hide-mobile" style="margin-left: 4px">Refresh</span>
        </button>

        <button class="btn btn-primary btn-sm" @click="openCreateModal" style="height: 32px">
          <MdiIcon :path="mdiPlus" size="15" style="margin-right: 4px" />
          Create Task
        </button>
      </div>
    </div>

    <!-- Docker Module Prerequisite Notice -->
    <div
      v-if="!getModuleAvailability('docker').enabled"
      class="card"
      style="padding: 12px 16px; margin-bottom: 16px; background: rgba(234, 179, 8, 0.08); border-color: rgba(234, 179, 8, 0.3)"
    >
      <div style="display: flex; align-items: center; gap: 10px">
        <MdiIcon :path="mdiAlertCircle" size="20" style="color: #eab308; flex-shrink: 0" />
        <div style="font-size: 12.5px; color: var(--text-primary)">
          <strong>Docker Runtime Required:</strong> Scheduled tasks execute inside isolated Docker containers. Please install or start Docker to run scheduled tasks.
        </div>
      </div>
    </div>

    <!-- Empty State -->
    <div
      v-if="filteredJobs.length === 0"
      class="card"
      style="text-align: center; padding: 48px 24px"
    >
      <div style="margin-bottom: 12px; color: var(--text-muted)">
        <MdiIcon :path="mdiCalendarClock" size="44" />
      </div>
      <h3 style="font-size: 18px; font-weight: 700; color: var(--text-primary); margin-bottom: 6px">
        No Scheduled Tasks
      </h3>
      <p style="font-size: 13px; color: var(--text-secondary); max-width: 440px; margin: 0 auto 18px">
        Create automated backup syncs with <code>rclone</code>, data scrub scripts, or scheduled container workloads.
      </p>
      <button class="btn btn-primary" @click="openCreateModal">
        <MdiIcon :path="mdiPlus" size="16" style="margin-right: 4px" />
        Create Scheduled Task
      </button>
    </div>

    <!-- Cron Jobs Table -->
    <div v-else class="card" style="padding: 0; overflow-x: auto">
      <table class="data-table" style="width: 100%; border-collapse: collapse">
        <thead>
          <tr style="border-bottom: 1px solid var(--border-subtle); text-align: left; font-size: 11px; color: var(--text-secondary); text-transform: uppercase">
            <SortableTh :sort="jobSort" sort-key="name" style="padding: 10px 14px; width: 280px">Job Name & Image</SortableTh>
            <SortableTh :sort="jobSort" sort-key="schedule" style="padding: 10px 14px; width: 200px">Schedule</SortableTh>
            <SortableTh :sort="jobSort" sort-key="nextRun" class="col-desktop-only" style="padding: 10px 14px; width: 140px">Next Run</SortableTh>
            <SortableTh :sort="jobSort" sort-key="lastRun" style="padding: 10px 14px; width: 180px">Last Run Status</SortableTh>
            <th style="padding: 10px 14px; min-width: 220px; text-align: right">Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="job in jobSort.sorted.value"
            :key="job.id"
            style="border-bottom: 1px solid var(--border-subtle); transition: background 0.15s ease"
          >
            <!-- Job Name & Container Details -->
            <td style="padding: 10px 14px">
              <div style="display: flex; align-items: flex-start; gap: 10px">
                <!-- Status Dot -->
                <div style="margin-top: 3px; flex-shrink: 0">
                  <span
                    v-if="isJobActive(job)"
                    class="spinner-sm"
                    style="width: 11px; height: 11px; border-width: 2px"
                    title="Running..."
                  ></span>
                  <span
                    v-else-if="!job.enabled"
                    style="display: inline-block; width: 9px; height: 9px; border-radius: 50%; background: var(--text-muted)"
                    title="Disabled / Paused"
                  ></span>
                  <span
                    v-else-if="job.lastRunStatus === 'FAILED' || job.state === 'ERROR'"
                    style="display: inline-block; width: 9px; height: 9px; border-radius: 50%; background: var(--danger)"
                    title="Last run failed"
                  ></span>
                  <span
                    v-else-if="job.lastRunStatus === 'ABORTED' || job.lastRunStatus === 'TIMEOUT'"
                    style="display: inline-block; width: 9px; height: 9px; border-radius: 50%; background: var(--warning)"
                    :title="job.lastRunStatus === 'TIMEOUT' ? 'Last run timed out' : 'Last run aborted'"
                  ></span>
                  <span
                    v-else
                    style="display: inline-block; width: 9px; height: 9px; border-radius: 50%; background: var(--success)"
                    title="Scheduled & Active"
                  ></span>
                </div>

                <div>
                  <div style="display: flex; align-items: center; gap: 6px">
                    <span style="font-weight: 600; font-size: 13.5px; color: var(--text-primary)">
                      {{ job.title || job.id }}
                    </span>
                    <span v-if="!job.enabled" class="badge badge-neutral" style="font-size: 10px; padding: 1px 5px">
                      Paused
                    </span>
                  </div>
                  <div style="font-size: 11.5px; color: var(--text-secondary); margin-top: 2px">
                    <span class="mono" style="color: var(--accent-primary)">{{ job.image }}</span>
                    <span v-if="job.volumes && job.volumes.length > 0" style="margin-left: 6px" :title="job.volumes.map(v => `${v.hostPath} -> ${v.mountPath}`).join('\n')">
                      · {{ job.volumes.length }} mount{{ job.volumes.length === 1 ? '' : 's' }}
                    </span>
                  </div>
                  <div v-if="job.description" style="font-size: 11px; color: var(--text-muted); margin-top: 2px">
                    {{ job.description }}
                  </div>
                </div>
              </div>
            </td>

            <!-- Schedule -->
            <td style="padding: 10px 14px">
              <div style="font-size: 12.5px; font-weight: 500; color: var(--text-primary)">
                {{ humanizeCron(job.schedule) }}
              </div>
              <div style="display: flex; align-items: center; gap: 6px; margin-top: 2px">
                <span class="mono" style="font-size: 11px; color: var(--text-muted)">
                  {{ job.schedule }}
                </span>
                <span
                  v-if="job.timeoutSeconds && job.timeoutSeconds > 0"
                  style="font-size: 10.5px; color: var(--text-secondary)"
                  :title="`Execution timeout: ${formatTimeout(job.timeoutSeconds)}`"
                >
                  · ⏱ {{ formatTimeout(job.timeoutSeconds) }}
                </span>
              </div>
            </td>

            <!-- Next Run -->
            <td class="col-desktop-only" style="padding: 10px 14px; font-size: 12px">
              <div v-if="job.enabled && job.nextScheduleTime">
                <span style="color: var(--text-primary); font-weight: 500">
                  {{ formatRelativeNext(job.nextScheduleTime) }}
                </span>
                <div style="font-size: 11px; color: var(--text-secondary); margin-top: 1px">
                  {{ formatDate(job.nextScheduleTime) }}
                </div>
              </div>
              <span v-else style="color: var(--text-muted)">—</span>
            </td>

            <!-- Last Run Status -->
            <td style="padding: 10px 14px">
              <div v-if="isJobActive(job)">
                <div style="display: flex; align-items: center; gap: 6px">
                  <span class="badge badge-info" style="font-size: 11px; display: inline-flex; align-items: center; gap: 4px">
                    <span class="spinner-sm" style="width: 10px; height: 10px; border-width: 2px"></span>
                    RUNNING
                  </span>
                  <span style="font-size: 11.5px; color: var(--accent-primary); font-variant-numeric: tabular-nums; font-weight: 500">
                    ⏱ {{ formatElapsed(job.lastScheduleTime) }}
                  </span>
                </div>
                <div style="font-size: 10.5px; color: var(--text-muted); margin-top: 2px">
                  Started {{ formatDate(job.lastScheduleTime) }}
                </div>
              </div>
              <div v-else-if="job.lastRunStatus">
                <div style="display: flex; align-items: center; gap: 6px">
                  <span
                    class="badge"
                    :class="job.lastRunStatus === 'SUCCESS' ? 'badge-success' : (job.lastRunStatus === 'ABORTED' || job.lastRunStatus === 'TIMEOUT') ? 'badge-warning' : 'badge-danger'"
                    style="font-size: 11px"
                    :title="job.lastRunError || undefined"
                  >
                    {{ job.lastRunStatus }}
                  </span>
                  <span v-if="job.lastRunDuration" style="font-size: 11.5px; color: var(--text-secondary); font-variant-numeric: tabular-nums">
                    {{ job.lastRunDuration }}
                  </span>
                </div>
                <div style="font-size: 10.5px; color: var(--text-muted); margin-top: 2px">
                  {{ formatDate(job.lastScheduleTime) }}
                </div>
                <div
                  v-if="job.lastRunError && job.lastRunStatus !== 'SUCCESS'"
                  style="font-size: 10px; color: var(--danger); margin-top: 2px; max-width: 200px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis"
                  :title="job.lastRunError"
                >
                  {{ job.lastRunError }}
                </div>
              </div>
              <span v-else style="font-size: 12px; color: var(--text-muted)">Never run</span>
            </td>

            <!-- Actions -->
            <td style="padding: 10px 14px; text-align: right">
              <div style="display: inline-flex; align-items: center; gap: 6px">
                <!-- Run / Running Button -->
                <button
                  class="btn btn-sm"
                  style="padding: 4px 8px; font-size: 12px; display: inline-flex; align-items: center; gap: 4px"
                  :disabled="isJobActive(job)"
                  :title="isJobActive(job) ? 'Job is currently running' : 'Trigger immediate execution'"
                  @click.stop="handleRunNow(job)"
                >
                  <MdiIcon
                    v-if="isJobActive(job)"
                    :path="mdiRefresh"
                    size="13"
                    class="spin-icon"
                  />
                  <MdiIcon v-else :path="mdiPlay" size="13" style="color: var(--success)" />
                  {{ isJobActive(job) ? 'Running...' : 'Run' }}
                </button>

                <!-- Stop / Abort if Active -->
                <button
                  v-if="isJobActive(job)"
                  class="btn btn-sm btn-danger"
                  style="padding: 4px 8px; font-size: 12px; display: inline-flex; align-items: center; gap: 4px"
                  title="Stop / Abort running job"
                  @click.stop="handleStopJob(job)"
                >
                  <MdiIcon :path="mdiStop" size="13" />
                  Stop
                </button>

                <!-- View Logs -->
                <button
                  class="btn btn-sm btn-icon"
                  title="View execution logs"
                  @click.stop="openLogModal(job)"
                >
                  <MdiIcon :path="mdiFileDocumentOutline" size="14" />
                </button>

                <!-- Toggle Pause/Enable -->
                <button
                  class="btn btn-sm btn-icon"
                  :title="job.enabled ? 'Pause scheduled task' : 'Enable scheduled task'"
                  @click.stop="handleToggleEnabled(job)"
                >
                  <MdiIcon
                    :path="job.enabled ? mdiPause : mdiPlay"
                    size="14"
                    :style="{ color: job.enabled ? 'var(--warning)' : 'var(--success)' }"
                  />
                </button>

                <!-- Edit -->
                <button
                  class="btn btn-sm btn-icon"
                  title="Edit configuration"
                  @click.stop="openEditModal(job)"
                >
                  <MdiIcon :path="mdiPencil" size="14" />
                </button>

                <!-- Delete -->
                <button
                  class="btn btn-sm btn-icon"
                  title="Delete CronJob"
                  style="color: var(--danger)"
                  @click.stop="handleDelete(job)"
                >
                  <MdiIcon :path="mdiDelete" size="14" />
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Log Viewer Modal -->
    <div v-if="isLogModalOpen && logJob" class="modal-backdrop" @click="closeLogModal">
      <div class="modal" style="max-width: 800px; width: 90%" @click.stop>
        <div class="modal-header">
          <div>
            <h3 class="modal-title" style="display: flex; align-items: center; gap: 8px">
              <MdiIcon :path="mdiFileDocumentOutline" size="20" style="color: var(--accent-primary)" />
              Logs: {{ logJob.title || logJob.id }}
            </h3>
            <div style="font-size: 11.5px; color: var(--text-secondary); margin-top: 2px">
              Run ID: <span class="mono">{{ logRunId || 'Latest' }}</span>
              <span v-if="logStatus" style="margin-left: 8px">
                · Status:
                <span
                  class="badge font-xs"
                  :class="logStatus === 'SUCCESS' ? 'badge-success' : logStatus === 'RUNNING' ? 'badge-info' : (logStatus === 'ABORTED' || logStatus === 'TIMEOUT') ? 'badge-warning' : 'badge-danger'"
                >
                  <span v-if="logStatus === 'RUNNING'" class="spinner-sm" style="width: 8px; height: 8px; border-width: 1.5px; margin-right: 3px; display: inline-block"></span>
                  {{ logStatus === 'RUNNING' ? 'RUNNING (Live Output)' : logStatus }}
                </span>
              </span>
              <span v-if="logExitCode !== undefined && logStatus !== 'RUNNING'" style="margin-left: 6px">
                · Exit Code: <span class="mono">{{ logExitCode }}</span>
              </span>
            </div>
          </div>
          <div style="display: flex; align-items: center; gap: 8px">
            <button
              class="btn btn-sm"
              :class="autoScroll ? 'btn-primary' : 'btn-secondary'"
              :title="autoScroll ? 'Auto-scroll enabled (click to pause)' : 'Click to jump to bottom and follow output'"
              @click="toggleAutoScroll"
            >
              <MdiIcon :path="mdiChevronDoubleDown" size="14" style="margin-right: 4px" />
              Auto-scroll
            </button>
            <button class="btn btn-sm btn-secondary" title="Refresh logs" :disabled="isLoadingLogs" @click="refreshLogs(false)">
              <MdiIcon :path="mdiRefresh" size="14" :class="{ 'spin-icon': isLoadingLogs }" />
              Refresh
            </button>
            <button class="btn btn-sm btn-secondary" title="Copy logs" @click="copyLogs">
              <MdiIcon :path="mdiContentCopy" size="14" />
              Copy
            </button>
            <button class="btn btn-icon btn-sm" title="Close" @click="closeLogModal">
              <MdiIcon :path="mdiClose" size="16" />
            </button>
          </div>
        </div>

        <div class="modal-body" style="padding: 14px">
          <div
            ref="logContainerRef"
            class="mono"
            style="background: var(--bg-canvas); color: var(--text-primary); border: 1px solid var(--border-subtle); border-radius: 6px; padding: 12px; font-size: 11.5px; line-height: 1.5; max-height: 480px; overflow-y: auto; white-space: pre-wrap; word-break: break-all"
            @scroll="onLogScroll"
          >
            <span v-if="isLoadingLogs" style="color: var(--text-muted)">Loading execution output...</span>
            <span v-else>{{ logContent }}</span>
          </div>
        </div>

        <div class="modal-footer" style="display: flex; justify-content: flex-end">
          <button class="btn btn-secondary" @click="closeLogModal">Close</button>
        </div>
      </div>
    </div>

    <!-- Create / Edit Modal -->
    <div v-if="isEditModalOpen" class="modal-backdrop" @click="isEditModalOpen = false">
      <div class="modal" style="max-width: 720px; width: 92%" @click.stop>
        <div class="modal-header">
          <div>
            <h3 class="modal-title" style="display: flex; align-items: center; gap: 8px">
              <MdiIcon :path="mdiCalendarClock" size="20" style="color: var(--accent-primary)" />
              {{ isEditing ? `Edit Task: ${form.title || form.id}` : 'Create Scheduled Task' }}
            </h3>
            <div style="font-size: 11.5px; color: var(--text-secondary); margin-top: 2px">
              Configure container image, cron schedule, and storage volume mounts.
            </div>
          </div>
          <button class="btn btn-icon btn-sm" title="Close" @click="isEditModalOpen = false">
            <MdiIcon :path="mdiClose" size="16" />
          </button>
        </div>

        <form class="modal-body" style="padding: 16px; display: flex; flex-direction: column; gap: 14px" @submit.prevent="saveForm">
          <!-- ID & Title -->
          <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 12px">
            <div>
              <label class="form-label font-xs" style="display: block; margin-bottom: 4px">Task ID *</label>
              <input
                v-model="form.id"
                type="text"
                class="form-control"
                placeholder="e.g. rclone-pictures"
                :disabled="isEditing"
                required
                style="font-size: 12.5px"
              />
            </div>
            <div>
              <label class="form-label font-xs" style="display: block; margin-bottom: 4px">Display Title</label>
              <input
                v-model="form.title"
                type="text"
                class="form-control"
                placeholder="e.g. Nightly Picture Sync to GDrive"
                style="font-size: 12.5px"
              />
            </div>
          </div>

          <!-- Schedule Preset & Expression -->
          <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 12px">
            <div>
              <label class="form-label font-xs" style="display: block; margin-bottom: 4px">Schedule Preset</label>
              <select v-model="form.schedulePreset" class="form-control" style="font-size: 12.5px" @change="onPresetChange">
                <option v-for="p in SCHEDULE_PRESETS" :key="p.value" :value="p.value">
                  {{ p.label }}
                </option>
              </select>
            </div>
            <div>
              <label class="form-label font-xs" style="display: block; margin-bottom: 4px">Cron Expression *</label>
              <input
                v-model="form.schedule"
                type="text"
                class="form-control mono"
                placeholder="0 23 * * *"
                required
                style="font-size: 12.5px"
              />
              <span style="font-size: 11px; color: var(--text-muted); display: block; margin-top: 2px">
                Preview: {{ humanizeCron(form.schedule) }}
              </span>
            </div>
          </div>

          <!-- Starter Templates Quick Bar -->
          <div v-if="!isEditing" style="display: flex; align-items: center; gap: 8px; padding: 8px 12px; background: rgba(255,255,255,0.03); border-radius: 6px; border: 1px solid var(--border-color, rgba(255,255,255,0.1))">
            <span style="font-size: 11.5px; color: var(--text-secondary); font-weight: 500">Starter Templates:</span>
            <button type="button" class="btn btn-xs btn-secondary" @click="applyTemplate('empty')">Blank</button>
            <button type="button" class="btn btn-xs btn-secondary" @click="applyTemplate('shell')">Alpine Shell Script</button>
            <button type="button" class="btn btn-xs btn-secondary" @click="applyTemplate('rclone')">Rclone Sync</button>
          </div>

          <!-- Image -->
          <div>
            <label class="form-label font-xs" style="display: block; margin-bottom: 4px">Docker Container Image *</label>
            <input
              v-model="form.image"
              type="text"
              class="form-control mono"
              placeholder="e.g. rclone/rclone:1 or alpine:latest"
              required
              style="font-size: 12.5px"
            />
          </div>

          <!-- Command Override -->
          <div>
            <label class="form-label font-xs" style="display: block; margin-bottom: 4px">
              Entrypoint / Command Override (optional)
            </label>
            <input
              v-model="form.commandStr"
              type="text"
              class="form-control mono"
              placeholder="e.g. /bin/sh -c or backup.sh"
              style="font-size: 12.5px"
            />
            <span style="font-size: 11px; color: var(--text-muted); display: block; margin-top: 2px">
              Overrides the container image entrypoint or command binary.
            </span>
          </div>

          <!-- Arguments -->
          <div>
            <label class="form-label font-xs" style="display: block; margin-bottom: 4px">
              Command Arguments (one per line)
            </label>
            <textarea
              v-model="form.argsStr"
              class="form-control mono"
              rows="3"
              placeholder="sync&#10;/data/library/pictures&#10;google_drive:/Backups/Pictures"
              style="font-size: 11.5px; resize: vertical"
            ></textarea>
            <span style="font-size: 11px; color: var(--text-muted); display: block; margin-top: 2px">
              Arguments passed to the container command (e.g. rclone subcommands and remote targets).
            </span>
          </div>

          <!-- Volume Mounts -->
          <div>
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 6px">
              <label class="form-label font-xs" style="margin: 0">Volume Mounts</label>
              <button type="button" class="btn btn-sm btn-secondary" style="padding: 2px 8px; font-size: 11px" @click="addVolumeMount">
                <MdiIcon :path="mdiPlus" size="12" /> Add Mount
              </button>
            </div>

            <div v-if="form.volumes.length === 0" style="font-size: 11.5px; color: var(--text-muted); font-style: italic; padding: 6px 0">
              No volume mounts configured.
            </div>

            <div v-else style="display: flex; flex-direction: column; gap: 8px">
              <div
                v-for="(v, idx) in form.volumes"
                :key="idx"
                style="display: grid; grid-template-columns: 1fr 1fr auto auto; gap: 8px; align-items: center"
              >
                <div>
                  <PathSelector
                    v-model="v.hostPath"
                    placeholder="Host Path (e.g. /mnt/tank/...)"
                    button-label="Browse"
                    :select-files="true"
                    modal-title="Select Host Folder or File"
                  />
                </div>
                <div>
                  <input
                    v-model="v.mountPath"
                    type="text"
                    class="form-control mono"
                    placeholder="Mount Path in container (e.g. /data)"
                    style="font-size: 11.5px"
                  />
                </div>
                <label style="display: inline-flex; align-items: center; gap: 4px; font-size: 11px; color: var(--text-secondary); cursor: pointer">
                  <input v-model="v.readOnly" type="checkbox" />
                  Read-only
                </label>
                <button type="button" class="btn btn-sm btn-icon" style="color: var(--danger)" @click="removeVolumeMount(idx)">
                  <MdiIcon :path="mdiDelete" size="14" />
                </button>
              </div>
            </div>
          </div>

          <!-- Environment Variables -->
          <div>
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 6px">
              <label class="form-label font-xs" style="margin: 0">Environment Variables</label>
              <button type="button" class="btn btn-sm btn-secondary" style="padding: 2px 8px; font-size: 11px" @click="addEnvVar">
                <MdiIcon :path="mdiPlus" size="12" /> Add Variable
              </button>
            </div>

            <div v-if="form.env.length === 0" style="font-size: 11.5px; color: var(--text-muted); font-style: italic; padding: 6px 0">
              No custom environment variables.
            </div>

            <div v-else style="display: flex; flex-direction: column; gap: 8px">
              <div
                v-for="(e, idx) in form.env"
                :key="idx"
                style="display: grid; grid-template-columns: 1fr 1fr auto; gap: 8px; align-items: center"
              >
                <input
                  v-model="e.name"
                  type="text"
                  class="form-control mono"
                  placeholder="NAME"
                  style="font-size: 11.5px"
                />
                <input
                  v-model="e.value"
                  type="text"
                  class="form-control mono"
                  placeholder="Value"
                  style="font-size: 11.5px"
                />
                <button type="button" class="btn btn-sm btn-icon" style="color: var(--danger)" @click="removeEnvVar(idx)">
                  <MdiIcon :path="mdiDelete" size="14" />
                </button>
              </div>
            </div>
          </div>

          <!-- Options -->
          <div style="display: flex; gap: 16px; align-items: center; flex-wrap: wrap; padding-top: 8px; border-top: 1px solid var(--border-subtle)">
            <label style="display: inline-flex; align-items: center; gap: 6px; font-size: 12.5px; cursor: pointer">
              <input v-model="form.enabled" type="checkbox" />
              <span>Enable Schedule</span>
            </label>

            <div style="display: inline-flex; align-items: center; gap: 6px; font-size: 12px">
              <span style="color: var(--text-secondary)">Concurrency:</span>
              <select v-model="form.concurrencyPolicy" class="form-control" style="font-size: 11.5px; padding: 2px 6px">
                <option value="Forbid">Forbid (Skip if running)</option>
                <option value="Allow">Allow (Concurrent)</option>
                <option value="Replace">Replace (Cancel running)</option>
              </select>
            </div>

            <div style="display: inline-flex; align-items: center; gap: 6px; font-size: 12px">
              <span style="color: var(--text-secondary)">Restart Policy:</span>
              <select v-model="form.restartPolicy" class="form-control" style="font-size: 11.5px; padding: 2px 6px">
                <option value="no">no (Do not restart)</option>
                <option value="on-failure">on-failure</option>
                <option value="unless-stopped">unless-stopped</option>
              </select>
            </div>

            <div style="display: inline-flex; align-items: center; gap: 6px; font-size: 12px">
              <span style="color: var(--text-secondary)">Timeout:</span>
              <select v-model="form.timeoutPreset" class="form-control" style="font-size: 11.5px; padding: 2px 6px">
                <option v-for="tp in TIMEOUT_PRESETS" :key="tp.value" :value="tp.value">
                  {{ tp.label }}
                </option>
              </select>
              <div v-if="form.timeoutPreset === 'custom'" style="display: inline-flex; align-items: center; gap: 4px">
                <input
                  v-model.number="form.timeoutCustomMinutes"
                  type="number"
                  min="1"
                  max="10080"
                  class="form-control mono"
                  style="width: 72px; font-size: 11.5px; padding: 2px 6px"
                  placeholder="Mins"
                />
                <span style="font-size: 11px; color: var(--text-secondary)">min</span>
              </div>
            </div>
          </div>

          <div class="modal-footer" style="display: flex; justify-content: flex-end; gap: 8px; margin-top: 10px">
            <button type="button" class="btn btn-secondary" @click="isEditModalOpen = false">Cancel</button>
            <button type="submit" class="btn btn-primary" :disabled="isSaving">
              <MdiIcon v-if="isSaving" :path="mdiRefresh" size="14" class="spin-icon" />
              {{ isEditing ? 'Save Changes' : 'Create Task' }}
            </button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>
