<script setup lang="ts">
import SkeletonBox from './SkeletonBox.vue';
</script>

<template>
  <div class="overview-skeleton-container">
    <!-- 1. System & Hardware Information (Fastfetch layout) -->
    <div class="card fastfetch-skeleton-card">
      <div class="card-header" style="margin-bottom: 16px; border-bottom: 1px solid var(--border-subtle); padding-bottom: 12px">
        <div style="display: flex; flex-direction: column; gap: 6px">
          <SkeletonBox width="220px" height="18px" />
          <SkeletonBox width="280px" height="12px" />
        </div>
        <SkeletonBox width="70px" height="22px" rounded="12px" />
      </div>

      <div class="fastfetch-skeleton-layout">
        <!-- Terminal / Logo Art Side Placeholder -->
        <div class="fastfetch-skeleton-logo">
          <SkeletonBox width="110px" height="85px" rounded="8px" />
          <div class="color-palette-dots-skeleton">
            <SkeletonBox v-for="i in 6" :key="i" width="10px" height="10px" circle />
          </div>
        </div>

        <!-- Fastfetch Key-Value Rows Placeholder -->
        <div class="fastfetch-skeleton-info">
          <div v-for="i in 9" :key="i" class="fetch-row-skeleton">
            <SkeletonBox width="85px" height="13px" style="flex-shrink: 0" />
            <SkeletonBox :width="i % 2 === 0 ? '240px' : '180px'" height="13px" />
          </div>
        </div>
      </div>
    </div>

    <!-- 2. Top Summary Metric Cards (3 Cards) -->
    <div class="summary-cards-grid">
      <!-- CPU Card -->
      <div class="card">
        <div class="card-header" style="margin-bottom: 10px">
          <SkeletonBox width="110px" height="13px" />
          <SkeletonBox width="60px" height="20px" rounded="12px" />
        </div>
        <div style="display: flex; align-items: baseline; gap: 10px; margin-bottom: 8px">
          <SkeletonBox width="80px" height="26px" />
          <SkeletonBox width="110px" height="14px" />
        </div>
        <SkeletonBox width="190px" height="12px" />
      </div>

      <!-- Memory Card -->
      <div class="card">
        <div class="card-header" style="margin-bottom: 10px">
          <SkeletonBox width="100px" height="13px" />
          <SkeletonBox width="85px" height="20px" rounded="12px" />
        </div>
        <div style="display: flex; align-items: baseline; gap: 10px; margin-bottom: 8px">
          <SkeletonBox width="90px" height="26px" />
          <SkeletonBox width="90px" height="14px" />
        </div>
        <SkeletonBox width="210px" height="12px" />
      </div>

      <!-- Workloads Card -->
      <div class="card">
        <div class="card-header" style="margin-bottom: 10px">
          <SkeletonBox width="120px" height="13px" />
          <SkeletonBox width="65px" height="20px" rounded="12px" />
        </div>
        <div style="display: flex; align-items: baseline; gap: 10px; margin-bottom: 8px">
          <SkeletonBox width="110px" height="26px" />
        </div>
        <SkeletonBox width="170px" height="12px" />
      </div>
    </div>

    <!-- 3. ZFS Storage Pools Section (2 Cards) -->
    <div>
      <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px">
        <div style="display: flex; flex-direction: column; gap: 6px">
          <SkeletonBox width="180px" height="18px" />
          <SkeletonBox width="320px" height="12px" />
        </div>
        <SkeletonBox width="130px" height="30px" rounded="6px" />
      </div>

      <div class="pool-cards-skeleton-grid">
        <div v-for="p in 2" :key="p" class="card pool-skeleton-card">
          <!-- Pool Header -->
          <div class="card-header" style="border-bottom: 1px solid var(--border-subtle); padding-bottom: 10px; margin-bottom: 14px">
            <div style="display: flex; align-items: center; gap: 10px">
              <SkeletonBox width="100px" height="18px" />
              <SkeletonBox width="55px" height="20px" rounded="10px" />
              <SkeletonBox width="45px" height="20px" rounded="10px" />
            </div>
            <SkeletonBox width="60px" height="20px" rounded="10px" />
          </div>

          <!-- Dual Pie Chart Donut Skeletons -->
          <div class="pool-charts-skeleton-container">
            <!-- Usage Donut Placeholder -->
            <div class="pool-chart-skeleton-box">
              <SkeletonBox width="90px" height="13px" style="margin-bottom: 8px" />
              <div class="donut-skeleton-wrap">
                <SkeletonBox width="110px" height="110px" circle />
                <div class="donut-cutout"></div>
              </div>
              <div style="display: flex; gap: 12px; margin-top: 10px">
                <SkeletonBox width="85px" height="12px" />
                <SkeletonBox width="85px" height="12px" />
              </div>
            </div>

            <!-- Fragmentation Donut Placeholder -->
            <div class="pool-chart-skeleton-box">
              <SkeletonBox width="95px" height="13px" style="margin-bottom: 8px" />
              <div class="donut-skeleton-wrap">
                <SkeletonBox width="110px" height="110px" circle />
                <div class="donut-cutout"></div>
              </div>
              <div style="display: flex; gap: 12px; margin-top: 10px">
                <SkeletonBox width="85px" height="12px" />
                <SkeletonBox width="85px" height="12px" />
              </div>
            </div>
          </div>

          <!-- Pool Details Footer -->
          <div style="display: flex; justify-content: space-between; align-items: center; margin-top: 14px; padding-top: 12px; border-top: 1px solid var(--border-subtle)">
            <SkeletonBox width="140px" height="13px" />
            <SkeletonBox width="100px" height="13px" />
          </div>
        </div>
      </div>
    </div>

    <!-- 4. System Network Interface & Real-time Traffic Graph -->
    <div class="card" style="padding: 18px 20px">
      <div class="card-header" style="margin-bottom: 14px">
        <div style="display: flex; flex-direction: column; gap: 6px">
          <SkeletonBox width="260px" height="18px" />
          <SkeletonBox width="340px" height="12px" />
        </div>
        <SkeletonBox width="90px" height="22px" rounded="12px" />
      </div>

      <!-- Interface Info Header Pill Bar -->
      <div class="system-iface-skeleton-header">
        <div style="display: flex; align-items: center; gap: 10px">
          <SkeletonBox width="70px" height="22px" rounded="6px" />
          <SkeletonBox width="60px" height="20px" rounded="10px" />
          <SkeletonBox width="110px" height="20px" rounded="10px" />
          <SkeletonBox width="130px" height="20px" rounded="10px" />
        </div>
        <div style="display: flex; gap: 16px">
          <SkeletonBox width="100px" height="16px" />
          <SkeletonBox width="100px" height="16px" />
        </div>
      </div>

      <!-- Traffic Chart Moving Shimmer Placeholder -->
      <div class="traffic-chart-skeleton-box">
        <SkeletonBox width="100%" height="190px" rounded="8px" />
      </div>
    </div>

    <!-- 5. CPU Cores & System Memory Composition -->
    <div class="telemetry-details-skeleton-grid">
      <!-- CPU Core Breakdown -->
      <div class="card">
        <div class="card-header" style="margin-bottom: 12px">
          <div style="display: flex; flex-direction: column; gap: 4px">
            <SkeletonBox width="150px" height="16px" />
            <SkeletonBox width="200px" height="12px" />
          </div>
          <SkeletonBox width="65px" height="20px" rounded="12px" />
        </div>

        <div class="cores-skeleton-grid">
          <div v-for="c in 8" :key="c" class="core-skeleton-box">
            <div style="display: flex; justify-content: space-between; margin-bottom: 6px">
              <SkeletonBox width="45px" height="11px" />
              <SkeletonBox width="30px" height="11px" />
            </div>
            <SkeletonBox width="100%" height="7px" rounded="4px" />
          </div>
        </div>
      </div>

      <!-- System Memory Composition -->
      <div class="card">
        <div class="card-header" style="margin-bottom: 12px">
          <div style="display: flex; flex-direction: column; gap: 4px">
            <SkeletonBox width="190px" height="16px" />
            <SkeletonBox width="230px" height="12px" />
          </div>
          <SkeletonBox width="80px" height="20px" rounded="12px" />
        </div>

        <div style="display: flex; align-items: center; justify-content: space-around; gap: 20px; padding: 10px 0">
          <div class="donut-skeleton-wrap" style="width: 140px; height: 140px">
            <SkeletonBox width="140px" height="140px" circle />
            <div class="donut-cutout" style="width: 74px; height: 74px"></div>
          </div>

          <div style="display: flex; flex-direction: column; gap: 10px; flex: 1; max-width: 200px">
            <div v-for="m in 4" :key="m" style="display: flex; justify-content: space-between; align-items: center">
              <div style="display: flex; align-items: center; gap: 6px">
                <SkeletonBox width="10px" height="10px" circle />
                <SkeletonBox width="60px" height="12px" />
              </div>
              <SkeletonBox width="45px" height="12px" />
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.overview-skeleton-container {
  display: flex;
  flex-direction: column;
  gap: 24px;
  width: 100%;
}

.fastfetch-skeleton-card {
  padding: 18px 20px;
}

.fastfetch-skeleton-layout {
  display: grid;
  grid-template-columns: 154px 1fr;
  gap: 18px;
  align-items: flex-start;
}

.fastfetch-skeleton-logo {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  padding: 16px 8px;
}

.color-palette-dots-skeleton {
  display: flex;
  gap: 6px;
}

.fastfetch-skeleton-info {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.fetch-row-skeleton {
  display: flex;
  align-items: center;
  gap: 12px;
}

.summary-cards-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 18px;
}

.pool-cards-skeleton-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(380px, 1fr));
  gap: 18px;
}

.pool-skeleton-card {
  padding: 18px;
}

.pool-charts-skeleton-container {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
}

.pool-chart-skeleton-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  padding: 14px 10px;
}

.donut-skeleton-wrap {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 110px;
  height: 110px;
}

.donut-cutout {
  position: absolute;
  width: 60px;
  height: 60px;
  border-radius: 50%;
  background: var(--bg-base);
}

.system-iface-skeleton-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  padding: 12px 14px;
  margin-bottom: 16px;
}

.traffic-chart-skeleton-box {
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  padding: 8px;
}

.telemetry-details-skeleton-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(380px, 1fr));
  gap: 18px;
}

.cores-skeleton-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(110px, 1fr));
  gap: 10px;
}

.core-skeleton-box {
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  padding: 8px 10px;
}
</style>
