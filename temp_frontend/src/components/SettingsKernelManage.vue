<template>
  <div class="settings-kernel-manage">
    <v-card rounded="lg" variant="outlined" :loading="loadingOverview || loadingPackages">
      <v-card-title class="d-flex align-center justify-space-between ga-3 flex-wrap">
        <v-select
          v-model="provider"
          :items="providerItems"
          item-title="label"
          item-value="value"
          density="compact"
          variant="outlined"
          hide-details
          class="kernel-provider-select"
          :label="t('kernelManager.provider')"
          :placeholder="t('kernelManager.providerPlaceholder')"
          clearable
          :disabled="busy || !canManageKernelPackages" />
        <v-chip size="small" :color="providerStatusColor">
          {{ providerStatusText }}
        </v-chip>
      </v-card-title>
      <v-divider />
      <v-card-text>
        <v-alert
          v-if="runtimeSupportMessage"
          type="warning"
          variant="tonal"
          density="comfortable"
          class="mb-4">
          {{ runtimeSupportMessage }}
        </v-alert>

        <v-alert
          v-if="!hasProvider && canManageKernelPackages"
          type="info"
          variant="tonal"
          density="comfortable"
          class="mb-4">
          {{ t('kernelManager.providerHint') }}
        </v-alert>

        <v-alert
          v-if="hasProvider && !overview.supported && overview.reason"
          type="warning"
          variant="tonal"
          density="comfortable"
          class="mb-4">
          {{ overview.reason }}
        </v-alert>

        <v-row v-if="hasProvider && canManageKernelPackages" class="mb-2">
          <template v-if="isXanMod">
            <v-col cols="12" sm="6" md="4">
              <v-select
                v-model="selectedLine"
                :items="lineItems"
                item-title="label"
                item-value="value"
                :label="t('kernelManager.line')"
                :placeholder="t('kernelManager.linePlaceholder')"
                clearable
                :disabled="busy || !canManageKernelPackages" />
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-select
                v-model="selectedVersion"
                :items="versionItems"
                item-title="name"
                item-value="name"
                :label="t('kernelManager.version')"
                :placeholder="t('kernelManager.versionPlaceholder')"
                clearable
                :disabled="busy || !canManageKernelPackages || versionItems.length === 0" />
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-select
                v-model="selectedArch"
                :items="archItems"
                item-title="arch"
                item-value="arch"
                :label="t('kernelManager.arch')"
                :placeholder="t('kernelManager.archPlaceholder')"
                clearable
                :disabled="busy || !canManageKernelPackages || archItems.length === 0" />
            </v-col>
          </template>
          <template v-else>
            <v-col cols="12" sm="6" md="4">
              <v-select
                v-model="selectedVersion"
                :items="versionItems"
                item-title="name"
                item-value="name"
                :label="t('kernelManager.version')"
                :placeholder="t('kernelManager.versionPlaceholder')"
                clearable
                :disabled="busy || !canManageKernelPackages || versionItems.length === 0" />
            </v-col>
          </template>
        </v-row>

        <div class="text-caption text-medium-emphasis mb-3">
          {{ t('kernelManager.currentKernel') }}: {{ activeOverview.currentKernel || '-' }}
          <span class="mx-2">|</span>
          {{ t('kernelManager.downloadDir') }}: {{ downloadDirText }}
        </div>

        <v-alert
          v-if="feedback.message"
          :type="feedback.type"
          variant="tonal"
          density="comfortable"
          class="mb-4">
          {{ feedback.message }}
        </v-alert>

        <v-table density="comfortable" class="mb-4">
          <thead>
            <tr>
              <th>{{ t('kernelManager.packageName') }}</th>
              <th>{{ t('kernelManager.packageType') }}</th>
              <th>{{ t('kernelManager.directLink') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="pkg in packages" :key="pkg.name">
              <td>{{ pkg.name }}</td>
              <td>{{ pkg.type }}</td>
              <td>
                <a :href="pkg.downloadUrl" target="_blank" rel="noopener noreferrer">{{ t('kernelManager.open') }}</a>
              </td>
            </tr>
            <tr v-if="packages.length === 0">
              <td colspan="3" class="text-center text-medium-emphasis">{{ t('noData') }}</td>
            </tr>
          </tbody>
        </v-table>

        <v-row class="kernel-download-actions">
          <v-col cols="12" md="4">
            <v-btn
              class="kernel-action-btn kernel-action-btn--download"
              block
              color="primary"
              :prepend-icon="kernelDownloadTaskActive ? (kernelDownloadTaskApplying ? 'mdi-progress-wrench' : 'mdi-stop') : 'mdi-download'"
              :disabled="kernelDownloadTaskActive
                ? kernelDownloadStopRequestPending || !downloadProgress.canCancel
                : !canDownload"
              @click="kernelDownloadTaskActive ? stopKernelDownload() : downloadPackages()">
              {{ kernelDownloadButtonText }}
            </v-btn>
          </v-col>
          <v-col cols="12" md="4">
            <v-btn
              class="kernel-action-btn kernel-action-btn--install"
              block
              color="secondary"
              prepend-icon="mdi-package-variant-closed-check"
              :loading="installing"
              :disabled="!canInstall"
              @click="installPackages">
              {{ t('kernelManager.install') }}
            </v-btn>
          </v-col>
          <v-col cols="12" md="4">
            <v-btn
              class="kernel-action-btn kernel-action-btn--reboot"
              block
              color="warning"
              prepend-icon="mdi-restart-alert"
              :loading="rebooting"
              :disabled="!canRebootHost || operationBusy"
              @click="rebootHost">
              {{ t('kernelManager.reboot') }}
            </v-btn>
          </v-col>
        </v-row>

        <v-row v-if="hasKernelDownloadProgress" class="mt-1">
          <v-col cols="12">
            <div class="kernel-download-progress" aria-live="polite">
              <div class="d-flex align-center justify-space-between flex-wrap" style="gap: 8px;">
                <span class="font-weight-medium kernel-download-progress__status">{{ kernelDownloadStatusText }}</span>
                <span class="text-caption text-medium-emphasis">{{ downloadProgressText }}</span>
              </div>
              <v-progress-linear
                class="mt-2"
                :indeterminate="kernelDownloadTaskActive && downloadProgress.totalBytes <= 0"
                :model-value="downloadProgress.percent"
                :color="kernelDownloadProgressColor"
                height="8"
                rounded />
              <div class="kernel-download-progress__detail text-caption text-medium-emphasis mt-2">
                <span v-if="downloadProgress.currentPackage">{{ downloadProgress.currentPackage }}</span>
                <span v-else-if="downloadProgress.error">{{ downloadProgress.error }}</span>
                <span v-else-if="downloadProgress.phase">{{ downloadProgress.phase }}</span>
              </div>
            </div>
          </v-col>
        </v-row>

        <div class="text-caption text-medium-emphasis mt-3">
          {{ t('kernelManager.rebootNotice') }}
        </div>
        <div v-if="hasDownloadedKernel" class="d-flex align-center justify-space-between ga-2 mt-3 flex-wrap">
          <div class="text-caption text-medium-emphasis">
            {{ t('kernelManager.downloadedKernel') }}: {{ downloadedKernelLabel }}
            <span class="mx-2">|</span>
            {{ downloadedKernelDirectory }}
          </div>
          <v-btn
            size="small"
            variant="tonal"
            color="error"
            prepend-icon="mdi-delete"
            :disabled="!canManageKernelPackages || operationBusy"
            @click="clearDownloadedKernel">
            {{ t('kernelManager.clearDownloaded') }}
          </v-btn>
        </div>
      </v-card-text>
    </v-card>

    <v-card rounded="lg" variant="outlined" class="mt-4" :loading="cleanupLoading">
      <v-card-title class="d-flex align-center justify-space-between ga-3 flex-wrap">
        <div class="text-subtitle-1 font-weight-medium">{{ t('kernelManager.cleanupTitle') }}</div>
        <v-chip size="small" color="info">{{ t('kernelManager.pinnedKernel') }}: {{ cleanupPinnedKernelText }}</v-chip>
      </v-card-title>
      <v-divider />
      <v-card-text>
        <div class="text-caption text-medium-emphasis mb-3">
          {{ t('kernelManager.currentKernel') }}: {{ cleanupCurrentKernelText }}
        </div>

        <v-alert
          v-if="cleanupWarningMessage"
          type="warning"
          variant="tonal"
          density="comfortable"
          class="mb-4">
          {{ cleanupWarningMessage }}
        </v-alert>

        <v-row class="mb-2">
          <v-col cols="12" md="4">
            <v-btn
              class="kernel-action-btn kernel-action-btn--scan"
              block
              color="primary"
              prepend-icon="mdi-magnify"
              :loading="cleanupLoading"
              :disabled="!canManageKernelCleanup || operationBusy"
              @click="scanCleanupPackages(true)">
              {{ t('kernelManager.cleanupScan') }}
            </v-btn>
          </v-col>
          <v-col cols="12" md="4">
            <v-btn
              class="kernel-action-btn kernel-action-btn--purge"
              block
              color="error"
              prepend-icon="mdi-delete-sweep"
              :loading="cleanupPurging"
              :disabled="!canManageKernelCleanup || operationBusy || cleanupSelectedPackages.length === 0"
              @click="purgeSelectedCleanupPackages">
              {{ t('kernelManager.cleanupPurgeSelected') }} ({{ cleanupSelectedPackages.length }})
            </v-btn>
          </v-col>
          <v-col cols="12" md="4">
            <v-btn
              class="kernel-action-btn kernel-action-btn--auto-clean"
              block
              color="warning"
              prepend-icon="mdi-auto-fix"
              :loading="cleanupAutoPurging"
              :disabled="!canManageKernelCleanup || operationBusy || !cleanupHasScanned"
              @click="autoCleanupKernelPackages">
              {{ t('kernelManager.cleanupAuto') }}
            </v-btn>
          </v-col>
        </v-row>

        <div class="d-flex align-center justify-space-between mb-2 flex-wrap ga-2">
          <div class="text-caption text-medium-emphasis">
            {{ t('kernelManager.cleanupSelectHint') }}
          </div>
          <v-checkbox
            :model-value="cleanupSelectAllChecked"
            :label="t('kernelManager.cleanupSelectAll')"
            hide-details
            density="compact"
            :disabled="!canManageKernelCleanup || operationBusy || cleanupPackages.length === 0"
            @update:model-value="toggleCleanupSelectAll" />
        </div>

        <v-table density="comfortable">
          <thead>
            <tr>
              <th style="width: 56px;">{{ t('kernelManager.cleanupSelect') }}</th>
              <th>{{ t('kernelManager.packageName') }}</th>
              <th>{{ t('kernelManager.cleanupStatus') }}</th>
              <th>{{ t('kernelManager.cleanupTag') }}</th>
              <th>{{ t('kernelManager.cleanupRisk') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="pkg in cleanupPackages" :key="pkg.name">
              <td>
                <v-checkbox
                  :model-value="cleanupSelectedMap[pkg.name] === true"
                  hide-details
                  density="compact"
                  :disabled="!canManageKernelCleanup || operationBusy"
                  @update:model-value="toggleCleanupSelection(pkg.name, $event)" />
              </td>
              <td>{{ pkg.name }}</td>
              <td>{{ pkg.status || '-' }}</td>
              <td>
                <v-chip v-if="pkg.isPinnedKernel" size="x-small" color="success" variant="flat" class="mr-1">{{ t('kernelManager.cleanupPinned') }}</v-chip>
                <v-chip v-if="pkg.isCurrentKernel" size="x-small" color="info" variant="flat" class="mr-1">{{ t('kernelManager.cleanupCurrent') }}</v-chip>
                <v-chip v-if="pkg.isImage" size="x-small" color="primary" variant="flat" class="mr-1 kernel-cleanup-tag--image">image</v-chip>
                <v-chip v-if="pkg.isHeaders" size="x-small" color="secondary" variant="flat" class="mr-1">headers</v-chip>
              </td>
              <td>
                <v-chip
                  size="x-small"
                  variant="flat"
                  :color="pkg.risk === 'high' ? 'error' : 'success'"
                  :class="[
                    'kernel-cleanup-risk-chip',
                    pkg.risk === 'high' ? 'kernel-cleanup-risk-chip--high' : 'kernel-cleanup-risk-chip--normal',
                  ]">
                  {{ pkg.risk === 'high' ? t('kernelManager.cleanupRiskHigh') : t('kernelManager.cleanupRiskNormal') }}
                </v-chip>
              </td>
            </tr>
            <tr v-if="cleanupPackages.length === 0">
              <td colspan="5" class="text-center text-medium-emphasis">{{ t('noData') }}</td>
            </tr>
          </tbody>
        </v-table>
      </v-card-text>
    </v-card>

    <v-overlay :model-value="rebootOverlay" class="align-center justify-center" persistent>
      <v-card width="380" rounded="lg">
        <v-card-text class="text-center py-8">
          <v-progress-circular indeterminate size="52" width="5" color="primary" class="mb-4" />
          <div class="text-subtitle-1 font-weight-medium">{{ t('kernelManager.rebootingTitle') }}</div>
          <div class="text-caption text-medium-emphasis mt-2">{{ t('kernelManager.rebootingDesc') }}</div>
        </v-card-text>
      </v-card>
    </v-overlay>
  </div>
</template>

<script setup lang="ts">
import HttpUtils from '@/plugins/httputil'
import { confirm } from '@/plugins/confirm'
import router from '@/router'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { Ref } from 'vue'
import { useI18n } from 'vue-i18n'

type KernelOverview = {
  supported: boolean
  linux: boolean
  reason: string
  currentKernel: string
  downloadRoot: string
  downloadedKernel?: string
  downloadedDirectory?: string
  downloadedProvider?: string
  downloadedLine?: string
  downloadedVersion?: string
  downloadedArch?: string
}

type KernelVersionItem = { name: string }
type KernelArchItem = { arch: string; dirName: string }
type KernelPackageItem = { name: string; type: string; downloadUrl: string }
type KernelCleanupPackageItem = {
  name: string
  status: string
  isImage: boolean
  isHeaders: boolean
  isPinnedKernel: boolean
  isCurrentKernel: boolean
  risk: string
}

type KernelDownloadProgress = {
  id: string
  status: string
  state: string
  phase: string
  canCancel: boolean
  stopRequested: boolean
  deadlineExceeded: boolean
  percent: number
  approximate: boolean
  downloadedBytes: number
  totalBytes: number
  currentPackage: string
  downloadedCount: number
  totalCount: number
  error: string
  startedAt: number
  updatedAt: number
  deadlineAt: number
  finishedAt: number
}

type KernelSystemCleanupInfo = {
  done: boolean
  warnings: string[]
  summary: string
}

type KernelProvider = 'xanmod' | 'bbrplus'

const kernelDownloadRequestTimeout = 35 * 1000
const kernelPackageOperationTimeout = 41 * 60 * 1000
const kernelAutoCleanupRequestTimeout = 21 * 60 * 1000

const props = withDefaults(defineProps<{ active?: boolean }>(), {
  active: false,
})

const { t } = useI18n()
const providerItems = [
  { label: 'XanMod', value: 'xanmod' },
  { label: 'bbrplus', value: 'bbrplus' },
]
const lineItems = [
  { label: 'LTS', value: 'lts' },
  { label: 'MAIN', value: 'main' },
  { label: 'RT', value: 'rt' },
  { label: 'EDGE', value: 'edge' },
]

const loadingOverview = ref(false)
const loadingPackages = ref(false)
const downloadStartPending = ref(false)
const kernelDownloadStopRequestPending = ref(false)
const installing = ref(false)
const rebooting = ref(false)
const cleanupLoading = ref(false)
const cleanupPurging = ref(false)
const cleanupAutoPurging = ref(false)
const rebootOverlay = ref(false)
const reconnectTimerId = ref<number | null>(null)
const downloadProgressSessionId = ref('')
const downloadProgressTimerId = ref<number | null>(null)
let downloadProgressRequest: Promise<void> | null = null
const provider = ref('')
const kernelSelectionHydrating = ref(false)
const selectedLine = ref('')
const selectedVersion = ref('')
const selectedArch = ref('')

const createEmptyKernelOverview = (): KernelOverview => ({
  supported: false,
  linux: false,
  reason: '',
  currentKernel: '',
  downloadRoot: '',
  downloadedKernel: '',
  downloadedDirectory: '',
  downloadedProvider: '',
  downloadedLine: '',
  downloadedVersion: '',
  downloadedArch: '',
})

const overview = ref<KernelOverview>(createEmptyKernelOverview())
const runtimeOverview = ref<KernelOverview>(createEmptyKernelOverview())
const runtimeChecked = ref(false)

const versionItems = ref<KernelVersionItem[]>([])
const archItems = ref<KernelArchItem[]>([])
const packages = ref<KernelPackageItem[]>([])
const downloadDirectory = ref('')
const cleanupCurrentKernel = ref('')
const cleanupPinnedKernel = ref('')
const cleanupPackages = ref<KernelCleanupPackageItem[]>([])
const cleanupSelectedMap = ref<Record<string, boolean>>({})
const cleanupHasScanned = ref(false)
const downloadProgress = ref<KernelDownloadProgress>({
  id: '',
  status: 'missing',
  state: 'idle',
  phase: '',
  canCancel: false,
  stopRequested: false,
  deadlineExceeded: false,
  percent: 0,
  approximate: false,
  downloadedBytes: 0,
  totalBytes: 0,
  currentPackage: '',
  downloadedCount: 0,
  totalCount: 0,
  error: '',
  startedAt: 0,
  updatedAt: 0,
  deadlineAt: 0,
  finishedAt: 0,
})

const kernelDownloadTaskActive = computed(() => (
  ['queued', 'running', 'stopping'].includes(downloadProgress.value.state)
  || (downloadProgress.value.state === '' && downloadProgress.value.status === 'running')
))
const kernelDownloadTaskStopping = computed(() => (
  kernelDownloadTaskActive.value
  && (downloadProgress.value.stopRequested || downloadProgress.value.state === 'stopping')
))
const kernelDownloadTaskApplying = computed(() => (
  kernelDownloadTaskActive.value
  && !kernelDownloadTaskStopping.value
  && downloadProgress.value.canCancel === false
))
const downloading = computed(() => downloadStartPending.value || kernelDownloadTaskActive.value)
const hasKernelDownloadProgress = computed(() => (
  downloadStartPending.value
  || (downloadProgress.value.id !== '' && downloadProgress.value.status !== 'missing')
))
const kernelDownloadButtonText = computed(() => {
  if (downloadStartPending.value) return '正在提交'
  if (kernelDownloadStopRequestPending.value || kernelDownloadTaskStopping.value) return '正在停止'
  if (kernelDownloadTaskActive.value) return downloadProgress.value.canCancel ? '停止' : '正在应用'
  return t('kernelManager.download')
})
const kernelDownloadProgressColor = computed(() => {
  if (downloadProgress.value.state === 'success' || downloadProgress.value.status === 'success') return 'success'
  if (['error', 'cancelled', 'timed_out'].includes(downloadProgress.value.state) || downloadProgress.value.status === 'error') return 'error'
  return 'primary'
})
const kernelDownloadStatusText = computed(() => {
  const state = downloadProgress.value.state
  const phase = downloadProgress.value.phase
  if (downloadStartPending.value) return '正在提交内核下载任务'
  if (kernelDownloadTaskStopping.value) return '正在停止内核下载任务'
  if (kernelDownloadTaskApplying.value) return phase ? `正在应用：${phase}` : '正在应用内核下载结果'
  if (kernelDownloadTaskActive.value) return phase ? `正在下载：${phase}` : '正在下载内核包'
  if (state === 'success' || downloadProgress.value.status === 'success') return '内核包下载完成'
  if (state === 'cancelled') return '内核包下载已停止，临时文件已清理'
  if (state === 'timed_out') return '内核包下载超时，临时文件已清理'
  if (state === 'error' || downloadProgress.value.status === 'error') return '内核包下载失败'
  return phase || '内核下载状态'
})

const feedback = ref<{ type: 'success' | 'warning' | 'error' | 'info'; message: string }>({
  type: 'info',
  message: '',
})
let downloadFeedbackTimer: number | null = null

const createLoadingGuard = (loadingRef: Ref<boolean>) => {
  let pendingCount = 0
  return () => {
    pendingCount += 1
    loadingRef.value = true
    return () => {
      pendingCount = Math.max(0, pendingCount - 1)
      if (pendingCount === 0) {
        loadingRef.value = false
      }
    }
  }
}

const beginOverviewLoading = createLoadingGuard(loadingOverview)
const beginPackageLoading = createLoadingGuard(loadingPackages)
const beginCleanupLoading = createLoadingGuard(cleanupLoading)

let selectionRequestTokenSeed = 0
let latestSelectionRequestToken = 0
let cleanupScanRequestTokenSeed = 0
let latestCleanupScanRequestToken = 0

const beginSelectionRequest = () => {
  const token = ++selectionRequestTokenSeed
  latestSelectionRequestToken = token
  return token
}

const isLatestSelectionRequest = (token: number) => token === latestSelectionRequestToken

const beginCleanupScanRequest = () => {
  const token = ++cleanupScanRequestTokenSeed
  latestCleanupScanRequestToken = token
  return token
}

const isLatestCleanupScanRequest = (token: number) => token === latestCleanupScanRequestToken

const normalizeKernelProviderSelection = (value: unknown): KernelProvider | '' => {
  const normalized = String(value ?? '').trim().toLowerCase()
  if (normalized === 'xanmod' || normalized === 'bbrplus') {
    return normalized
  }
  return ''
}

const selectedProvider = computed(() => normalizeKernelProviderSelection(provider.value))
const hasProvider = computed(() => selectedProvider.value !== '')
const activeOverview = computed(() => (hasProvider.value ? overview.value : runtimeOverview.value))
const runtimePackageSupportAvailable = computed(() => runtimeChecked.value && runtimeOverview.value.supported)
const runtimeLinuxAvailable = computed(() => runtimeChecked.value && runtimeOverview.value.linux)
const runtimeSupportMessage = computed(() => {
  if (!runtimeChecked.value) {
    return ''
  }
  if (!runtimeLinuxAvailable.value || !runtimePackageSupportAvailable.value) {
    return String(runtimeOverview.value.reason || '').trim()
  }
  return ''
})
const canManageKernelPackages = computed(() => runtimePackageSupportAvailable.value)
const canManageKernelCleanup = computed(() => runtimeLinuxAvailable.value)

const operationBusy = computed(() => (
  downloading.value ||
  installing.value ||
  rebooting.value ||
  cleanupLoading.value ||
  cleanupPurging.value ||
  cleanupAutoPurging.value
))
const busy = computed(() => (
  loadingOverview.value ||
  loadingPackages.value ||
  operationBusy.value
))
const providerStatusText = computed(() => {
  if (!hasProvider.value) {
    return t('kernelManager.providerEmpty')
  }
  return overview.value.supported ? t('kernelManager.supported') : t('kernelManager.unsupported')
})
const providerStatusColor = computed(() => {
  if (!hasProvider.value) {
    return 'info'
  }
  return overview.value.supported ? 'success' : 'warning'
})
const isXanMod = computed(() => selectedProvider.value === 'xanmod')
const downloadDirText = computed(() => downloadDirectory.value || activeOverview.value.downloadRoot || '-')
// A completed download belongs to the host, rather than to the selection that
// happens to be visible in the form. Keep install availability independent of
// provider/line/version changes made after a download has completed.
const downloadedKernelLabel = computed(() => String(runtimeOverview.value.downloadedKernel || '').trim())
const downloadedKernelDirectory = computed(() => String(runtimeOverview.value.downloadedDirectory || '').trim())
const hasDownloadedKernel = computed(() => (
  downloadedKernelLabel.value.length > 0 && downloadedKernelDirectory.value.length > 0
))
const packageListHasKernelPair = computed(() => (
  packages.value.some(item => String(item.type || '').trim().toLowerCase() === 'image') &&
  packages.value.some(item => String(item.type || '').trim().toLowerCase() === 'headers')
))
const isKernelSelectionComplete = computed(() => {
  if (!hasProvider.value || !canManageKernelPackages.value || !selectedVersion.value || !packageListHasKernelPair.value) {
    return false
  }
  if (isXanMod.value) {
    return selectedLine.value.length > 0 && selectedArch.value.length > 0
  }
  return true
})
const canDownload = computed(() => (
  isKernelSelectionComplete.value &&
  !loadingPackages.value &&
  !operationBusy.value
))
const canInstall = computed(() => (
  canManageKernelPackages.value &&
  hasDownloadedKernel.value &&
  !operationBusy.value
))
const canRebootHost = computed(() => runtimeLinuxAvailable.value)
const cleanupCurrentKernelText = computed(() => cleanupCurrentKernel.value || activeOverview.value.currentKernel || '-')
const cleanupPinnedKernelText = computed(() => cleanupPinnedKernel.value || '-')
const cleanupSelectedPackages = computed(() => (
  cleanupPackages.value
    .map(item => item.name)
    .filter(name => cleanupSelectedMap.value[name] === true)
))
const cleanupSelectAllChecked = computed(() => (
  cleanupPackages.value.length > 0 && cleanupSelectedPackages.value.length === cleanupPackages.value.length
))
const cleanupWarningMessage = computed(() => (
  cleanupPackages.value.some(item => item.risk === 'high') ? t('kernelManager.cleanupRiskWarning') : ''
))
const downloadProgressText = computed(() => {
  const percent = Math.max(0, Math.min(100, Number(downloadProgress.value.percent) || 0))
  const percentText = `${downloadProgress.value.approximate ? '~' : ''}${percent.toFixed(1)}%`
  const downloaded = formatMiB(downloadProgress.value.downloadedBytes)
  const total = formatMiB(downloadProgress.value.totalBytes)
  return `${percentText} (${downloaded}/${total})`
})

const setFeedback = (type: 'success' | 'warning' | 'error' | 'info', message: string) => {
  feedback.value = { type, message }
}

const clearFeedback = () => {
  if (downloadFeedbackTimer != null) {
    window.clearTimeout(downloadFeedbackTimer)
    downloadFeedbackTimer = null
  }
  feedback.value.message = ''
}

const showTransientDownloadFeedback = (type: 'success' | 'warning' | 'error' | 'info', message: string, duration = 4500) => {
  if (downloadFeedbackTimer != null) {
    window.clearTimeout(downloadFeedbackTimer)
  }
  feedback.value = { type, message }
  downloadFeedbackTimer = window.setTimeout(() => {
    if (feedback.value.message === message) {
      feedback.value.message = ''
    }
    downloadFeedbackTimer = null
  }, duration)
}

const normalizeKernelSystemCleanup = (raw: any): KernelSystemCleanupInfo => {
  const warnings = Array.isArray(raw?.systemCleanupWarnings)
    ? raw.systemCleanupWarnings
      .map((item: unknown) => String(item ?? '').trim())
      .filter((item: string) => item.length > 0)
    : []
  const summary = String(raw?.systemCleanupSummary ?? '').trim()
  const doneFlag = raw?.systemCleanupDone
  const done = doneFlag === false ? false : true
  return {
    done,
    warnings,
    summary,
  }
}

const applyKernelSuccessFeedback = (
  baseMessage: string,
  raw: any,
  baseType: 'success' | 'warning' = 'success',
) => {
  const cleanup = normalizeKernelSystemCleanup(raw)
  if (!cleanup.done || cleanup.warnings.length > 0) {
    const detail = cleanup.summary || cleanup.warnings.join('; ')
    setFeedback(baseType, detail ? `${baseMessage} ${detail}` : baseMessage)
    return
  }
  setFeedback(baseType, baseMessage)
}

const formatMiB = (value: number) => {
  const bytes = Number.isFinite(value) ? Math.max(0, value) : 0
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

const normalizeKernelDownloadProgress = (raw: any): KernelDownloadProgress => ({
  id: String(raw?.id ?? '').trim(),
  status: String(raw?.status ?? '').trim().toLowerCase() || 'missing',
  state: String(raw?.state ?? '').trim().toLowerCase() || 'idle',
  phase: String(raw?.phase ?? '').trim(),
  canCancel: raw?.canCancel === true,
  stopRequested: raw?.stopRequested === true,
  deadlineExceeded: raw?.deadlineExceeded === true,
  percent: Number.isFinite(Number(raw?.percent)) ? Number(raw.percent) : 0,
  approximate: raw?.approximate === true,
  downloadedBytes: Number.isFinite(Number(raw?.downloadedBytes)) ? Math.max(0, Number(raw.downloadedBytes)) : 0,
  totalBytes: Number.isFinite(Number(raw?.totalBytes)) ? Math.max(0, Number(raw.totalBytes)) : 0,
  currentPackage: String(raw?.currentPackage ?? '').trim(),
  downloadedCount: Number.isFinite(Number(raw?.downloadedCount)) ? Math.max(0, Number(raw.downloadedCount)) : 0,
  totalCount: Number.isFinite(Number(raw?.totalCount)) ? Math.max(0, Number(raw.totalCount)) : 0,
  error: String(raw?.error ?? '').trim(),
  startedAt: Number(raw?.startedAt) || 0,
  updatedAt: Number(raw?.updatedAt) || 0,
  deadlineAt: Number(raw?.deadlineAt) || 0,
  finishedAt: Number(raw?.finishedAt) || 0,
})

const resetDownloadProgress = () => {
  downloadProgress.value = {
    id: '',
    status: 'missing',
    state: 'idle',
    phase: '',
    canCancel: false,
    stopRequested: false,
    deadlineExceeded: false,
    percent: 0,
    approximate: false,
    downloadedBytes: 0,
    totalBytes: 0,
    currentPackage: '',
    downloadedCount: 0,
    totalCount: 0,
    error: '',
    startedAt: 0,
    updatedAt: 0,
    deadlineAt: 0,
    finishedAt: 0,
  }
}

const resetKernelSelection = (nextProvider: string) => {
  selectedLine.value = ''
  selectedVersion.value = ''
  selectedArch.value = ''
  versionItems.value = []
  archItems.value = []
  packages.value = []
  downloadDirectory.value = ''
  overview.value = createEmptyKernelOverview()
  cleanupCurrentKernel.value = ''
  cleanupPinnedKernel.value = ''
  cleanupPackages.value = []
  cleanupHasScanned.value = false
  resetCleanupSelection()
  resetDownloadProgress()
}

const normalizeKernelOverview = (raw: any): KernelOverview => ({
  supported: raw?.supported === true,
  linux: raw?.linux === true,
  reason: String(raw?.reason ?? ''),
  currentKernel: String(raw?.currentKernel ?? ''),
  downloadRoot: String(raw?.downloadRoot ?? ''),
  downloadedKernel: String(raw?.downloadedKernel ?? ''),
  downloadedDirectory: String(raw?.downloadedDirectory ?? ''),
  downloadedProvider: String(raw?.downloadedProvider ?? ''),
  downloadedLine: String(raw?.downloadedLine ?? ''),
  downloadedVersion: String(raw?.downloadedVersion ?? ''),
  downloadedArch: String(raw?.downloadedArch ?? ''),
})

const loadRuntimeOverview = async () => {
  const stopLoading = beginOverviewLoading()
  let requestCancelled = false
  try {
    const msg = await HttpUtils.get('api/kernel-overview', { provider: 'xanmod' })
    if (msg.failureKind === 'cancelled') {
      requestCancelled = true
      return
    }
    if (msg.success && msg.obj) {
      runtimeOverview.value = normalizeKernelOverview(msg.obj)
    } else {
      runtimeOverview.value = {
        ...createEmptyKernelOverview(),
        reason: String(msg.msg ?? ''),
      }
    }
  } catch {
    runtimeOverview.value = createEmptyKernelOverview()
  } finally {
    if (!requestCancelled) {
      runtimeChecked.value = true
    }
    stopLoading()
  }
}

const loadOverview = async (requestToken = beginSelectionRequest()) => {
  const currentProvider = selectedProvider.value
  if (!currentProvider) {
    if (isLatestSelectionRequest(requestToken)) {
      overview.value = createEmptyKernelOverview()
    }
    return
  }
  const stopLoading = beginOverviewLoading()
  try {
    const msg = await HttpUtils.get('api/kernel-overview', { provider: currentProvider })
    if (!isLatestSelectionRequest(requestToken)) return
    if (msg.success && msg.obj) {
      overview.value = normalizeKernelOverview(msg.obj)
    }
  } finally {
    stopLoading()
  }
}

const loadVersions = async (requestToken = beginSelectionRequest()) => {
  const currentProvider = selectedProvider.value
  const currentLine = selectedLine.value
  if (!currentProvider) {
    if (isLatestSelectionRequest(requestToken)) {
      versionItems.value = []
      archItems.value = []
      packages.value = []
      downloadDirectory.value = ''
    }
    return
  }
  if (currentProvider === 'xanmod' && !currentLine) {
    if (isLatestSelectionRequest(requestToken)) {
      versionItems.value = []
      archItems.value = []
      packages.value = []
      downloadDirectory.value = ''
      kernelSelectionHydrating.value = true
      selectedVersion.value = ''
      selectedArch.value = ''
      kernelSelectionHydrating.value = false
    }
    return
  }
  const stopLoading = beginPackageLoading()
  try {
    const query: Record<string, string> = { provider: currentProvider }
    if (currentProvider === 'xanmod') {
      query.line = currentLine
    }
    const msg = await HttpUtils.get('api/kernel-versions', query)
    if (!isLatestSelectionRequest(requestToken)) return
    versionItems.value = msg.success && msg.obj?.versions ? msg.obj.versions as KernelVersionItem[] : []
    kernelSelectionHydrating.value = true
    if (!versionItems.value.some(item => item.name === selectedVersion.value)) {
      selectedVersion.value = ''
    }
    selectedArch.value = ''
    kernelSelectionHydrating.value = false
    if (selectedVersion.value.length === 0 && isLatestSelectionRequest(requestToken)) {
      archItems.value = []
      packages.value = []
      downloadDirectory.value = ''
    }
  } finally {
    stopLoading()
  }
}

const loadArches = async (requestToken = beginSelectionRequest()) => {
  const currentProvider = selectedProvider.value
  const currentLine = selectedLine.value
  const currentVersion = selectedVersion.value
  if (currentProvider !== 'xanmod' || !currentLine || !currentVersion) {
    if (isLatestSelectionRequest(requestToken)) {
      archItems.value = []
      selectedArch.value = ''
      packages.value = []
      downloadDirectory.value = ''
    }
    return
  }
  const stopLoading = beginPackageLoading()
  try {
    const msg = await HttpUtils.get('api/kernel-arches', {
      provider: currentProvider,
      line: currentLine,
      version: currentVersion,
    })
    if (!isLatestSelectionRequest(requestToken)) return
    archItems.value = msg.success && msg.obj?.arches ? msg.obj.arches as KernelArchItem[] : []
    kernelSelectionHydrating.value = true
    if (!archItems.value.some(item => item.arch === selectedArch.value)) {
      selectedArch.value = ''
    }
    kernelSelectionHydrating.value = false
    if (!selectedArch.value && isLatestSelectionRequest(requestToken)) {
      packages.value = []
      downloadDirectory.value = ''
    }
  } finally {
    stopLoading()
  }
}

const loadPackages = async (requestToken = beginSelectionRequest()) => {
  const currentProvider = selectedProvider.value
  const currentVersion = selectedVersion.value
  const currentLine = selectedLine.value
  const currentArch = selectedArch.value
  if (!currentProvider) {
    if (isLatestSelectionRequest(requestToken)) {
      packages.value = []
      downloadDirectory.value = ''
    }
    return
  }
  if (!currentVersion) {
    if (isLatestSelectionRequest(requestToken)) {
      packages.value = []
      downloadDirectory.value = ''
    }
    return
  }
  if (currentProvider === 'xanmod' && (!currentLine || !currentArch)) {
    if (isLatestSelectionRequest(requestToken)) {
      packages.value = []
      downloadDirectory.value = ''
    }
    return
  }
  const stopLoading = beginPackageLoading()
  try {
    const query: Record<string, string> = {
      provider: currentProvider,
      version: currentVersion,
    }
    if (currentProvider === 'xanmod') {
      query.line = currentLine
      query.arch = currentArch
    }
    const msg = await HttpUtils.get('api/kernel-packages', query)
    if (!isLatestSelectionRequest(requestToken)) return
    packages.value = msg.success && msg.obj?.packages ? msg.obj.packages as KernelPackageItem[] : []
    downloadDirectory.value = msg.success ? String(msg.obj?.directory ?? '') : ''
  } finally {
    stopLoading()
  }
}

const normalizeCleanupPackage = (raw: any): KernelCleanupPackageItem => ({
  name: String(raw?.name ?? '').trim(),
  status: String(raw?.status ?? '').trim(),
  isImage: raw?.isImage === true,
  isHeaders: raw?.isHeaders === true,
  isPinnedKernel: raw?.isPinnedKernel === true,
  isCurrentKernel: raw?.isCurrentKernel === true,
  risk: String(raw?.risk ?? '').trim().toLowerCase() === 'high' ? 'high' : 'normal',
})

const resetCleanupSelection = () => {
  cleanupSelectedMap.value = {}
}

const applyCleanupScanResult = (obj: any) => {
  cleanupCurrentKernel.value = String(obj?.currentKernel ?? '').trim()
  cleanupPinnedKernel.value = String(obj?.pinnedKernel ?? '').trim()
  const list = Array.isArray(obj?.packages) ? obj.packages : []
  cleanupPackages.value = list
    .map((item: unknown) => normalizeCleanupPackage(item))
    .filter((item: KernelCleanupPackageItem) => item.name.length > 0)
  resetCleanupSelection()
  cleanupHasScanned.value = true
}

const scanCleanupPackages = async (needConfirm = false, requestToken?: number) => {
  const currentRequestToken = requestToken ?? beginCleanupScanRequest()
  if (!runtimeLinuxAvailable.value) {
    if (isLatestCleanupScanRequest(currentRequestToken)) {
      cleanupPackages.value = []
      cleanupCurrentKernel.value = ''
      cleanupPinnedKernel.value = ''
      cleanupHasScanned.value = false
      resetCleanupSelection()
    }
    return
  }
  if (needConfirm) {
    const confirmed = await confirm({
      message: t('kernelManager.cleanupScanConfirm'),
      severity: 'info',
      confirmText: t('confirmDialog.actions.scan'),
    })
    if (!confirmed || !runtimeLinuxAvailable.value || !isLatestCleanupScanRequest(currentRequestToken)) return
  }
  const stopLoading = beginCleanupLoading()
  try {
    const msg = await HttpUtils.get('api/kernel-cleanup-scan')
    if (!isLatestCleanupScanRequest(currentRequestToken)) return
    if (msg.success && msg.obj) {
      applyCleanupScanResult(msg.obj)
      setFeedback('info', t('kernelManager.cleanupScanDone', { count: cleanupPackages.value.length }))
    } else if (!msg.success) {
      setFeedback('error', String(msg.msg || t('kernelManager.cleanupScanFailed')))
    }
  } finally {
    stopLoading()
  }
}

const toggleCleanupSelection = (name: string, checked: unknown) => {
  const key = String(name || '').trim()
  if (!key) return
  const enabled = checked === true
  cleanupSelectedMap.value = {
    ...cleanupSelectedMap.value,
    [key]: enabled,
  }
}

const toggleCleanupSelectAll = (checked: unknown) => {
  const enabled = checked === true
  const next: Record<string, boolean> = {}
  for (const item of cleanupPackages.value) {
    next[item.name] = enabled
  }
  cleanupSelectedMap.value = next
}

const purgeSelectedCleanupPackages = async () => {
  if (!canManageKernelCleanup.value || operationBusy.value) {
    return
  }
  const targets = cleanupSelectedPackages.value
  if (targets.length === 0) {
    setFeedback('warning', t('kernelManager.cleanupNeedSelection'))
    return
  }
  const confirmed = await confirm({
    message: t('kernelManager.cleanupPurgeConfirm', { count: targets.length }),
    severity: 'danger',
    confirmText: t('confirmDialog.actions.purge'),
  })
  if (
    !confirmed
    || !canManageKernelCleanup.value
    || operationBusy.value
    || !targets.every(target => cleanupPackages.value.some(item => item.name === target))
  ) return

  cleanupPurging.value = true
  try {
    const msg = await HttpUtils.post('api/kernel-cleanup-purge', { packages: targets }, {
      headers: {
        'Content-Type': 'application/json',
      },
      timeout: kernelPackageOperationTimeout,
    })
    if (msg.success) {
      applyKernelSuccessFeedback(t('kernelManager.cleanupPurgeDone', { count: targets.length }), msg.obj)
      await loadRuntimeOverview()
      await loadOverview()
      await scanCleanupPackages()
    } else {
      setFeedback('error', String(msg.msg || t('kernelManager.cleanupPurgeFailed')))
    }
  } finally {
    cleanupPurging.value = false
  }
}

const autoCleanupKernelPackages = async () => {
  if (!canManageKernelCleanup.value || operationBusy.value || !cleanupHasScanned.value) {
    return
  }
  const confirmed = await confirm({
    message: t('kernelManager.cleanupAutoConfirm'),
    severity: 'danger',
    confirmText: t('confirmDialog.actions.cleanup'),
  })
  if (!confirmed || !canManageKernelCleanup.value || operationBusy.value || !cleanupHasScanned.value) return

  cleanupAutoPurging.value = true
  try {
    const msg = await HttpUtils.post('api/kernel-cleanup-auto', {}, { timeout: kernelAutoCleanupRequestTimeout })
    if (msg.success) {
      const count = Array.isArray(msg.obj?.requested) ? msg.obj.requested.length : 0
      applyKernelSuccessFeedback(t('kernelManager.cleanupAutoDone', { count }), msg.obj)
      await loadRuntimeOverview()
      await loadOverview()
      await scanCleanupPackages()
    } else {
      setFeedback('error', String(msg.msg || t('kernelManager.cleanupAutoFailed')))
    }
  } finally {
    cleanupAutoPurging.value = false
  }
}

const stopDownloadProgressPolling = () => {
  if (downloadProgressTimerId.value != null) {
    window.clearInterval(downloadProgressTimerId.value)
    downloadProgressTimerId.value = null
  }
}

const isTerminalKernelDownload = (progress: KernelDownloadProgress) => (
  ['success', 'error', 'cancelled', 'timed_out'].includes(progress.state)
  || ['success', 'error'].includes(progress.status)
  || (progress.id !== '' && progress.status === 'missing')
)

let completedKernelDownloadTaskID = ''

const clearCompletedKernelDownloadTask = (id: string) => {
  if (downloadProgress.value.id === id && isTerminalKernelDownload(downloadProgress.value)) {
    resetDownloadProgress()
  }
  if (downloadProgressSessionId.value === id) {
    downloadProgressSessionId.value = ''
  }
  kernelDownloadStopRequestPending.value = false
}

const completeKernelDownloadTask = async (progress: KernelDownloadProgress, allowTerminal = true) => {
  if (!progress.id) return
  if (completedKernelDownloadTaskID === progress.id) {
    clearCompletedKernelDownloadTask(progress.id)
    return
  }
  if (!allowTerminal) {
    clearCompletedKernelDownloadTask(progress.id)
    return
  }
  completedKernelDownloadTaskID = progress.id
  kernelDownloadStopRequestPending.value = false
  try {
    if (progress.state === 'success' || progress.status === 'success') {
      const count = Math.max(0, progress.downloadedCount)
      showTransientDownloadFeedback('success', t('kernelManager.downloadDone', { count }))
      await loadRuntimeOverview()
      await loadOverview()
      return
    }
    if (progress.state === 'cancelled') {
      showTransientDownloadFeedback('info', '内核包下载已停止，未完成的临时文件已清理')
      return
    }
    if (progress.state === 'timed_out') {
      showTransientDownloadFeedback('warning', '内核包下载超过 20 分钟，已停止并清理临时文件')
      return
    }
    showTransientDownloadFeedback('error', progress.error || t('kernelManager.downloadFailed'), 5500)
  } finally {
    clearCompletedKernelDownloadTask(progress.id)
  }
}

const applyKernelDownloadProgress = (raw: any, allowTerminal = true) => {
  const nextProgress = normalizeKernelDownloadProgress(raw)
  downloadProgress.value = nextProgress
  if (nextProgress.id !== '') {
    downloadProgressSessionId.value = nextProgress.id
  }
  if (isTerminalKernelDownload(nextProgress)) {
    stopDownloadProgressPolling()
    void completeKernelDownloadTask(nextProgress, allowTerminal)
  }
}

const pollDownloadProgress = async (): Promise<void> => {
  if (downloadProgressRequest) return downloadProgressRequest
  const sessionId = downloadProgressSessionId.value.trim()
  if (!sessionId) return
  const request = (async () => {
    const msg = await HttpUtils.get('api/kernel-download-progress', { id: sessionId }, { silentAuthCheck: true })
    if (!msg.success || sessionId !== downloadProgressSessionId.value.trim()) return
    applyKernelDownloadProgress(msg.obj)
  })()
  downloadProgressRequest = request
  try {
    await request
  } finally {
    if (downloadProgressRequest === request) {
      downloadProgressRequest = null
    }
  }
}

const startDownloadProgressPolling = (sessionId: string) => {
  stopDownloadProgressPolling()
  downloadProgressSessionId.value = sessionId.trim()
  if (!downloadProgressSessionId.value || !props.active || (typeof document !== 'undefined' && document.visibilityState !== 'visible')) return
  downloadProgressTimerId.value = window.setInterval(() => {
    void pollDownloadProgress()
  }, 800)
  void pollDownloadProgress()
}

const recoverKernelDownloadTask = async (allowTerminal = false): Promise<boolean> => {
  const msg = await HttpUtils.get('api/kernel-download-progress', {}, { silentAuthCheck: true })
  if (!msg.success || !msg.obj) return false
  const nextProgress = normalizeKernelDownloadProgress(msg.obj)
  if (nextProgress.id === '' || nextProgress.state === 'idle' || nextProgress.status === 'missing') return false
  const terminal = isTerminalKernelDownload(nextProgress)
  applyKernelDownloadProgress(nextProgress, allowTerminal)
  if (terminal) return allowTerminal
  if (kernelDownloadTaskActive.value) {
    startDownloadProgressPolling(nextProgress.id)
  }
  return true
}

const handleVisibilityChange = () => {
  if (document.visibilityState === 'visible') {
    if (!props.active) return
    void recoverKernelDownloadTask()
    return
  }
  stopDownloadProgressPolling()
}

const buildSelectionFormData = () => {
  const formData = new FormData()
  if (selectedProvider.value) {
    formData.append('provider', selectedProvider.value)
  }
  if (isXanMod.value) {
    formData.append('line', selectedLine.value)
    formData.append('arch', selectedArch.value)
  }
  formData.append('version', selectedVersion.value)
  return formData
}

const downloadPackages = async () => {
  if (!canDownload.value || kernelDownloadTaskActive.value || downloadStartPending.value) return
  const confirmed = await confirm({
    message: t('kernelManager.downloadConfirm'),
    severity: 'info',
    confirmText: t('confirmDialog.actions.download'),
  })
  if (!confirmed || !canDownload.value) return
  clearFeedback()
  resetDownloadProgress()
  completedKernelDownloadTaskID = ''
  downloadStartPending.value = true
  try {
    const msg = await HttpUtils.post('api/kernel-download', buildSelectionFormData(), { timeout: kernelDownloadRequestTimeout })
    if (msg.success && msg.obj) {
      applyKernelDownloadProgress(msg.obj)
      if (downloadProgress.value.id) {
        startDownloadProgressPolling(downloadProgress.value.id)
      }
      return
    }
    const recovered = await recoverKernelDownloadTask()
    if (!recovered) {
      setFeedback('error', String(msg.msg || t('kernelManager.downloadFailed')))
    }
  } catch (error: any) {
    const recovered = await recoverKernelDownloadTask()
    if (!recovered) {
      setFeedback('error', String(error?.message || t('kernelManager.downloadFailed')))
    }
  } finally {
    downloadStartPending.value = false
  }
}

const stopKernelDownload = async () => {
  const id = downloadProgress.value.id.trim()
  if (!id || !downloadProgress.value.canCancel || kernelDownloadStopRequestPending.value) return
  kernelDownloadStopRequestPending.value = true
  downloadProgress.value = {
    ...downloadProgress.value,
    state: 'stopping',
    phase: 'stopping',
    canCancel: false,
    stopRequested: true,
  }
  try {
    const msg = await HttpUtils.post('api/kernel-download-stop', { id }, { silentAuthCheck: true })
    if (msg.success && msg.obj) {
      applyKernelDownloadProgress(msg.obj)
    }
  } catch {
    // 状态轮询会确认已经受理但响应丢失的停止请求。
  } finally {
    if (kernelDownloadTaskActive.value) {
      startDownloadProgressPolling(id)
    }
  }
}

const installPackages = async () => {
  if (!canInstall.value) return
  const confirmed = await confirm({
    message: t('kernelManager.installConfirm'),
    severity: 'warning',
    confirmText: t('confirmDialog.actions.install'),
  })
  if (!confirmed || !canInstall.value) return
  clearFeedback()
  installing.value = true
  try {
    const msg = await HttpUtils.post('api/kernel-install', {}, { timeout: kernelPackageOperationTimeout })
    if (msg.success) {
      const installed = msg.obj?.installed === true
      const needsReboot = msg.obj?.needsReboot === true
      const pinnedUpdated = msg.obj?.pinnedUpdated === true
      const pinnedKernel = String(msg.obj?.pinnedKernel ?? '').trim()
      let successText = ''
      let successType: 'success' | 'warning' = 'success'
      if (installed) {
        if (pinnedUpdated && pinnedKernel) {
          successText = t('kernelManager.installDonePinned', { kernel: pinnedKernel })
        } else {
          successText = needsReboot ? t('kernelManager.installDoneNeedReboot') : t('kernelManager.installDone')
        }
      } else {
        successText = t('kernelManager.installUnverified')
        successType = 'warning'
      }
      applyKernelSuccessFeedback(successText, msg.obj, successType)
      await loadRuntimeOverview()
      await loadOverview()
      await scanCleanupPackages()
    } else {
      setFeedback('error', String(msg.msg || t('kernelManager.installFailed')))
    }
  } finally {
    installing.value = false
  }
}

const clearReconnectTimer = () => {
  if (reconnectTimerId.value !== null) {
    window.clearTimeout(reconnectTimerId.value)
    reconnectTimerId.value = null
  }
}

const startReconnectPolling = () => {
  rebootOverlay.value = true
  const poll = async () => {
    try {
      const body = await HttpUtils.get('api/session', {}, {
        timeout: 5000,
        silentAuthCheck: true,
        silentErrorToast: true,
      })
      if (body.success) {
        window.location.reload()
        return
      }
      if (body.failureKind === 'api') {
        clearReconnectTimer()
        rebootOverlay.value = false
        await router.replace('/login')
        return
      }
    } catch {
      // wait for service to come back
    }
    reconnectTimerId.value = window.setTimeout(poll, 4000)
  }
  reconnectTimerId.value = window.setTimeout(poll, 6000)
}

const rebootHost = async () => {
  if (!canRebootHost.value || operationBusy.value) return
  const confirmed = await confirm({
    message: t('kernelManager.rebootConfirm'),
    severity: 'danger',
    confirmText: t('confirmDialog.actions.reboot'),
  })
  if (!confirmed || !canRebootHost.value || operationBusy.value) return
  clearFeedback()
  rebooting.value = true
  try {
    const msg = await HttpUtils.post('api/kernel-reboot', {})
    if (msg.success) {
      startReconnectPolling()
      return
    }
    setFeedback('error', String(msg.msg || t('kernelManager.rebootFailed')))
  } finally {
    rebooting.value = false
  }
}

const clearDownloadedKernel = async () => {
  if (!canManageKernelPackages.value || operationBusy.value) return
  const confirmed = await confirm({
    message: t('kernelManager.clearDownloadedConfirm'),
    severity: 'danger',
    confirmText: t('confirmDialog.actions.clear'),
  })
  if (!confirmed || !canManageKernelPackages.value || operationBusy.value) return
  clearFeedback()
  const stopLoading = beginCleanupLoading()
  try {
    const msg = await HttpUtils.post('api/kernel-downloaded-clear', {})
    if (msg.success) {
      overview.value.downloadedKernel = ''
      overview.value.downloadedDirectory = ''
      downloadDirectory.value = ''
      setFeedback('success', t('kernelManager.clearDownloadedDone'))
      await loadRuntimeOverview()
      await loadOverview()
      await loadPackages()
    } else {
      setFeedback('error', String(msg.msg || t('kernelManager.clearDownloadedFailed')))
    }
  } finally {
    stopLoading()
  }
}

const refreshKernelData = async (
  selectionRequestToken = beginSelectionRequest(),
  cleanupRequestToken = beginCleanupScanRequest(),
) => {
  await loadRuntimeOverview()
  if (!selectedProvider.value) {
    return
  }
  if (!canManageKernelPackages.value) {
    return
  }
  await loadOverview(selectionRequestToken)
  await loadVersions(selectionRequestToken)
  if (canManageKernelCleanup.value) {
    await scanCleanupPackages(false, cleanupRequestToken)
  }
}

const refreshCurrentKernelData = async () => {
  const selectionRequestToken = beginSelectionRequest()
  const cleanupRequestToken = beginCleanupScanRequest()
  await refreshKernelData(selectionRequestToken, cleanupRequestToken)
}

watch(provider, async (nextProvider) => {
  clearFeedback()
  kernelSelectionHydrating.value = true
  resetKernelSelection(nextProvider)
  await nextTick()
  kernelSelectionHydrating.value = false
  await refreshCurrentKernelData()
}, { immediate: true })

watch(selectedLine, async () => {
  if (kernelSelectionHydrating.value || !isXanMod.value) return
  kernelSelectionHydrating.value = true
  selectedVersion.value = ''
  selectedArch.value = ''
  kernelSelectionHydrating.value = false
  await loadVersions(beginSelectionRequest())
})

watch(selectedVersion, async () => {
  if (kernelSelectionHydrating.value || !selectedProvider.value) return
  if (!selectedVersion.value) {
    kernelSelectionHydrating.value = true
    selectedArch.value = ''
    kernelSelectionHydrating.value = false
    archItems.value = []
    packages.value = []
    downloadDirectory.value = ''
    return
  }
  const requestToken = beginSelectionRequest()
  if (isXanMod.value) {
    kernelSelectionHydrating.value = true
    selectedArch.value = ''
    kernelSelectionHydrating.value = false
    await loadArches(requestToken)
    return
  }
  await loadPackages(requestToken)
})

watch(selectedArch, async () => {
  if (kernelSelectionHydrating.value || !isXanMod.value) return
  if (!selectedArch.value) {
    packages.value = []
    downloadDirectory.value = ''
    return
  }
  await loadPackages(beginSelectionRequest())
})

onMounted(() => {
  if (typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', handleVisibilityChange)
  }
  if (props.active) {
    void recoverKernelDownloadTask()
  }
})

watch(() => props.active, (active) => {
  if (active) {
    void recoverKernelDownloadTask()
    return
  }
  stopDownloadProgressPolling()
})

onBeforeUnmount(() => {
  stopDownloadProgressPolling()
  clearReconnectTimer()
  if (downloadFeedbackTimer != null) {
    window.clearTimeout(downloadFeedbackTimer)
    downloadFeedbackTimer = null
  }
  if (typeof document !== 'undefined') {
    document.removeEventListener('visibilitychange', handleVisibilityChange)
  }
})
</script>

<style scoped>
.kernel-provider-select {
  min-width: 220px;
  max-width: 320px;
}

.kernel-action-btn {
  min-height: 38px;
  border: 1px solid transparent !important;
  font-weight: 700;
  letter-spacing: 0;
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.1);
  transition:
    background-color 0.16s ease,
    border-color 0.16s ease,
    box-shadow 0.16s ease,
    transform 0.16s ease;
}

.kernel-action-btn--download {
  background: #1d4ed8 !important;
  border-color: #3b82f6 !important;
  color: #eff6ff !important;
}

.kernel-action-btn--install {
  background: #047857 !important;
  border-color: #10b981 !important;
  color: #ecfdf5 !important;
}

.kernel-action-btn--reboot {
  background: #92400e !important;
  border-color: #d97706 !important;
  color: #fef3c7 !important;
}

.kernel-action-btn--scan {
  background: #075985 !important;
  border-color: #0ea5e9 !important;
  color: #e0f2fe !important;
}

.kernel-action-btn--purge {
  background: #b91c1c !important;
  border-color: #ef4444 !important;
  color: #fff1f2 !important;
}

.kernel-action-btn--auto-clean {
  background: #9a3412 !important;
  border-color: #f97316 !important;
  color: #fff7ed !important;
}

.kernel-action-btn:hover:not(.v-btn--disabled) {
  box-shadow:
    inset 0 0 0 1px rgba(255, 255, 255, 0.18),
    0 8px 18px rgba(15, 23, 42, 0.22);
  transform: translateY(-1px);
}

.kernel-action-btn:focus-visible {
  outline: 2px solid rgba(255, 255, 255, 0.82);
  outline-offset: 2px;
}

.kernel-action-btn:not(.v-btn--loading) :deep(.v-btn__content),
.kernel-action-btn:not(.v-btn--loading) :deep(.v-icon) {
  color: inherit !important;
  opacity: 1;
}

.kernel-action-btn.v-btn--disabled {
  opacity: 1 !important;
  background: #52525b !important;
  border-color: #71717a !important;
  color: #e4e4e7 !important;
  box-shadow: none;
}

.kernel-action-btn.v-btn--disabled :deep(.v-btn__overlay) {
  background: #27272a !important;
  opacity: 0.34 !important;
}

.kernel-action-btn.v-btn--disabled :deep(.v-btn__content),
.kernel-action-btn.v-btn--disabled :deep(.v-icon) {
  color: #e4e4e7 !important;
}

.kernel-cleanup-risk-chip {
  min-width: 54px;
  justify-content: center;
  font-weight: 700;
  letter-spacing: 0.02em;
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.08);
}

.kernel-cleanup-risk-chip--high {
  background: rgba(239, 68, 68, 0.28) !important;
  color: #fee2e2 !important;
}

.kernel-cleanup-risk-chip--normal {
  background: rgba(34, 197, 94, 0.24) !important;
  color: #dcfce7 !important;
}

.kernel-cleanup-risk-chip :deep(.v-chip__content) {
  color: inherit !important;
}

.kernel-cleanup-tag--image,
.kernel-cleanup-tag--image :deep(.v-chip__content) {
  color: #fff !important;
}

.kernel-download-progress {
  width: 100%;
  min-width: 0;
  padding: 12px;
  border: 1px solid rgba(59, 130, 246, 0.26);
  border-radius: 6px;
}

.kernel-download-progress__status,
.kernel-download-progress__detail {
  overflow-wrap: anywhere;
}

.kernel-download-progress__detail {
  min-height: 18px;
}
</style>
