<template>
  <section class="monitor-page">
    <v-card rounded="xl" class="monitor-hero" variant="tonal">
      <v-card-text class="monitor-hero__content">
        <div>
          <div class="monitor-eyebrow">System Probe</div>
          <div class="monitor-title-row">
            <h2 class="monitor-title">本机监控</h2>
            <v-chip size="small" color="success" variant="flat">实时采集</v-chip>
          </div>
          <p class="monitor-subtitle">
            监控页现在按“时间窗 + 桶宽”工作。滚轮负责缩放当前时间窗，左键按住图表可以左右拖动平移；右侧默认贴近当前时间，缩到 1 小时时按分钟看，缩到 1 天时按小时看，继续放大时最高可看 8 秒桶。
          </p>
        </div>

        <div class="monitor-hero__meta">
          <div class="monitor-meta-pill">
            <span>采样周期</span>
            <strong>{{ sampleIntervalText }}</strong>
          </div>
          <div class="monitor-meta-pill">
            <span>短期保留</span>
            <strong>{{ primaryRetentionText }}</strong>
          </div>
          <div class="monitor-meta-pill">
            <span>归档保留</span>
            <strong>{{ archiveRetentionText }}</strong>
          </div>
          <div class="monitor-meta-pill">
            <span>数据库占用</span>
            <strong>{{ databaseSizeText }}</strong>
          </div>
        </div>
      </v-card-text>
    </v-card>

    <v-alert
      v-if="overviewError"
      type="warning"
      variant="tonal"
      density="comfortable"
      class="mt-4">
      {{ overviewError }}
    </v-alert>

    <v-alert
      v-if="historyError"
      type="warning"
      variant="tonal"
      density="comfortable"
      class="mt-4">
      {{ historyError }}
    </v-alert>

    <v-row class="mt-2">
      <v-col cols="12" xl="8">
        <v-card rounded="xl" class="monitor-config-card" variant="outlined">
          <v-card-title class="monitor-card-title">
            <div>
              <div class="monitor-chart-card__eyebrow">Collection Settings</div>
              <div class="monitor-chart-card__heading">采样与保留设置</div>
            </div>
            <div class="monitor-config-actions">
              <v-btn
                color="primary"
                variant="flat"
                prepend-icon="mdi-content-save-outline"
                :disabled="savingSettings || clearingStats || !hasPendingSettingsChanges"
                :loading="savingSettings"
                @click="saveSettings">
                保存设置
              </v-btn>
            </div>
          </v-card-title>
          <v-divider />
          <v-card-text>
            <v-row class="monitor-config-form-row" align="start">
              <v-col cols="12" md="4">
                <div class="monitor-config-group">
                  <div class="monitor-config-group__label">采样间隔</div>
                  <div class="monitor-config-field-with-unit">
                    <v-text-field
                      v-model.number="sampleIntervalInput"
                      type="number"
                      :step="sampleIntervalInputStep"
                      density="comfortable"
                      variant="outlined"
                      hide-details />
                    <v-select
                      :model-value="sampleIntervalUnit"
                      :items="sampleIntervalUnitItems"
                      item-title="label"
                      item-value="value"
                      density="comfortable"
                      variant="outlined"
                      hide-details
                      class="monitor-config-unit-select"
                      @update:model-value="onSampleIntervalUnitChanged" />
                  </div>
                  <div class="monitor-config-group__hint">{{ sampleIntervalHint }}</div>
                </div>
              </v-col>
              <v-col cols="12" md="4">
                <div class="monitor-config-group">
                  <div class="monitor-config-group__label">短期保留</div>
                  <div class="monitor-config-field-with-unit">
                    <v-text-field
                      v-model.number="primaryRetentionHoursInput"
                      type="number"
                      :step="primaryRetentionInputStep"
                      density="comfortable"
                      variant="outlined"
                      hide-details />
                    <v-select
                      :model-value="primaryRetentionUnit"
                      :items="primaryRetentionUnitItems"
                      item-title="label"
                      item-value="value"
                      density="comfortable"
                      variant="outlined"
                      hide-details
                      class="monitor-config-unit-select"
                      @update:model-value="onPrimaryRetentionUnitChanged" />
                  </div>
                  <div class="monitor-config-group__hint">{{ primaryRetentionHint }}</div>
                </div>
              </v-col>
              <v-col cols="12" md="4">
                <div class="monitor-config-group">
                  <div class="monitor-config-group__label">归档保留（天）</div>
                  <div class="monitor-config-field-with-unit monitor-config-field-with-unit--single">
                    <v-text-field
                      v-model.number="archiveRetentionDaysInput"
                      type="number"
                      min="1"
                      density="comfortable"
                      variant="outlined"
                      hide-details />
                  </div>
                  <div class="monitor-config-group__hint">长时段历史按 30 分钟桶保留</div>
                </div>
              </v-col>
            </v-row>
            <div class="monitor-config-note">
              保存后会立即按新保留时长清理超出范围的旧统计；采样间隔修改后会在后续采样周期自动生效。
            </div>

            <div class="monitor-storage-guide">
              <div class="monitor-storage-guide__item">
                <div class="monitor-storage-guide__label">短期保留是什么</div>
                <strong>{{ primaryRetentionText }}</strong>
                <p>用于分钟级与小时级历史。现在还会额外保留一段 8 秒高精度窗口，便于滚轮放大后继续查看更细的波动。</p>
              </div>
              <div class="monitor-storage-guide__item">
                <div class="monitor-storage-guide__label">归档保留是什么</div>
                <strong>{{ archiveRetentionText }}</strong>
                <p>用于 7 天、30 天、90 天这类长时段查询。拖动到更早历史时，服务端会自动切到更粗的桶来源，不再固定只显示几档刻度。</p>
              </div>
              <div class="monitor-storage-guide__item">
                <div class="monitor-storage-guide__label">数据库占用是什么</div>
                <strong>{{ databaseSizeText }}</strong>
                <p>这是监控库 <code>monitor.db</code> 当前文件体积，只统计监控数据，不是面板主数据库大小。</p>
              </div>
            </div>

            <div class="monitor-danger-box">
              <div>
                <div class="monitor-retention-box__title">数据库维护</div>
                <p class="monitor-danger-box__text">
                  清除统计会删除当前监控历史曲线与聚合桶，并尝试压缩 <code>monitor.db</code>；不会删除监控设置，也不会影响主数据库。
                </p>
              </div>
              <v-btn
                color="error"
                variant="outlined"
                prepend-icon="mdi-delete-alert-outline"
                :disabled="clearingStats || savingSettings"
                :loading="clearingStats"
                @click="clearMonitorStats">
                清空监控历史
              </v-btn>
            </div>
          </v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12" xl="4">
        <v-card rounded="xl" class="monitor-side-card" variant="outlined">
          <v-card-title class="monitor-side-card__title">
            <div>
              <div class="monitor-chart-card__eyebrow">Sampling</div>
              <div class="monitor-chart-card__heading">当前采样</div>
            </div>
            <div class="monitor-side-card__timestamp">
              {{ updatedAtLabel }}
            </div>
          </v-card-title>
          <v-divider />
          <v-card-text>
            <div class="monitor-side-grid">
              <div class="monitor-side-grid__item">
                <span>CPU</span>
                <strong>{{ formatPercent(overview.current.cpuPercent) }}</strong>
              </div>
              <div class="monitor-side-grid__item">
                <span>内存</span>
                <strong>{{ formatPercent(overview.current.memoryPercent) }}</strong>
              </div>
              <div class="monitor-side-grid__item">
                <span>磁盘读</span>
                <strong>{{ formatBytesPerSecond(overview.current.diskReadBps) }}</strong>
              </div>
              <div class="monitor-side-grid__item">
                <span>磁盘写</span>
                <strong>{{ formatBytesPerSecond(overview.current.diskWriteBps) }}</strong>
              </div>
              <div class="monitor-side-grid__item">
                <span>网卡下行</span>
                <strong>{{ formatBytesPerSecond(overview.current.networkDownBps) }}</strong>
              </div>
              <div class="monitor-side-grid__item">
                <span>网卡上行</span>
                <strong>{{ formatBytesPerSecond(overview.current.networkUpBps) }}</strong>
              </div>
            </div>

            <div class="monitor-interface-box mt-4">
              <div class="monitor-retention-box__title">参与统计的物理网卡</div>
              <div v-if="overview.current.physicalInterfaces.length > 0" class="monitor-interface-list">
                <v-chip
                  v-for="iface in overview.current.physicalInterfaces"
                  :key="iface"
                  size="small"
                  color="info"
                  variant="tonal">
                  {{ iface }}
                </v-chip>
              </div>
              <div v-else class="monitor-interface-empty">
                当前没有识别到可统计的物理网卡，或物理网卡还没有第二次采样。
              </div>
            </div>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <v-row class="mt-2 monitor-range-row" align="start">
      <v-col cols="12" lg="5">
        <div class="monitor-range-switch">
          <button
            v-for="item in visibleRangeOptions"
            :key="item.windowSec"
            type="button"
            class="monitor-range-switch__item"
            :class="{ 'monitor-range-switch__item--active': selectedPresetWindow === item.windowSec }"
            @click="selectPresetRange(item.windowSec)">
            <span>{{ item.label }}</span>
          </button>
        </div>
      </v-col>
      <v-col cols="12" lg="4">
        <div class="monitor-custom-range">
          <v-text-field
            v-model.number="customRangeValue"
            type="number"
            min="1"
            density="comfortable"
            variant="outlined"
            hide-details
            label="自定义窗口" />
          <v-select
            v-model="customRangeUnit"
            :items="customRangeUnitItems"
            item-title="label"
            item-value="value"
            density="comfortable"
            variant="outlined"
            hide-details
            label="单位" />
          <v-btn color="secondary" variant="tonal" @click="applyCustomRange">查看</v-btn>
        </div>
      </v-col>
      <v-col cols="12" lg="3">
        <div class="monitor-toolbar-actions">
          <v-btn color="secondary" variant="tonal" @click="zoomIn">放大</v-btn>
          <v-btn color="secondary" variant="tonal" @click="zoomOut">缩小</v-btn>
          <v-btn color="info" variant="text" @click="jumpToNow">回到现在</v-btn>
          <v-btn
            color="primary"
            variant="flat"
            prepend-icon="mdi-refresh"
            :loading="loadingOverview || loadingHistory"
            @click="refreshAll">
            刷新
          </v-btn>
        </div>
      </v-col>
    </v-row>

    <v-row class="mt-2 monitor-granularity-row" align="start">
      <v-col cols="12" lg="8">
        <div class="monitor-granularity-toolbar">
          <div class="monitor-granularity-toolbar__group">
            <span class="monitor-granularity-toolbar__label">当前视窗</span>
            <button type="button" class="monitor-granularity-chip monitor-granularity-chip--active">
              {{ currentWindowLabel }}
            </button>
            <button type="button" class="monitor-granularity-chip">
              每格 {{ currentBucketLabel }}
            </button>
            <button type="button" class="monitor-granularity-chip">
              {{ denseTimeline.length }} 格
            </button>
          </div>
          <div class="monitor-granularity-toolbar__meta">
            <span>视窗范围：{{ currentViewRangeLabel }}</span>
            <span>数据来源：{{ history.sourceBucketSeconds > 0 ? `服务端 ${formatDurationLabel(history.sourceBucketSeconds)}` : '等待查询' }}</span>
          </div>
        </div>
      </v-col>
      <v-col cols="12" lg="4">
        <div class="monitor-wheel-guide" :class="{ 'monitor-wheel-guide--active': isDragging }">
          <div class="monitor-wheel-guide__title">
            {{ isDragging ? '正在拖拽时间轴' : (isPinnedToNow ? '右侧贴近当前时间' : '正在查看历史窗口') }}
          </div>
          <div class="monitor-wheel-guide__text">
            滚轮缩放时间窗，左键按住图表左右拖动平移。8 秒精度当前保留最近 {{ highResRetentionLabel }}，更早的数据请先缩小观察尺度。
          </div>
        </div>
      </v-col>
    </v-row>

    <v-row class="mt-1">
      <v-col cols="12" md="6" xl="4">
        <v-card rounded="xl" class="metric-card metric-card--cpu" variant="outlined">
          <v-card-text>
            <div class="metric-card__label">CPU 占用</div>
            <div class="metric-card__value">{{ formatPercent(overview.current.cpuPercent) }}</div>
            <div class="metric-card__meta">
              区间平均 {{ formatPercent(summary.cpu.avg) }}
              <span class="metric-card__divider">/</span>
              峰值 {{ formatPercent(summary.cpu.peak) }}
            </div>
          </v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12" md="6" xl="4">
        <v-card rounded="xl" class="metric-card metric-card--memory" variant="outlined">
          <v-card-text>
            <div class="metric-card__label">内存占用</div>
            <div class="metric-card__value">{{ formatPercent(overview.current.memoryPercent) }}</div>
            <div class="metric-card__meta">
              {{ formatBytes(overview.current.memoryUsedBytes) }} / {{ formatBytes(overview.current.memoryTotalBytes) }}
            </div>
          </v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12" md="6" xl="4">
        <v-card rounded="xl" class="metric-card metric-card--read" variant="outlined">
          <v-card-text>
            <div class="metric-card__label">磁盘读取</div>
            <div class="metric-card__value">{{ formatBytesPerSecond(overview.current.diskReadBps) }}</div>
            <div class="metric-card__meta">
              区间平均 {{ formatBytesPerSecond(summary.diskRead.avg) }}
              <span class="metric-card__divider">/</span>
              峰值 {{ formatBytesPerSecond(summary.diskRead.peak) }}
            </div>
          </v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12" md="6" xl="4">
        <v-card rounded="xl" class="metric-card metric-card--write" variant="outlined">
          <v-card-text>
            <div class="metric-card__label">磁盘写入</div>
            <div class="metric-card__value">{{ formatBytesPerSecond(overview.current.diskWriteBps) }}</div>
            <div class="metric-card__meta">
              区间平均 {{ formatBytesPerSecond(summary.diskWrite.avg) }}
              <span class="metric-card__divider">/</span>
              峰值 {{ formatBytesPerSecond(summary.diskWrite.peak) }}
            </div>
          </v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12" md="6" xl="4">
        <v-card rounded="xl" class="metric-card metric-card--network-down" variant="outlined">
          <v-card-text>
            <div class="metric-card__label">物理网卡下行</div>
            <div class="metric-card__value">{{ formatBytesPerSecond(overview.current.networkDownBps) }}</div>
            <div class="metric-card__meta">
              区间平均 {{ formatBytesPerSecond(summary.networkDown.avg) }}
              <span class="metric-card__divider">/</span>
              峰值 {{ formatBytesPerSecond(summary.networkDown.peak) }}
            </div>
          </v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12" md="6" xl="4">
        <v-card rounded="xl" class="metric-card metric-card--network-up" variant="outlined">
          <v-card-text>
            <div class="metric-card__label">物理网卡上行</div>
            <div class="metric-card__value">{{ formatBytesPerSecond(overview.current.networkUpBps) }}</div>
            <div class="metric-card__meta">
              区间平均 {{ formatBytesPerSecond(summary.networkUp.avg) }}
              <span class="metric-card__divider">/</span>
              峰值 {{ formatBytesPerSecond(summary.networkUp.peak) }}
            </div>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <v-row class="mt-1">
      <v-col cols="12" xl="8">
        <v-card rounded="xl" class="monitor-chart-card" variant="outlined">
          <v-card-title class="monitor-card-title">
            <div>
              <div class="monitor-chart-card__eyebrow">Utilization</div>
              <div class="monitor-chart-card__heading">CPU / 内存占用趋势</div>
            </div>
            <div class="monitor-chart-card__caption">{{ chartCaption }}</div>
          </v-card-title>
          <v-divider />
          <v-card-text>
            <div
              class="monitor-chart-shell"
              :class="{ 'monitor-chart-shell--active': dragState?.scope === 'utilization' }"
              @mousedown.left="startChartDrag($event, 'utilization')"
              @wheel.prevent="handleChartWheel">
              <div class="monitor-chart-shell__hint">
                {{ dragState?.scope === 'utilization' ? '拖拽平移中，松开后固定视窗' : '滚轮缩放，左键按住左右拖动平移' }}
              </div>
              <div class="monitor-chart-card__canvas">
                <v-skeleton-loader
                  v-if="loadingHistory"
                  type="image"
                  width="100%"
                  height="320" />
                <Line
                  v-else-if="hasHistory"
                  :data="utilizationChartData"
                  :options="utilizationChartOptions" />
                <div v-else class="monitor-empty">
                  还没有历史采样数据，稍等一个采样周期后这里会开始出现曲线。
                </div>
              </div>
            </div>
          </v-card-text>
        </v-card>

        <v-card rounded="xl" class="monitor-chart-card mt-4" variant="outlined">
          <v-card-title class="monitor-card-title">
            <div>
              <div class="monitor-chart-card__eyebrow">Disk Throughput</div>
              <div class="monitor-chart-card__heading">磁盘读写吞吐趋势</div>
            </div>
            <div class="monitor-chart-card__caption">{{ chartCaption }}</div>
          </v-card-title>
          <v-divider />
          <v-card-text>
            <div
              class="monitor-chart-shell"
              :class="{ 'monitor-chart-shell--active': dragState?.scope === 'disk' }"
              @mousedown.left="startChartDrag($event, 'disk')"
              @wheel.prevent="handleChartWheel">
              <div class="monitor-chart-shell__hint">
                {{ dragState?.scope === 'disk' ? '拖拽平移中，松开后固定视窗' : '滚轮缩放，左键按住左右拖动平移' }}
              </div>
              <div class="monitor-chart-card__canvas">
                <v-skeleton-loader
                  v-if="loadingHistory"
                  type="image"
                  width="100%"
                  height="320" />
                <Line
                  v-else-if="hasHistory"
                  :data="diskChartData"
                  :options="throughputChartOptions" />
                <div v-else class="monitor-empty">
                  还没有历史采样数据，稍等一个采样周期后这里会开始出现曲线。
                </div>
              </div>
            </div>
          </v-card-text>
        </v-card>

        <v-card rounded="xl" class="monitor-chart-card mt-4" variant="outlined">
          <v-card-title class="monitor-card-title">
            <div>
              <div class="monitor-chart-card__eyebrow">Physical Network</div>
              <div class="monitor-chart-card__heading">物理网卡上下行趋势</div>
            </div>
            <div class="monitor-chart-card__caption">{{ chartCaption }}</div>
          </v-card-title>
          <v-divider />
          <v-card-text>
            <div
              class="monitor-chart-shell"
              :class="{ 'monitor-chart-shell--active': dragState?.scope === 'network' }"
              @mousedown.left="startChartDrag($event, 'network')"
              @wheel.prevent="handleChartWheel">
              <div class="monitor-chart-shell__hint">
                {{ dragState?.scope === 'network' ? '拖拽平移中，松开后固定视窗' : '滚轮缩放，左键按住左右拖动平移' }}
              </div>
              <div class="monitor-chart-card__canvas">
                <v-skeleton-loader
                  v-if="loadingHistory"
                  type="image"
                  width="100%"
                  height="320" />
                <Line
                  v-else-if="hasHistory"
                  :data="networkChartData"
                  :options="throughputChartOptions" />
                <div v-else class="monitor-empty">
                  还没有历史采样数据，稍等一个采样周期后这里会开始出现曲线。
                </div>
              </div>
            </div>
          </v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12" xl="4">
        <v-card rounded="xl" class="monitor-side-card" variant="outlined">
          <v-card-title class="monitor-side-card__title">
            <div>
              <div class="monitor-chart-card__eyebrow">Range Summary</div>
              <div class="monitor-chart-card__heading">当前视窗摘要</div>
            </div>
          </v-card-title>
          <v-divider />
          <v-card-text>
            <div class="summary-row">
              <div class="summary-row__label">CPU</div>
              <div class="summary-row__stats">
                <div><span>{{ summaryCurrentLabel }}</span><strong>{{ formatPercent(summary.cpu.current) }}</strong></div>
                <div><span>平均</span><strong>{{ formatPercent(summary.cpu.avg) }}</strong></div>
                <div><span>峰值</span><strong>{{ formatPercent(summary.cpu.peak) }}</strong></div>
              </div>
            </div>

            <div class="summary-row">
              <div class="summary-row__label">内存</div>
              <div class="summary-row__stats">
                <div><span>{{ summaryCurrentLabel }}</span><strong>{{ formatPercent(summary.memory.current) }}</strong></div>
                <div><span>平均</span><strong>{{ formatPercent(summary.memory.avg) }}</strong></div>
                <div><span>峰值</span><strong>{{ formatPercent(summary.memory.peak) }}</strong></div>
              </div>
            </div>

            <div class="summary-row">
              <div class="summary-row__label">磁盘读取</div>
              <div class="summary-row__stats">
                <div><span>{{ summaryCurrentLabel }}</span><strong>{{ formatBytesPerSecond(summary.diskRead.current) }}</strong></div>
                <div><span>平均</span><strong>{{ formatBytesPerSecond(summary.diskRead.avg) }}</strong></div>
                <div><span>峰值</span><strong>{{ formatBytesPerSecond(summary.diskRead.peak) }}</strong></div>
              </div>
            </div>

            <div class="summary-row">
              <div class="summary-row__label">磁盘写入</div>
              <div class="summary-row__stats">
                <div><span>{{ summaryCurrentLabel }}</span><strong>{{ formatBytesPerSecond(summary.diskWrite.current) }}</strong></div>
                <div><span>平均</span><strong>{{ formatBytesPerSecond(summary.diskWrite.avg) }}</strong></div>
                <div><span>峰值</span><strong>{{ formatBytesPerSecond(summary.diskWrite.peak) }}</strong></div>
              </div>
            </div>

            <div class="summary-row">
              <div class="summary-row__label">物理网卡下行</div>
              <div class="summary-row__stats">
                <div><span>{{ summaryCurrentLabel }}</span><strong>{{ formatBytesPerSecond(summary.networkDown.current) }}</strong></div>
                <div><span>平均</span><strong>{{ formatBytesPerSecond(summary.networkDown.avg) }}</strong></div>
                <div><span>峰值</span><strong>{{ formatBytesPerSecond(summary.networkDown.peak) }}</strong></div>
              </div>
            </div>

            <div class="summary-row summary-row--last">
              <div class="summary-row__label">物理网卡上行</div>
              <div class="summary-row__stats">
                <div><span>{{ summaryCurrentLabel }}</span><strong>{{ formatBytesPerSecond(summary.networkUp.current) }}</strong></div>
                <div><span>平均</span><strong>{{ formatBytesPerSecond(summary.networkUp.avg) }}</strong></div>
                <div><span>峰值</span><strong>{{ formatBytesPerSecond(summary.networkUp.peak) }}</strong></div>
              </div>
            </div>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>
  </section>
</template>

<script setup lang="ts">
import HttpUtils from '@/plugins/httputil'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Line } from 'vue-chartjs'
import { push } from 'notivue'
import {
  Chart as ChartJS,
  CategoryScale,
  Filler,
  Legend,
  LineElement,
  LinearScale,
  PointElement,
  Title,
  Tooltip,
  type ChartData,
  type ChartOptions,
} from 'chart.js'

ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  Filler,
)
ChartJS.defaults.font.family = 'Vazirmatn, Microsoft YaHei UI, sans-serif'

type MonitorSettings = {
  sampleIntervalSec: number
  primaryRetentionHours: number
  archiveRetentionDays: number
}

type MonitorOverview = {
  available: boolean
  updatedAt: number
  current: MonitorCurrent
  storage: MonitorStorage
  settings: MonitorSettings
  error?: string
}

type MonitorCurrent = {
  cpuPercent: number
  memoryPercent: number
  memoryUsedBytes: number
  memoryTotalBytes: number
  diskReadBps: number
  diskWriteBps: number
  networkUpBps: number
  networkDownBps: number
  physicalInterfaces: string[]
  sampleWindowSec: number
}

type MonitorStorage = {
  databaseSizeBytes: number
  sampleIntervalSec: number
  highResBucketSec: number
  highResKeepHours: number
  primaryBucketMin: number
  primaryKeepHours: number
  archiveBucketMin: number
  archiveKeepDays: number
}

type MonitorHistoryPoint = {
  timestamp: number
  cpuAvg: number
  cpuPeak: number
  memoryAvg: number
  memoryPeak: number
  diskReadAvg: number
  diskReadPeak: number
  diskWriteAvg: number
  diskWritePeak: number
  networkUpAvg: number
  networkUpPeak: number
  networkDownAvg: number
  networkDownPeak: number
}

type MonitorHistory = {
  range: string
  granularity: string
  bucketMinutes: number
  bucketSeconds: number
  sourceBucketSeconds: number
  updatedAt: number
  queryStart: number
  queryEnd: number
  points: MonitorHistoryPoint[]
}

type DenseHistoryBucket = {
  timestamp: number
  point: MonitorHistoryPoint | null
}

type MetricSummary = {
  current: number
  avg: number
  peak: number
}

type SampleIntervalUnit = 's' | 'm' | 'h'
type PrimaryRetentionUnit = 'h' | 'd'
type CustomRangeUnit = 'm' | 'h' | 'd'
type ChartScope = 'utilization' | 'disk' | 'network'
type WindowPreset = {
  label: string
  windowSec: number
}
type DragState = {
  scope: ChartScope
  startX: number
  initialEndSec: number
  width: number
}

const props = withDefaults(defineProps<{ active?: boolean }>(), {
  active: false,
})

const windowPresets: WindowPreset[] = [
  { label: '8 分钟', windowSec: 8 * 60 },
  { label: '16 分钟', windowSec: 16 * 60 },
  { label: '32 分钟', windowSec: 32 * 60 },
  { label: '1 小时', windowSec: 60 * 60 },
  { label: '2 小时', windowSec: 2 * 60 * 60 },
  { label: '4 小时', windowSec: 4 * 60 * 60 },
  { label: '8 小时', windowSec: 8 * 60 * 60 },
  { label: '24 小时', windowSec: 24 * 60 * 60 },
  { label: '7 天', windowSec: 7 * 24 * 60 * 60 },
  { label: '30 天', windowSec: 30 * 24 * 60 * 60 },
  { label: '90 天', windowSec: 90 * 24 * 60 * 60 },
]

const customRangeUnitItems = [
  { label: '分钟', value: 'm' },
  { label: '小时', value: 'h' },
  { label: '天', value: 'd' },
] as const

const sampleIntervalUnitItems = [
  { label: '秒', value: 's' },
  { label: '分', value: 'm' },
  { label: '时', value: 'h' },
] as const

const primaryRetentionUnitItems = [
  { label: '小时', value: 'h' },
  { label: '天', value: 'd' },
] as const

const supportedBucketCandidates = [8, 16, 32, 60, 120, 240, 480, 720, 3600, 7200, 86400]

const overview = ref<MonitorOverview>({
  available: false,
  updatedAt: 0,
  current: {
    cpuPercent: 0,
    memoryPercent: 0,
    memoryUsedBytes: 0,
    memoryTotalBytes: 0,
    diskReadBps: 0,
    diskWriteBps: 0,
    networkUpBps: 0,
    networkDownBps: 0,
    physicalInterfaces: [],
    sampleWindowSec: 10,
  },
  storage: {
    databaseSizeBytes: 0,
    sampleIntervalSec: 10,
    highResBucketSec: 8,
    highResKeepHours: 8,
    primaryBucketMin: 1,
    primaryKeepHours: 48,
    archiveBucketMin: 30,
    archiveKeepDays: 120,
  },
  settings: {
    sampleIntervalSec: 10,
    primaryRetentionHours: 48,
    archiveRetentionDays: 120,
  },
})

const history = ref<MonitorHistory>({
  range: 'window',
  granularity: '1h',
  bucketMinutes: 60,
  bucketSeconds: 3600,
  sourceBucketSeconds: 1800,
  updatedAt: 0,
  queryStart: 0,
  queryEnd: 0,
  points: [],
})

const loadingOverview = ref(false)
const loadingHistory = ref(false)
const savingSettings = ref(false)
const clearingStats = ref(false)
const overviewError = ref('')
const historyError = ref('')

const sampleIntervalInput = ref(10)
const primaryRetentionHoursInput = ref(48)
const archiveRetentionDaysInput = ref(120)
const savedSampleInterval = ref(10)
const savedPrimaryRetentionHours = ref(48)
const savedArchiveRetentionDays = ref(120)

const customRangeValue = ref(24)
const customRangeUnit = ref<CustomRangeUnit>('h')
const sampleIntervalUnit = ref<SampleIntervalUnit>('s')
const primaryRetentionUnit = ref<PrimaryRetentionUnit>('h')

const viewWindowSec = ref(24 * 60 * 60)
const viewEndSec = ref(0)
const selectedPresetWindow = ref(24 * 60 * 60)
const dragState = ref<DragState | null>(null)

let overviewTimer: number | null = null
let historyTimer: number | null = null
let scheduledHistoryLoadTimer: number | null = null
let historyRequestSerial = 0

const sampleIntervalText = computed(() => `${Math.max(1, overview.value.storage.sampleIntervalSec)} 秒`)
const primaryRetentionText = computed(() => `${Math.max(1, overview.value.storage.primaryBucketMin)} 分钟桶 / ${overview.value.storage.primaryKeepHours} 小时`)
const archiveRetentionText = computed(() => `${Math.max(1, overview.value.storage.archiveBucketMin)} 分钟桶 / ${overview.value.storage.archiveKeepDays} 天`)
const databaseSizeText = computed(() => formatBytes(overview.value.storage.databaseSizeBytes))
const updatedAtLabel = computed(() => formatDateTime(overview.value.updatedAt))
const highResRetentionLabel = computed(() => `${Math.max(1, overview.value.storage.highResKeepHours)} 小时`)
const sampleIntervalInputStep = computed(() => (sampleIntervalUnit.value === 's' ? 1 : 0.01))
const primaryRetentionInputStep = computed(() => (primaryRetentionUnit.value === 'h' ? 1 : 0.01))
const sampleIntervalHint = computed(() => {
  switch (sampleIntervalUnit.value) {
    case 'm':
      return '按分钟填写，换算后最小 1 秒，最大 60 分钟'
    case 'h':
      return '按小时填写，换算后最小 1 秒，最大 1 小时'
    default:
      return '按秒填写，最小 1 秒，最大 3600 秒'
  }
})
const primaryRetentionHint = computed(() => (
  primaryRetentionUnit.value === 'd'
    ? '按天填写，分钟级与小时级历史会先从短期桶查询'
    : '按小时填写，分钟级与小时级历史会先从短期桶查询'
))

const maxViewWindowSec = computed(() => (
  Math.max(24 * 60 * 60, overview.value.storage.archiveKeepDays * 24 * 60 * 60)
))
const minViewWindowSec = 8 * 60

const visibleRangeOptions = computed(() => (
  windowPresets.filter(item => item.windowSec <= maxViewWindowSec.value)
))

const hasPendingSettingsChanges = computed(() => (
  sanitizeSampleInterval(sampleIntervalInput.value, sampleIntervalUnit.value) !== savedSampleInterval.value ||
  sanitizePrimaryRetentionHours(primaryRetentionHoursInput.value, primaryRetentionUnit.value) !== savedPrimaryRetentionHours.value ||
  sanitizeArchiveRetentionDays(archiveRetentionDaysInput.value) !== savedArchiveRetentionDays.value
))

const currentBucketSeconds = computed(() => (
  history.value.bucketSeconds > 0 ? history.value.bucketSeconds : resolveBucketSeconds(viewWindowSec.value)
))
const currentBucketLabel = computed(() => formatDurationLabel(currentBucketSeconds.value))
const currentWindowLabel = computed(() => formatWindowLabel(viewWindowSec.value))
const currentViewRangeLabel = computed(() => (
  `${formatDateTimeCompact(viewEndSec.value - viewWindowSec.value)} - ${formatDateTimeCompact(viewEndSec.value)}`
))
const isPinnedToNow = computed(() => Math.abs(viewEndSec.value - currentNowSec()) <= Math.max(2, currentBucketSeconds.value))
const isDragging = computed(() => dragState.value !== null)
const summaryCurrentLabel = computed(() => (isPinnedToNow.value ? '当前' : '右端'))

const chartCaption = computed(() => `${currentWindowLabel.value} · 每格 ${currentBucketLabel.value} · ${denseTimeline.value.length} 格`)

const denseTimeline = computed<DenseHistoryBucket[]>(() => buildDenseTimeline(
  history.value.queryStart,
  history.value.queryEnd,
  currentBucketSeconds.value,
  history.value.points,
))

const axisLabels = computed(() => denseTimeline.value.map(bucket => formatAxisLabel(bucket.timestamp, currentBucketSeconds.value, viewWindowSec.value)))
const axisLabelStep = computed(() => chooseAxisLabelStep(denseTimeline.value.length, currentBucketSeconds.value, viewWindowSec.value))
const hasHistory = computed(() => denseTimeline.value.some(bucket => bucket.point !== null))

const latestViewportPoint = computed<MonitorHistoryPoint | null>(() => {
  for (let index = denseTimeline.value.length - 1; index >= 0; index -= 1) {
    const point = denseTimeline.value[index]?.point
    if (point) return point
  }
  return null
})

const utilizationChartData = computed<ChartData<'line'>>(() => ({
  labels: axisLabels.value,
  datasets: [
    buildLineDataset('CPU', denseTimeline.value.map(bucket => bucket.point ? round1(bucket.point.cpuAvg) : null), '#f59e0b', 'rgba(245, 158, 11, 0.14)'),
    buildLineDataset('内存', denseTimeline.value.map(bucket => bucket.point ? round1(bucket.point.memoryAvg) : null), '#38bdf8', 'rgba(56, 189, 248, 0.12)'),
  ],
}))

const diskChartData = computed<ChartData<'line'>>(() => ({
  labels: axisLabels.value,
  datasets: [
    buildLineDataset('读取', denseTimeline.value.map(bucket => bucket.point ? bucket.point.diskReadAvg : null), '#22c55e', 'rgba(34, 197, 94, 0.12)'),
    buildLineDataset('写入', denseTimeline.value.map(bucket => bucket.point ? bucket.point.diskWriteAvg : null), '#a855f7', 'rgba(168, 85, 247, 0.10)'),
  ],
}))

const networkChartData = computed<ChartData<'line'>>(() => ({
  labels: axisLabels.value,
  datasets: [
    buildLineDataset('下行', denseTimeline.value.map(bucket => bucket.point ? bucket.point.networkDownAvg : null), '#0ea5e9', 'rgba(14, 165, 233, 0.12)'),
    buildLineDataset('上行', denseTimeline.value.map(bucket => bucket.point ? bucket.point.networkUpAvg : null), '#fb7185', 'rgba(251, 113, 133, 0.10)'),
  ],
}))

const utilizationChartOptions = computed<ChartOptions<'line'>>(() => buildPercentChartOptions())
const throughputChartOptions = computed<ChartOptions<'line'>>(() => buildThroughputChartOptions())

const summary = computed(() => {
  const currentPoint = latestViewportPoint.value
  return {
    cpu: buildMetricSummary(currentPoint?.cpuAvg ?? overview.value.current.cpuPercent, denseTimeline.value.map(bucket => bucket.point?.cpuAvg ?? null), denseTimeline.value.map(bucket => bucket.point?.cpuPeak ?? null)),
    memory: buildMetricSummary(currentPoint?.memoryAvg ?? overview.value.current.memoryPercent, denseTimeline.value.map(bucket => bucket.point?.memoryAvg ?? null), denseTimeline.value.map(bucket => bucket.point?.memoryPeak ?? null)),
    diskRead: buildMetricSummary(currentPoint?.diskReadAvg ?? overview.value.current.diskReadBps, denseTimeline.value.map(bucket => bucket.point?.diskReadAvg ?? null), denseTimeline.value.map(bucket => bucket.point?.diskReadPeak ?? null)),
    diskWrite: buildMetricSummary(currentPoint?.diskWriteAvg ?? overview.value.current.diskWriteBps, denseTimeline.value.map(bucket => bucket.point?.diskWriteAvg ?? null), denseTimeline.value.map(bucket => bucket.point?.diskWritePeak ?? null)),
    networkDown: buildMetricSummary(currentPoint?.networkDownAvg ?? overview.value.current.networkDownBps, denseTimeline.value.map(bucket => bucket.point?.networkDownAvg ?? null), denseTimeline.value.map(bucket => bucket.point?.networkDownPeak ?? null)),
    networkUp: buildMetricSummary(currentPoint?.networkUpAvg ?? overview.value.current.networkUpBps, denseTimeline.value.map(bucket => bucket.point?.networkUpAvg ?? null), denseTimeline.value.map(bucket => bucket.point?.networkUpPeak ?? null)),
  }
})

const refreshAll = async () => {
  await loadOverview()
  await loadHistory(isPinnedToNow.value)
}

const loadOverview = async () => {
  loadingOverview.value = true
  overviewError.value = ''
  try {
    const msg = await HttpUtils.get('api/system-monitor-overview', {}, { silentAuthCheck: true })
    if (msg.success && msg.obj) {
      overview.value = normalizeOverview(msg.obj)
      syncSettingsInputs()
      normalizeSelectedPresetWindow()
      if (overview.value.error) {
        overviewError.value = overview.value.error
      }
    } else if (msg.msg) {
      overviewError.value = msg.msg
    }
  } finally {
    loadingOverview.value = false
  }
}

const loadHistory = async (pinToNow = false) => {
  const requestSerial = ++historyRequestSerial
  const nextWindowSec = clampWindowSec(viewWindowSec.value)
  const nextBucketSec = resolveBucketSeconds(nextWindowSec)
  const nextEndSec = clampViewEnd(nextWindowSec, nextBucketSec, pinToNow ? currentNowSec() : normalizedViewEnd())
  const nextStartSec = nextEndSec - nextWindowSec

  viewWindowSec.value = nextWindowSec
  viewEndSec.value = nextEndSec
  selectedPresetWindow.value = findPresetWindow(nextWindowSec)

  loadingHistory.value = true
  historyError.value = ''
  try {
    const msg = await HttpUtils.get('api/system-monitor-history', {
      start: nextStartSec,
      end: nextEndSec,
      bucket_sec: nextBucketSec,
    }, { silentAuthCheck: true })
    if (requestSerial !== historyRequestSerial) {
      return
    }
    if (msg.success && msg.obj) {
      history.value = normalizeHistory(msg.obj)
      if (history.value.queryEnd > 0) {
        viewEndSec.value = history.value.queryEnd
      }
    } else if (msg.msg) {
      historyError.value = msg.msg
    }
  } finally {
    if (requestSerial === historyRequestSerial) {
      loadingHistory.value = false
    }
  }
}

const scheduleHistoryLoad = (delay = 80) => {
  if (scheduledHistoryLoadTimer != null) {
    window.clearTimeout(scheduledHistoryLoadTimer)
    scheduledHistoryLoadTimer = null
  }
  scheduledHistoryLoadTimer = window.setTimeout(() => {
    scheduledHistoryLoadTimer = null
    void loadHistory(false)
  }, delay)
}

const saveSettings = async () => {
  const payload = {
    sample_interval_sec: sanitizeSampleInterval(sampleIntervalInput.value, sampleIntervalUnit.value),
    primary_retention_hours: sanitizePrimaryRetentionHours(primaryRetentionHoursInput.value, primaryRetentionUnit.value),
    archive_retention_days: sanitizeArchiveRetentionDays(archiveRetentionDaysInput.value),
  }

  savingSettings.value = true
  overviewError.value = ''
  try {
    const msg = await HttpUtils.post('api/system-monitor-settings', payload, { silentAuthCheck: true })
    if (msg.success && msg.obj) {
      overview.value = normalizeOverview(msg.obj)
      syncSettingsInputs()
      normalizeSelectedPresetWindow()
      await loadHistory(isPinnedToNow.value)
      startTimers()
    } else if (msg.msg) {
      overviewError.value = msg.msg
    }
  } finally {
    savingSettings.value = false
  }
}

const clearMonitorStats = async () => {
  const confirmed = window.confirm(
    '确认清空监控统计并压缩 monitor.db 吗？\n\n这会删除当前监控历史曲线与聚合桶数据，但不会删除监控设置，也不会影响主数据库。',
  )
  if (!confirmed) {
    return
  }

  clearingStats.value = true
  overviewError.value = ''
  historyError.value = ''
  try {
    const msg = await HttpUtils.post('api/system-monitor-reset', {}, { silentAuthCheck: true })
    if (msg.success && msg.obj) {
      overview.value = normalizeOverview(msg.obj)
      history.value = {
        range: 'window',
        granularity: history.value.granularity,
        bucketMinutes: history.value.bucketMinutes,
        bucketSeconds: history.value.bucketSeconds,
        sourceBucketSeconds: history.value.sourceBucketSeconds,
        updatedAt: overview.value.updatedAt,
        queryStart: history.value.queryStart,
        queryEnd: history.value.queryEnd,
        points: [],
      }
      syncSettingsInputs()
      await loadHistory(isPinnedToNow.value)
      startTimers()
      push.success({
        title: '监控统计已清空',
        duration: 4000,
        message: '历史聚合数据已删除，监控库已尝试压缩，新的曲线会从后续采样重新开始。',
      })
    } else if (msg.msg) {
      overviewError.value = msg.msg
    }
  } finally {
    clearingStats.value = false
  }
}

const selectPresetRange = async (windowSec: number) => {
  viewWindowSec.value = clampWindowSec(windowSec)
  selectedPresetWindow.value = windowSec
  viewEndSec.value = clampViewEnd(viewWindowSec.value, resolveBucketSeconds(viewWindowSec.value), currentNowSec())
  await loadHistory(true)
  restartHistoryTimer()
}

const applyCustomRange = async () => {
  const nextWindowSec = customWindowSeconds()
  viewWindowSec.value = clampWindowSec(nextWindowSec)
  selectedPresetWindow.value = findPresetWindow(viewWindowSec.value)
  viewEndSec.value = clampViewEnd(viewWindowSec.value, resolveBucketSeconds(viewWindowSec.value), currentNowSec())
  await loadHistory(true)
  restartHistoryTimer()
}

const zoomIn = async () => {
  await applyZoom(0.8, 1)
}

const zoomOut = async () => {
  await applyZoom(1.25, 1)
}

const jumpToNow = async () => {
  viewEndSec.value = currentNowSec()
  await loadHistory(true)
  restartHistoryTimer()
}

const applyZoom = async (factor: number, anchorRatio: number) => {
  const currentWindow = clampWindowSec(viewWindowSec.value)
  const nextWindow = clampWindowSec(Math.round(currentWindow * factor))
  if (nextWindow === currentWindow) {
    return
  }
  const safeRatio = clamp(anchorRatio, 0, 1)
  const currentEnd = normalizedViewEnd()
  const currentStart = currentEnd - currentWindow
  const anchorTimestamp = currentStart + (currentWindow * safeRatio)
  const nextEnd = anchorTimestamp + (nextWindow * (1 - safeRatio))
  viewWindowSec.value = nextWindow
  selectedPresetWindow.value = findPresetWindow(nextWindow)
  viewEndSec.value = clampViewEnd(nextWindow, resolveBucketSeconds(nextWindow), nextEnd)
  await loadHistory(false)
  restartHistoryTimer()
}

const handleChartWheel = async (event: WheelEvent) => {
  const target = event.currentTarget as HTMLElement | null
  const rect = target?.getBoundingClientRect()
  const ratio = rect && rect.width > 0
    ? clamp((event.clientX - rect.left) / rect.width, 0, 1)
    : 1
  await applyZoom(event.deltaY < 0 ? 0.8 : 1.25, ratio)
}

const startChartDrag = (event: MouseEvent, scope: ChartScope) => {
  if (event.button !== 0) {
    return
  }
  const target = event.currentTarget as HTMLElement | null
  const width = Math.max(target?.getBoundingClientRect().width ?? 0, 1)
  dragState.value = {
    scope,
    startX: event.clientX,
    initialEndSec: normalizedViewEnd(),
    width,
  }
  event.preventDefault()
}

const handleWindowMouseMove = (event: MouseEvent) => {
  if (!dragState.value) {
    return
  }
  const deltaX = event.clientX - dragState.value.startX
  const shiftRatio = deltaX / Math.max(1, dragState.value.width)
  const shiftedEnd = dragState.value.initialEndSec - (shiftRatio * viewWindowSec.value)
  viewEndSec.value = clampViewEnd(viewWindowSec.value, resolveBucketSeconds(viewWindowSec.value), shiftedEnd)
  scheduleHistoryLoad(50)
}

const handleWindowMouseUp = () => {
  if (!dragState.value) {
    return
  }
  dragState.value = null
  void loadHistory(false)
  restartHistoryTimer()
}

const syncSettingsInputs = () => {
  const sampleIntervalDisplay = parseSampleIntervalDisplay(overview.value.settings.sampleIntervalSec)
  const primaryRetentionDisplay = parsePrimaryRetentionDisplay(overview.value.settings.primaryRetentionHours)
  sampleIntervalInput.value = sampleIntervalDisplay.value
  sampleIntervalUnit.value = sampleIntervalDisplay.unit
  primaryRetentionHoursInput.value = primaryRetentionDisplay.value
  primaryRetentionUnit.value = primaryRetentionDisplay.unit
  archiveRetentionDaysInput.value = overview.value.settings.archiveRetentionDays
  savedSampleInterval.value = overview.value.settings.sampleIntervalSec
  savedPrimaryRetentionHours.value = overview.value.settings.primaryRetentionHours
  savedArchiveRetentionDays.value = overview.value.settings.archiveRetentionDays
}

const onSampleIntervalUnitChanged = (nextUnit: SampleIntervalUnit) => {
  const currentSeconds = sanitizeSampleInterval(sampleIntervalInput.value, sampleIntervalUnit.value)
  sampleIntervalUnit.value = nextUnit
  sampleIntervalInput.value = convertSampleIntervalSecondsToDisplayValue(currentSeconds, nextUnit)
}

const onPrimaryRetentionUnitChanged = (nextUnit: PrimaryRetentionUnit) => {
  const currentHours = sanitizePrimaryRetentionHours(primaryRetentionHoursInput.value, primaryRetentionUnit.value)
  primaryRetentionUnit.value = nextUnit
  primaryRetentionHoursInput.value = convertPrimaryRetentionHoursToDisplayValue(currentHours, nextUnit)
}

const normalizeSelectedPresetWindow = () => {
  const currentMatch = findPresetWindow(viewWindowSec.value)
  if (currentMatch > 0) {
    selectedPresetWindow.value = currentMatch
    return
  }
  const lastVisible = visibleRangeOptions.value[visibleRangeOptions.value.length - 1]
  if (lastVisible && viewWindowSec.value > lastVisible.windowSec) {
    viewWindowSec.value = lastVisible.windowSec
    selectedPresetWindow.value = lastVisible.windowSec
  }
}

const stopTimers = () => {
  if (overviewTimer != null) {
    window.clearInterval(overviewTimer)
    overviewTimer = null
  }
  if (historyTimer != null) {
    window.clearInterval(historyTimer)
    historyTimer = null
  }
  if (scheduledHistoryLoadTimer != null) {
    window.clearTimeout(scheduledHistoryLoadTimer)
    scheduledHistoryLoadTimer = null
  }
}

const restartHistoryTimer = () => {
  if (!props.active) return
  if (historyTimer != null) {
    window.clearInterval(historyTimer)
    historyTimer = null
  }
  if (!isPinnedToNow.value) {
    return
  }

  const interval = currentBucketSeconds.value < 60
    ? 8000
    : currentBucketSeconds.value < 3600
      ? 15000
      : 30000

  historyTimer = window.setInterval(() => {
    void loadHistory(true)
  }, interval)
}

const startTimers = () => {
  stopTimers()
  const overviewInterval = Math.max(2000, Math.min(15000, Math.max(1, overview.value.settings.sampleIntervalSec) * 1000))
  overviewTimer = window.setInterval(() => {
    void loadOverview()
  }, overviewInterval)
  restartHistoryTimer()
}

watch(() => props.active, async active => {
  if (!active) {
    stopTimers()
    return
  }
  if (viewEndSec.value <= 0) {
    viewEndSec.value = currentNowSec()
  }
  await refreshAll()
  startTimers()
}, { immediate: true })

onMounted(() => {
  viewEndSec.value = currentNowSec()
  window.addEventListener('mousemove', handleWindowMouseMove)
  window.addEventListener('mouseup', handleWindowMouseUp)
})

onBeforeUnmount(() => {
  stopTimers()
  window.removeEventListener('mousemove', handleWindowMouseMove)
  window.removeEventListener('mouseup', handleWindowMouseUp)
})

function normalizeOverview(raw: any): MonitorOverview {
  const current = raw?.current ?? {}
  const storage = raw?.storage ?? {}
  const settings = raw?.settings ?? {}
  return {
    available: readBool(raw?.available),
    updatedAt: readNumber(raw?.updatedAt),
    current: {
      cpuPercent: readNumber(current?.cpuPercent),
      memoryPercent: readNumber(current?.memoryPercent),
      memoryUsedBytes: readNumber(current?.memoryUsedBytes),
      memoryTotalBytes: readNumber(current?.memoryTotalBytes),
      diskReadBps: readNumber(current?.diskReadBps),
      diskWriteBps: readNumber(current?.diskWriteBps),
      networkUpBps: readNumber(current?.networkUpBps),
      networkDownBps: readNumber(current?.networkDownBps),
      physicalInterfaces: Array.isArray(current?.physicalInterfaces)
        ? current.physicalInterfaces.map((item: unknown) => String(item ?? '').trim()).filter((item: string) => item.length > 0)
        : [],
      sampleWindowSec: readNumber(current?.sampleWindowSec, 10),
    },
    storage: {
      databaseSizeBytes: readNumber(storage?.databaseSizeBytes),
      sampleIntervalSec: readNumber(storage?.sampleIntervalSec, 10),
      highResBucketSec: readNumber(storage?.highResBucketSec, 8),
      highResKeepHours: readNumber(storage?.highResKeepHours, 8),
      primaryBucketMin: readNumber(storage?.primaryBucketMin, 1),
      primaryKeepHours: readNumber(storage?.primaryKeepHours, 48),
      archiveBucketMin: readNumber(storage?.archiveBucketMin, 30),
      archiveKeepDays: readNumber(storage?.archiveKeepDays, 120),
    },
    settings: {
      sampleIntervalSec: readNumber(settings?.sampleIntervalSec, 10),
      primaryRetentionHours: readNumber(settings?.primaryRetentionHours, 48),
      archiveRetentionDays: readNumber(settings?.archiveRetentionDays, 120),
    },
    error: typeof raw?.error === 'string' ? raw.error : '',
  }
}

function normalizeHistory(raw: any): MonitorHistory {
  const pointsRaw = Array.isArray(raw?.points) ? raw.points : []
  return {
    range: typeof raw?.range === 'string' ? raw.range : 'window',
    granularity: String(raw?.granularity ?? ''),
    bucketMinutes: readNumber(raw?.bucketMinutes, 0),
    bucketSeconds: readNumber(raw?.bucketSeconds, 3600),
    sourceBucketSeconds: readNumber(raw?.sourceBucketSeconds, 1800),
    updatedAt: readNumber(raw?.updatedAt),
    queryStart: readNumber(raw?.queryStart),
    queryEnd: readNumber(raw?.queryEnd),
    points: pointsRaw.map((point: any) => ({
      timestamp: readNumber(point?.timestamp),
      cpuAvg: readNumber(point?.cpuAvg),
      cpuPeak: readNumber(point?.cpuPeak),
      memoryAvg: readNumber(point?.memoryAvg),
      memoryPeak: readNumber(point?.memoryPeak),
      diskReadAvg: readNumber(point?.diskReadAvg),
      diskReadPeak: readNumber(point?.diskReadPeak),
      diskWriteAvg: readNumber(point?.diskWriteAvg),
      diskWritePeak: readNumber(point?.diskWritePeak),
      networkUpAvg: readNumber(point?.networkUpAvg),
      networkUpPeak: readNumber(point?.networkUpPeak),
      networkDownAvg: readNumber(point?.networkDownAvg),
      networkDownPeak: readNumber(point?.networkDownPeak),
    })),
  }
}

function sanitizeSampleInterval(value: number, unit: SampleIntervalUnit): number {
  if (!Number.isFinite(value) || value <= 0) return 10
  const factor = unit === 'h' ? 3600 : unit === 'm' ? 60 : 1
  return Math.max(1, Math.min(3600, Math.round(value * factor)))
}

function sanitizePrimaryRetentionHours(value: number, unit: PrimaryRetentionUnit): number {
  if (!Number.isFinite(value) || value <= 0) return 48
  const factor = unit === 'd' ? 24 : 1
  return Math.max(1, Math.min(24 * 365, Math.round(value * factor)))
}

function sanitizeArchiveRetentionDays(value: number): number {
  if (!Number.isFinite(value) || value <= 0) return 120
  return Math.max(1, Math.min(3650, Math.floor(value)))
}

function sanitizeCustomRangeValue(value: number): number {
  if (!Number.isFinite(value) || value <= 0) return 1
  return Math.max(1, Math.floor(value))
}

function parseSampleIntervalDisplay(seconds: number): { value: number, unit: SampleIntervalUnit } {
  const normalizedSeconds = Math.max(1, Math.floor(seconds))
  if (normalizedSeconds % 3600 === 0) {
    return {
      value: convertSampleIntervalSecondsToDisplayValue(normalizedSeconds, 'h'),
      unit: 'h',
    }
  }
  if (normalizedSeconds % 60 === 0) {
    return {
      value: convertSampleIntervalSecondsToDisplayValue(normalizedSeconds, 'm'),
      unit: 'm',
    }
  }
  return {
    value: normalizedSeconds,
    unit: 's',
  }
}

function parsePrimaryRetentionDisplay(hours: number): { value: number, unit: PrimaryRetentionUnit } {
  const normalizedHours = Math.max(1, Math.floor(hours))
  if (normalizedHours % 24 === 0) {
    return {
      value: convertPrimaryRetentionHoursToDisplayValue(normalizedHours, 'd'),
      unit: 'd',
    }
  }
  return {
    value: normalizedHours,
    unit: 'h',
  }
}

function convertSampleIntervalSecondsToDisplayValue(seconds: number, unit: SampleIntervalUnit): number {
  switch (unit) {
    case 'm':
      return roundUnitValue(seconds / 60, 3)
    case 'h':
      return roundUnitValue(seconds / 3600, 6)
    default:
      return Math.max(1, Math.floor(seconds))
  }
}

function convertPrimaryRetentionHoursToDisplayValue(hours: number, unit: PrimaryRetentionUnit): number {
  if (unit === 'd') {
    return roundUnitValue(hours / 24, 4)
  }
  return Math.max(1, Math.floor(hours))
}

function roundUnitValue(value: number, digits: number): number {
  const factor = 10 ** digits
  return Math.round(value * factor) / factor
}

function buildMetricSummary(current: number, averages: Array<number | null>, peaks: Array<number | null>): MetricSummary {
  const safeAverages = averages.filter((value): value is number => typeof value === 'number' && Number.isFinite(value) && value >= 0)
  const safePeaks = peaks.filter((value): value is number => typeof value === 'number' && Number.isFinite(value) && value >= 0)
  return {
    current,
    avg: safeAverages.length > 0 ? safeAverages.reduce((sum, value) => sum + value, 0) / safeAverages.length : current,
    peak: safePeaks.length > 0 ? Math.max(...safePeaks) : current,
  }
}

function buildLineDataset(label: string, data: Array<number | null>, borderColor: string, backgroundColor: string) {
  return {
    label,
    data,
    borderColor,
    backgroundColor,
    fill: true,
    tension: 0.28,
    spanGaps: false,
    pointRadius: 0,
    pointHoverRadius: 3,
    borderWidth: 2,
  }
}

function buildPercentChartOptions(): ChartOptions<'line'> {
  return buildBaseChartOptions(true)
}

function buildThroughputChartOptions(): ChartOptions<'line'> {
  return buildBaseChartOptions(false)
}

function buildBaseChartOptions(percentMode: boolean): ChartOptions<'line'> {
  const labelStep = axisLabelStep.value
  return {
    responsive: true,
    maintainAspectRatio: false,
    animation: false,
    interaction: {
      mode: 'index',
      intersect: false,
    },
    plugins: {
      legend: {
        position: 'top',
        labels: {
          color: '#cbd5e1',
          boxWidth: 12,
          boxHeight: 12,
        },
      },
      tooltip: {
        callbacks: {
          title(items) {
            const index = items[0]?.dataIndex ?? 0
            const bucket = denseTimeline.value[index]
            if (!bucket) return ''
            return formatDateTime(bucket.timestamp)
          },
          label(context) {
            const value = typeof context.parsed.y === 'number' ? context.parsed.y : 0
            return percentMode
              ? `${context.dataset.label}: ${formatPercent(value)}`
              : `${context.dataset.label}: ${formatBytesPerSecond(value)}`
          },
        },
      },
    },
    scales: {
      x: {
        ticks: {
          color: '#94a3b8',
          autoSkip: false,
          maxRotation: 0,
          minRotation: 0,
          font: {
            size: 10,
          },
          callback(_value, index) {
            const safeIndex = Number(index)
            const raw = axisLabels.value[safeIndex] ?? ''
            const lastIndex = axisLabels.value.length - 1
            if (safeIndex === 0 || safeIndex === lastIndex || safeIndex % labelStep === 0) {
              return raw
            }
            return ''
          },
        },
        grid: {
          color: 'rgba(148, 163, 184, 0.10)',
        },
      },
      y: percentMode
        ? {
            min: 0,
            max: 100,
            ticks: {
              color: '#94a3b8',
              callback(value) {
                return `${value}%`
              },
            },
            grid: {
              color: 'rgba(148, 163, 184, 0.10)',
            },
          }
        : {
            ticks: {
              color: '#94a3b8',
              callback(value) {
                return formatBytesPerSecond(Number(value))
              },
            },
            grid: {
              color: 'rgba(148, 163, 184, 0.10)',
            },
          },
    },
  }
}

function readNumber(value: unknown, fallback = 0): number {
  if (typeof value === 'number' && Number.isFinite(value)) {
    return value
  }
  if (typeof value === 'string') {
    const parsed = Number(value.trim())
    if (Number.isFinite(parsed)) {
      return parsed
    }
  }
  return fallback
}

function readBool(value: unknown): boolean {
  if (typeof value === 'boolean') return value
  if (typeof value === 'number') return value !== 0
  if (typeof value === 'string') {
    const normalized = value.trim().toLowerCase()
    return normalized === 'true' || normalized === '1'
  }
  return false
}

function round1(value: number): number {
  return Math.round(value * 10) / 10
}

function formatPercent(value: number): string {
  const normalized = Number.isFinite(value) ? Math.max(0, Math.min(100, value)) : 0
  if (normalized >= 100) return '100%'
  if (normalized >= 10) return `${normalized.toFixed(1)}%`
  return `${normalized.toFixed(2)}%`
}

function formatBytes(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let current = value
  let unitIndex = 0
  while (current >= 1024 && unitIndex < units.length - 1) {
    current /= 1024
    unitIndex += 1
  }
  const digits = current >= 100 ? 0 : current >= 10 ? 1 : 2
  return `${current.toFixed(digits)} ${units[unitIndex]}`
}

function formatBytesPerSecond(value: number): string {
  return `${formatBytes(value)}/s`
}

function formatDateTime(timestamp: number): string {
  if (!Number.isFinite(timestamp) || timestamp <= 0) return '等待首个样本'
  return new Date(timestamp * 1000).toLocaleString()
}

function formatDateTimeCompact(timestamp: number): string {
  if (!Number.isFinite(timestamp) || timestamp <= 0) return '--'
  return new Date(timestamp * 1000).toLocaleString([], {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: currentBucketSeconds.value < 60 ? '2-digit' : undefined,
    hour12: false,
  })
}

function formatAxisLabel(timestamp: number, bucketSeconds: number, windowSeconds: number): string {
  const date = new Date(timestamp * 1000)
  if (bucketSeconds < 60) {
    return date.toLocaleTimeString([], {
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
    })
  }
  if (bucketSeconds < 3600) {
    return date.toLocaleTimeString([], {
      hour: windowSeconds > 3 * 60 * 60 ? '2-digit' : undefined,
      minute: '2-digit',
      hour12: false,
    })
  }
  if (bucketSeconds < 86400) {
    return date.toLocaleString([], {
      month: windowSeconds > 24 * 60 * 60 ? '2-digit' : undefined,
      day: windowSeconds > 24 * 60 * 60 ? '2-digit' : undefined,
      hour: '2-digit',
      hour12: false,
    })
  }
  return date.toLocaleDateString([], {
    month: '2-digit',
    day: '2-digit',
  })
}

function chooseAxisLabelStep(total: number, bucketSeconds: number, windowSeconds: number): number {
  if (total <= 12) return 1
  if (bucketSeconds < 60) return 5
  if (bucketSeconds < 3600) {
    return windowSeconds <= 60 * 60 ? 5 : Math.max(2, Math.ceil(total / 12))
  }
  if (bucketSeconds < 86400) {
    return Math.max(2, Math.ceil(total / 12))
  }
  return Math.max(2, Math.ceil(total / 10))
}

function buildDenseTimeline(queryStart: number, queryEnd: number, bucketSeconds: number, points: MonitorHistoryPoint[]): DenseHistoryBucket[] {
  if (queryStart <= 0 || queryEnd <= 0 || queryEnd <= queryStart || bucketSeconds <= 0) {
    return []
  }
  const count = Math.max(1, Math.ceil((queryEnd - queryStart) / bucketSeconds))
  const alignedEnd = Math.floor(queryEnd / bucketSeconds) * bucketSeconds
  const alignedStart = alignedEnd - ((count - 1) * bucketSeconds)
  const pointMap = new Map(points.map(point => [point.timestamp, point] as const))
  const buckets: DenseHistoryBucket[] = []
  for (let index = 0; index < count; index += 1) {
    const timestamp = alignedStart + (index * bucketSeconds)
    buckets.push({
      timestamp,
      point: pointMap.get(timestamp) ?? null,
    })
  }
  return buckets
}

function customWindowSeconds(): number {
  const value = sanitizeCustomRangeValue(customRangeValue.value)
  switch (customRangeUnit.value) {
    case 'd':
      return value * 24 * 60 * 60
    case 'h':
      return value * 60 * 60
    default:
      return value * 60
  }
}

function clampWindowSec(windowSeconds: number): number {
  if (!Number.isFinite(windowSeconds) || windowSeconds <= 0) {
    return minViewWindowSec
  }
  return Math.max(minViewWindowSec, Math.min(maxViewWindowSec.value, Math.round(windowSeconds)))
}

function resolveBucketSeconds(windowSeconds: number): number {
  const normalizedWindow = clampWindowSec(windowSeconds)
  let desiredBucket = 86400
  if (normalizedWindow <= 8 * 60) {
    desiredBucket = 8
  } else if (normalizedWindow <= 16 * 60) {
    desiredBucket = 16
  } else if (normalizedWindow <= 32 * 60) {
    desiredBucket = 32
  } else if (normalizedWindow <= 60 * 60) {
    desiredBucket = 60
  } else if (normalizedWindow <= 2 * 60 * 60) {
    desiredBucket = 120
  } else if (normalizedWindow <= 4 * 60 * 60) {
    desiredBucket = 240
  } else if (normalizedWindow <= 8 * 60 * 60) {
    desiredBucket = 480
  } else if (normalizedWindow <= 12 * 60 * 60) {
    desiredBucket = 720
  } else if (normalizedWindow <= 24 * 60 * 60) {
    desiredBucket = 3600
  } else if (normalizedWindow <= 48 * 60 * 60) {
    desiredBucket = 7200
  }

  const desiredIndex = supportedBucketCandidates.findIndex(candidate => candidate === desiredBucket)
  const startIndex = desiredIndex >= 0 ? desiredIndex : supportedBucketCandidates.length - 1
  for (let index = startIndex; index < supportedBucketCandidates.length; index += 1) {
    const candidate = supportedBucketCandidates[index]
    if (retentionSecondsForBucket(candidate) >= normalizedWindow) {
      return candidate
    }
  }
  return supportedBucketCandidates[supportedBucketCandidates.length - 1]
}

function retentionSecondsForBucket(bucketSeconds: number): number {
  if (bucketSeconds < 60) {
    return Math.max(1, overview.value.storage.highResKeepHours) * 60 * 60
  }
  if (bucketSeconds < 1800) {
    return Math.max(1, overview.value.storage.primaryKeepHours) * 60 * 60
  }
  return Math.max(1, overview.value.storage.archiveKeepDays) * 24 * 60 * 60
}

function clampViewEnd(windowSeconds: number, bucketSeconds: number, candidateEndSeconds: number): number {
  const now = currentNowSec()
  const retentionSeconds = retentionSecondsForBucket(bucketSeconds)
  const minEndSeconds = retentionSeconds > windowSeconds
    ? now - retentionSeconds + windowSeconds
    : now
  return Math.round(clamp(candidateEndSeconds, minEndSeconds, now))
}

function normalizedViewEnd(): number {
  return viewEndSec.value > 0 ? viewEndSec.value : currentNowSec()
}

function currentNowSec(): number {
  return Math.floor(Date.now() / 1000)
}

function findPresetWindow(windowSeconds: number): number {
  return visibleRangeOptions.value.find(item => item.windowSec === windowSeconds)?.windowSec ?? 0
}

function formatDurationLabel(seconds: number): string {
  if (seconds % 86400 === 0) return `${seconds / 86400} 天`
  if (seconds % 3600 === 0) return `${seconds / 3600} 小时`
  if (seconds % 60 === 0) return `${seconds / 60} 分钟`
  return `${seconds} 秒`
}

function formatWindowLabel(seconds: number): string {
  if (seconds % (24 * 60 * 60) === 0) return `${seconds / (24 * 60 * 60)} 天`
  if (seconds % (60 * 60) === 0) return `${seconds / (60 * 60)} 小时`
  return `${Math.round(seconds / 60)} 分钟`
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value))
}
</script>

<style scoped>
.monitor-page {
  --monitor-border: rgba(94, 234, 212, 0.18);
  --monitor-surface: linear-gradient(180deg, rgba(15, 23, 42, 0.96), rgba(2, 8, 23, 0.94));
}

.monitor-hero {
  overflow: hidden;
  border: 1px solid rgba(94, 234, 212, 0.22);
  background:
    radial-gradient(circle at top right, rgba(34, 211, 238, 0.18), transparent 34%),
    radial-gradient(circle at left bottom, rgba(14, 165, 233, 0.14), transparent 38%),
    linear-gradient(135deg, rgba(10, 20, 38, 0.96), rgba(4, 11, 24, 0.98));
}

.monitor-hero__content {
  display: grid;
  gap: 18px;
  padding: 22px;
}

.monitor-eyebrow {
  font-size: 12px;
  letter-spacing: 0.22em;
  text-transform: uppercase;
  color: rgba(125, 211, 252, 0.82);
}

.monitor-title-row {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  margin-top: 8px;
}

.monitor-title {
  margin: 0;
  font-size: clamp(28px, 3vw, 34px);
  line-height: 1.05;
  letter-spacing: 0.01em;
  font-weight: 800;
}

.monitor-subtitle {
  margin: 10px 0 0;
  max-width: 960px;
  color: rgba(226, 232, 240, 0.78);
  line-height: 1.7;
}

.monitor-hero__meta {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

.monitor-meta-pill {
  border: 1px solid rgba(148, 163, 184, 0.16);
  background: rgba(8, 15, 28, 0.72);
  border-radius: 18px;
  padding: 14px 16px;
  min-height: 80px;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
}

.monitor-meta-pill span {
  color: rgba(148, 163, 184, 0.82);
  font-size: 12px;
}

.monitor-meta-pill strong {
  color: #f8fafc;
  font-size: 15px;
  font-weight: 700;
}

.monitor-card-title,
.monitor-side-card__title {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
  flex-wrap: wrap;
}

.monitor-config-card,
.monitor-chart-card,
.monitor-side-card {
  border-color: rgba(148, 163, 184, 0.14);
  background: linear-gradient(180deg, rgba(6, 14, 27, 0.96), rgba(3, 9, 18, 0.96));
}

.monitor-config-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.monitor-config-form-row {
  row-gap: 6px;
}

.monitor-config-group {
  display: grid;
  grid-template-rows: auto auto auto;
  gap: 10px;
  min-height: 122px;
}

.monitor-config-group__label {
  font-size: 13px;
  color: rgba(226, 232, 240, 0.92);
  font-weight: 600;
  line-height: 1.2;
}

.monitor-config-group__hint {
  min-height: 22px;
  color: rgba(148, 163, 184, 0.82);
  font-size: 12px;
  line-height: 1.5;
}

.monitor-config-field-with-unit {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 96px;
  gap: 10px;
  align-items: stretch;
}

.monitor-config-unit-select {
  min-width: 96px;
}

.monitor-config-field-with-unit--single {
  grid-template-columns: minmax(0, 1fr);
}

.monitor-chart-card__eyebrow {
  font-size: 11px;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  color: rgba(94, 234, 212, 0.76);
}

.monitor-chart-card__heading {
  margin-top: 6px;
  font-size: 18px;
  font-weight: 700;
}

.monitor-config-note {
  margin-top: 8px;
  color: rgba(148, 163, 184, 0.86);
  line-height: 1.7;
}

.monitor-storage-guide {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
  margin-top: 16px;
}

.monitor-storage-guide__item {
  border: 1px solid rgba(148, 163, 184, 0.14);
  border-radius: 18px;
  padding: 16px;
  background: rgba(8, 15, 28, 0.64);
}

.monitor-storage-guide__label {
  font-size: 12px;
  letter-spacing: 0.16em;
  text-transform: uppercase;
  color: rgba(125, 211, 252, 0.82);
}

.monitor-storage-guide__item strong {
  display: block;
  margin-top: 10px;
  color: #f8fafc;
  font-size: 15px;
  font-weight: 700;
}

.monitor-storage-guide__item p {
  margin: 10px 0 0;
  color: rgba(203, 213, 225, 0.76);
  line-height: 1.7;
}

.monitor-danger-box {
  margin-top: 16px;
  border: 1px solid rgba(248, 113, 113, 0.18);
  border-radius: 20px;
  padding: 16px;
  background:
    radial-gradient(circle at right top, rgba(248, 113, 113, 0.10), transparent 42%),
    rgba(22, 10, 14, 0.64);
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
}

.monitor-danger-box__text {
  margin: 10px 0 0;
  color: rgba(254, 226, 226, 0.82);
  line-height: 1.7;
}

.monitor-range-row,
.monitor-granularity-row {
  row-gap: 12px;
}

.monitor-range-switch {
  display: flex;
  width: 100%;
  flex-wrap: wrap;
  gap: 8px;
  padding: 8px;
  border: 1px solid rgba(148, 163, 184, 0.14);
  border-radius: 18px;
  background: rgba(2, 8, 23, 0.62);
}

.monitor-range-switch__item {
  border: 0;
  outline: 0;
  cursor: pointer;
  border-radius: 12px;
  padding: 10px 14px;
  min-width: 82px;
  background: transparent;
  color: rgba(148, 163, 184, 0.92);
  transition: background-color 0.18s ease, color 0.18s ease, transform 0.18s ease;
}

.monitor-range-switch__item:hover {
  background: rgba(30, 41, 59, 0.72);
  color: #f8fafc;
}

.monitor-range-switch__item--active {
  background: linear-gradient(135deg, rgba(14, 165, 233, 0.92), rgba(45, 212, 191, 0.88));
  color: #04111f;
  font-weight: 800;
  transform: translateY(-1px);
}

.monitor-custom-range {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 120px 90px;
  gap: 10px;
}

.monitor-toolbar-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  justify-content: flex-start;
  width: 100%;
}

.monitor-toolbar-actions :deep(.v-btn) {
  min-width: 92px;
}

.monitor-granularity-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
  padding: 12px 14px;
  border: 1px solid rgba(148, 163, 184, 0.14);
  border-radius: 18px;
  background: rgba(2, 8, 23, 0.62);
}

.monitor-granularity-toolbar__group {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.monitor-granularity-toolbar__label {
  color: rgba(148, 163, 184, 0.82);
  font-size: 13px;
}

.monitor-granularity-toolbar__meta {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
  color: rgba(148, 163, 184, 0.82);
  font-size: 12px;
}

.monitor-granularity-chip {
  border: 1px solid rgba(148, 163, 184, 0.16);
  background: rgba(15, 23, 42, 0.74);
  color: rgba(203, 213, 225, 0.92);
  border-radius: 999px;
  padding: 8px 14px;
  cursor: pointer;
  transition: background-color 0.18s ease, color 0.18s ease, border-color 0.18s ease;
}

.monitor-granularity-chip--active {
  border-color: rgba(45, 212, 191, 0.58);
  background: linear-gradient(135deg, rgba(14, 165, 233, 0.92), rgba(45, 212, 191, 0.88));
  color: #04111f;
  font-weight: 800;
}

.monitor-granularity-chip--disabled {
  opacity: 0.38;
  cursor: not-allowed;
}

.monitor-wheel-guide {
  border-radius: 18px;
  border: 1px solid rgba(148, 163, 184, 0.14);
  background: rgba(8, 15, 28, 0.72);
  padding: 14px;
}

.monitor-wheel-guide--active {
  border-color: rgba(45, 212, 191, 0.34);
  box-shadow: inset 0 0 0 1px rgba(45, 212, 191, 0.14);
}

.monitor-wheel-guide__title {
  color: #f8fafc;
  font-weight: 700;
  font-size: 14px;
}

.monitor-wheel-guide__text {
  margin-top: 8px;
  color: rgba(148, 163, 184, 0.82);
  font-size: 13px;
  line-height: 1.7;
}

.metric-card {
  height: 100%;
  border-color: var(--monitor-border);
  background: var(--monitor-surface);
}

.metric-card--cpu { box-shadow: inset 0 0 0 1px rgba(245, 158, 11, 0.08); }
.metric-card--memory { box-shadow: inset 0 0 0 1px rgba(56, 189, 248, 0.08); }
.metric-card--read { box-shadow: inset 0 0 0 1px rgba(34, 197, 94, 0.08); }
.metric-card--write { box-shadow: inset 0 0 0 1px rgba(168, 85, 247, 0.08); }
.metric-card--network-down { box-shadow: inset 0 0 0 1px rgba(14, 165, 233, 0.08); }
.metric-card--network-up { box-shadow: inset 0 0 0 1px rgba(251, 113, 133, 0.08); }

.metric-card__label {
  font-size: 13px;
  text-transform: uppercase;
  letter-spacing: 0.14em;
  color: rgba(148, 163, 184, 0.8);
}

.metric-card__value {
  margin-top: 18px;
  font-size: clamp(28px, 2.2vw, 36px);
  font-weight: 800;
  line-height: 1;
  color: #f8fafc;
}

.metric-card__meta {
  margin-top: 12px;
  color: rgba(203, 213, 225, 0.72);
  font-size: 13px;
}

.metric-card__divider {
  display: inline-block;
  margin: 0 6px;
  color: rgba(148, 163, 184, 0.6);
}

.monitor-side-card__timestamp,
.monitor-chart-card__caption {
  color: rgba(148, 163, 184, 0.76);
  font-size: 13px;
}

.monitor-side-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.monitor-side-grid__item {
  border: 1px solid rgba(148, 163, 184, 0.12);
  border-radius: 14px;
  padding: 12px 14px;
  background: rgba(8, 15, 28, 0.72);
}

.monitor-side-grid__item span {
  display: block;
  color: rgba(148, 163, 184, 0.82);
  font-size: 12px;
}

.monitor-side-grid__item strong {
  display: block;
  margin-top: 8px;
  color: #f8fafc;
  font-size: 15px;
  font-weight: 700;
  word-break: break-all;
}

.monitor-interface-box {
  border-radius: 20px;
  border: 1px solid rgba(94, 234, 212, 0.12);
  background:
    radial-gradient(circle at right top, rgba(20, 184, 166, 0.10), transparent 42%),
    rgba(7, 14, 26, 0.82);
  padding: 14px;
}

.monitor-retention-box__title {
  font-size: 13px;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: rgba(125, 211, 252, 0.82);
}

.monitor-interface-list {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 12px;
}

.monitor-interface-empty {
  margin-top: 12px;
  color: rgba(148, 163, 184, 0.82);
  line-height: 1.7;
}

.monitor-chart-shell {
  position: relative;
  border-radius: 20px;
  border: 1px solid rgba(148, 163, 184, 0.08);
  padding: 10px;
  cursor: grab;
  user-select: none;
  transition: border-color 0.18s ease, box-shadow 0.18s ease;
}

.monitor-chart-shell--active {
  border-color: rgba(45, 212, 191, 0.30);
  box-shadow: inset 0 0 0 1px rgba(45, 212, 191, 0.12);
  cursor: grabbing;
}

.monitor-chart-shell__hint {
  position: absolute;
  top: 10px;
  right: 12px;
  z-index: 2;
  pointer-events: none;
  color: rgba(148, 163, 184, 0.82);
  font-size: 12px;
  background: rgba(2, 8, 23, 0.72);
  border-radius: 999px;
  padding: 6px 10px;
}

.monitor-chart-card__canvas {
  min-height: 320px;
}

.monitor-empty {
  min-height: 320px;
  border: 1px dashed rgba(148, 163, 184, 0.2);
  border-radius: 18px;
  display: flex;
  align-items: center;
  justify-content: center;
  text-align: center;
  color: rgba(148, 163, 184, 0.82);
  padding: 28px;
  line-height: 1.8;
}

.summary-row {
  padding: 14px 0;
  border-bottom: 1px solid rgba(148, 163, 184, 0.12);
}

.summary-row--last {
  border-bottom: 0;
}

.summary-row__label {
  font-size: 12px;
  letter-spacing: 0.16em;
  text-transform: uppercase;
  color: rgba(94, 234, 212, 0.78);
  margin-bottom: 12px;
}

.summary-row__stats {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
}

.summary-row__stats div {
  border: 1px solid rgba(148, 163, 184, 0.12);
  background: rgba(8, 15, 28, 0.68);
  border-radius: 14px;
  padding: 12px;
}

.summary-row__stats span {
  display: block;
  color: rgba(148, 163, 184, 0.82);
  font-size: 12px;
}

.summary-row__stats strong {
  display: block;
  margin-top: 10px;
  color: #f8fafc;
  font-size: 14px;
  font-weight: 700;
}

@media (max-width: 1264px) {
  .monitor-hero__meta {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .monitor-storage-guide {
    grid-template-columns: 1fr;
  }

  .monitor-toolbar-actions {
    justify-content: flex-start;
  }
}

@media (max-width: 960px) {
  .monitor-config-field-with-unit,
  .monitor-custom-range {
    grid-template-columns: 1fr;
  }

  .monitor-toolbar-actions :deep(.v-btn) {
    flex: 1 1 calc(50% - 4px);
  }
}

@media (max-width: 760px) {
  .monitor-hero__content {
    padding: 18px;
  }

  .monitor-hero__meta,
  .monitor-side-grid,
  .summary-row__stats {
    grid-template-columns: 1fr;
  }

  .monitor-danger-box {
    flex-direction: column;
    align-items: stretch;
  }

  .monitor-range-switch {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    width: 100%;
  }

  .monitor-range-switch__item {
    min-width: 0;
  }

  .monitor-granularity-toolbar {
    align-items: flex-start;
  }

  .monitor-toolbar-actions {
    justify-content: flex-start;
  }

  .monitor-toolbar-actions :deep(.v-btn) {
    flex: 1 1 100%;
  }
}
</style>
