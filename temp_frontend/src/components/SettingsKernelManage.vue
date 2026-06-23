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
          :disabled="busy" />
        <v-chip size="small" :color="overview.supported ? 'success' : 'warning'">
          {{ overview.supported ? t('kernelManager.supported') : t('kernelManager.unsupported') }}
        </v-chip>
      </v-card-title>
      <v-divider />
      <v-card-text>
        <v-alert
          v-if="!overview.supported && overview.reason"
          type="warning"
          variant="tonal"
          density="comfortable"
          class="mb-4">
          {{ overview.reason }}
        </v-alert>

        <v-row class="mb-2">
          <template v-if="isXanMod">
            <v-col cols="12" sm="6" md="4">
              <v-select
                v-model="selectedLine"
                :items="lineItems"
                item-title="label"
                item-value="value"
                :label="t('kernelManager.line')"
                :disabled="busy" />
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-select
                v-model="selectedVersion"
                :items="versionItems"
                item-title="name"
                item-value="name"
                :label="t('kernelManager.version')"
                :disabled="busy || versionItems.length === 0" />
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-select
                v-model="selectedArch"
                :items="archItems"
                item-title="arch"
                item-value="arch"
                :label="t('kernelManager.arch')"
                :disabled="busy || archItems.length === 0" />
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
                :disabled="busy || versionItems.length === 0" />
            </v-col>
          </template>
        </v-row>

        <div class="text-caption text-medium-emphasis mb-3">
          {{ t('kernelManager.currentKernel') }}: {{ overview.currentKernel || '-' }}
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

        <v-row>
          <v-col cols="12" md="4">
            <v-btn
              block
              color="primary"
              prepend-icon="mdi-download"
              :loading="downloading"
              :disabled="!canOperate || packages.length === 0 || downloading"
              @click="downloadPackages">
              <template #loader>
                <span class="kernel-download-loader">
                  <v-progress-circular
                    indeterminate
                    size="16"
                    width="2"
                    color="white" />
                  <span class="kernel-download-loader__text">{{ downloadProgressText }}</span>
                </span>
              </template>
              {{ t('kernelManager.download') }}
            </v-btn>
          </v-col>
          <v-col cols="12" md="4">
            <v-btn
              block
              color="secondary"
              prepend-icon="mdi-package-variant-closed-check"
              :loading="installing"
              :disabled="!canOperate || packages.length === 0 || installing"
              @click="installPackages">
              {{ t('kernelManager.install') }}
            </v-btn>
          </v-col>
          <v-col cols="12" md="4">
            <v-btn
              block
              color="warning"
              prepend-icon="mdi-restart-alert"
              :loading="rebooting"
              :disabled="!canOperate || rebooting"
              @click="rebootHost">
              {{ t('kernelManager.reboot') }}
            </v-btn>
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
            :disabled="busy"
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
              block
              color="primary"
              prepend-icon="mdi-magnify"
              :loading="cleanupLoading"
              :disabled="busy || !overview.supported"
              @click="scanCleanupPackages(true)">
              {{ t('kernelManager.cleanupScan') }}
            </v-btn>
          </v-col>
          <v-col cols="12" md="4">
            <v-btn
              block
              color="error"
              prepend-icon="mdi-delete-sweep"
              :loading="cleanupPurging"
              :disabled="busy || !overview.supported || cleanupSelectedPackages.length === 0"
              @click="purgeSelectedCleanupPackages">
              {{ t('kernelManager.cleanupPurgeSelected') }} ({{ cleanupSelectedPackages.length }})
            </v-btn>
          </v-col>
          <v-col cols="12" md="4">
            <v-btn
              block
              color="warning"
              prepend-icon="mdi-auto-fix"
              :loading="cleanupAutoPurging"
              :disabled="busy || !overview.supported || cleanupPackages.length === 0"
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
                  @update:model-value="toggleCleanupSelection(pkg.name, $event)" />
              </td>
              <td>{{ pkg.name }}</td>
              <td>{{ pkg.status || '-' }}</td>
              <td>
                <v-chip v-if="pkg.isPinnedKernel" size="x-small" color="success" class="mr-1">{{ t('kernelManager.cleanupPinned') }}</v-chip>
                <v-chip v-if="pkg.isCurrentKernel" size="x-small" color="info" class="mr-1">{{ t('kernelManager.cleanupCurrent') }}</v-chip>
                <v-chip v-if="pkg.isImage" size="x-small" color="primary" class="mr-1 kernel-cleanup-tag--image">image</v-chip>
                <v-chip v-if="pkg.isHeaders" size="x-small" color="secondary" class="mr-1">headers</v-chip>
              </td>
              <td>
                <v-chip size="x-small" :color="pkg.risk === 'high' ? 'error' : 'success'">
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
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import type { Ref } from 'vue'
import { useI18n } from 'vue-i18n'

type KernelOverview = {
  supported: boolean
  reason: string
  currentKernel: string
  downloadRoot: string
  downloadedKernel?: string
  downloadedDirectory?: string
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
  percent: number
  approximate: boolean
  downloadedBytes: number
  totalBytes: number
  currentPackage: string
  downloadedCount: number
  totalCount: number
  error: string
}

type KernelSystemCleanupInfo = {
  done: boolean
  warnings: string[]
  summary: string
}

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
const downloading = ref(false)
const installing = ref(false)
const rebooting = ref(false)
const cleanupLoading = ref(false)
const cleanupPurging = ref(false)
const cleanupAutoPurging = ref(false)
const rebootOverlay = ref(false)
const reconnectTimerId = ref<number | null>(null)
const downloadProgressSessionId = ref('')
const downloadProgressTimerId = ref<number | null>(null)
const provider = ref('xanmod')
const kernelSelectionHydrating = ref(false)
const pendingReturnToDefaultProvider = ref(false)

const selectedLine = ref('lts')
const selectedVersion = ref('')
const selectedArch = ref('x64v3')

const overview = ref<KernelOverview>({
  supported: false,
  reason: '',
  currentKernel: '',
  downloadRoot: '',
})

const versionItems = ref<KernelVersionItem[]>([])
const archItems = ref<KernelArchItem[]>([])
const packages = ref<KernelPackageItem[]>([])
const downloadDirectory = ref('')
const cleanupCurrentKernel = ref('')
const cleanupPinnedKernel = ref('')
const cleanupPackages = ref<KernelCleanupPackageItem[]>([])
const cleanupSelectedMap = ref<Record<string, boolean>>({})
const downloadProgress = ref<KernelDownloadProgress>({
  id: '',
  status: 'missing',
  percent: 0,
  approximate: false,
  downloadedBytes: 0,
  totalBytes: 0,
  currentPackage: '',
  downloadedCount: 0,
  totalCount: 0,
  error: '',
})

const feedback = ref<{ type: 'success' | 'warning' | 'error' | 'info'; message: string }>({
  type: 'info',
  message: '',
})

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

const busy = computed(() => (
  loadingOverview.value ||
  loadingPackages.value ||
  downloading.value ||
  installing.value ||
  rebooting.value ||
  cleanupLoading.value ||
  cleanupPurging.value ||
  cleanupAutoPurging.value
))
const canOperate = computed(() => overview.value.supported && !busy.value)
const isXanMod = computed(() => provider.value === 'xanmod')
const downloadDirText = computed(() => downloadDirectory.value || overview.value.downloadRoot || '-')
const downloadedKernelLabel = computed(() => String(overview.value.downloadedKernel || '').trim())
const downloadedKernelDirectory = computed(() => String(overview.value.downloadedDirectory || '').trim())
const hasDownloadedKernel = computed(() => (
  downloadedKernelLabel.value.length > 0 && downloadedKernelDirectory.value.length > 0
))
const cleanupCurrentKernelText = computed(() => cleanupCurrentKernel.value || overview.value.currentKernel || '-')
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
  feedback.value.message = ''
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
  percent: Number.isFinite(Number(raw?.percent)) ? Number(raw.percent) : 0,
  approximate: raw?.approximate === true,
  downloadedBytes: Number.isFinite(Number(raw?.downloadedBytes)) ? Math.max(0, Number(raw.downloadedBytes)) : 0,
  totalBytes: Number.isFinite(Number(raw?.totalBytes)) ? Math.max(0, Number(raw.totalBytes)) : 0,
  currentPackage: String(raw?.currentPackage ?? '').trim(),
  downloadedCount: Number.isFinite(Number(raw?.downloadedCount)) ? Math.max(0, Number(raw.downloadedCount)) : 0,
  totalCount: Number.isFinite(Number(raw?.totalCount)) ? Math.max(0, Number(raw.totalCount)) : 0,
  error: String(raw?.error ?? '').trim(),
})

const resetDownloadProgress = () => {
  downloadProgress.value = {
    id: '',
    status: 'missing',
    percent: 0,
    approximate: false,
    downloadedBytes: 0,
    totalBytes: 0,
    currentPackage: '',
    downloadedCount: 0,
    totalCount: 0,
    error: '',
  }
}

const resetKernelSelection = (nextProvider: string) => {
  selectedLine.value = nextProvider === 'xanmod' ? 'lts' : ''
  selectedVersion.value = ''
  selectedArch.value = nextProvider === 'xanmod' ? 'x64v3' : ''
  versionItems.value = []
  archItems.value = []
  packages.value = []
  downloadDirectory.value = ''
}

const loadOverview = async (requestToken = beginSelectionRequest()) => {
  const currentProvider = provider.value
  const stopLoading = beginOverviewLoading()
  try {
    const msg = await HttpUtils.get('api/kernel-overview', { provider: currentProvider })
    if (!isLatestSelectionRequest(requestToken)) return
    if (msg.success && msg.obj) {
      overview.value = {
        supported: msg.obj.supported === true,
        reason: String(msg.obj.reason ?? ''),
        currentKernel: String(msg.obj.currentKernel ?? ''),
        downloadRoot: String(msg.obj.downloadRoot ?? ''),
        downloadedKernel: String(msg.obj.downloadedKernel ?? ''),
        downloadedDirectory: String(msg.obj.downloadedDirectory ?? ''),
      }
    }
  } finally {
    stopLoading()
  }
}

const loadVersions = async (requestToken = beginSelectionRequest()) => {
  const currentProvider = provider.value
  const currentLine = selectedLine.value
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
    selectedVersion.value = versionItems.value.length > 0 ? versionItems.value[0].name : ''
    if (currentProvider === 'xanmod' && !selectedVersion.value) {
      selectedArch.value = ''
    }
    kernelSelectionHydrating.value = false
    if (selectedVersion.value.length > 0) {
      if (currentProvider === 'xanmod') {
        await loadArches(requestToken)
      } else {
        await loadPackages(requestToken)
      }
    } else if (isLatestSelectionRequest(requestToken)) {
      archItems.value = []
      packages.value = []
      downloadDirectory.value = ''
    }
  } finally {
    stopLoading()
  }
}

const loadArches = async (requestToken = beginSelectionRequest()) => {
  const currentProvider = provider.value
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
    const preferred = archItems.value.find(item => item.arch === 'x64v3')
    kernelSelectionHydrating.value = true
    selectedArch.value = preferred?.arch || archItems.value[0]?.arch || ''
    kernelSelectionHydrating.value = false
    await loadPackages(requestToken)
  } finally {
    stopLoading()
  }
}

const loadPackages = async (requestToken = beginSelectionRequest()) => {
  const currentProvider = provider.value
  const currentVersion = selectedVersion.value
  const currentLine = selectedLine.value
  const currentArch = selectedArch.value
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
}

const scanCleanupPackages = async (needConfirm = false, requestToken?: number) => {
  const currentRequestToken = requestToken ?? beginCleanupScanRequest()
  if (!overview.value.supported) {
    if (isLatestCleanupScanRequest(currentRequestToken)) {
      cleanupPackages.value = []
      cleanupCurrentKernel.value = ''
      cleanupPinnedKernel.value = ''
      resetCleanupSelection()
    }
    return
  }
  if (needConfirm) {
    const confirmed = window.confirm(t('kernelManager.cleanupScanConfirm'))
    if (!confirmed) return
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
  const targets = cleanupSelectedPackages.value
  if (targets.length === 0) {
    setFeedback('warning', t('kernelManager.cleanupNeedSelection'))
    return
  }
  const confirmed = window.confirm(t('kernelManager.cleanupPurgeConfirm', { count: targets.length }))
  if (!confirmed) return

  cleanupPurging.value = true
  try {
    const msg = await HttpUtils.post('api/kernel-cleanup-purge', { packages: targets }, {
      headers: {
        'Content-Type': 'application/json',
      },
    })
    if (msg.success) {
      applyKernelSuccessFeedback(t('kernelManager.cleanupPurgeDone', { count: targets.length }), msg.obj)
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
  const confirmed = window.confirm(t('kernelManager.cleanupAutoConfirm'))
  if (!confirmed) return

  cleanupAutoPurging.value = true
  try {
    const msg = await HttpUtils.post('api/kernel-cleanup-auto', {})
    if (msg.success) {
      const count = Array.isArray(msg.obj?.requested) ? msg.obj.requested.length : 0
      applyKernelSuccessFeedback(t('kernelManager.cleanupAutoDone', { count }), msg.obj)
      await loadOverview()
      await scanCleanupPackages()
    } else {
      setFeedback('error', String(msg.msg || t('kernelManager.cleanupAutoFailed')))
    }
  } finally {
    cleanupAutoPurging.value = false
  }
}

const makeDownloadSessionId = () => {
  const randomPart = typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function'
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(16).slice(2)}`
  return `kernel-download-${randomPart}`
}

const stopDownloadProgressPolling = () => {
  if (downloadProgressTimerId.value != null) {
    window.clearInterval(downloadProgressTimerId.value)
    downloadProgressTimerId.value = null
  }
}

const pollDownloadProgress = async () => {
  const sessionId = downloadProgressSessionId.value.trim()
  if (!sessionId) return
  const msg = await HttpUtils.get('api/kernel-download-progress', { id: sessionId }, { silentAuthCheck: true })
  if (!msg.success) return
  const nextProgress = normalizeKernelDownloadProgress(msg.obj)
  if (nextProgress.status === 'missing' && downloading.value) {
    return
  }
  downloadProgress.value = nextProgress
  if (downloadProgress.value.status === 'success' || downloadProgress.value.status === 'error' || downloadProgress.value.status === 'missing') {
    stopDownloadProgressPolling()
  }
}

const startDownloadProgressPolling = (sessionId: string) => {
  stopDownloadProgressPolling()
  downloadProgressSessionId.value = sessionId.trim()
  if (!downloadProgressSessionId.value) return
  downloadProgressTimerId.value = window.setInterval(() => {
    void pollDownloadProgress()
  }, 800)
  void pollDownloadProgress()
}

const buildSelectionFormData = (downloadSessionId = '') => {
  const formData = new FormData()
  formData.append('provider', provider.value)
  if (isXanMod.value) {
    formData.append('line', selectedLine.value)
    formData.append('arch', selectedArch.value)
  }
  formData.append('version', selectedVersion.value)
  if (downloadSessionId.trim()) {
    formData.append('downloadSessionId', downloadSessionId.trim())
  }
  return formData
}

const downloadPackages = async () => {
  const confirmed = window.confirm(t('kernelManager.downloadConfirm'))
  if (!confirmed) return
  clearFeedback()
  const sessionId = makeDownloadSessionId()
  resetDownloadProgress()
  downloadProgress.value.id = sessionId
  downloadProgress.value.status = 'running'
  downloadProgressSessionId.value = sessionId
  downloading.value = true
  startDownloadProgressPolling(sessionId)
  try {
    const msg = await HttpUtils.post('api/kernel-download', buildSelectionFormData(sessionId))
    if (msg.success && msg.obj?.sessionId) {
      const normalizedSessionId = String(msg.obj.sessionId).trim()
      if (normalizedSessionId && normalizedSessionId !== downloadProgressSessionId.value) {
        startDownloadProgressPolling(normalizedSessionId)
      }
    }
    await pollDownloadProgress()
    if (msg.success) {
      const count = Array.isArray(msg.obj?.downloaded) ? msg.obj.downloaded.length : 0
      downloadDirectory.value = String(msg.obj?.directory ?? downloadDirectory.value)
      await loadOverview()
      setFeedback('success', t('kernelManager.downloadDone', { count }))
    } else {
      setFeedback('error', String(msg.msg || t('kernelManager.downloadFailed')))
    }
  } finally {
    stopDownloadProgressPolling()
    downloading.value = false
  }
}

const installPackages = async () => {
  const confirmed = window.confirm(t('kernelManager.installConfirm'))
  if (!confirmed) return
  clearFeedback()
  installing.value = true
  try {
    const msg = await HttpUtils.post('api/kernel-install', buildSelectionFormData())
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
      const resp = await fetch('./api/session', {
        method: 'GET',
        credentials: 'include',
        cache: 'no-store',
      })
      if (resp.ok) {
        const body = await resp.json()
        if (body?.success === true) {
          window.location.reload()
          return
        }
      }
    } catch {
      // wait for service to come back
    }
    reconnectTimerId.value = window.setTimeout(poll, 4000)
  }
  reconnectTimerId.value = window.setTimeout(poll, 6000)
}

const rebootHost = async () => {
  const confirmed = window.confirm(t('kernelManager.rebootConfirm'))
  if (!confirmed) return
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
  const confirmed = window.confirm(t('kernelManager.clearDownloadedConfirm'))
  if (!confirmed) return
  clearFeedback()
  const stopLoading = beginCleanupLoading()
  try {
    const msg = await HttpUtils.post('api/kernel-downloaded-clear', {})
    if (msg.success) {
      overview.value.downloadedKernel = ''
      overview.value.downloadedDirectory = ''
      downloadDirectory.value = ''
      setFeedback('success', t('kernelManager.clearDownloadedDone'))
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
  await loadOverview(selectionRequestToken)
  await loadVersions(selectionRequestToken)
  if (overview.value.supported) {
    await scanCleanupPackages(false, cleanupRequestToken)
  }
}

const refreshCurrentKernelData = async () => {
  const selectionRequestToken = beginSelectionRequest()
  const cleanupRequestToken = beginCleanupScanRequest()
  await refreshKernelData(selectionRequestToken, cleanupRequestToken)
}

const processReturnToDefaultProvider = async () => {
  if (!props.active) return
  if (busy.value) {
    pendingReturnToDefaultProvider.value = true
    return
  }
  pendingReturnToDefaultProvider.value = false
  if (provider.value !== 'xanmod') {
    provider.value = 'xanmod'
    return
  }
  await refreshCurrentKernelData()
}

watch(provider, async (nextProvider) => {
  clearFeedback()
  kernelSelectionHydrating.value = true
  resetKernelSelection(nextProvider)
  await nextTick()
  kernelSelectionHydrating.value = false
  await refreshCurrentKernelData()
}, { immediate: true })

watch(() => props.active, async (active, previousActive) => {
  if (!active || previousActive === undefined) return
  await processReturnToDefaultProvider()
})

watch(busy, async (nextBusy) => {
  if (nextBusy || !pendingReturnToDefaultProvider.value || !props.active) return
  await processReturnToDefaultProvider()
})

watch(selectedLine, async () => {
  if (kernelSelectionHydrating.value || !isXanMod.value) return
  await loadVersions(beginSelectionRequest())
})

watch(selectedVersion, async () => {
  if (kernelSelectionHydrating.value || !selectedVersion.value) return
  const requestToken = beginSelectionRequest()
  if (isXanMod.value) {
    await loadArches(requestToken)
    return
  }
  await loadPackages(requestToken)
})

watch(selectedArch, async () => {
  if (kernelSelectionHydrating.value || !isXanMod.value) return
  await loadPackages(beginSelectionRequest())
})

onBeforeUnmount(() => {
  stopDownloadProgressPolling()
  clearReconnectTimer()
})
</script>

<style scoped>
.kernel-provider-select {
  min-width: 220px;
  max-width: 320px;
}

.kernel-cleanup-tag--image,
.kernel-cleanup-tag--image :deep(.v-chip__content) {
  color: #fff !important;
}

.kernel-download-loader {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.kernel-download-loader__text {
  white-space: nowrap;
  font-size: 12px;
  letter-spacing: 0.1px;
  color: #fff;
}
</style>
