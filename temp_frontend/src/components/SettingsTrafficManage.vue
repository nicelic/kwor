<template>
  <div class="settings-traffic-manage">
    <v-row>
      <v-col cols="12" lg="8">
        <v-card rounded="lg" variant="tonal" class="mb-4">
          <v-card-title class="traffic-card-title">
            <div class="text-subtitle-1 font-weight-medium">vnstat {{ t('stats.graphTitle') }}</div>
            <div class="traffic-toolbar">
              <v-switch
                v-model="enabledInput"
                color="success"
                density="compact"
                hide-details
                inset
                label="流量统计"
                :loading="togglingTraffic"
                :disabled="loading || togglingTraffic || installingVnstat || removingVnstat || checkingVnstatUpdate"
                @update:model-value="onTrafficEnabledChanged" />
              <v-chip size="small" :color="statusColor">
                {{ statusLabel }}
              </v-chip>
            </div>
          </v-card-title>
          <v-divider />
          <v-card-text>
            <v-alert
              v-if="overview.error"
              type="warning"
              variant="tonal"
              density="comfortable"
              class="mb-4">
              {{ overview.error }}
            </v-alert>
            <div class="text-caption text-medium-emphasis mb-3">
              网卡: {{ overview.interface || '-' }}
              <span class="mx-2">|</span>
              来源: {{ overview.source || 'vnstat' }}
              <span class="mx-2">|</span>
              更新时间: {{ updatedAtLabel }}
            </div>
            <div class="traffic-runtime__actions mb-4">
              <v-select
                v-model="selectedVnstatVersion"
                :items="vnstatVersionSelectItems"
                item-title="title"
                item-value="value"
                label="安装版本"
                density="comfortable"
                hide-details
                class="traffic-version-select"
                :loading="loadingVnstatVersions"
                :disabled="loading || togglingTraffic || installingVnstat || removingVnstat || checkingVnstatUpdate || !overview.vnstat.supported || !overview.vnstat.canManage"
                @update:menu="onVnstatVersionMenuUpdate" />
              <div class="traffic-runtime__button-group">
                <v-btn
                  color="primary"
                  prepend-icon="mdi-download"
                  :loading="installingVnstat"
                  :disabled="loading || togglingTraffic || installingVnstat || removingVnstat || checkingVnstatUpdate || !overview.vnstat.supported || !overview.vnstat.canManage"
                  @click="installVnstat">
                  {{ overview.vnstat.installed ? '下载 / 重装' : '下载 / 安装' }}
                </v-btn>
                <v-btn
                  variant="outlined"
                  color="primary"
                  prepend-icon="mdi-cloud-search"
                  :loading="checkingVnstatUpdate"
                  :disabled="loading || togglingTraffic || installingVnstat || removingVnstat || checkingVnstatUpdate || !overview.vnstat.supported || !overview.vnstat.canManage"
                  @click="checkVnstatUpdate">
                  检测更新
                </v-btn>
                <v-btn
                  variant="outlined"
                  color="error"
                  prepend-icon="mdi-delete-outline"
                  :loading="removingVnstat"
                  :disabled="loading || togglingTraffic || installingVnstat || removingVnstat || checkingVnstatUpdate || !overview.vnstat.supported || !overview.vnstat.canManage || !overview.vnstat.installed"
                  @click="removeVnstat">
                  删除
                </v-btn>
              </div>
            </div>
            <div class="text-caption text-medium-emphasis mb-3">
              重装 / 更新默认保留现有 vnstat 流量数据库；只有删除操作才会清理流量统计数据。
            </div>
            <v-alert
              v-if="overview.vnstat.supported && !overview.vnstat.canManage && overview.vnstat.manageHint"
              type="info"
              variant="tonal"
              density="comfortable"
              class="mb-4">
              {{ overview.vnstat.manageHint }}
            </v-alert>
            <v-row>
              <v-col cols="12" sm="4">
                <v-card variant="outlined" class="metric-card">
                  <div class="text-caption text-medium-emphasis">{{ t('stats.volume') }}</div>
                  <div class="text-h6 mt-1">{{ periodTotalText }}</div>
                  <div class="accum-badge">{{ accumTotalText }}</div>
                </v-card>
              </v-col>
              <v-col cols="12" sm="4">
                <v-card variant="outlined" class="metric-card">
                  <div class="text-caption text-medium-emphasis">{{ t('stats.upload') }}</div>
                  <div class="text-h6 mt-1 text-orange">{{ periodUpText }}</div>
                  <div class="accum-badge">{{ accumUpText }}</div>
                </v-card>
              </v-col>
              <v-col cols="12" sm="4">
                <v-card variant="outlined" class="metric-card">
                  <div class="text-caption text-medium-emphasis">{{ t('stats.download') }}</div>
                  <div class="text-h6 mt-1 text-success">{{ periodDownText }}</div>
                  <div class="accum-badge">{{ accumDownText }}</div>
                </v-card>
              </v-col>
            </v-row>
            <div class="traffic-runtime__rows mt-4">
              <div class="traffic-runtime__row">
                <span>当前版本</span>
                <strong>{{ vnstatVersionText }}</strong>
              </div>
              <div class="traffic-runtime__row">
                <span>最新版本</span>
                <strong>{{ vnstatLatestVersionText }}</strong>
              </div>
              <div class="traffic-runtime__row">
                <span>更新状态</span>
                <strong>{{ vnstatUpdateMessageText }}</strong>
              </div>
              <div class="traffic-runtime__row">
                <span>检测来源</span>
                <strong>{{ vnstatUpdateSourceText }}</strong>
              </div>
              <div class="traffic-runtime__row">
                <span>系统系列</span>
                <strong>{{ overview.vnstat.systemFamily || '-' }}</strong>
              </div>
              <div class="traffic-runtime__row">
                <span>安装方式</span>
                <strong>{{ vnstatInstallMethodText }}</strong>
              </div>
              <div class="traffic-runtime__row">
                <span>包管理器</span>
                <strong>{{ overview.vnstat.packageManager || '-' }}</strong>
              </div>
              <div class="traffic-runtime__row">
                <span>程序路径</span>
                <strong class="traffic-code">{{ overview.vnstat.binaryPath || '-' }}</strong>
              </div>
              <div class="traffic-runtime__row">
                <span>跟踪文件</span>
                <strong>{{ overview.vnstat.fileCount > 0 ? `${overview.vnstat.fileCount} 个` : '-' }}</strong>
              </div>
              <div class="traffic-runtime__row">
                <span>数据目录</span>
                <strong class="traffic-code">{{ vnstatDataPathText }}</strong>
              </div>
            </div>
          </v-card-text>
        </v-card>
      </v-col>
      <v-col cols="12" lg="4">
          <v-card rounded="lg" variant="outlined" class="h-100">
            <v-card-title class="text-subtitle-1 font-weight-medium">
              {{ t('stats.usage') }} / {{ resetDayLabel }}
            </v-card-title>
          <v-divider />
          <v-card-text>
            <v-text-field
              v-model.number="limitGiBInput"
              type="number"
              min="0"
              step="0.01"
              :label="`${t('stats.volume')} (GB)`"
              hide-details />
            <div class="mt-3">
              <DatePick
                :expiry="resetPickerEpoch"
                picker-type="date"
                :label-text="resetDayLabel"
                :zero-text="disabledLabel"
                @submit="onSubmitResetDayPicker" />
            </div>
            <div class="text-caption text-medium-emphasis mt-2">
              {{ monthlyHint }}
            </div>
            <v-btn
              class="mt-3"
              color="primary"
              variant="tonal"
              :loading="savingSettings"
              :disabled="savingSettings || !hasPendingSettingsChanges"
              @click="saveTrafficSettings">
              {{ t('actions.save') }}
            </v-btn>
            <div class="mt-4 text-body-2">{{ usageText }}</div>
            <v-progress-linear
              v-if="limitBytes > 0"
              :model-value="usagePercent"
              :color="usageColor"
              rounded
              height="8"
              class="mt-2" />
            <div class="mt-2 text-caption text-medium-emphasis">
              {{ resetDayLabel }}: {{ resetDayInput > 0 ? `${resetDayInput} ${daySuffix}` : disabledLabel }}
            </div>
            <div class="mt-1 text-caption text-medium-emphasis">
              {{ nextResetLabel }}: {{ nextResetAtLabel }}
            </div>
            <div class="traffic-settings__actions mt-4">
              <v-btn
                color="warning"
                variant="tonal"
                :loading="resettingPeriod"
                :disabled="loading || savingSettings || togglingTraffic || installingVnstat || removingVnstat || resettingPeriod || resettingTotal"
                @click="confirmResetPeriodTraffic">
                重置流量
              </v-btn>
              <v-btn
                color="error"
                variant="outlined"
                :loading="resettingTotal"
                :disabled="loading || savingSettings || togglingTraffic || installingVnstat || removingVnstat || resettingPeriod || resettingTotal"
                @click="confirmResetTotalTraffic">
                重置总流量
              </v-btn>
            </div>
          </v-card-text>
        </v-card>
      </v-col>
  </v-row>
  </div>
</template>

<script setup lang="ts">
import DatePick from '@/components/DateTime.vue'
import HttpUtils from '@/plugins/httputil'
import { onBeforeUnmount, onMounted, computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { push } from 'notivue'

type TrafficOverview = {
  source: string
  interface: string
  enabled: boolean
  status: string
  available: boolean
  up: number
  down: number
  total: number
  accumUp: number
  accumDown: number
  accumTotal: number
  limitGiB: number
  resetDay: number
  updatedAt: number
  vnstat: VnstatStatus
  error?: string
}

type VnstatStatus = {
  supported: boolean
  canManage: boolean
  installed: boolean
  managed: boolean
  running: boolean
  version: string
  systemFamily: string
  packageManager: string
  installMethod: string
  binaryPath: string
  fileCount: number
  dataPaths: string[]
  manageHint: string
  error?: string
}

type VnstatVersionItem = {
  value: string
  title: string
  description: string
}

type VnstatVersionListResult = {
  versions: VnstatVersionItem[]
}

type VnstatUpdateInfo = {
  supported: boolean
  canManage: boolean
  installed: boolean
  managed: boolean
  currentVersion: string
  latestVersion: string
  hasUpdate: boolean
  source: string
  message: string
}

type TrafficOverviewRaw = Record<string, unknown>

const props = withDefaults(defineProps<{
  active?: boolean
}>(), {
  active: false,
})

const { t } = useI18n()
const resetDayLabel = '\u6d41\u91cf\u91cd\u7f6e\u65e5\u671f'
const disabledLabel = '\u672a\u542f\u7528'
const daySuffix = '\u53f7'
const monthlyHint = '\u6bcf\u6708\u6309\u8be5\u65e5\u671f\u91cd\u7f6e\uff1b\u82e5\u5f53\u6708\u5929\u6570\u4e0d\u8db3\u5219\u81ea\u52a8\u5728\u6708\u5e95\u91cd\u7f6e\u3002'
const resetPeriodConfirmText = '\u662f\u5426\u91cd\u7f6e\u5de6\u4fa7\u6d41\u91cf\u7edf\u8ba1\uff1f'
const resetTotalConfirmText = '\u662f\u5426\u91cd\u7f6e\u603b\u4f7f\u7528\u6d41\u91cf\uff1f'
const nextResetLabel = '\u4e0b\u4e00\u6b21\u91cd\u7f6e\u65f6\u95f4'
const removeVnstatConfirmText = '确认删除 vnstat 吗？将停止运行的 vnstat，卸载软件包，并删除已跟踪的 vnstat 流量数据。'
const removeExternalVnstatConfirmText = '检测到系统已有 vnstat。确认删除会停止并卸载 vnstat，同时清理 vnstat 流量数据，是否继续？'
const disableTrafficConfirmText = '确认关闭流量统计吗？关闭期间产生的流量不会计入面板统计，再次开启会从当前数值继续统计。'
const defaultVnstatVersionOption: VnstatVersionItem = {
  value: 'system',
  title: '自动安装（系统源优先）',
  description: '默认通过当前系统软件源安装或重装 vnstat；系统软件源不可用时自动回退到 GitHub 官方版本。',
}

const loading = ref(false)
const savingSettings = ref(false)
const resettingPeriod = ref(false)
const resettingTotal = ref(false)
const togglingTraffic = ref(false)
const installingVnstat = ref(false)
const removingVnstat = ref(false)
const checkingVnstatUpdate = ref(false)
const loadingVnstatVersions = ref(false)
const vnstatVersionLoaded = ref(false)
const enabledInput = ref(true)
const selectedVnstatVersion = ref(defaultVnstatVersionOption.value)
const vnstatVersionItems = ref<VnstatVersionItem[]>([defaultVnstatVersionOption])
const overview = ref<TrafficOverview>({
  source: 'vnstat',
  interface: '',
  enabled: true,
  status: 'stopped',
  available: false,
  up: 0,
  down: 0,
  total: 0,
  accumUp: 0,
  accumDown: 0,
  accumTotal: 0,
  limitGiB: 0,
  resetDay: 0,
  updatedAt: 0,
  vnstat: {
    supported: false,
    canManage: false,
    installed: false,
    managed: false,
    running: false,
    version: '',
    systemFamily: '',
    packageManager: '',
    installMethod: '',
    binaryPath: '',
    fileCount: 0,
    dataPaths: [],
    manageHint: '',
  },
})
const vnstatUpdateInfo = ref<VnstatUpdateInfo>({
  supported: false,
  canManage: false,
  installed: false,
  managed: false,
  currentVersion: '',
  latestVersion: '',
  hasUpdate: false,
  source: '',
  message: '',
})

const limitGiBInput = ref(0)
const resetDayInput = ref(0)
const resetPickerEpoch = ref(0)
const savedLimitGiB = ref(0)
const savedResetDay = ref(0)
let pollingTimer: number | null = null

const createIdleVnstatUpdateInfo = (status: VnstatStatus = overview.value.vnstat): VnstatUpdateInfo => ({
  supported: status.supported,
  canManage: status.canManage,
  installed: status.installed,
  managed: status.managed,
  currentVersion: status.version,
  latestVersion: '',
  hasUpdate: false,
  source: '',
  message: '',
})

const normalizeLimitGiB = (value: number) => {
  if (!Number.isFinite(value) || value <= 0) return 0
  const rounded = Math.round(value * 100) / 100
  if (rounded > 0 && rounded < 0.01) return 0.01
  return rounded
}

const readNumberField = (raw: TrafficOverviewRaw, keys: string[], fallback = 0) => {
  for (const key of keys) {
    const value = raw[key]
    if (typeof value === 'number' && Number.isFinite(value)) {
      return value
    }
    if (typeof value === 'string') {
      const parsed = Number(value.trim())
      if (Number.isFinite(parsed)) {
        return parsed
      }
    }
  }
  return fallback
}

const readStringField = (raw: TrafficOverviewRaw, keys: string[], fallback = '') => {
  for (const key of keys) {
    const value = raw[key]
    if (typeof value === 'string') {
      return value
    }
  }
  return fallback
}

const readBoolField = (raw: TrafficOverviewRaw, keys: string[], fallback = false) => {
  for (const key of keys) {
    const value = raw[key]
    if (typeof value === 'boolean') {
      return value
    }
    if (typeof value === 'number') {
      return value !== 0
    }
    if (typeof value === 'string') {
      const normalized = value.trim().toLowerCase()
      if (normalized === 'true' || normalized === '1') {
        return true
      }
      if (normalized === 'false' || normalized === '0') {
        return false
      }
    }
  }
  return fallback
}

const readStringArrayField = (raw: TrafficOverviewRaw, keys: string[], fallback: string[] = []) => {
  for (const key of keys) {
    const value = raw[key]
    if (Array.isArray(value)) {
      return value.map(item => String(item ?? '').trim()).filter(item => item.length > 0)
    }
  }
  return [...fallback]
}

const normalizeVnstatStatus = (raw: unknown): VnstatStatus => {
  const input = (raw ?? {}) as TrafficOverviewRaw
  return {
    supported: readBoolField(input, ['supported'], false),
    canManage: readBoolField(input, ['canManage', 'can_manage'], false),
    installed: readBoolField(input, ['installed'], false),
    managed: readBoolField(input, ['managed'], false),
    running: readBoolField(input, ['running'], false),
    version: readStringField(input, ['version'], ''),
    systemFamily: readStringField(input, ['systemFamily', 'system_family'], ''),
    packageManager: readStringField(input, ['packageManager', 'package_manager'], ''),
    installMethod: readStringField(input, ['installMethod', 'install_method'], ''),
    binaryPath: readStringField(input, ['binaryPath', 'binary_path'], ''),
    fileCount: readNumberField(input, ['fileCount', 'file_count'], 0),
    dataPaths: readStringArrayField(input, ['dataPaths', 'data_paths'], []),
    manageHint: readStringField(input, ['manageHint', 'manage_hint'], ''),
    error: readStringField(input, ['error'], ''),
  }
}

const normalizeVnstatVersionItem = (raw: unknown): VnstatVersionItem | null => {
  const input = (raw ?? {}) as TrafficOverviewRaw
  const value = readStringField(input, ['value'], '')
  if (value.trim() === '') {
    return null
  }
  return {
    value,
    title: readStringField(input, ['title'], value),
    description: readStringField(input, ['description'], ''),
  }
}

const normalizeVnstatUpdateInfo = (raw: unknown): VnstatUpdateInfo => {
  const input = (raw ?? {}) as TrafficOverviewRaw
  return {
    supported: readBoolField(input, ['supported'], false),
    canManage: readBoolField(input, ['canManage', 'can_manage'], false),
    installed: readBoolField(input, ['installed'], false),
    managed: readBoolField(input, ['managed'], false),
    currentVersion: readStringField(input, ['currentVersion', 'current_version'], ''),
    latestVersion: readStringField(input, ['latestVersion', 'latest_version'], ''),
    hasUpdate: readBoolField(input, ['hasUpdate', 'has_update'], false),
    source: readStringField(input, ['source'], ''),
    message: readStringField(input, ['message'], ''),
  }
}

const limitBytes = computed(() => (
  limitGiBInput.value > 0 ? limitGiBInput.value * 1024 * 1024 * 1024 : 0
))
const hasPendingSettingsChanges = computed(() => (
  normalizeLimitGiB(limitGiBInput.value) !== savedLimitGiB.value ||
  normalizeResetDay(resetDayInput.value) !== savedResetDay.value
))
const usageBytes = computed(() => overview.value.accumTotal)
const usagePercent = computed(() => (
  limitBytes.value > 0 ? Math.min(100, Math.round(usageBytes.value * 100 / limitBytes.value)) : 0
))
const usageColor = computed(() => (
  usagePercent.value >= 100 ? 'error' : usagePercent.value >= 90 ? 'warning' : 'success'
))
const usageText = computed(() => {
  if (limitBytes.value <= 0) {
    return `${formatGB(usageBytes.value)} / -`
  }
  return `${formatGB(usageBytes.value)} / ${formatGB(limitBytes.value)} (${usagePercent.value}%)`
})

const periodUpText = computed(() => formatGB(overview.value.up))
const periodDownText = computed(() => formatGB(overview.value.down))
const periodTotalText = computed(() => formatGB(overview.value.total))
const accumUpText = computed(() => formatGB(overview.value.accumUp))
const accumDownText = computed(() => formatGB(overview.value.accumDown))
const accumTotalText = computed(() => formatGB(overview.value.accumTotal))
const updatedAtLabel = computed(() => (
  overview.value.updatedAt > 0 ? new Date(overview.value.updatedAt * 1000).toLocaleString() : '-'
))
const statusLabel = computed(() => {
  if (!overview.value.enabled) {
    return '已暂停'
  }
  if (overview.value.available) {
    return '已运行'
  }
  if (!overview.value.vnstat.installed) {
    return '未安装'
  }
  return '已停止'
})
const statusColor = computed(() => {
  if (!overview.value.enabled) return 'warning'
  if (overview.value.available) return 'success'
  if (!overview.value.vnstat.installed) return 'error'
  return 'warning'
})
const vnstatVersionSelectItems = computed(() => (
  vnstatVersionItems.value.length > 0 ? vnstatVersionItems.value : [defaultVnstatVersionOption]
))
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
  if (source === 'apt-get' || source === 'dnf' || source === 'yum' || source === 'zypper' || source === 'pacman' || source === 'apk') {
    return `系统软件源 (${source})`
  }
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
const vnstatDataPathText = computed(() => (
  overview.value.vnstat.dataPaths.length > 0 ? overview.value.vnstat.dataPaths.join(' / ') : '-'
))
const nextResetAtLabel = computed(() => {
  const date = getNextResetAt(resetDayInput.value)
  if (date == null) {
    return disabledLabel
  }
  return date.toLocaleString()
})

const normalizeResetDay = (value: number) => {
  if (!Number.isFinite(value) || value <= 0) return 0
  if (value > 31) return 31
  return Math.floor(value)
}

const daysInMonth = (year: number, monthIndex: number) => (
  new Date(year, monthIndex + 1, 0).getDate()
)

const buildResetDayDisplayDate = (day: number): Date | null => {
  const normalizedDay = normalizeResetDay(day)
  if (normalizedDay <= 0) {
    return null
  }

  const now = new Date()
  let year = now.getFullYear()
  let month = now.getMonth()
  let maxDay = daysInMonth(year, month)
  let candidateDay = Math.min(normalizedDay, maxDay)
  let candidate = new Date(year, month, candidateDay, 0, 0, 0, 0)
  if (now.getTime() >= candidate.getTime()) {
    month += 1
    if (month > 11) {
      month = 0
      year += 1
    }
    maxDay = daysInMonth(year, month)
    candidateDay = Math.min(normalizedDay, maxDay)
    candidate = new Date(year, month, candidateDay, 0, 0, 0, 0)
  }
  return candidate
}

const computeResetBoundary = (day: number, year: number, month: number) => {
  const maxDay = daysInMonth(year, month)
  const effectiveDay = Math.min(day, maxDay)
  return new Date(year, month, effectiveDay + 1, 0, 0, 0, 0)
}

const getNextResetAt = (day: number): Date | null => {
  const normalizedDay = normalizeResetDay(day)
  if (normalizedDay <= 0) {
    return null
  }

  const now = new Date()
  const thisBoundary = computeResetBoundary(normalizedDay, now.getFullYear(), now.getMonth())
  if (now.getTime() < thisBoundary.getTime()) {
    return thisBoundary
  }

  const nextMonthDate = new Date(now.getFullYear(), now.getMonth() + 1, 1, 0, 0, 0, 0)
  return computeResetBoundary(normalizedDay, nextMonthDate.getFullYear(), nextMonthDate.getMonth())
}

const buildPickerEpochFromResetDay = (day: number) => {
  const next = buildResetDayDisplayDate(day)
  if (next == null) {
    return 0
  }
  return Math.floor(next.getTime() / 1000)
}

const parseEpochSeconds = (value: unknown): number | null => {
  if (typeof value === 'number' && Number.isFinite(value)) {
    const abs = Math.abs(value)
    return abs > 0 && abs < 1e11 ? Math.floor(value) : Math.floor(value / 1000)
  }

  if (typeof value === 'string') {
    const trimmed = value.trim()
    if (trimmed.length === 0) {
      return null
    }

    if (/^-?\d+(?:\.\d+)?$/.test(trimmed)) {
      return parseEpochSeconds(Number(trimmed))
    }

    const parsed = Date.parse(trimmed)
    if (!Number.isFinite(parsed)) {
      return null
    }
    return Math.floor(parsed / 1000)
  }

  if (value instanceof Date) {
    const millis = value.getTime()
    if (!Number.isFinite(millis)) {
      return null
    }
    return Math.floor(millis / 1000)
  }

  return null
}

const onSubmitResetDayPicker = (rawValue: unknown) => {
  const epochSeconds = parseEpochSeconds(rawValue)
  if (epochSeconds == null) {
    return
  }

  if (epochSeconds <= 0) {
    resetDayInput.value = 0
    resetPickerEpoch.value = 0
    return
  }
  const selected = new Date(epochSeconds * 1000)
  resetDayInput.value = normalizeResetDay(selected.getDate())
  resetPickerEpoch.value = Math.floor(selected.getTime() / 1000)
}

type ApplyOverviewOptions = {
  forceSyncDraft?: boolean
}

const syncDraftFromSavedSettings = () => {
  limitGiBInput.value = savedLimitGiB.value
  resetDayInput.value = savedResetDay.value
  resetPickerEpoch.value = buildPickerEpochFromResetDay(savedResetDay.value)
}

const applyOverview = (raw: Partial<TrafficOverview>, options: ApplyOverviewOptions = {}) => {
  const input = raw as TrafficOverviewRaw
  const normalizedLimitGiB = normalizeLimitGiB(readNumberField(input, ['limitGiB', 'limit_gib'], 0))
  const normalizedResetDay = normalizeResetDay(readNumberField(input, ['resetDay', 'reset_day'], 0))
  const shouldSyncDraft = options.forceSyncDraft || !hasPendingSettingsChanges.value
  const previousVnstat = overview.value.vnstat
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
    updatedAt: readNumberField(input, ['updatedAt', 'updated_at'], Math.floor(Date.now() / 1000)),
    vnstat,
    error: readStringField(input, ['error'], ''),
  }
  enabledInput.value = overview.value.enabled
  savedLimitGiB.value = normalizedLimitGiB
  savedResetDay.value = normalizedResetDay

  if (shouldSyncDraft) {
    syncDraftFromSavedSettings()
  }

  if (
    vnstatUpdateInfo.value.message.trim() === '' ||
    previousVnstat.installed !== vnstat.installed ||
    previousVnstat.version !== vnstat.version ||
    previousVnstat.supported !== vnstat.supported ||
    previousVnstat.canManage !== vnstat.canManage ||
    previousVnstat.managed !== vnstat.managed
  ) {
    vnstatUpdateInfo.value = createIdleVnstatUpdateInfo(vnstat)
  }
}

const fetchOverview = async (silent = false) => {
  if (!silent) {
    loading.value = true
  }
  try {
    const msg = await HttpUtils.get('api/traffic-overview')
    if (msg.success && msg.obj) {
      applyOverview(msg.obj as Partial<TrafficOverview>)
    }
  } finally {
    if (!silent) {
      loading.value = false
    }
  }
}

const onTrafficEnabledChanged = async (value: boolean | null) => {
  const nextEnabled = value === true
  const previousEnabled = overview.value.enabled
  if (!nextEnabled) {
    const confirmed = window.confirm(disableTrafficConfirmText)
    if (!confirmed) {
      enabledInput.value = previousEnabled
      return
    }
  }
  togglingTraffic.value = true
  try {
    const msg = await HttpUtils.post('api/traffic-overview-switch', { enabled: nextEnabled }, {
      headers: {
        'Content-Type': 'application/json',
      },
    })
    if (msg.success && msg.obj) {
      applyOverview(msg.obj as Partial<TrafficOverview>)
    } else {
      enabledInput.value = previousEnabled
    }
  } finally {
    togglingTraffic.value = false
  }
}

const fetchVnstatVersions = async () => {
  if (loadingVnstatVersions.value) return
  loadingVnstatVersions.value = true
  try {
    const msg = await HttpUtils.get('api/traffic-overview-vnstat-versions')
    if (!msg.success || msg.obj == null) {
      return
    }
    const data = msg.obj as Partial<VnstatVersionListResult>
    const items = Array.isArray(data.versions)
      ? data.versions.map(normalizeVnstatVersionItem).filter((item): item is VnstatVersionItem => item != null)
      : []
    vnstatVersionItems.value = items.length > 0 ? items : [defaultVnstatVersionOption]
    if (!vnstatVersionItems.value.some(item => item.value === selectedVnstatVersion.value)) {
      selectedVnstatVersion.value = vnstatVersionItems.value[0]?.value || defaultVnstatVersionOption.value
    }
    vnstatVersionLoaded.value = true
  } finally {
    loadingVnstatVersions.value = false
  }
}

const ensureVnstatVersionsLoaded = async () => {
  if (vnstatVersionLoaded.value) return
  await fetchVnstatVersions()
}

const onVnstatVersionMenuUpdate = (opened: boolean) => {
  if (!opened) return
  void ensureVnstatVersionsLoaded()
}

const installVnstat = async () => {
  const beforeVersion = overview.value.vnstat.version.trim()
  const targetVersion = selectedVnstatVersion.value.trim() || defaultVnstatVersionOption.value
  installingVnstat.value = true
  try {
    const msg = await HttpUtils.post('api/traffic-overview-vnstat-install', {
      version: targetVersion,
    }, {
      headers: {
        'Content-Type': 'application/json',
      },
    })
    if (msg.success && msg.obj) {
      applyOverview(msg.obj as Partial<TrafficOverview>, { forceSyncDraft: true })
      const afterVersion = overview.value.vnstat.version.trim()
      if (beforeVersion === '') {
        push.success({
          duration: 3500,
          message: `vnstat 已安装，当前版本：${afterVersion || '未知版本'}`,
        })
      } else if (afterVersion !== '' && afterVersion !== beforeVersion) {
        push.success({
          duration: 3500,
          message: `vnstat 已重装：${beforeVersion} -> ${afterVersion}`,
        })
      } else {
        push.success({
          duration: 3500,
          message: 'vnstat 已重装，现有流量数据已保留',
        })
      }
      selectedVnstatVersion.value = targetVersion
      vnstatUpdateInfo.value = createIdleVnstatUpdateInfo(overview.value.vnstat)
    }
  } finally {
    installingVnstat.value = false
  }
}

const checkVnstatUpdate = async (silent = false) => {
  checkingVnstatUpdate.value = true
  try {
    const msg = await HttpUtils.get('api/traffic-overview-vnstat-update-info')
    if (!msg.success) {
      return
    }
    vnstatUpdateInfo.value = normalizeVnstatUpdateInfo(msg.obj)
    if (!silent && vnstatUpdateInfo.value.message.trim() !== '') {
      push.success({
        duration: 4200,
        message: vnstatUpdateInfo.value.message,
      })
    }
  } finally {
    checkingVnstatUpdate.value = false
  }
}

const removeVnstat = async () => {
  const confirmed = window.confirm(overview.value.vnstat.managed ? removeVnstatConfirmText : removeExternalVnstatConfirmText)
  if (!confirmed) {
    return
  }
  removingVnstat.value = true
  try {
    const msg = await HttpUtils.post('api/traffic-overview-vnstat-remove', {})
    if (msg.success && msg.obj) {
      applyOverview(msg.obj as Partial<TrafficOverview>, { forceSyncDraft: true })
      push.success({
        duration: 3500,
        message: 'vnstat 已删除，流量统计数据已清理',
      })
      selectedVnstatVersion.value = defaultVnstatVersionOption.value
      vnstatUpdateInfo.value = createIdleVnstatUpdateInfo(overview.value.vnstat)
    }
  } finally {
    removingVnstat.value = false
  }
}

const saveTrafficSettings = async () => {
  savingSettings.value = true
  const payload = {
    limit_gib: normalizeLimitGiB(limitGiBInput.value),
    reset_day: normalizeResetDay(resetDayInput.value),
  }
  try {
    const msg = await HttpUtils.post('api/traffic-overview-settings', payload, {
      headers: {
        'Content-Type': 'application/json',
      },
    })
    if (msg.success && msg.obj) {
      applyOverview(msg.obj as Partial<TrafficOverview>, { forceSyncDraft: true })
    }
  } finally {
    savingSettings.value = false
  }
}

const confirmResetPeriodTraffic = async () => {
  const confirmed = window.confirm(resetPeriodConfirmText)
  if (!confirmed) {
    return
  }
  resettingPeriod.value = true
  try {
    const msg = await HttpUtils.post('api/traffic-overview-period-reset', {})
    if (msg.success && msg.obj) {
      applyOverview(msg.obj as Partial<TrafficOverview>)
    }
  } finally {
    resettingPeriod.value = false
  }
}

const confirmResetTotalTraffic = async () => {
  const confirmed = window.confirm(resetTotalConfirmText)
  if (!confirmed) {
    return
  }
  resettingTotal.value = true
  try {
    const msg = await HttpUtils.post('api/traffic-overview-total-reset', {})
    if (msg.success && msg.obj) {
      applyOverview(msg.obj as Partial<TrafficOverview>)
    }
  } finally {
    resettingTotal.value = false
  }
}

const formatGB = (bytes: number) => {
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return '0.00 GB'
  }
  let gb = bytes / (1024 * 1024 * 1024)
  if (gb > 0 && gb < 0.01) {
    gb = 0.01
  }
  return `${gb.toFixed(2)} GB`
}

const stopPolling = () => {
  if (pollingTimer != null) {
    window.clearInterval(pollingTimer)
    pollingTimer = null
  }
}

const startPolling = () => {
  stopPolling()
  if (!props.active) {
    return
  }
  if (typeof document !== 'undefined' && document.visibilityState !== 'visible') {
    return
  }
  pollingTimer = window.setInterval(() => {
    fetchOverview(true)
  }, 3000)
}

const handleVisibilityChange = () => {
  if (document.visibilityState === 'visible') {
    void fetchOverview(true)
    startPolling()
    return
  }
  stopPolling()
}

watch(() => props.active, (active) => {
  if (active) {
    void fetchOverview(true)
    startPolling()
    return
  }
  stopPolling()
})

onMounted(() => {
  void fetchOverview()
  startPolling()
  if (typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', handleVisibilityChange)
  }
})

onBeforeUnmount(() => {
  stopPolling()
  if (typeof document !== 'undefined') {
    document.removeEventListener('visibilitychange', handleVisibilityChange)
  }
})
</script>

<style scoped>
.settings-traffic-manage {
  min-height: 420px;
  width: 100%;
}

.traffic-card-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.traffic-toolbar {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 10px;
  flex-wrap: wrap;
}

.traffic-runtime__actions {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  flex-wrap: wrap;
}

.traffic-version-select {
  flex: 1 1 280px;
  min-width: 220px;
}

.traffic-runtime__button-group {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
}

.metric-card {
  padding: 12px;
  min-height: 110px;
  position: relative;
}

.accum-badge {
  position: absolute;
  right: 12px;
  bottom: 8px;
  border: 1px solid #f5d000;
  color: #f5d000;
  border-radius: 4px;
  padding: 1px 8px;
  font-size: 12px;
  line-height: 18px;
  min-width: 88px;
  text-align: center;
}

.traffic-runtime__rows {
  border-top: 1px dashed rgba(var(--v-border-color), var(--v-border-opacity));
}

.traffic-runtime__row {
  display: grid;
  grid-template-columns: minmax(90px, 140px) minmax(0, 1fr);
  gap: 12px;
  align-items: center;
  min-height: 38px;
  border-bottom: 1px dashed rgba(var(--v-border-color), var(--v-border-opacity));
  font-size: 13px;
}

.traffic-runtime__row span {
  color: rgba(var(--v-theme-on-surface), 0.72);
}

.traffic-runtime__row strong {
  min-width: 0;
  text-align: right;
  overflow-wrap: anywhere;
}

.traffic-code {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace;
}

.traffic-settings__actions {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
}

.traffic-settings__actions .v-btn {
  flex: 1 1 140px;
}

@media (max-width: 720px) {
  .traffic-toolbar {
    justify-content: flex-start;
    width: 100%;
  }

  .traffic-runtime__button-group {
    width: 100%;
  }

  .traffic-runtime__button-group .v-btn {
    flex: 1 1 140px;
  }

  .traffic-runtime__row {
    grid-template-columns: 1fr;
    gap: 2px;
    padding: 8px 0;
  }

  .traffic-runtime__row strong {
    text-align: left;
  }
}
</style>
