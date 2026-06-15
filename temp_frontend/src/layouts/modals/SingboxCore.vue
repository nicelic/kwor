<template>
  <v-dialog
    v-model="dialogVisible"
    transition="dialog-bottom-transition"
    width="760"
    max-width="95vw"
  >
    <v-card class="rounded-lg">
      <v-card-title>
        <v-row align="center">
          <v-col cols="auto" class="d-flex align-center" style="gap: 8px;">
            <v-icon icon="mdi-engine"></v-icon>
            <span>{{ t(coreMeta.modalTitle) }}</span>
          </v-col>
          <v-spacer></v-spacer>
          <v-col cols="auto">
            <v-icon
              icon="mdi-close"
              style="cursor: pointer"
              @click="close"
            ></v-icon>
          </v-col>
        </v-row>
      </v-card-title>

      <v-divider></v-divider>

      <v-card-text style="padding: 20px; min-height: 450px">
        <div class="d-flex align-center mb-2" style="gap: 8px;">
          <span class="text-h6 font-weight-bold">{{ coreMeta.coreName }}</span>
          <v-btn icon size="x-small" variant="text" @click="openReleasePage">
            <v-icon size="18">mdi-open-in-new</v-icon>
            <v-tooltip activator="parent" location="top">{{ t('coreManager.releasePage') }}</v-tooltip>
          </v-btn>
          <v-btn
            icon
            size="x-small"
            variant="text"
            :loading="statusLoading"
            @click="refreshAll(true)"
          >
            <v-icon size="18">mdi-refresh</v-icon>
            <v-tooltip activator="parent" location="top">{{ t('refresh') }}</v-tooltip>
          </v-btn>
          <v-chip
            v-if="platform"
            variant="tonal"
            size="x-small"
            label
          >
            {{ platform }}
          </v-chip>
          <v-spacer></v-spacer>
          <v-btn
            color="error"
            variant="tonal"
            size="small"
            prepend-icon="mdi-delete"
            :disabled="!localVersion || downloading || startingCore || stoppingCore || restartingCore || deletingCore"
            :loading="deletingCore"
            @click="deleteCore"
          >
            {{ t('coreManager.deleteCore') }}
          </v-btn>
        </div>

        <div class="d-flex align-center mb-4" style="gap: 12px">
          <v-chip
            variant="outlined"
            :color="localVersion ? 'success' : 'error'"
            size="small"
            label
          >
            {{ t('coreManager.local') }}: {{ localVersion || t('coreManager.notInstalled') }}
          </v-chip>

          <v-chip variant="outlined" color="info" size="small" label>
            <v-progress-circular
              v-if="remoteLoading"
              indeterminate
              size="12"
              width="2"
              class="mr-1"
            ></v-progress-circular>
            {{ t('coreManager.remote') }}: {{ remoteVersionLabel }}
          </v-chip>
        </div>

        <v-card
          v-if="versionInfo"
          variant="tonal"
          rounded="lg"
          class="mb-4"
          color="surface-variant"
        >
          <v-card-text
            style="
              font-size: 12px;
              font-family: monospace;
              line-height: 1.6;
              word-break: break-all;
            "
          >
            {{ versionInfo }}
          </v-card-text>
        </v-card>

        <v-alert
          v-if="feedbackMsg"
          :type="feedbackType"
          variant="tonal"
          density="compact"
          class="mb-4"
          closable
          @click:close="feedbackMsg = ''"
        >
          {{ feedbackMsg }}
        </v-alert>

        <v-card v-if="downloading" variant="outlined" rounded="lg" class="mb-4">
          <v-card-text>
            <div class="text-caption text-medium-emphasis mb-2">
              {{ t('coreManager.downloading', { coreName: coreMeta.coreName, version: downloadingVersionLabel }) }}
            </div>
            <v-progress-linear
              indeterminate
              color="primary"
              height="6"
              rounded
            ></v-progress-linear>
          </v-card-text>
        </v-card>

        <v-divider class="mb-6"></v-divider>

        <div class="text-subtitle-1 font-weight-medium mb-3">{{ t('version') }}</div>
        <v-row align="center">
          <v-col v-if="supportsPrereleaseChannel" cols="auto">
            <v-btn-toggle
              v-model="selectedChannel"
              mandatory
              density="compact"
              variant="outlined"
              divided
            >
              <v-btn value="stable" size="small">{{ t('coreManager.stable') }}</v-btn>
              <v-btn value="alpha" size="small">{{ t('coreManager.alpha') }}</v-btn>
            </v-btn-toggle>
          </v-col>

          <v-col>
            <v-select
              v-model="selectedVersion"
              :items="displayedVersionItems"
              :loading="remoteLoading"
              :label="t('coreManager.selectVersion')"
              variant="outlined"
              density="compact"
              hide-details
              :disabled="versionItems.length === 0"
              :menu-props="{ maxHeight: 260 }"
            >
              <template #item="{ props: itemProps, item }">
                <v-list-item
                  v-bind="itemProps"
                  :subtitle="item.raw.assetName || undefined"
                >
                  <template #append>
                    <v-chip
                      v-if="supportsPrereleaseChannel && item.raw.prerelease"
                      size="x-small"
                      color="warning"
                      variant="flat"
                    >
                      {{ t('coreManager.alpha') }}
                    </v-chip>
                  </template>
                </v-list-item>
              </template>
            </v-select>
          </v-col>

          <v-col cols="auto">
            <v-btn
              color="secondary"
              variant="tonal"
              prepend-icon="mdi-cloud-download"
              :loading="remoteLoading"
              :disabled="remoteLoading || loadingMoreVersions"
              @click="loadRemoteVersions(false)"
            >
              {{ versionList.length > 0 || remoteLoaded ? t('coreManager.refreshRemoteVersions') : t('coreManager.loadRemoteVersions') }}
            </v-btn>
          </v-col>

          <v-col cols="auto">
            <v-btn
              color="primary"
              variant="flat"
              prepend-icon="mdi-download"
              :loading="downloading"
              :disabled="!canDownloadSelectedVersion || downloading"
              @click="downloadCore"
            >
              {{ t('coreManager.download') }}
            </v-btn>
          </v-col>
        </v-row>

        <v-row v-if="versionItems.length > 0" class="mt-1">
          <v-col cols="12" class="d-flex align-center justify-space-between">
            <span class="text-caption text-medium-emphasis">
              {{ t('coreManager.loadedVersions', { count: versionItems.length }) }}
            </span>
            <div class="d-flex align-center" style="gap: 8px">
              <v-btn
                v-if="hasMoreVersions"
                size="x-small"
                variant="text"
                :loading="loadingMoreVersions"
                @click="loadMoreVersions"
              >
                {{ t('coreManager.showMore', { count: nextRemoteLoadCount }) }}
              </v-btn>
              <v-btn
                v-if="versionItems.length > 5"
                size="x-small"
                variant="text"
                @click="resetVersionDisplay"
              >
                {{ t('coreManager.resetLatest', { count: 5 }) }}
              </v-btn>
            </div>
          </v-col>
        </v-row>

        <v-row align="center" class="mt-2">
          <v-col>
            <v-text-field
              v-model="customDownloadURL"
              :label="t('coreManager.customDownloadUrl')"
              :placeholder="customUrlPlaceholder"
              variant="outlined"
              density="compact"
              hide-details
              clearable
              @blur="() => saveDownloadPreference()"
            ></v-text-field>
          </v-col>
          <v-col cols="auto">
            <v-btn
              color="secondary"
              variant="flat"
              prepend-icon="mdi-link-variant-plus"
              :loading="downloading"
              :disabled="!canDownloadCustom || downloading"
              @click="downloadCoreFromCustomURL"
            >
              {{ t('coreManager.customDownload') }}
            </v-btn>
          </v-col>
        </v-row>

        <v-row v-if="showLinuxArchSelector" class="mt-3">
          <v-col cols="12">
            <div class="text-caption text-medium-emphasis mb-2">
              {{ t('coreManager.linuxArchitecture') }}
            </div>
            <v-btn-toggle
              v-model="selectedLinuxArch"
              density="compact"
              variant="outlined"
              divided
              class="w-100"
            >
              <v-btn value="amd64" class="text-none flex-grow-1">amd64</v-btn>
              <v-btn value="arm64" class="text-none flex-grow-1">arm64</v-btn>
            </v-btn-toggle>
          </v-col>

          <v-col v-if="showLinuxAmd64LevelSelector" cols="12" class="pt-2">
            <div class="text-caption text-medium-emphasis mb-2">
              {{ t('coreManager.linuxAmd64Level') }}
            </div>
            <v-select
              v-model="selectedAmd64Level"
              :items="amd64LevelItems"
              item-title="title"
              item-value="value"
              variant="outlined"
              density="compact"
              :placeholder="t('coreManager.notDetected')"
              hide-details
              :menu-props="{ maxHeight: 180 }"
            ></v-select>
          </v-col>

          <v-col v-if="showLinuxLibcSelector" cols="12" class="pt-2">
            <div class="text-caption text-medium-emphasis mb-2">
              {{ t('coreManager.linuxPackageVariant') }}
            </div>
            <v-btn-toggle
              v-model="selectedLinuxLibc"
              density="compact"
              variant="outlined"
              divided
              class="w-100"
            >
              <v-btn value="glibc" class="text-none flex-grow-1">glibc</v-btn>
              <v-btn value="musl" class="text-none flex-grow-1">musl</v-btn>
              <v-btn value="universal" class="text-none flex-grow-1">
                {{ t('coreManager.universalPackage') }}
              </v-btn>
            </v-btn-toggle>
          </v-col>
        </v-row>

        <v-divider class="my-6"></v-divider>

        <div class="text-subtitle-1 font-weight-medium mb-3">{{ t('coreManager.autoCheck') }}</div>
        <v-row align="center">
          <v-col cols="12" sm="5">
            <v-switch
              v-model="autoCheckEnabled"
              color="primary"
              density="compact"
              hide-details
              :label="t('coreManager.enableAutoCheck')"
            ></v-switch>
          </v-col>
          <v-col cols="12" sm="4">
            <v-text-field
              v-model="autoCheckIntervalInput"
              :disabled="!autoCheckEnabled"
              :label="t('coreManager.checkInterval')"
              suffix="h"
              variant="outlined"
              density="compact"
              hide-details
              :placeholder="t('coreManager.intervalPlaceholder')"
            ></v-text-field>
          </v-col>
          <v-col cols="auto">
            <v-btn
              color="primary"
              variant="flat"
              size="small"
              :loading="autoCheckSaving"
              @click="saveAutoCheckSettings"
            >
              {{ t('actions.save') }}
            </v-btn>
          </v-col>
        </v-row>

        <v-row class="mt-1" align="center">
          <v-col cols="12" class="d-flex align-center flex-wrap" style="gap: 8px">
            <v-chip variant="outlined" size="small" color="success" label>
              {{ t('coreManager.stable') }}: {{ latestStableVersionDisplay || t('coreManager.unknown') }}
            </v-chip>
            <v-chip v-if="supportsPrereleaseChannel" variant="outlined" size="small" color="warning" label>
              {{ t('coreManager.alpha') }}: {{ latestAlphaVersionDisplay || t('coreManager.unknown') }}
            </v-chip>
            <span class="text-caption text-medium-emphasis">
              {{ t('coreManager.lastChecked', { time: lastCheckedAtDisplay || t('coreManager.never') }) }}
            </span>
          </v-col>
        </v-row>

        <v-alert
          v-if="hasPendingUpdates"
          type="warning"
          variant="tonal"
          density="compact"
          class="mt-2"
          closable
          @click:close="ackCoreUpdateNotice"
        >
          {{ pendingUpdateText }}
        </v-alert>

        <v-divider class="my-6"></v-divider>

        <div class="text-subtitle-1 font-weight-medium mb-3">{{ t('coreManager.coreControl') }}</div>
        <v-card variant="outlined" rounded="lg">
          <v-card-text>
            <v-row align="center">
              <v-col cols="auto">
                <div class="text-caption text-medium-emphasis">{{ t('coreManager.status') }}</div>
                <v-chip
                  :color="coreRunning ? 'success' : 'error'"
                  variant="flat"
                  size="small"
                  class="mt-1"
                >
                  <v-icon start size="x-small">
                    {{ coreRunning ? 'mdi-check-circle' : 'mdi-close-circle' }}
                  </v-icon>
                  {{ coreRunning ? t('coreManager.running') : t('coreManager.stopped') }}
                </v-chip>
              </v-col>

              <v-spacer></v-spacer>

              <v-col cols="auto" class="d-flex" style="gap: 8px">
                <v-btn
                  color="success"
                  variant="flat"
                  size="small"
                  prepend-icon="mdi-play"
                  :disabled="coreRunning || !localVersion"
                  :loading="startingCore"
                  @click="startCore"
                >
                  {{ t('coreManager.start') }}
                </v-btn>
                <v-btn
                  color="error"
                  variant="flat"
                  size="small"
                  prepend-icon="mdi-stop"
                  :disabled="!coreRunning"
                  :loading="stoppingCore"
                  @click="stopCore"
                >
                  {{ t('coreManager.stop') }}
                </v-btn>
                <v-btn
                  color="warning"
                  variant="flat"
                  size="small"
                  prepend-icon="mdi-restart"
                  :disabled="!coreRunning"
                  :loading="restartingCore"
                  @click="restartCore"
                >
                  {{ t('coreManager.restart') }}
                </v-btn>
              </v-col>
            </v-row>

            <v-row class="mt-3">
              <v-col cols="12">
                <div class="text-caption text-medium-emphasis mb-1">{{ t('coreManager.configFile') }}</div>
                <v-chip variant="tonal" size="small" label>
                  <v-icon start size="x-small">mdi-file-cog</v-icon>
                  {{ coreMeta.configPath }}
                </v-chip>
              </v-col>
            </v-row>

            <v-row class="mt-1">
              <v-col cols="12">
                <div class="text-caption text-medium-emphasis mb-1">{{ t('coreManager.binaryPath') }}</div>
                <v-chip variant="tonal" size="small" label>
                  <v-icon start size="x-small">mdi-application-cog</v-icon>
                  {{ binaryPath }}
                </v-chip>
              </v-col>
            </v-row>

            <v-row v-if="hasActiveDownloadProgress" class="mt-4">
              <v-col cols="12">
                <div class="text-caption text-medium-emphasis mb-2">{{ t('coreManager.downloadTaskStatus') }}</div>
                <v-card variant="tonal" rounded="lg" color="surface-variant">
                  <v-card-text>
                    <div class="d-flex align-center flex-wrap" style="gap: 8px;">
                      <v-chip
                        :color="downloadProgressStatusColor"
                        variant="flat"
                        size="small"
                        label
                      >
                        {{ downloadProgressStageText }}
                      </v-chip>
                      <span
                        v-if="downloadProgressDetail"
                        class="text-caption text-medium-emphasis"
                      >
                        {{ downloadProgressDetail }}
                      </span>
                    </div>
                    <v-progress-linear
                      class="mt-3"
                      :indeterminate="downloadProgress.stage === 'downloading' && downloadProgress.totalBytes <= 0"
                      :model-value="downloadProgressPercent"
                      :color="downloadProgressStatusColor"
                      height="8"
                      rounded
                    ></v-progress-linear>
                    <div class="text-caption text-medium-emphasis mt-2">
                      {{ t('coreManager.downloadTaskHint') }}
                    </div>
                  </v-card-text>
                </v-card>
              </v-col>
            </v-row>
          </v-card-text>
        </v-card>
      </v-card-text>
    </v-card>
  </v-dialog>
</template>

<script lang="ts" setup>
import { computed, ref, watch } from 'vue'
import HttpUtils from '@/plugins/httputil'
import { getNamespaceCore, type UiNamespace } from '@/store/uiNamespace'
import { useI18n } from 'vue-i18n'
import { HumanReadable } from '@/plugins/utils'

const props = withDefaults(defineProps<{
  visible: boolean
  namespace?: UiNamespace
}>(), {
  namespace: 'default',
})

const emit = defineEmits(['close', 'update:modelValue'])
const { t } = useI18n()

const dialogVisible = ref(props.visible)
const coreMeta = computed(() => getNamespaceCore(props.namespace))
const supportsPrereleaseChannel = computed(() => coreMeta.value.supportsPrereleaseChannel)
const showLinuxArchSelector = computed(() => (
  coreMeta.value.binaryBaseName === 'sing-box' || coreMeta.value.binaryBaseName === 'mihomo'
))
const showLinuxLibcSelector = computed(() => coreMeta.value.binaryBaseName === 'sing-box')

const statusLoading = ref(false)
const localVersion = ref('')
const versionInfo = ref('')
const platform = ref('')
const coreRunning = ref(false)

const remoteLoading = ref(false)
const remoteLoaded = ref(false)
const selectedChannel = ref('stable')
const selectedVersion = ref('')
type LinuxArchValue = 'amd64' | 'arm64'
type LinuxLibcValue = 'glibc' | 'musl' | 'universal'
type Amd64LevelValue = 'v3' | 'v2' | 'v1'
type OptionalLinuxArchValue = LinuxArchValue | null
type OptionalLinuxLibcValue = LinuxLibcValue | null
type OptionalAmd64LevelValue = Amd64LevelValue | null
const selectedLinuxArch = ref<OptionalLinuxArchValue>(null)
const selectedLinuxLibc = ref<OptionalLinuxLibcValue>(null)
const selectedAmd64Level = ref<OptionalAmd64LevelValue>(null)
type LinuxTargetPreference = {
  arch?: string
  libc?: string
  amd64Level?: string
  customUrl?: string
}
type CoreDownloadTarget = {
  os?: string
  arch?: string
  libc?: string
  amd64Level?: string
}
type CoreDownloadPreference = {
  target?: CoreDownloadTarget
  customUrl?: string
}
const amd64LevelItems = [
  { title: 'v3', value: 'v3' },
  { title: 'v2', value: 'v2' },
  { title: 'v1', value: 'v1' },
]
const showLinuxAmd64LevelSelector = computed(() => (
  (coreMeta.value.binaryBaseName === 'sing-box' || coreMeta.value.binaryBaseName === 'mihomo') &&
  selectedLinuxArch.value === 'amd64'
))
const requiresAmd64LevelForDownload = computed(() => coreMeta.value.binaryBaseName === 'mihomo')
const versionList = ref<any[]>([])
const hasMoreVersions = ref(false)
const loadingMoreVersions = ref(false)
const versionRequestSeq = ref(0)
const customDownloadURL = ref('')
const preferenceSaving = ref(false)

const downloading = ref(false)
const downloadingVersion = ref('')
const downloadProgressSessionId = ref('')
const downloadProgressTimerId = ref<number | null>(null)
const startingCore = ref(false)
const stoppingCore = ref(false)
const restartingCore = ref(false)
const deletingCore = ref(false)
const autoCheckSaving = ref(false)
const feedbackMsg = ref('')
const feedbackType = ref<'success' | 'error' | 'info'>('info')

const autoCheckEnabled = ref(false)
const autoCheckIntervalInput = ref('12')
const latestStableVersion = ref('')
const latestAlphaVersion = ref('')
const pendingStableVersion = ref('')
const pendingAlphaVersion = ref('')
const lastCheckedAt = ref(0)

type CoreDownloadProgress = {
  id: string
  core: string
  status: string
  stage: string
  runningBefore: boolean
  percent: number
  approximate: boolean
  downloadedBytes: number
  totalBytes: number
  error: string
}

const downloadProgress = ref<CoreDownloadProgress>({
  id: '',
  core: '',
  status: 'missing',
  stage: '',
  runningBefore: false,
  percent: 0,
  approximate: false,
  downloadedBytes: 0,
  totalBytes: 0,
  error: '',
})

const versionItems = computed(() => {
  return versionList.value.map((item) => ({
    title: (item.tag_name || item.tagName || item.name || '').replace(/^v/, ''),
    value: item.tag_name || item.tagName || '',
    prerelease: item.prerelease === true,
    assetName: item.asset_name || '',
  }))
})

const displayedVersionItems = computed(() => versionItems.value)

const canDownloadCustom = computed(() => /^https?:\/\/.+/i.test(customDownloadURL.value.trim()))

const hasCompleteLinuxTargetSelection = computed(() => {
  if (!showLinuxArchSelector.value) {
    return true
  }
  if (!selectedLinuxArch.value) {
    return false
  }
  if (showLinuxAmd64LevelSelector.value && requiresAmd64LevelForDownload.value && !selectedAmd64Level.value) {
    return false
  }
  if (showLinuxLibcSelector.value && !selectedLinuxLibc.value) {
    return false
  }
  return true
})

const canDownloadSelectedVersion = computed(() => (
  Boolean(selectedVersion.value) && hasCompleteLinuxTargetSelection.value
))

const latestRemoteVersion = computed(() => {
  if (versionList.value.length === 0) {
    return ''
  }
  const current = versionList.value[0]
  return (current.tag_name || current.tagName || '').replace(/^v/, '')
})
const remoteVersionLabel = computed(() => {
  if (remoteLoading.value) {
    return t('loading')
  }
  if (latestRemoteVersion.value) {
    return latestRemoteVersion.value
  }
  return remoteLoaded.value ? t('coreManager.unknown') : t('coreManager.notLoaded')
})

const latestStableVersionDisplay = computed(() => latestStableVersion.value.replace(/^v/, ''))
const latestAlphaVersionDisplay = computed(() => latestAlphaVersion.value.replace(/^v/, ''))
const effectiveChannel = computed(() => (
  supportsPrereleaseChannel.value ? selectedChannel.value : 'stable'
))
const nextRemoteLoadCount = computed(() => (
  versionList.value.length < 15 ? 5 : 20
))
const selectedLinuxPackageLabel = computed(() => {
  if (!showLinuxArchSelector.value) {
    return ''
  }
  if (!selectedLinuxArch.value) {
    return t('coreManager.notDetected')
  }
  const parts = ['linux', selectedLinuxArch.value]
  if (showLinuxAmd64LevelSelector.value) {
    parts.push(selectedAmd64Level.value || t('coreManager.notDetected'))
  }
  if (showLinuxLibcSelector.value) {
    parts.push(selectedLinuxLibc.value === 'universal'
      ? 'universal'
      : selectedLinuxLibc.value || t('coreManager.notDetected'))
  }
  return parts.join('/')
})
const downloadingVersionLabel = computed(() => (
  downloadingVersion.value === 'custom'
    ? t('coreManager.customBuild')
    : showLinuxArchSelector.value
      ? `${downloadingVersion.value} ${selectedLinuxPackageLabel.value}`
      : downloadingVersion.value
))

const hasActiveDownloadProgress = computed(() => (
  downloading.value || (
    downloadProgress.value.id.length > 0 &&
    downloadProgress.value.status !== 'missing'
  )
))

const downloadProgressPercent = computed(() => {
  const value = Number(downloadProgress.value.percent)
  if (!Number.isFinite(value)) {
    return 0
  }
  return Math.max(0, Math.min(100, value))
})

const downloadProgressStageText = computed(() => {
  switch (downloadProgress.value.stage) {
    case 'stopping':
      return t('coreManager.stageStopping')
    case 'downloading':
      return t('coreManager.stageDownloading')
    case 'replacing':
      return t('coreManager.stageReplacing')
    case 'validating':
      return t('coreManager.stageValidating')
    case 'starting':
      return t('coreManager.stageStarting')
    case 'started':
      return t('coreManager.stageStarted')
    case 'completed':
      return t('coreManager.stageCompleted')
    default:
      return downloading.value ? t('coreManager.stageDownloading') : t('coreManager.unknown')
  }
})

const downloadProgressStatusColor = computed(() => {
  if (downloadProgress.value.status === 'success') {
    return 'success'
  }
  if (downloadProgress.value.status === 'error') {
    return 'error'
  }
  return 'info'
})

const downloadProgressDetail = computed(() => {
  if (downloadProgress.value.status === 'error' && downloadProgress.value.error) {
    return downloadProgress.value.error
  }
  if (downloadProgress.value.totalBytes > 0) {
    const downloaded = HumanReadable.sizeFormat(downloadProgress.value.downloadedBytes)
    const total = HumanReadable.sizeFormat(downloadProgress.value.totalBytes)
    if (downloadProgress.value.approximate) {
      return `${downloaded} / ${total} (${t('coreManager.approximateProgress')})`
    }
    return `${downloaded} / ${total}`
  }
  if (downloadProgress.value.downloadedBytes > 0) {
    return HumanReadable.sizeFormat(downloadProgress.value.downloadedBytes)
  }
  return ''
})

const hasPendingUpdates = computed(() => {
  if (!supportsPrereleaseChannel.value) {
    return pendingStableVersion.value !== ''
  }
  return pendingStableVersion.value !== '' || pendingAlphaVersion.value !== ''
})

const pendingUpdateText = computed(() => {
  const parts: string[] = []
  if (pendingStableVersion.value) {
    parts.push(t('coreManager.pendingStable', { version: pendingStableVersion.value.replace(/^v/, '') }))
  }
  if (supportsPrereleaseChannel.value && pendingAlphaVersion.value) {
    parts.push(t('coreManager.pendingAlpha', { version: pendingAlphaVersion.value.replace(/^v/, '') }))
  }
  return parts.join(' | ')
})

const lastCheckedAtDisplay = computed(() => {
  if (!lastCheckedAt.value) {
    return ''
  }
  return new Date(lastCheckedAt.value * 1000).toLocaleString()
})

const customUrlPlaceholder = computed(() => `https://github.com/.../${coreMeta.value.coreName}-xxx.tar.gz`)

const binaryPath = computed(() => {
  const suffix = platform.value.startsWith('windows') ? '.exe' : ''
  const subdir = props.namespace === 'mihomo' ? 'mihomo' : 'singbox'
  return `Promanager_data/core/${subdir}/${coreMeta.value.binaryBaseName}${suffix}`
})
const getVersionTargetQuery = () => {
  const query: Record<string, string> = {}
  if (showLinuxArchSelector.value) {
    if (!selectedLinuxArch.value) {
      return query
    }
    query.target_os = 'linux'
    query.target_arch = selectedLinuxArch.value
    if (showLinuxAmd64LevelSelector.value && selectedAmd64Level.value) {
      query.target_amd64_level = selectedAmd64Level.value
    }
    if (showLinuxLibcSelector.value && selectedLinuxLibc.value) {
      query.target_libc = selectedLinuxLibc.value
    }
  }
  return query
}

const resetRemoteVersions = (clearSelection = true) => {
  versionRequestSeq.value += 1
  versionList.value = []
  hasMoreVersions.value = false
  remoteLoaded.value = false
  remoteLoading.value = false
  loadingMoreVersions.value = false
  if (clearSelection) {
    selectedVersion.value = ''
  }
}

const legacyLinuxTargetPreferenceStorageKey = computed(() => (
  `core-manager-linux-target:${coreMeta.value.binaryBaseName}:${props.namespace}`
))

const normalizeLinuxArch = (value: unknown): OptionalLinuxArchValue => {
  if (value === 'amd64' || value === 'arm64') {
    return value
  }
  return null
}

const normalizeLinuxLibc = (value: unknown): OptionalLinuxLibcValue => {
  if (value === 'glibc' || value === 'musl' || value === 'universal') {
    return value
  }
  return null
}

const normalizeAmd64LevelValue = (value: unknown): OptionalAmd64LevelValue => {
  if (value === 'v3' || value === 'v2' || value === 'v1') {
    return value
  }
  return null
}

const clearLinuxTargetSelection = () => {
  selectedLinuxArch.value = null
  selectedLinuxLibc.value = null
  selectedAmd64Level.value = null
}

const applyTargetSelection = (target: CoreDownloadTarget | undefined | null) => {
  if (!target) {
    clearLinuxTargetSelection()
    return
  }
  const arch = normalizeLinuxArch(target.arch)
  selectedLinuxArch.value = arch
  selectedLinuxLibc.value = normalizeLinuxLibc(target.libc)
  selectedAmd64Level.value = arch === 'amd64'
    ? normalizeAmd64LevelValue(target.amd64Level)
    : null
}

const readLegacyLinuxTargetPreference = (): LinuxTargetPreference | null => {
  const raw = localStorage.getItem(legacyLinuxTargetPreferenceStorageKey.value)
  if (!raw) {
    return null
  }

  try {
    return JSON.parse(raw) as LinuxTargetPreference
  } catch (error) {
    console.warn('Failed to parse core linux target preference:', error)
    return null
  }
}

const buildCurrentDownloadPreference = (): CoreDownloadPreference => {
  const target: CoreDownloadTarget = {}
  if (showLinuxArchSelector.value && selectedLinuxArch.value) {
    target.os = 'linux'
    target.arch = selectedLinuxArch.value
    if (selectedLinuxArch.value === 'amd64' && selectedAmd64Level.value) {
      target.amd64Level = selectedAmd64Level.value
    }
    if (showLinuxLibcSelector.value && selectedLinuxLibc.value) {
      target.libc = selectedLinuxLibc.value
    }
  }
  return {
    target,
    customUrl: customDownloadURL.value.trim(),
  }
}

const buildDownloadPreferenceFormData = (includeTarget = true) => {
  const preference = buildCurrentDownloadPreference()
  const formData = new FormData()
  formData.append('custom_url', preference.customUrl || '')
  if (!includeTarget) {
    return formData
  }
  if (preference.target?.os) {
    formData.append('target_os', preference.target.os)
  }
  if (preference.target?.arch) {
    formData.append('target_arch', preference.target.arch)
  }
  if (preference.target?.amd64Level) {
    formData.append('target_amd64_level', preference.target.amd64Level)
  }
  if (preference.target?.libc) {
    formData.append('target_libc', preference.target.libc)
  }
  return formData
}

const applyDownloadPreference = (preference: CoreDownloadPreference | undefined | null) => {
  if (!preference) {
    return
  }
  if (typeof preference.customUrl === 'string') {
    customDownloadURL.value = preference.customUrl
  }
}

const applyStatusDownloadState = (status: any) => {
  applyDownloadPreference(status?.downloadPreference)
  if (!status?.downloadPreference?.customUrl && !customDownloadURL.value) {
    const legacyPreference = readLegacyLinuxTargetPreference()
    if (legacyPreference?.customUrl) {
      customDownloadURL.value = legacyPreference.customUrl
    }
  }
  const installedTarget = status?.installedTarget as CoreDownloadTarget | undefined
  if (installedTarget && (installedTarget.arch || installedTarget.os)) {
    applyTargetSelection(installedTarget)
    return
  }
  clearLinuxTargetSelection()
}

const saveDownloadPreference = async (includeTarget = false) => {
  if (preferenceSaving.value) {
    return
  }
  preferenceSaving.value = true
  try {
    const data = await HttpUtils.post(
      coreMeta.value.downloadPreferenceEndpoint,
      buildDownloadPreferenceFormData(includeTarget),
      { silentAuthCheck: true },
    )
    if (data.success && data.obj) {
      applyDownloadPreference(data.obj as CoreDownloadPreference)
    } else if (!data.success && data.msg) {
      feedbackMsg.value = data.msg || t('coreManager.downloadPreferenceSaveFailed')
      feedbackType.value = 'error'
    }
  } catch (error: any) {
    feedbackMsg.value = t('coreManager.downloadPreferenceSaveFailedWithReason', {
      reason: error.message || t('coreManager.unknown'),
    })
    feedbackType.value = 'error'
  } finally {
    preferenceSaving.value = false
  }
}

watch(
  () => props.visible,
  (newValue) => {
    dialogVisible.value = newValue
    if (newValue) {
      if (downloadProgressSessionId.value) {
        startDownloadProgressPolling(downloadProgressSessionId.value)
      }
      void refreshAll()
    }
  },
)

watch(dialogVisible, (newValue) => {
  if (!newValue) {
    stopDownloadProgressPolling()
    close()
  }
})

watch(selectedChannel, () => {
  if (!supportsPrereleaseChannel.value) {
    return
  }
  const shouldReload = remoteLoaded.value
  resetRemoteVersions()
  if (shouldReload) {
    void loadRemoteVersions(false)
  }
})

watch([selectedLinuxArch, selectedLinuxLibc, selectedAmd64Level], () => {
  if (!remoteLoaded.value) {
    return
  }
  const shouldReload = true
  resetRemoteVersions()
  if (shouldReload) {
    void loadRemoteVersions(false)
  }
})

watch(
  () => props.namespace,
  () => {
    stopDownloadProgressPolling()
    resetDownloadProgress()
    downloadProgressSessionId.value = ''
    clearLinuxTargetSelection()
    customDownloadURL.value = ''
    selectedChannel.value = 'stable'
    resetRemoteVersions()
    feedbackMsg.value = ''
    if (dialogVisible.value) {
      void refreshAll()
    }
  },
)

const close = () => {
  stopDownloadProgressPolling()
  emit('close')
  emit('update:modelValue', false)
}

const normalizeCoreDownloadProgress = (raw: any): CoreDownloadProgress => ({
  id: String(raw?.id ?? '').trim(),
  core: String(raw?.core ?? '').trim(),
  status: String(raw?.status ?? '').trim().toLowerCase() || 'missing',
  stage: String(raw?.stage ?? '').trim().toLowerCase(),
  runningBefore: raw?.runningBefore === true,
  percent: Number.isFinite(Number(raw?.percent)) ? Number(raw.percent) : 0,
  approximate: raw?.approximate === true,
  downloadedBytes: Number.isFinite(Number(raw?.downloadedBytes)) ? Math.max(0, Number(raw.downloadedBytes)) : 0,
  totalBytes: Number.isFinite(Number(raw?.totalBytes)) ? Math.max(0, Number(raw.totalBytes)) : 0,
  error: String(raw?.error ?? '').trim(),
})

const resetDownloadProgress = () => {
  downloadProgress.value = {
    id: '',
    core: '',
    status: 'missing',
    stage: '',
    runningBefore: false,
    percent: 0,
    approximate: false,
    downloadedBytes: 0,
    totalBytes: 0,
    error: '',
  }
}

const makeDownloadSessionId = () => {
  const randomPart = typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function'
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(16).slice(2)}`
  return `core-download-${coreMeta.value.binaryBaseName}-${randomPart}`
}

const stopDownloadProgressPolling = () => {
  if (downloadProgressTimerId.value != null) {
    window.clearInterval(downloadProgressTimerId.value)
    downloadProgressTimerId.value = null
  }
}

const pollDownloadProgress = async () => {
  const sessionId = downloadProgressSessionId.value.trim()
  if (!sessionId) {
    return
  }
  const data = await HttpUtils.get(coreMeta.value.progressEndpoint, { id: sessionId }, { silentAuthCheck: true })
  if (!data.success) {
    if (!downloading.value) {
      downloadProgress.value = {
        ...downloadProgress.value,
        status: 'error',
        error: data.msg || t('coreManager.downloadProgressUnavailable'),
      }
      stopDownloadProgressPolling()
    }
    return
  }
  const nextProgress = normalizeCoreDownloadProgress(data.obj)
  if (nextProgress.status === 'missing' && downloading.value) {
    return
  }
  downloadProgress.value = nextProgress
  if (nextProgress.status === 'success' || nextProgress.status === 'error' || nextProgress.status === 'missing') {
    stopDownloadProgressPolling()
  }
}

const startDownloadProgressPolling = (sessionId: string) => {
  stopDownloadProgressPolling()
  downloadProgressSessionId.value = sessionId.trim()
  if (!downloadProgressSessionId.value) {
    return
  }
  downloadProgressTimerId.value = window.setInterval(() => {
    void pollDownloadProgress()
  }, 800)
  void pollDownloadProgress()
}

const refreshAll = async (forceUpdateCheck = false) => {
  statusLoading.value = true
  try {
    await Promise.all([
      loadCoreStatus(),
      loadCoreUpdateInfo(forceUpdateCheck),
    ])
  } finally {
    statusLoading.value = false
  }
}

const loadMoreVersions = () => {
  void loadRemoteVersions(true)
}

const resetVersionDisplay = () => {
  resetRemoteVersions()
  void loadRemoteVersions(false)
}

const loadCoreStatus = async () => {
  try {
    const data = await HttpUtils.get(coreMeta.value.statusEndpoint)
    if (data.success && data.obj) {
      localVersion.value = data.obj.localVersion || ''
      versionInfo.value = data.obj.versionInfo || ''
      coreRunning.value = data.obj.running === true
      platform.value = data.obj.platform || ''
      applyStatusDownloadState(data.obj)
    }
  } catch (error) {
    console.error('Failed to load core status:', error)
  }
}

const loadRemoteVersions = async (append: boolean) => {
  if (append && !hasMoreVersions.value) {
    return
  }

  const requestId = ++versionRequestSeq.value
  const requestOffset = append ? versionList.value.length : 0
  const requestLimit = append ? nextRemoteLoadCount.value : 5
  if (append) {
    loadingMoreVersions.value = true
  } else {
    remoteLoading.value = true
  }

  try {
    const data = await HttpUtils.get(coreMeta.value.versionsEndpoint, {
      channel: effectiveChannel.value,
      offset: requestOffset,
      limit: requestLimit,
      ...getVersionTargetQuery(),
    })

    if (requestId !== versionRequestSeq.value) {
      return
    }

    if (data.success && data.obj && Array.isArray(data.obj.versions)) {
      const incoming = data.obj.versions
      versionList.value = append ? [...versionList.value, ...incoming] : incoming
      hasMoreVersions.value = data.obj.has_more === true
      remoteLoaded.value = true
      if (versionList.value.length > 0) {
        const selectedStillExists = versionList.value.some((item) => (
          (item.tag_name || item.tagName || '') === selectedVersion.value
        ))
        if (!selectedVersion.value || !selectedStillExists) {
          selectedVersion.value = versionList.value[0].tag_name || versionList.value[0].tagName || ''
        }
      } else if (!append) {
        selectedVersion.value = ''
      }
      return
    }

    if (!append) {
      versionList.value = []
      hasMoreVersions.value = false
      selectedVersion.value = ''
    }
  } catch (error) {
    if (requestId !== versionRequestSeq.value) {
      return
    }
    console.error('Failed to fetch remote versions:', error)
    if (!append) {
      versionList.value = []
      hasMoreVersions.value = false
      selectedVersion.value = ''
    }
  } finally {
    if (requestId === versionRequestSeq.value) {
      remoteLoaded.value = true
    }
    if (append) {
      if (requestId === versionRequestSeq.value) {
        loadingMoreVersions.value = false
      }
    } else {
      if (requestId === versionRequestSeq.value) {
        remoteLoading.value = false
      }
    }
  }
}

const applyCoreUpdateInfo = (info: any) => {
  autoCheckEnabled.value = info.enabled === true
  autoCheckIntervalInput.value = String(info.intervalHours || 12)
  latestStableVersion.value = info.latestStable || ''
  latestAlphaVersion.value = supportsPrereleaseChannel.value ? (info.latestAlpha || '') : ''
  pendingStableVersion.value = info.pendingStable || ''
  pendingAlphaVersion.value = supportsPrereleaseChannel.value ? (info.pendingAlpha || '') : ''
  lastCheckedAt.value = Number(info.lastCheckedAt || 0)
}

const loadCoreUpdateInfo = async (forceCheck: boolean) => {
  try {
    const data = await HttpUtils.get(
      coreMeta.value.updateInfoEndpoint,
      forceCheck ? { force: 'true' } : {},
    )
    if (data.success && data.obj) {
      applyCoreUpdateInfo(data.obj)
    }
  } catch (error) {
    console.error('Failed to load core update info:', error)
  }
}

const normalizeIntervalHours = (raw: string): number | null => {
  const trimmed = raw.trim().toLowerCase().replace(/h$/, '').trim()
  if (!/^\d+$/.test(trimmed)) {
    return null
  }
  const value = Number(trimmed)
  if (!Number.isInteger(value) || value <= 0) {
    return null
  }
  return value
}

const saveAutoCheckSettings = async () => {
  const intervalHours = normalizeIntervalHours(autoCheckIntervalInput.value)
  if (autoCheckEnabled.value && intervalHours == null) {
    feedbackMsg.value = t('coreManager.intervalInvalid')
    feedbackType.value = 'error'
    return
  }

  autoCheckSaving.value = true
  feedbackMsg.value = ''
  try {
    const data = await HttpUtils.post(coreMeta.value.updateSettingsEndpoint, {
      enabled: autoCheckEnabled.value ? 'true' : 'false',
      interval: String(intervalHours ?? 12),
    })
    if (data.success && data.obj) {
      applyCoreUpdateInfo(data.obj)
      feedbackMsg.value = t('coreManager.autoCheckSaved')
      feedbackType.value = 'success'
    } else {
      feedbackMsg.value = data.msg || t('coreManager.autoCheckSaveFailed')
      feedbackType.value = 'error'
    }
  } catch (error: any) {
    feedbackMsg.value = t('coreManager.autoCheckSaveFailedWithReason', {
      reason: error.message || t('coreManager.unknown'),
    })
    feedbackType.value = 'error'
  } finally {
    autoCheckSaving.value = false
  }
}

const ackCoreUpdateNotice = async () => {
  pendingStableVersion.value = ''
  pendingAlphaVersion.value = ''
  try {
    const data = await HttpUtils.post(coreMeta.value.updateAckEndpoint, {})
    if (data.success && data.obj) {
      applyCoreUpdateInfo(data.obj)
    }
  } catch (error) {
    console.error('Failed to acknowledge core update notice:', error)
  }
}

const downloadCore = async () => {
  if (!selectedVersion.value || downloading.value) {
    return
  }
  if (!hasCompleteLinuxTargetSelection.value) {
    feedbackMsg.value = t('coreManager.downloadTargetRequired')
    feedbackType.value = 'error'
    return
  }

  const sessionId = makeDownloadSessionId()
  downloading.value = true
  downloadingVersion.value = selectedVersion.value.replace(/^v/, '')
  downloadProgressSessionId.value = sessionId
  resetDownloadProgress()
  downloadProgress.value.id = sessionId
  downloadProgress.value.core = coreMeta.value.coreName
  downloadProgress.value.status = 'running'
  startDownloadProgressPolling(sessionId)
  feedbackMsg.value = ''

  try {
    const formData = new FormData()
    formData.append('version', selectedVersion.value)
    formData.append('downloadSessionId', sessionId)
    if (showLinuxArchSelector.value && selectedLinuxArch.value) {
      formData.append('target_os', 'linux')
      formData.append('target_arch', selectedLinuxArch.value)
      if (showLinuxAmd64LevelSelector.value && selectedAmd64Level.value) {
        formData.append('target_amd64_level', selectedAmd64Level.value)
      }
    }
    if (showLinuxLibcSelector.value && selectedLinuxLibc.value) {
      formData.append('target_libc', selectedLinuxLibc.value)
    }
    const data = await HttpUtils.post(coreMeta.value.downloadEndpoint, formData)
    await pollDownloadProgress()
    if (data.success) {
      const version = data.obj?.version || ''
      feedbackMsg.value = t('coreManager.downloadSuccess', {
        coreName: coreMeta.value.coreName,
        version,
      })
      feedbackType.value = 'success'
      setTimeout(() => {
        void loadCoreStatus()
      }, 1200)
      await loadCoreUpdateInfo(false)
      return
    }
    if (downloadProgress.value.status === 'running') {
      downloadProgress.value = {
        ...downloadProgress.value,
        status: 'error',
        error: data.msg || t('coreManager.downloadFailed'),
      }
    }
    feedbackMsg.value = data.msg || t('coreManager.downloadFailed')
    feedbackType.value = 'error'
  } catch (error: any) {
    if (downloadProgress.value.status === 'running') {
      downloadProgress.value = {
        ...downloadProgress.value,
        status: 'error',
        error: error.message || t('coreManager.unknown'),
      }
    }
    feedbackMsg.value = t('coreManager.downloadFailedWithReason', {
      reason: error.message || t('coreManager.unknown'),
    })
    feedbackType.value = 'error'
  } finally {
    stopDownloadProgressPolling()
    downloading.value = false
  }
}

const downloadCoreFromCustomURL = async () => {
  const url = customDownloadURL.value.trim()
  if (!/^https?:\/\/.+/i.test(url) || downloading.value) {
    return
  }

  const sessionId = makeDownloadSessionId()
  downloading.value = true
  downloadingVersion.value = 'custom'
  downloadProgressSessionId.value = sessionId
  resetDownloadProgress()
  downloadProgress.value.id = sessionId
  downloadProgress.value.core = coreMeta.value.coreName
  downloadProgress.value.status = 'running'
  startDownloadProgressPolling(sessionId)
  feedbackMsg.value = ''

  try {
    const formData = new FormData()
    formData.append('custom_url', url)
    formData.append('downloadSessionId', sessionId)
    const data = await HttpUtils.post(coreMeta.value.downloadEndpoint, formData)
    await pollDownloadProgress()
    if (data.success) {
      const version = data.obj?.version || ''
      feedbackMsg.value = t('coreManager.customDownloadSuccess', { version })
      feedbackType.value = 'success'
      setTimeout(() => {
        void loadCoreStatus()
      }, 1200)
      await loadCoreUpdateInfo(false)
      return
    }
    if (downloadProgress.value.status === 'running') {
      downloadProgress.value = {
        ...downloadProgress.value,
        status: 'error',
        error: data.msg || t('coreManager.customDownloadFailed'),
      }
    }
    feedbackMsg.value = data.msg || t('coreManager.customDownloadFailed')
    feedbackType.value = 'error'
  } catch (error: any) {
    if (downloadProgress.value.status === 'running') {
      downloadProgress.value = {
        ...downloadProgress.value,
        status: 'error',
        error: error.message || t('coreManager.unknown'),
      }
    }
    feedbackMsg.value = t('coreManager.customDownloadFailedWithReason', {
      reason: error.message || t('coreManager.unknown'),
    })
    feedbackType.value = 'error'
  } finally {
    stopDownloadProgressPolling()
    downloading.value = false
  }
}

const openReleasePage = () => {
  window.open(coreMeta.value.repoUrl, '_blank')
}

const startCore = async () => {
  startingCore.value = true
  feedbackMsg.value = ''
  try {
    const data = await HttpUtils.post(coreMeta.value.startEndpoint, {})
    if (data.success) {
      feedbackMsg.value = t('coreManager.startSuccess', { coreName: coreMeta.value.coreName })
      feedbackType.value = 'success'
    } else {
      feedbackMsg.value = data.msg || t('coreManager.startFailed')
      feedbackType.value = 'error'
    }
    setTimeout(() => {
      void loadCoreStatus()
    }, 1500)
  } catch (error: any) {
    feedbackMsg.value = t('coreManager.startFailedWithReason', {
      reason: error.message || t('coreManager.unknown'),
    })
    feedbackType.value = 'error'
  } finally {
    setTimeout(() => {
      startingCore.value = false
    }, 1500)
  }
}

const stopCore = async () => {
  stoppingCore.value = true
  feedbackMsg.value = ''
  try {
    const data = await HttpUtils.post(coreMeta.value.stopEndpoint, {})
    if (data.success) {
      feedbackMsg.value = t('coreManager.stopSuccess', { coreName: coreMeta.value.coreName })
      feedbackType.value = 'info'
    } else {
      feedbackMsg.value = data.msg || t('coreManager.stopFailed')
      feedbackType.value = 'error'
    }
    setTimeout(() => {
      void loadCoreStatus()
    }, 1500)
  } catch (error: any) {
    feedbackMsg.value = t('coreManager.stopFailedWithReason', {
      reason: error.message || t('coreManager.unknown'),
    })
    feedbackType.value = 'error'
  } finally {
    setTimeout(() => {
      stoppingCore.value = false
    }, 1500)
  }
}

const restartCore = async () => {
  restartingCore.value = true
  feedbackMsg.value = ''
  try {
    const data = await HttpUtils.post(coreMeta.value.restartEndpoint, {})
    if (data.success) {
      feedbackMsg.value = t('coreManager.restartSuccess', { coreName: coreMeta.value.coreName })
      feedbackType.value = 'success'
    } else {
      feedbackMsg.value = data.msg || t('coreManager.restartFailed')
      feedbackType.value = 'error'
    }
    setTimeout(() => {
      void loadCoreStatus()
    }, 2500)
  } catch (error: any) {
    feedbackMsg.value = t('coreManager.restartFailedWithReason', {
      reason: error.message || t('coreManager.unknown'),
    })
    feedbackType.value = 'error'
  } finally {
    setTimeout(() => {
      restartingCore.value = false
    }, 2500)
  }
}

const deleteCore = async () => {
  if (deletingCore.value || !localVersion.value) {
    return
  }

  const confirmDelete = window.confirm(
    t('coreManager.deleteCoreConfirm', { coreName: coreMeta.value.coreName }),
  )
  if (!confirmDelete) {
    return
  }

  deletingCore.value = true
  feedbackMsg.value = ''
  try {
    const data = await HttpUtils.post(coreMeta.value.deleteEndpoint, {})
    if (data.success) {
      feedbackMsg.value = t('coreManager.deleteSuccess', { coreName: coreMeta.value.coreName })
      feedbackType.value = 'success'
      localVersion.value = ''
      versionInfo.value = ''
      coreRunning.value = false
      await loadCoreStatus()
      await loadCoreUpdateInfo(false)
      return
    }
    feedbackMsg.value = data.msg || t('coreManager.deleteFailed')
    feedbackType.value = 'error'
  } catch (error: any) {
    feedbackMsg.value = t('coreManager.deleteFailedWithReason', {
      reason: error.message || t('coreManager.unknown'),
    })
    feedbackType.value = 'error'
  } finally {
    deletingCore.value = false
  }
}
</script>
