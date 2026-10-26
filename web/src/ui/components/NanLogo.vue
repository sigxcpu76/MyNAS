<script setup lang="ts">
withDefaults(
  defineProps<{
    size?: number;
    showText?: boolean;
    subtitle?: string;
  }>(),
  {
    size: 36,
    showText: true,
    subtitle: 'Not Another NAS',
  }
);
</script>

<template>
  <div class="nan-brand-wrapper">
    <svg
      :width="size"
      :height="size"
      viewBox="0 0 64 64"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      class="nan-logo-svg"
    >
      <defs>
        <linearGradient id="nanLogoBg" x1="0" y1="0" x2="64" y2="64" gradientUnits="userSpaceOnUse">
          <stop offset="0%" stop-color="#0f172a" />
          <stop offset="100%" stop-color="#1e1e2e" />
        </linearGradient>
        <linearGradient id="nanLogoAccent" x1="0" y1="0" x2="64" y2="64" gradientUnits="userSpaceOnUse">
          <stop offset="0%" stop-color="#38bdf8" />
          <stop offset="50%" stop-color="#818cf8" />
          <stop offset="100%" stop-color="#c084fc" />
        </linearGradient>
        <linearGradient id="nanLogoDrive" x1="0" y1="0" x2="48" y2="0" gradientUnits="userSpaceOnUse">
          <stop offset="0%" stop-color="#334155" />
          <stop offset="100%" stop-color="#1e293b" />
        </linearGradient>
      </defs>

      <!-- Squircle Chassis Base -->
      <rect x="2" y="2" width="60" height="60" rx="14" fill="url(#nanLogoBg)" stroke="url(#nanLogoAccent)" stroke-width="2.5" />

      <!-- Server Drive Bays -->
      <g transform="translate(8, 9)">
        <!-- Drive 1 -->
        <rect x="0" y="0" width="48" height="9" rx="2.5" fill="url(#nanLogoDrive)" stroke="#475569" stroke-width="0.75" />
        <circle cx="5" cy="4.5" r="1.6" fill="#38bdf8" />
        <circle cx="10" cy="4.5" r="1.1" fill="#4ade80" />
        <line x1="16" y1="4.5" x2="43" y2="4.5" stroke="#64748b" stroke-width="1.2" stroke-linecap="round" stroke-dasharray="2 3" />

        <!-- Drive 2 -->
        <rect x="0" y="12" width="48" height="9" rx="2.5" fill="url(#nanLogoDrive)" stroke="#475569" stroke-width="0.75" />
        <circle cx="5" cy="16.5" r="1.6" fill="#818cf8" />
        <circle cx="10" cy="16.5" r="1.1" fill="#4ade80" />
        <line x1="16" y1="16.5" x2="43" y2="16.5" stroke="#64748b" stroke-width="1.2" stroke-linecap="round" stroke-dasharray="2 3" />

        <!-- Drive 3 -->
        <rect x="0" y="24" width="48" height="9" rx="2.5" fill="url(#nanLogoDrive)" stroke="#475569" stroke-width="0.75" />
        <circle cx="5" cy="28.5" r="1.6" fill="#c084fc" />
        <circle cx="10" cy="28.5" r="1.1" fill="#4ade80" />
        <line x1="16" y1="28.5" x2="43" y2="28.5" stroke="#64748b" stroke-width="1.2" stroke-linecap="round" stroke-dasharray="2 3" />
      </g>

      <!-- NaN Badge Baseplate -->
      <rect x="12" y="44" width="40" height="14" rx="4" fill="#0f172a" stroke="url(#nanLogoAccent)" stroke-width="1.2" />
      <text x="32" y="54.5" fill="url(#nanLogoAccent)" font-family="system-ui, -apple-system, sans-serif" font-size="9.5" font-weight="900" text-anchor="middle" letter-spacing="1">NaN</text>
    </svg>

    <div v-if="showText" class="nan-text-group">
      <div class="nan-title">
        <span class="nan-title-main">NaN</span>
      </div>
      <div class="nan-subtitle">{{ subtitle }}</div>
    </div>
  </div>
</template>

<style scoped>
.nan-brand-wrapper {
  display: inline-flex;
  align-items: center;
  gap: 11px;
  user-select: none;
}

.nan-logo-svg {
  flex-shrink: 0;
  transition: transform 0.2s ease;
  filter: drop-shadow(0 2px 6px rgba(56, 189, 248, 0.2));
}

.nan-brand-wrapper:hover .nan-logo-svg {
  transform: scale(1.04);
}

.nan-text-group {
  display: flex;
  flex-direction: column;
}

.nan-title {
  display: flex;
  align-items: baseline;
  gap: 6px;
  line-height: 1.1;
}

.nan-title-main {
  font-size: 19px;
  font-weight: 800;
  letter-spacing: -0.5px;
  background: linear-gradient(135deg, var(--accent-primary, #38bdf8), var(--accent-secondary, #bb9af7));
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
}

.nan-subtitle {
  font-size: 11px;
  font-weight: 500;
  color: var(--text-secondary);
  letter-spacing: 0.3px;
  margin-top: 1px;
}
</style>
