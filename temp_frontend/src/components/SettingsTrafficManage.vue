<template>
  <div class="settings-traffic-manage">
    <!-- 顶部全局加载/错误警报 -->
    <v-alert
      v-if="loadError"
      type="error"
      variant="tonal"
      density="comfortable"
      class="mb-4"
    >
      <div class="d-flex align-center justify-space-between flex-wrap ga-3">
        <span>{{ loadError }}</span>
        <v-btn
          variant="outlined"
          prepend-icon="mdi-refresh"
          :loading="loading"
          @click="fetchOverview(false)"
        >
          重新加载
        </v-btn>
      </div>
    </v-alert>

    <v-alert
      v-else-if="!hasLoaded"
      type="info"
      variant="tonal"
      density="comfortable"
      class="mb-4"
    >
      <div class="d-flex align-center ga-2">
        <v-progress-circular indeterminate size="18" width="2" />
        <span>正在读取流量概览及运行状态...</span>
      </div>
    </v-alert>

    <v-alert
      v-if="overview.error"
      type="warning"
      variant="tonal"
      density="comfortable"
      class="mb-4"
    >
      {{ overview.error }}
    </v-alert>

    <!-- Card 1: 流量统计与运行概览 -->
    <v-card rounded="xl" variant="outlined" class="mb-4 card-cyan" :loading="loading && !hasLoaded">
      <v-card-title class="d-flex align-center justify-space-between py-3 px-4 flex-wrap ga-2">
        <div class="text-subtitle-1 font-weight-medium d-flex align-center ga-2">
          <v-icon size="small" color="primary">mdi-chart-line</v-icon>
          <span>流量统计与运行概览</span>
        </div>
        <div class="d-flex align-center flex-wrap ga-3">
          <v-switch
            v-model="enabledInput"
            color="success"
            density="compact"
            hide-details
            inset
            label="流量统计"
            :loading="togglingTraffic"
            :disabled="trafficOperationBusy"
            @update:model-value="onTrafficEnabledChanged"
          />
          <v-chip
            size="small"
            :color="statusColor"
            variant="flat"
            class="traffic-status-chip"
            :class="statusChipClass"
          >
            {{ statusLabel }}
          </v-chip>
        </div>
      </v-card-title>
      <v-divider />
      <v-card-text class="pt-4">
        <!-- 运行基础信息条 -->
        <div class="d-flex align-center justify-space-between flex-wrap ga-2 mb-4 text-caption text-medium-emphasis">
          <div class="d-flex align-center flex-wrap ga-2">
            <span>网卡接口: <strong>{{ overview.interface || '-' }}</strong></span>
            <span class="mx-1">|</span>
            <span>统计数据源: <strong>{{ overview.source || 'vnstat' }}</strong></span>
            <span class="mx-1">|</span>
            <span>更新时间: <strong>{{ updatedAtLabel }}</strong></span>
          </div>
          <v-btn
            size="small"
            variant="text"
            prepend-icon="mdi-refresh"
            :loading="loading"
            :disabled="trafficOperationBusy"
            @click="fetchOverview(false)"
          >
            立即刷新
          </v-btn>
        </div>

        <!-- 3 大核心指标卡 (PC 端 3 列，移动端单列) -->
        <v-row class="mb-2">
          <v-col cols="12" sm="4">
            <v-card variant="outlined" class="metric-card pa-3">
              <div class="text-caption text-medium-emphasis">{{ t('stats.volume') || '周期总用量' }}</div>
              <div class="text-h6 font-weight-bold mt-1">{{ periodTotalText }}</div>
              <div class="accum-badge mt-2">
                <span class="accum-badge__label">历史累计</span>
                <span class="font-weight-medium">{{ accumTotalText }}</span>
              </div>
            </v-card>
          </v-col>
          <v-col cols="12" sm="4">
            <v-card variant="outlined" class="metric-card pa-3">
              <div class="text-caption text-medium-emphasis">{{ t('stats.upload') || '周期上传' }}</div>
              <div class="text-h6 font-weight-bold mt-1 text-orange">{{ periodUpText }}</div>
              <div class="accum-badge mt-2">
                <span class="accum-badge__label">历史累计</span>
                <span class="font-weight-medium">{{ accumUpText }}</span>
              </div>
            </v-card>
          </v-col>
          <v-col cols="12" sm="4">
            <v-card variant="outlined" class="metric-card pa-3">
              <div class="text-caption text-medium-emphasis">{{ t('stats.download') || '周期下载' }}</div>
              <div class="text-h6 font-weight-bold mt-1 text-success">{{ periodDownText }}</div>
              <div class="accum-badge mt-2">
                <span class="accum-badge__label">历史累计</span>
                <span class="font-weight-medium">{{ accumDownText }}</span>
              </div>
            </v-card>
          </v-col>
        </v-row>

        <!-- 用量概览条 -->
        <div class="mt-4 pt-3 border-t">
          <div class="d-flex justify-space-between align-center mb-1 flex-wrap ga-2">
            <span class="text-body-2 font-weight-medium">周期配额用量</span>
            <span class="text-caption text-medium-emphasis">{{ usageText }}</span>
          </div>
          <v-progress-linear
            :model-value="limitBytes > 0 ? usagePercent : 0"
            :color="limitBytes > 0 ? usageColor : 'medium-emphasis'"
            rounded
            height="8"
          />
        </div>
      </v-card-text>
    </v-card>

    <!-- Card 2: 流量配额与周期计划 -->
    <v-card rounded="xl" variant="outlined" class="mb-4 card-green">
      <v-card-title class="d-flex align-center justify-space-between py-3 px-4 flex-wrap ga-2">
        <div class="text-subtitle-1 font-weight-medium d-flex align-center ga-2">
          <v-icon size="small" color="primary">mdi-calendar-clock</v-icon>
          <span>流量配额与重置计划</span>
        </div>
        <v-btn
          color="primary"
          variant="tonal"
          prepend-icon="mdi-content-save-outline"
          :loading="savingSettings"
          :disabled="trafficOperationBusy || !hasPendingSettingsChanges"
          @click="saveTrafficSettings"
        >
          保存配额设置
        </v-btn>
      </v-card-title>
      <v-divider />
      <v-card-text class="pt-4">
        <v-row>
          <v-col cols="12" sm="6" md="4">
            <v-text-field
              v-model.number="limitGiBInput"
              type="number"
              min="0"
              step="0.01"
              :label="`${t('stats.volume')} 限额 (GB)`"
              density="comfortable"
              prepend-inner-icon="mdi-speedometer"
              placeholder="0 代表不限制"
              :disabled="trafficOperationBusy"
              hide-details
            />
          </v-col>
          <v-col cols="12" sm="6" md="4">
            <v-select
              v-model="resetDayInput"
              :items="resetDayOptions"
              item-title="title"
              item-value="value"
              :label="resetDayLabel"
              density="comfortable"
              prepend-inner-icon="mdi-calendar-repeat"
              :disabled="trafficOperationBusy"
              hide-details
            />
          </v-col>
          <v-col cols="12" sm="12" md="4">
            <DatePick
              :expiry="expiryPickerEpoch"
              input-id="traffic-expiry-date-picker"
              picker-type="date"
              :label-text="expiryDateLabel"
              :zero-text="disabledLabel"
              :disabled="trafficOperationBusy"
              @submit="onSubmitExpiryDatePicker"
            />
          </v-col>
        </v-row>

        <!-- 计划说明与状态提示 -->
        <div class="mt-3 text-caption text-medium-emphasis">
          <div>• {{ monthlyHint }}</div>
          <div class="mt-1">• {{ expiryHint }}</div>
        </div>

        <div class="d-flex flex-wrap ga-4 mt-3 pt-2 text-caption">
          <div>{{ resetDayLabel }}: <strong>{{ resetDayInput > 0 ? `${resetDayInput} ${daySuffix}` : disabledLabel }}</strong></div>
          <div>{{ nextResetLabel }}: <strong>{{ nextResetAtLabel }}</strong></div>
          <div>{{ expiryDateLabel }}: <strong>{{ expiryStatusLabel }}</strong></div>
        </div>

        <!-- 配额用量进度 (旧版核心进度条，常驻展示) -->
        <div class="quota-progress-box mt-4 pt-3 border-t">
          <div class="d-flex justify-space-between align-center mb-1 flex-wrap ga-2">
            <div class="d-flex align-center ga-2">
              <v-icon size="small" :color="limitBytes > 0 ? usageColor : 'primary'">mdi-chart-bell-curve-cumulative</v-icon>
              <span class="text-body-2 font-weight-medium">配额用量进度</span>
              <v-chip
                v-if="limitBytes > 0"
                size="x-small"
                :color="usageColor"
                variant="tonal"
                class="font-weight-medium"
              >
                {{ usagePercent }}%
              </v-chip>
              <v-chip
                v-else
                size="x-small"
                variant="outlined"
                color="medium-emphasis"
              >
                未设限额
              </v-chip>
            </div>
            <span
              class="text-caption font-weight-bold"
              :class="limitBytes > 0 ? `text-${usageColor}` : 'text-medium-emphasis'"
            >
              {{ usageText }}
            </span>
          </div>
          <v-progress-linear
            :model-value="limitBytes > 0 ? usagePercent : 0"
            :color="limitBytes > 0 ? usageColor : 'grey'"
            rounded
            height="12"
            :striped="limitBytes > 0 && usagePercent >= 90"
            class="mt-1"
          />
        </div>

        <!-- 危险重置操作区分隔区 -->
        <div class="d-flex flex-wrap align-center ga-3 mt-4 pt-3 border-t">
          <v-btn
            color="warning"
            variant="tonal"
            prepend-icon="mdi-restart"
            :loading="resettingPeriod"
            :disabled="trafficOperationBusy"
            @click="confirmResetPeriodTraffic"
          >
            重置当期流量
          </v-btn>
          <v-btn
            color="error"
            variant="outlined"
            prepend-icon="mdi-delete-clock-outline"
            :loading="resettingTotal"
            :disabled="trafficOperationBusy"
            @click="confirmResetTotalTraffic"
          >
            重置总流量 (清零历史)
          </v-btn>
        </div>
      </v-card-text>
    </v-card>

    <!-- Card 3: vnStat 引擎管理与系统诊断 -->
    <v-card rounded="xl" variant="outlined" class="card-cyan">
      <v-card-title class="d-flex align-center justify-space-between py-3 px-4 flex-wrap ga-2">
        <div class="text-subtitle-1 font-weight-medium d-flex align-center ga-2">
          <v-icon size="small" color="primary">mdi-server-network</v-icon>
          <span>vnStat 引擎管理与底层状态</span>
        </div>
        <div class="d-flex align-center ga-2">
          <v-chip size="x-small" color="info" variant="outlined">
            当前: {{ vnstatVersionText }}
          </v-chip>
          <v-chip size="x-small" :color="overview.vnstat.managed ? 'success' : 'medium-emphasis'" variant="outlined">
            {{ vnstatOwnershipText }}
          </v-chip>
        </div>
      </v-card-title>
      <v-divider />
      <v-card-text class="pt-4">
        <!-- 运行冲突预警 -->
        <v-alert
          v-if="overview.vnstat.runtimeConflict"
          type="error"
          variant="tonal"
          density="comfortable"
          class="traffic-vnstat-conflict mb-4"
        >
          <div class="font-weight-medium">{{ overview.vnstat.runtimeConflict.message }}</div>
          <div class="mt-2 text-caption">
            <div v-if="overview.vnstat.runtimeConflict.paths.length > 0">
              冲突路径: <code>{{ overview.vnstat.runtimeConflict.paths.join(' / ') }}</code>
            </div>
            <div v-if="overview.vnstat.runtimeConflict.pids.length > 0">
              进程 PID: <code>{{ overview.vnstat.runtimeConflict.pids.join(', ') }}</code>
            </div>
            <div v-if="overview.vnstat.runtimeConflict.units.length > 0">
              已核验服务: <code>{{ overview.vnstat.runtimeConflict.units.join(', ') }}</code>
            </div>
          </div>
        </v-alert>

        <!-- 管理受限提示 -->
        <v-alert
          v-if="overview.vnstat.supported && !overview.vnstat.canManage && overview.vnstat.manageHint"
          type="info"
          variant="tonal"
          density="comfortable"
          class="mb-4"
        >
          {{ overview.vnstat.manageHint }}
        </v-alert>

        <!-- 安装与操作区 (完全适配移动端，流式响应) -->
        <v-row class="align-center mb-2">
          <v-col cols="12" md="5">
            <v-select
              v-model="selectedVnstatVersion"
              :items="vnstatVersionSelectItems"
              item-title="title"
              item-value="value"
              label="安装来源"
              placeholder="请先选择来源"
              :hint="vnstatSourceAvailabilityHint"
              :persistent-hint="vnstatSourceAvailabilityHint !== ''"
              density="comfortable"
              prepend-inner-icon="mdi-source-branch"
              hide-details
              :disabled="trafficOperationBusy || !overview.vnstat.supported || !overview.vnstat.canManage"
              clearable
            />
          </v-col>
          <v-col cols="12" md="7">
            <div class="d-flex flex-wrap align-center ga-2">
              <v-btn
                v-if="hasActiveVnstatInstall"
                color="error"
                prepend-icon="mdi-stop-circle-outline"
                :disabled="vnstatStopRequestPending || !vnstatInstallCanCancel"
                @click="stopVnstatInstall"
              >
                {{ vnstatStopButtonLabel }}
              </v-btn>
              <v-btn
                v-else
                color="primary"
                prepend-icon="mdi-download"
                :disabled="trafficOperationBusy || !overview.vnstat.supported || !overview.vnstat.canManage || !hasSelectedVnstatSource"
                @click="installVnstat"
              >
                {{ vnstatInstallButtonLabel }}
              </v-btn>
              <v-btn
                variant="outlined"
                color="primary"
                prepend-icon="mdi-cloud-search-outline"
                :loading="checkingVnstatUpdate"
                :disabled="trafficOperationBusy || !overview.vnstat.supported || !overview.vnstat.canManage || !hasSelectedVnstatSource"
                @click="checkVnstatUpdate"
              >
                检测更新
              </v-btn>
              <v-btn
                variant="outlined"
                color="error"
                prepend-icon="mdi-delete-outline"
                :loading="removingVnstat"
                :disabled="trafficOperationBusy || !overview.vnstat.supported || !overview.vnstat.canManage || !overview.vnstat.managed"
                @click="removeVnstat"
              >
                卸载删除
              </v-btn>
            </div>
          </v-col>
        </v-row>

        <!-- 任务进行中进度条 -->
        <div v-if="hasActiveVnstatInstall" class="traffic-install-progress my-3 pa-2 rounded bg-surface-variant d-flex align-center ga-2">
          <v-progress-circular indeterminate size="18" width="2" color="primary" />
          <span class="text-caption">{{ vnstatInstallPhase || '正在等待 vnStat 安装任务响应...' }}</span>
        </div>
        <div v-if="hasActiveVnstatRemoval" class="traffic-install-progress my-3 pa-2 rounded bg-surface-variant d-flex align-center ga-2">
          <v-progress-circular indeterminate size="18" width="2" color="error" />
          <span class="text-caption">{{ vnstatRemovalPhase || '正在删除受管 vnStat...' }}</span>
        </div>

        <div class="text-caption text-medium-emphasis mb-3">
          安装操作只会检查正在运行的非面板 vnstatd；外部目录的程序、配置和统计文件会保留。只有面板明确安装并完成凭据核验的 vnStat，才可由面板删除。
        </div>

        <!-- 详细状态表格 -->
        <div class="traffic-runtime__rows mt-4 border rounded">
          <div class="traffic-runtime__row px-3 py-2 border-b d-flex justify-space-between align-center">
            <span class="text-medium-emphasis text-caption">当前版本</span>
            <strong class="text-body-2">{{ vnstatVersionText }}</strong>
          </div>
          <div class="traffic-runtime__row px-3 py-2 border-b d-flex justify-space-between align-center">
            <span class="text-medium-emphasis text-caption">最新版本</span>
            <strong class="text-body-2">{{ vnstatLatestVersionText }}</strong>
          </div>
          <div class="traffic-runtime__row px-3 py-2 border-b d-flex justify-space-between align-center">
            <span class="text-medium-emphasis text-caption">更新状态</span>
            <strong class="text-body-2">{{ vnstatUpdateMessageText }}</strong>
          </div>
          <div class="traffic-runtime__row px-3 py-2 border-b d-flex justify-space-between align-center">
            <span class="text-medium-emphasis text-caption">检测来源</span>
            <strong class="text-body-2">{{ vnstatUpdateSourceText }}</strong>
          </div>
          <div class="traffic-runtime__row px-3 py-2 border-b d-flex justify-space-between align-center">
            <span class="text-medium-emphasis text-caption">系统系列</span>
            <strong class="text-body-2">{{ vnstatSystemPlatformText }}</strong>
          </div>
          <div class="traffic-runtime__row px-3 py-2 border-b d-flex justify-space-between align-center">
            <span class="text-medium-emphasis text-caption">管理状态</span>
            <strong class="text-body-2">{{ vnstatOwnershipText }}</strong>
          </div>
          <div class="traffic-runtime__row px-3 py-2 border-b d-flex justify-space-between align-center">
            <span class="text-medium-emphasis text-caption">安装方式</span>
            <strong class="text-body-2">{{ vnstatInstallMethodText }}</strong>
          </div>
          <div class="traffic-runtime__row px-3 py-2 border-b d-flex justify-space-between align-center">
            <span class="text-medium-emphasis text-caption">包管理器</span>
            <strong class="text-body-2">{{ overview.vnstat.packageManager || '-' }}</strong>
          </div>
          <div class="traffic-runtime__row px-3 py-2 border-b d-flex justify-space-between align-center">
            <span class="text-medium-emphasis text-caption">程序路径</span>
            <code class="text-caption">{{ overview.vnstat.binaryPath || '-' }}</code>
          </div>
          <div class="traffic-runtime__row px-3 py-2 border-b d-flex justify-space-between align-center">
            <span class="text-medium-emphasis text-caption">跟踪文件</span>
            <strong class="text-body-2">{{ overview.vnstat.fileCount > 0 ? `${overview.vnstat.fileCount} 个` : '-' }}</strong>
          </div>
          <div class="traffic-runtime__row px-3 py-2 d-flex justify-space-between align-center">
            <span class="text-medium-emphasis text-caption">数据目录</span>
            <code class="text-caption">{{ vnstatDataPathText }}</code>
          </div>
        </div>
      </v-card-text>
    </v-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import DatePick from '@/components/DateTime.vue'
import HttpUtils, { type Msg } from '@/plugins/httputil'
import { confirm } from '@/plugins/confirm'
import {
  formatPanelDateTime,
  panelCalendarParts,
  panelNowUnix,
} from '@/plugins/panelTime'
import { push } from 'notivue'

import {
  DAY_SUFFIX,
  DISABLED_LABEL,
  DISABLE_TRAFFIC_CONFIRM_TEXT,
  EXPIRY_DATE_LABEL,
  EXPIRY_HINT,
  INSTALL_VNSTAT_CONFIRM_TEXT,
  MONTHLY_HINT,
  NEXT_RESET_LABEL,
  REMOVE_VNSTAT_CONFIRM_TEXT,
  RESET_DAY_LABEL,
  RESET_PERIOD_CONFIRM_TEXT,
  RESET_TOTAL_CONFIRM_TEXT,
  SELECT_VNSTAT_SOURCE_HINT,
  TrafficOverview,
  TrafficOverviewRaw,
  VnstatInstallJob,
  VnstatRemovalJob,
  VnstatStatus,
  VnstatUpdateInfo,
  VnstatVersionItem,
  buildPickerEpochFromExpiryDate,
  createDefaultOverview,
  createIdleVnstatUpdateInfo,
  formatGB,
  getNextResetAt,
  normalizeExpiryDateInput,
  normalizeLimitGiB,
  normalizeResetDay,
  normalizeVnstatInstallJob,
  normalizeVnstatRemovalJob,
  normalizeVnstatStatus,
  normalizeVnstatUpdateInfo,
  normalizeVnstatVersionItems,
  parseEpochSeconds,
  readBoolField,
  readNumberField,
  readStringField,
} from './SettingsTrafficManage.shared'

const props = withDefaults(defineProps<{
  active?: boolean
}>(), {
  active: false,
})

const { t } = useI18n()

// 模板常量
const resetDayLabel = RESET_DAY_LABEL
const expiryDateLabel = EXPIRY_DATE_LABEL
const disabledLabel = DISABLED_LABEL
const daySuffix = DAY_SUFFIX
const monthlyHint = MONTHLY_HINT
const expiryHint = EXPIRY_HINT
const nextResetLabel = NEXT_RESET_LABEL

// 状态管理
const loading = ref(false)
const hasLoaded = ref(false)
const loadError = ref('')
const savingSettings = ref(false)
const resettingPeriod = ref(false)
const resettingTotal = ref(false)
const trafficActionConfirming = ref(false)
const togglingTraffic = ref(false)
const installingVnstat = ref(false)
const vnstatInstallJobId = ref('')
const vnstatInstallPhase = ref('')
const vnstatInstallState = ref('idle')
const vnstatInstallCanCancel = ref(false)
const vnstatStopRequestPending = ref(false)
const vnstatInstallBeforeVersion = ref('')
const removingVnstat = ref(false)
const vnstatRemovalJobId = ref('')
const vnstatRemovalPhase = ref('')
const vnstatRemovalState = ref('idle')
const checkingVnstatUpdate = ref(false)
const overviewRequest = ref<Promise<Msg> | null>(null)
const enabledInput = ref(true)
const selectedVnstatVersion = ref('')
const vnstatVersionItems = ref<VnstatVersionItem[]>([
  {
    value: 'system-package',
    title: '系统软件源',
    description: '使用当前系统的软件包管理器安装或重装 vnstat',
    available: false,
    reason: '正在检测系统软件源可用性',
  },
  {
    value: 'github-release',
    title: 'GitHub 官方源码包',
    description: '从 GitHub 官方 release 源码包编译安装或重装 vnstat',
    available: false,
    reason: '正在检测安装环境',
  },
])

const overview = ref<TrafficOverview>(createDefaultOverview())
const vnstatUpdateInfo = ref<VnstatUpdateInfo>(createIdleVnstatUpdateInfo())

const limitGiBInput = ref(0)
const resetDayInput = ref(0)
const expiryDateInput = ref('')
const expiryPickerEpoch = ref(0)
const savedLimitGiB = ref(0)
const savedResetDay = ref(0)
const savedExpiryDate = ref('')
let pollingTimer: number | null = null
let vnstatInstallPollingTimer: number | null = null
let vnstatRemovalPollingTimer: number | null = null
let overviewAbortController: AbortController | null = null
let overviewRequestGeneration = 0
let vnstatVersionOptionsGeneration = 0

// 计算属性
const limitBytes = computed(() => (
  limitGiBInput.value > 0 ? limitGiBInput.value * 1024 * 1024 * 1024 : 0
))

const hasPendingSettingsChanges = computed(() => (
  normalizeLimitGiB(limitGiBInput.value) !== savedLimitGiB.value ||
  normalizeResetDay(resetDayInput.value) !== savedResetDay.value ||
  normalizeExpiryDateInput(expiryDateInput.value) !== savedExpiryDate.value
))

const hasPendingResetDayChanges = computed(() => (
  normalizeResetDay(resetDayInput.value) !== savedResetDay.value
))

const hasPendingExpiryDateChanges = computed(() => (
  normalizeExpiryDateInput(expiryDateInput.value) !== savedExpiryDate.value
))

const currentPeriodUsageBytes = computed(() => overview.value.total)
const usagePercent = computed(() => (
  limitBytes.value > 0 ? Math.min(100, Math.round(currentPeriodUsageBytes.value * 100 / limitBytes.value)) : 0
))

const usageColor = computed(() => (
  usagePercent.value >= 100 ? 'error' : usagePercent.value >= 90 ? 'warning' : 'success'
))

const usageText = computed(() => {
  if (limitBytes.value <= 0) {
    return `${formatGB(currentPeriodUsageBytes.value)} / 不限配额`
  }
  return `${formatGB(currentPeriodUsageBytes.value)} / ${formatGB(limitBytes.value)} (${usagePercent.value}%)`
})

const periodUpText = computed(() => formatGB(overview.value.up))
const periodDownText = computed(() => formatGB(overview.value.down))
const periodTotalText = computed(() => formatGB(overview.value.total))
const accumUpText = computed(() => formatGB(overview.value.accumUp))
const accumDownText = computed(() => formatGB(overview.value.accumDown))
const accumTotalText = computed(() => formatGB(overview.value.accumTotal))
const updatedAtLabel = computed(() => (
  overview.value.updatedAt > 0 ? formatPanelDateTime(overview.value.updatedAt * 1000) : '-'
))

const statusLabel = computed(() => {
  if (!hasLoaded.value) return '加载中'
  if (installingVnstat.value) return '安装中'
  if (!overview.value.enabled) return '已暂停'
  if (overview.value.available) return '已运行'
  if (!overview.value.vnstat.installed) return '未安装'
  return '已停止'
})

const statusColor = computed(() => {
  if (!hasLoaded.value) return 'info'
  if (installingVnstat.value) return 'info'
  if (!overview.value.enabled) return 'warning'
  if (overview.value.available) return 'success'
  if (!overview.value.vnstat.installed) return 'error'
  return 'warning'
})

const statusChipClass = computed(() => {
  if (!hasLoaded.value) return 'traffic-status-chip--loading'
  if (installingVnstat.value) return 'traffic-status-chip--installing'
  if (!overview.value.enabled) return 'traffic-status-chip--paused'
  if (overview.value.available) return 'traffic-status-chip--running'
  if (!overview.value.vnstat.installed) return 'traffic-status-chip--uninstalled'
  return 'traffic-status-chip--stopped'
})

const selectedVnstatSourceOption = computed(() => (
  vnstatVersionItems.value.find(item => item.value === selectedVnstatVersion.value.trim())
))
const hasSelectedVnstatSource = computed(() => selectedVnstatSourceOption.value?.available === true)

const vnstatVersionSelectItems = computed(() => (
  vnstatVersionItems.value.map(item => ({
    ...item,
    props: {
      disabled: !item.available,
      title: item.available ? item.description : (item.reason || item.description),
    },
  }))
))

const vnstatSourceAvailabilityHint = computed(() => (
  vnstatVersionItems.value
    .filter(item => !item.available && item.reason.trim() !== '')
    .map(item => `${item.title}：${item.reason}`)
    .join('；')
))

const vnstatInstallButtonLabel = computed(() => (
  overview.value.vnstat.installed ? '下载 / 重装' : '下载 / 安装'
))

const hasActiveVnstatInstall = computed(() => (
  installingVnstat.value && ['queued', 'running', 'stopping'].includes(vnstatInstallState.value)
))

const hasActiveVnstatRemoval = computed(() => (
  removingVnstat.value && ['queued', 'running'].includes(vnstatRemovalState.value)
))

const trafficOperationBusy = computed(() => (
  !hasLoaded.value
  || loading.value
  || savingSettings.value
  || resettingPeriod.value
  || resettingTotal.value
  || trafficActionConfirming.value
  || togglingTraffic.value
  || installingVnstat.value
  || removingVnstat.value
  || checkingVnstatUpdate.value
))

const resetDayOptions = computed(() => [
  { title: DISABLED_LABEL, value: 0 },
  ...Array.from({ length: 31 }, (_, index) => ({
    title: `${index + 1} ${DAY_SUFFIX}`,
    value: index + 1,
  })),
])

const vnstatStopButtonLabel = computed(() => {
  if (vnstatStopRequestPending.value || vnstatInstallState.value === 'stopping') return '正在停止'
  return vnstatInstallCanCancel.value ? '停止' : '正在应用'
})

const vnstatVersionText = computed(() => overview.value.vnstat.version || '-')
const vnstatLatestVersionText = computed(() => vnstatUpdateInfo.value.latestVersion || '-')
const vnstatUpdateMessageText = computed(() => {
  const message = vnstatUpdateInfo.value.message.trim()
  if (message !== '') return message
  return overview.value.vnstat.installed ? '未检测更新' : '未安装'
})

const vnstatUpdateSourceText = computed(() => {
  const source = vnstatUpdateInfo.value.source.trim().toLowerCase()
  if (source === '') return '-'
  if (source === 'github-release') return 'GitHub 官方版本'
  if (source === 'system-package') return '系统软件源'
  return source
})

const vnstatInstallMethodText = computed(() => {
  const method = overview.value.vnstat.installMethod.trim().toLowerCase()
  if (method === 'system-package') {
    return overview.value.vnstat.packageManager
      ? `系统软件源 (${overview.value.vnstat.packageManager})`
      : '系统软件源'
  }
  if (method === 'github-release') {
    return 'GitHub 官方源码包'
  }
  return overview.value.vnstat.packageManager || '-'
})

const vnstatSystemPlatformText = computed(() => {
  const family = overview.value.vnstat.systemFamily.trim()
  const systemId = overview.value.vnstat.systemId.trim()
  const version = overview.value.vnstat.systemVersion.trim()
  if (family === '') return '-'
  if (systemId === '') return family
  return `${family}（${systemId}${version}）`
})

const vnstatOwnershipText = computed(() => {
  const ownershipState = overview.value.vnstat.ownershipState.trim().toLowerCase()
  if (ownershipState !== 'managed' || !overview.value.vnstat.managed) {
    return overview.value.vnstat.ownershipHint.trim() !== ''
      ? `未由本面板安装（${overview.value.vnstat.ownershipHint.trim()}）`
      : '未由本面板安装'
  }
  return overview.value.vnstat.ownership.trim().toLowerCase() === 'panel-installed'
    ? '面板安装'
    : '面板受管'
})

const vnstatDataPathText = computed(() => (
  overview.value.vnstat.dataPaths.length > 0 ? overview.value.vnstat.dataPaths.join(' / ') : '-'
))

const draftNextResetAt = computed(() => getNextResetAt(resetDayInput.value))
const displayNextResetAt = computed(() => (
  hasPendingResetDayChanges.value
    ? draftNextResetAt.value
    : overview.value.nextResetAt > 0
      ? new Date(overview.value.nextResetAt * 1000)
      : null
))

const nextResetAtLabel = computed(() => {
  const date = displayNextResetAt.value
  if (date == null) return DISABLED_LABEL
  return formatPanelDateTime(date)
})

const expiryDateDisplay = computed(() => (
  normalizeExpiryDateInput(expiryDateInput.value)
))

const expiryStatusLabel = computed(() => {
  if (expiryDateDisplay.value === '') return DISABLED_LABEL
  if (overview.value.expired && !hasPendingExpiryDateChanges.value) {
    return `${expiryDateDisplay.value} (已到期)`
  }
  return `${expiryDateDisplay.value} (已生效)`
})

// 日期选择事件
const onSubmitExpiryDatePicker = (rawValue: unknown) => {
  const epochSeconds = parseEpochSeconds(rawValue)
  if (epochSeconds == null) return
  if (epochSeconds <= 0) {
    expiryDateInput.value = ''
    expiryPickerEpoch.value = 0
    return
  }
  const selected = panelCalendarParts(epochSeconds * 1000)
  const year = selected.year.toString().padStart(4, '0')
  const month = selected.month.toString().padStart(2, '0')
  const day = selected.day.toString().padStart(2, '0')
  expiryDateInput.value = `${year}-${month}-${day}`
  expiryPickerEpoch.value = epochSeconds
}

type ApplyOverviewOptions = {
  forceSyncDraft?: boolean
}

const syncDraftFromSavedSettings = () => {
  limitGiBInput.value = savedLimitGiB.value
  resetDayInput.value = savedResetDay.value
  expiryDateInput.value = savedExpiryDate.value
  expiryPickerEpoch.value = buildPickerEpochFromExpiryDate(savedExpiryDate.value)
}

const applyOverview = (raw: Partial<TrafficOverview>, options: ApplyOverviewOptions = {}) => {
  const input = raw as TrafficOverviewRaw
  const normalizedLimitGiB = normalizeLimitGiB(readNumberField(input, ['limitGiB', 'limit_gib'], 0))
  const normalizedResetDay = normalizeResetDay(readNumberField(input, ['resetDay', 'reset_day'], 0))
  const normalizedExpiryDate = normalizeExpiryDateInput(readStringField(input, ['expiryDate', 'expiry_date'], ''))
  const shouldSyncDraft = options.forceSyncDraft || !hasPendingSettingsChanges.value
  const vnstat = normalizeVnstatStatus(input.vnstat)

  overview.value = {
    source: readStringField(input, ['source'], 'vnstat'),
    interface: readStringField(input, ['interface'], ''),
    enabled: readBoolField(input, ['enabled'], true),
    status: readStringField(input, ['status'], ''),
    available: readBoolField(input, ['available'], false),
    up: readNumberField(input, ['up'], 0),
    down: readNumberField(input, ['down'], 0),
    total: readNumberField(input, ['total'], 0),
    accumUp: readNumberField(input, ['accumUp', 'accum_up'], 0),
    accumDown: readNumberField(input, ['accumDown', 'accum_down'], 0),
    accumTotal: readNumberField(input, ['accumTotal', 'accum_total'], 0),
    limitGiB: normalizedLimitGiB,
    resetDay: normalizedResetDay,
    expiryDate: normalizedExpiryDate,
    expired: readBoolField(input, ['expired'], false),
    nextResetAt: readNumberField(input, ['nextResetAt', 'next_reset_at'], 0),
    updatedAt: readNumberField(input, ['updatedAt', 'updated_at'], panelNowUnix()),
    vnstat,
    error: readStringField(input, ['error'], ''),
  }
  hasLoaded.value = true
  loadError.value = ''

  savedLimitGiB.value = normalizedLimitGiB
  savedResetDay.value = normalizedResetDay
  savedExpiryDate.value = normalizedExpiryDate
  enabledInput.value = overview.value.enabled

  if (shouldSyncDraft) {
    syncDraftFromSavedSettings()
  }

  if (selectedVnstatVersion.value === '') {
    if (vnstat.installMethod === 'system-package') {
      selectedVnstatVersion.value = 'system-package'
    } else if (vnstat.installMethod === 'github-release') {
      selectedVnstatVersion.value = 'github-release'
    }
  }

  schedulePolling(30000)
}

const isTrafficPageActiveAndVisible = () => (
  props.active && (typeof document === 'undefined' || document.visibilityState === 'visible')
)

const cancelOverviewRequest = () => {
  overviewRequestGeneration += 1
  overviewRequest.value = null
  if (overviewAbortController != null) {
    overviewAbortController.abort()
    overviewAbortController = null
  }
}

const stopPolling = () => {
  if (pollingTimer != null) {
    window.clearTimeout(pollingTimer)
    pollingTimer = null
  }
}

const schedulePolling = (delay = 30000) => {
  stopPolling()
  if (!isTrafficPageActiveAndVisible()) return
  pollingTimer = window.setTimeout(() => {
    void fetchOverview(true)
  }, delay)
}

const fetchOverview = async (silent = false) => {
  if (!isTrafficPageActiveAndVisible()) {
    return { success: false, msg: '', obj: null, failureKind: 'cancelled' as const }
  }
  if (overviewRequest.value) {
    return overviewRequest.value
  }
  if (!silent) {
    loading.value = true
    loadError.value = ''
  }
  const controller = new AbortController()
  const generation = ++overviewRequestGeneration
  overviewAbortController = controller

  const request = (async () => {
    const msg = await HttpUtils.get('api/traffic-overview', {}, {
      signal: controller.signal,
      silentAuthCheck: silent,
      silentErrorToast: silent,
    })
    if (generation === overviewRequestGeneration && isTrafficPageActiveAndVisible()) {
      if (msg.success && msg.obj) {
        applyOverview(msg.obj as Partial<TrafficOverview>)
      } else if (msg.failureKind !== 'cancelled') {
        loadError.value = msg.msg || '流量概览加载失败'
        schedulePolling(60000)
      }
    }
    return msg
  })()

  overviewRequest.value = request
  try {
    return await request
  } finally {
    if (overviewRequest.value === request) {
      overviewRequest.value = null
    }
    if (overviewAbortController === controller) {
      overviewAbortController = null
    }
    if (!silent) {
      loading.value = false
    }
  }
}

const onTrafficEnabledChanged = async (value: boolean | null) => {
  const nextEnabled = value === true
  const previousEnabled = overview.value.enabled
  if (!isTrafficPageActiveAndVisible() || trafficOperationBusy.value) {
    enabledInput.value = previousEnabled
    return
  }
  if (!nextEnabled) {
    trafficActionConfirming.value = true
    let confirmed = false
    try {
      confirmed = await confirm({
        message: DISABLE_TRAFFIC_CONFIRM_TEXT,
        severity: 'warning',
        confirmText: t('confirmDialog.actions.disable') || '确认关闭',
      })
    } finally {
      trafficActionConfirming.value = false
    }
    if (!confirmed) {
      enabledInput.value = previousEnabled
      return
    }
  }

  togglingTraffic.value = true
  try {
    const msg = await HttpUtils.post('api/traffic-overview-switch', { enabled: nextEnabled }, {
      headers: { 'Content-Type': 'application/json' },
    })
    if (msg.success && msg.obj) {
      applyOverview(msg.obj as Partial<TrafficOverview>)
      push.success(nextEnabled ? '流量统计已开启' : '流量统计已暂停')
    } else {
      enabledInput.value = previousEnabled
      push.error(`切换状态失败：${msg.msg || '未知错误'}`)
    }
  } finally {
    togglingTraffic.value = false
  }
}

const saveTrafficSettings = async () => {
  savingSettings.value = true
  const payload = {
    limit_gib: normalizeLimitGiB(limitGiBInput.value),
    reset_day: normalizeResetDay(resetDayInput.value),
    expiry_date: normalizeExpiryDateInput(expiryDateInput.value),
  }
  try {
    const msg = await HttpUtils.post('api/traffic-overview-settings', payload)
    if (msg.success && msg.obj) {
      applyOverview(msg.obj as Partial<TrafficOverview>, { forceSyncDraft: true })
      push.success('流量配额设置已保存')
    } else {
      push.error(`保存失败：${msg.msg || '未知错误'}`)
    }
  } finally {
    savingSettings.value = false
  }
}

const confirmResetPeriodTraffic = async () => {
  trafficActionConfirming.value = true
  let confirmed = false
  try {
    confirmed = await confirm({
      message: RESET_PERIOD_CONFIRM_TEXT,
      severity: 'warning',
      confirmText: '确认重置',
    })
  } finally {
    trafficActionConfirming.value = false
  }
  if (!confirmed) return

  resettingPeriod.value = true
  try {
    const msg = await HttpUtils.post('api/traffic-overview-period-reset', {})
    if (msg.success && msg.obj) {
      applyOverview(msg.obj as Partial<TrafficOverview>)
      push.success('当期流量统计已重置归零')
    } else {
      push.error(`重置失败：${msg.msg || '未知错误'}`)
    }
  } finally {
    resettingPeriod.value = false
  }
}

const confirmResetTotalTraffic = async () => {
  trafficActionConfirming.value = true
  let confirmed = false
  try {
    confirmed = await confirm({
      message: RESET_TOTAL_CONFIRM_TEXT,
      severity: 'danger',
      confirmText: '彻底清零',
    })
  } finally {
    trafficActionConfirming.value = false
  }
  if (!confirmed) return

  resettingTotal.value = true
  try {
    const msg = await HttpUtils.post('api/traffic-overview-total-reset', {})
    if (msg.success && msg.obj) {
      applyOverview(msg.obj as Partial<TrafficOverview>)
      push.success('历史累计总流量已重置')
    } else {
      push.error(`重置失败：${msg.msg || '未知错误'}`)
    }
  } finally {
    resettingTotal.value = false
  }
}

// vnStat 安装与管理流程
const loadVnstatVersionOptions = async () => {
  const generation = ++vnstatVersionOptionsGeneration
  try {
    const msg = await HttpUtils.get('api/traffic-overview-vnstat-versions', {}, { silentAuthCheck: true })
    if (generation !== vnstatVersionOptionsGeneration || !isTrafficPageActiveAndVisible()) return
    if (msg.success && msg.obj) {
      const items = normalizeVnstatVersionItems(msg.obj)
      if (items.length > 0) {
        vnstatVersionItems.value = items
      }
    }
  } catch {
    // 保持缺省项
  }
}

const checkVnstatUpdate = async () => {
  const selectedSource = selectedVnstatVersion.value.trim()
  if (selectedSource === '') {
    push.warning(SELECT_VNSTAT_SOURCE_HINT)
    return
  }
  checkingVnstatUpdate.value = true
  try {
    const msg = await HttpUtils.get(`api/traffic-overview-vnstat-update-info?source=${encodeURIComponent(selectedSource)}`)
    if (msg.success && msg.obj) {
      vnstatUpdateInfo.value = normalizeVnstatUpdateInfo(msg.obj)
      if (vnstatUpdateInfo.value.hasUpdate) {
        push.info(`发现新版本：${vnstatUpdateInfo.value.latestVersion}`)
      } else {
        push.success(vnstatUpdateInfo.value.message || '当前已是最新版本')
      }
    } else {
      push.error(`检测更新失败：${msg.msg || '未知错误'}`)
    }
  } finally {
    checkingVnstatUpdate.value = false
  }
}

const stopVnstatInstallPolling = () => {
  if (vnstatInstallPollingTimer != null) {
    window.clearTimeout(vnstatInstallPollingTimer)
    vnstatInstallPollingTimer = null
  }
}

const clearVnstatInstallTask = () => {
  stopVnstatInstallPolling()
  installingVnstat.value = false
  vnstatInstallJobId.value = ''
  vnstatInstallPhase.value = ''
  vnstatInstallState.value = 'idle'
  vnstatInstallCanCancel.value = false
  vnstatStopRequestPending.value = false
  vnstatInstallBeforeVersion.value = ''
}

const scheduleVnstatInstallPolling = () => {
  stopVnstatInstallPolling()
  if (!hasActiveVnstatInstall.value || vnstatInstallJobId.value === '' || !props.active) return
  if (typeof document !== 'undefined' && document.visibilityState !== 'visible') return
  vnstatInstallPollingTimer = window.setTimeout(() => {
    void pollVnstatInstallJob(vnstatInstallJobId.value)
  }, 1200)
}

const completeVnstatInstallJob = async (job: VnstatInstallJob) => {
  const source = job.source || selectedVnstatVersion.value
  const beforeVersion = vnstatInstallBeforeVersion.value
  clearVnstatInstallTask()

  if (job.state === 'success') {
    await fetchOverview(true)
    const afterVersion = overview.value.vnstat.version.trim()
    push.success({
      duration: 3500,
      message: `vnStat 安装/更新成功！当前版本：${afterVersion || '已就绪'}`,
    })
    if (source !== '') {
      selectedVnstatVersion.value = source
    }
    vnstatUpdateInfo.value = createIdleVnstatUpdateInfo(overview.value.vnstat)
    return
  }

  await fetchOverview(true)
  if (job.state === 'cancelled' || job.state === 'timed_out') {
    push.info(job.state === 'timed_out' ? 'vnStat 下载超时，任务已取消' : 'vnStat 安装已停止')
    return
  }
  push.warning({
    title: 'vnStat 安装未完成',
    message: job.error || job.phase || '安装任务未能成功执行',
  })
}

const applyVnstatInstallJob = async (job: VnstatInstallJob) => {
  if (['queued', 'running', 'stopping'].includes(job.state) && job.id !== '') {
    vnstatInstallJobId.value = job.id
    installingVnstat.value = true
    vnstatInstallState.value = job.state
    vnstatInstallCanCancel.value = job.canCancel === true
    vnstatStopRequestPending.value = job.stopRequested === true || job.state === 'stopping'
    vnstatInstallPhase.value = job.phase || (job.canCancel ? 'vnStat 正在下载中...' : '正在应用系统配置...')
    if (vnstatInstallBeforeVersion.value === '') {
      vnstatInstallBeforeVersion.value = overview.value.vnstat.version.trim()
    }
    scheduleVnstatInstallPolling()
    return
  }

  if (['success', 'error', 'cancelled', 'timed_out'].includes(job.state)) {
    await completeVnstatInstallJob(job)
    return
  }

  clearVnstatInstallTask()
  void fetchOverview(true)
}

const pollVnstatInstallJob = async (jobID: string) => {
  if (!props.active || (typeof document !== 'undefined' && document.visibilityState !== 'visible')) return
  if (jobID === '' || jobID !== vnstatInstallJobId.value) return
  const msg = await HttpUtils.get('api/traffic-overview-vnstat-install-status', { jobId: jobID }, { silentAuthCheck: true })
  if (jobID !== vnstatInstallJobId.value) return
  if (!msg.success || !msg.obj) {
    vnstatInstallPhase.value = '正在同步任务进度...'
    scheduleVnstatInstallPolling()
    return
  }
  await applyVnstatInstallJob(normalizeVnstatInstallJob(msg.obj))
}

const installVnstat = async () => {
  const selectedSource = selectedVnstatVersion.value.trim()
  if (selectedSource === '') {
    push.warning(SELECT_VNSTAT_SOURCE_HINT)
    return
  }
  trafficActionConfirming.value = true
  let confirmed = false
  try {
    confirmed = await confirm({
      message: INSTALL_VNSTAT_CONFIRM_TEXT,
      severity: 'info',
      confirmText: '确认安装',
    })
  } finally {
    trafficActionConfirming.value = false
  }
  if (!confirmed) return

  installingVnstat.value = true
  vnstatInstallPhase.value = '正在创建安装任务...'
  try {
    const msg = await HttpUtils.post('api/traffic-overview-vnstat-install', { source: selectedSource })
    if (msg.success && msg.obj) {
      await applyVnstatInstallJob(normalizeVnstatInstallJob(msg.obj))
    } else {
      clearVnstatInstallTask()
      push.error(`创建安装任务失败：${msg.msg || '未知错误'}`)
    }
  } catch (err: any) {
    clearVnstatInstallTask()
    push.error(`请求异常：${err.message || err}`)
  }
}

const stopVnstatInstall = async () => {
  const id = vnstatInstallJobId.value
  if (!id) return
  vnstatStopRequestPending.value = true
  try {
    const msg = await HttpUtils.post('api/traffic-overview-vnstat-install-stop', { id })
    if (msg.success && msg.obj) {
      await applyVnstatInstallJob(normalizeVnstatInstallJob(msg.obj))
    }
  } finally {
    vnstatStopRequestPending.value = false
  }
}

// 删除流程
const stopVnstatRemovalPolling = () => {
  if (vnstatRemovalPollingTimer != null) {
    window.clearTimeout(vnstatRemovalPollingTimer)
    vnstatRemovalPollingTimer = null
  }
}

const clearVnstatRemovalTask = () => {
  stopVnstatRemovalPolling()
  removingVnstat.value = false
  vnstatRemovalJobId.value = ''
  vnstatRemovalPhase.value = ''
  vnstatRemovalState.value = 'idle'
}

const scheduleVnstatRemovalPolling = () => {
  stopVnstatRemovalPolling()
  if (!hasActiveVnstatRemoval.value || vnstatRemovalJobId.value === '' || !isTrafficPageActiveAndVisible()) return
  vnstatRemovalPollingTimer = window.setTimeout(() => {
    void pollVnstatRemovalJob(vnstatRemovalJobId.value)
  }, 1500)
}

const completeVnstatRemovalJob = async (job: VnstatRemovalJob) => {
  clearVnstatRemovalTask()
  await fetchOverview(true)
  if (job.state === 'success') {
    selectedVnstatVersion.value = ''
    vnstatUpdateInfo.value = createIdleVnstatUpdateInfo(overview.value.vnstat)
    push.success('vnStat 已成功删除，相关服务已清理')
    return
  }
  push.warning({
    title: 'vnStat 卸载未完全完成',
    message: job.error || job.phase || '卸载流程未能完全清理',
  })
}

const applyVnstatRemovalJob = async (job: VnstatRemovalJob) => {
  if (['queued', 'running'].includes(job.state) && job.id !== '') {
    vnstatRemovalJobId.value = job.id
    removingVnstat.value = true
    vnstatRemovalState.value = job.state
    vnstatRemovalPhase.value = job.phase || '正在卸载清理受管 vnStat...'
    scheduleVnstatRemovalPolling()
    return
  }

  if (['success', 'error'].includes(job.state)) {
    await completeVnstatRemovalJob(job)
    return
  }

  clearVnstatRemovalTask()
  void fetchOverview(true)
}

const pollVnstatRemovalJob = async (jobID: string) => {
  if (!props.active || (typeof document !== 'undefined' && document.visibilityState !== 'visible')) return
  if (jobID === '' || jobID !== vnstatRemovalJobId.value) return
  const msg = await HttpUtils.get('api/traffic-overview-vnstat-removal-status', { jobId: jobID }, { silentAuthCheck: true })
  if (jobID !== vnstatRemovalJobId.value) return
  if (!msg.success || !msg.obj) {
    scheduleVnstatRemovalPolling()
    return
  }
  await applyVnstatRemovalJob(normalizeVnstatRemovalJob(msg.obj))
}

const removeVnstat = async () => {
  trafficActionConfirming.value = true
  let confirmed = false
  try {
    confirmed = await confirm({
      message: REMOVE_VNSTAT_CONFIRM_TEXT,
      severity: 'danger',
      confirmText: '确认卸载',
    })
  } finally {
    trafficActionConfirming.value = false
  }
  if (!confirmed) return

  removingVnstat.value = true
  vnstatRemovalPhase.value = '正在发起卸载请求...'
  try {
    const msg = await HttpUtils.post('api/traffic-overview-vnstat-remove', {})
    if (msg.success && msg.obj) {
      await applyVnstatRemovalJob(normalizeVnstatRemovalJob(msg.obj))
    } else {
      clearVnstatRemovalTask()
      push.error(`发起卸载失败：${msg.msg || '未知错误'}`)
    }
  } catch (err: any) {
    clearVnstatRemovalTask()
    push.error(`请求异常：${err.message || err}`)
  }
}

// 监听激活状态
watch(() => props.active, (active) => {
  if (active) {
    void fetchOverview()
    void loadVnstatVersionOptions()
  } else {
    stopPolling()
    stopVnstatInstallPolling()
    stopVnstatRemovalPolling()
    cancelOverviewRequest()
  }
}, { immediate: true })

onBeforeUnmount(() => {
  stopPolling()
  stopVnstatInstallPolling()
  stopVnstatRemovalPolling()
  cancelOverviewRequest()
})
</script>

<style scoped>
.metric-card {
  transition: all 0.2s ease-in-out;
}
.metric-card:hover {
  border-color: rgba(var(--v-theme-primary), 0.5);
}
.accum-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 8px;
  border-radius: 4px;
  background-color: rgba(var(--v-theme-on-surface), 0.06);
  font-size: 0.75rem;
}
.accum-badge__label {
  color: rgba(var(--v-theme-on-surface), 0.6);
}
.traffic-status-chip {
  font-weight: 500;
}
.border-t {
  border-top: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
}
.border-b {
  border-bottom: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
}
.border {
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
}

.card-cyan {
  border-color: rgba(34, 211, 238, 0.45) !important;
  transition: border-color 0.2s ease;
}
.card-cyan:hover {
  border-color: rgba(34, 211, 238, 0.75) !important;
}
.card-green {
  border-color: rgba(74, 222, 128, 0.45) !important;
  transition: border-color 0.2s ease;
}
.card-green:hover {
  border-color: rgba(74, 222, 128, 0.75) !important;
}
</style>
