<template>
  <v-dialog
    v-model="dialogVisible"
    transition="dialog-bottom-transition"
    width="760"
    max-width="95vw"
  >
    <v-card class="rounded-lg core-modal-card">
      <v-card-title>
        <v-row align="center">
          <v-col cols="auto" class="d-flex align-center" style="gap: 8px;">
            <v-icon icon="mdi-engine"></v-icon>
            <span>{{ t(mihomoCore.modalTitle) }}</span>
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

      <v-card-text class="core-modal-body">
        <div class="d-flex align-center flex-wrap mb-2 core-modal-summary" style="gap: 8px;">
          <span class="text-h6 font-weight-bold">{{ mihomoCore.coreName }}</span>
          <v-btn icon size="x-small" variant="text" @click="openReleasePage">
            <v-icon size="18">mdi-open-in-new</v-icon>
            <v-tooltip activator="parent" location="top">{{ t('coreManager.releasePage') }}</v-tooltip>
          </v-btn>
          <v-btn
            icon
            size="x-small"
            variant="text"
            :loading="statusLoading"
            :disabled="statusLoading || coreControlBusy"
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
            :disabled="!installed || downloading || coreDownloadTaskActive || coreControlBusy"
            :loading="deletingCore"
            @click="deleteCore"
          >
            {{ t('coreManager.deleteCore') }}
          </v-btn>
        </div>

        <div class="d-flex align-center flex-wrap mb-4" style="gap: 12px">
          <v-chip
            variant="outlined"
            :color="!installed ? 'error' : compatible ? 'success' : 'error'"
            size="small"
            label
            class="core-status-chip"
          >
            {{ t('coreManager.local') }}: {{ !installed ? t('coreManager.notInstalled') : compatible ? localVersion : t('coreManager.incompatible') }}
          </v-chip>

          <v-chip
            v-if="installed && installedTargetLabel"
            variant="outlined"
            color="primary"
            size="small"
            label
            class="core-status-chip"
          >
            {{ t('coreManager.installedTarget') }}: {{ installedTargetLabel }}
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
          v-if="compatible && versionInfo"
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
          v-if="installed && !compatible"
          type="error"
          variant="tonal"
          density="compact"
          class="mb-4 core-incompatible-alert"
        >
          <div>{{ t('coreManager.incompatibleHint') }}</div>
          <div class="core-incompatible-path">{{ binaryPath }}</div>
        </v-alert>

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

        <v-row align="center" class="mb-4">
          <v-col cols="12" sm="6" class="core-mobile-full">
            <v-select
              v-model="coreLogLevel"
              :items="coreLogLevelItems"
              :label="t('coreManager.logLevel')"
              variant="outlined"
              density="compact"
              hide-details
              :disabled="coreLogLevelSaving || coreControlBusy || coreDownloadTaskActive"
              @update:model-value="saveCoreLogLevel"
            />
          </v-col>
        </v-row>

        <v-card v-if="downloading" variant="outlined" rounded="lg" class="mb-4">
          <v-card-text>
            <div class="text-caption text-medium-emphasis mb-2">
              {{ t('coreManager.downloading', { coreName: mihomoCore.coreName, version: downloadingVersionLabel }) }}
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
        <v-row align="center" class="core-version-row">
          <v-col v-if="supportsPrereleaseChannel" cols="auto" class="core-mobile-full">
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

          <v-col class="core-mobile-full">
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

          <v-col cols="auto" class="core-action-col">
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

          <v-col cols="auto" class="core-action-col">
            <v-btn
              color="primary"
              variant="flat"
              :prepend-icon="coreDownloadTaskActive ? (coreDownloadTaskApplying ? 'mdi-progress-wrench' : 'mdi-stop') : 'mdi-download'"
              :disabled="coreControlBusy || coreDownloadTaskStopping || coreDownloadTaskApplying || (!coreDownloadTaskActive && (!canDownloadSelectedVersion || downloading))"
              @click="coreDownloadTaskActive ? stopCoreDownload() : downloadCore()"
            >
              {{ coreDownloadTaskActive ? coreDownloadStopLabel : t('coreManager.download') }}
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
          <v-col class="core-mobile-full">
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
          <v-col cols="auto" class="core-action-col">
            <v-btn
              color="secondary"
              variant="flat"
              prepend-icon="mdi-link-variant-plus"
              :disabled="coreControlBusy || coreDownloadTaskActive || !canDownloadCustom || downloading"
              @click="downloadCoreFromCustomURL"
            >
              {{ t('coreManager.customDownload') }}
            </v-btn>
          </v-col>
        </v-row>

        <v-row v-if="showLinuxArchSelector" class="mt-3">
          <v-col cols="12">
            <div class="text-caption text-medium-emphasis mb-2">
              {{ archSelectorLabel }}
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
              :loading="autoCheckSaving"
              :disabled="autoCheckSaving || autoUpdateSaving || intervalSaving"
              @update:model-value="saveAutoCheckEnabled"
            ></v-switch>
          </v-col>
          <v-col cols="12" sm="4">
            <v-text-field
              v-model="autoCheckIntervalInput"
              :disabled="!autoCheckEnabled || autoCheckSaving || autoUpdateSaving || intervalSaving"
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
              :disabled="!autoCheckEnabled || autoCheckSaving || autoUpdateSaving || intervalSaving"
              :loading="intervalSaving"
              @click="saveAutoCheckInterval"
            >
              {{ t('actions.save') }}
            </v-btn>
          </v-col>
        </v-row>

        <v-row align="center" class="mt-1">
          <v-col cols="12" sm="7">
            <v-switch
              v-model="autoUpdateEnabled"
              color="primary"
              density="compact"
              hide-details
              :label="t('coreManager.enableAutoUpdate')"
              :disabled="autoUpdateSwitchDisabled"
              :loading="autoUpdateSaving"
              @update:model-value="saveAutoUpdateEnabled"
            ></v-switch>
          </v-col>
          <v-col cols="12" sm="5">
            <div class="text-caption text-medium-emphasis core-auto-update-meta">
              {{ t('coreManager.autoUpdateLastAttempt', { time: autoUpdateLastAttemptDisplay || t('coreManager.never') }) }}
            </div>
            <div class="text-caption text-medium-emphasis core-auto-update-meta">
              {{ t('coreManager.autoUpdateLastSuccess', { time: autoUpdateLastSuccessDisplay || t('coreManager.never') }) }}
            </div>
          </v-col>
        </v-row>

        <v-row v-if="autoUpdateDisabledReasonText" class="mt-1">
          <v-col cols="12">
            <div class="text-caption text-warning core-auto-update-reason">
              {{ autoUpdateDisabledReasonText }}
            </div>
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

        <v-alert
          v-if="autoUpdateErrorText"
          type="error"
          variant="tonal"
          density="compact"
          class="mt-2"
          closable
          @click:close="ackCoreAutoUpdateError"
        >
          {{ autoUpdateErrorText }}
        </v-alert>

        <v-divider class="my-6"></v-divider>

        <div class="text-subtitle-1 font-weight-medium mb-3">{{ t('coreManager.coreControl') }}</div>
        <v-card variant="outlined" rounded="lg">
          <v-card-text>
            <v-row align="center">
              <v-col cols="auto" class="core-mobile-full">
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

              <v-col cols="auto" class="d-flex flex-wrap core-action-col core-control-actions" style="gap: 8px">
                <v-btn
                  color="success"
                  variant="flat"
                  size="small"
                  prepend-icon="mdi-play"
                  :disabled="coreDownloadTaskActive || coreControlBusy || coreRunning || !coreReady"
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
                  :disabled="coreDownloadTaskActive || coreControlBusy || !coreRunning"
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
                  :disabled="coreDownloadTaskActive || coreControlBusy || !coreRunning || !coreReady"
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
                <v-chip variant="tonal" size="small" label class="core-path-chip">
                  <v-icon start size="x-small">mdi-file-cog</v-icon>
                  {{ mihomoCore.configPath }}
                </v-chip>
              </v-col>
            </v-row>

            <v-row class="mt-1">
              <v-col cols="12">
                <div class="text-caption text-medium-emphasis mb-1">{{ t('coreManager.binaryPath') }}</div>
                <v-chip variant="tonal" size="small" label class="core-path-chip">
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
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import HttpUtils from '@/plugins/httputil'
import { confirm } from '@/plugins/confirm'
import { useI18n } from 'vue-i18n'
import { HumanReadable } from '@/plugins/utils'
import { formatPanelDateTime } from '@/plugins/panelTime'

const props = defineProps<{
  visible: boolean
}>()

const emit = defineEmits(['close', 'update:modelValue'])
const { t } = useI18n()

const dialogVisible = ref(props.visible)
const mihomoCore = {
  coreName: 'mihomo',
  modalTitle: 'coreManager.mihomoTitle',
  repoUrl: 'https://github.com/MetaCubeX/mihomo/releases',
  statusEndpoint: 'api/mihomo-core-status',
  progressEndpoint: 'api/mihomo-core-download-progress',
  versionsEndpoint: 'api/mihomo-core-versions',
  updateInfoEndpoint: 'api/mihomo-core-update-info',
  updateSettingsEndpoint: 'api/mihomo-core-update-settings',
  updateAckEndpoint: 'api/mihomo-core-update-ack',
  downloadPreferenceEndpoint: 'api/mihomo-core-download-preference',
  logLevelEndpoint: 'api/mihomo-core-log-level',
  downloadEndpoint: 'api/mihomo-coreDownload',
  startEndpoint: 'api/mihomo-coreStart',
  stopEndpoint: 'api/mihomo-coreStop',
  restartEndpoint: 'api/mihomo-coreRestart',
  deleteEndpoint: 'api/mihomo-coreDelete',
  configPath: 'Promanager_data/core/mihomo/server.yaml',
  binaryBaseName: 'mihomo',
} as const
const supportsPrereleaseChannel = computed(() => true)
const showLinuxArchSelector = computed(() => true)

const statusLoading = ref(false)
const statusRequestSeq = ref(0)
const updateInfoRequestSeq = ref(0)
const localVersion = ref('')
const versionInfo = ref('')
const platform = ref('')
const coreRunning = ref(false)
const installed = ref(false)
const compatible = ref(false)
const installedChannel = ref<'stable' | 'alpha' | ''>('')
const coreLogLevel = ref('silent')
const confirmedCoreLogLevel = ref('silent')
const coreLogLevelSaving = ref(false)
const coreLogLevelItems = [
  { title: 'silent', value: 'silent' },
  { title: 'error', value: 'error' },
  { title: 'warning', value: 'warning' },
  { title: 'info', value: 'info' },
  { title: 'debug', value: 'debug' },
]

const remoteLoading = ref(false)
const remoteLoaded = ref(false)
const selectedChannel = ref('stable')
const selectedVersion = ref('')
type LinuxArchValue = 'amd64' | 'arm64'
type Amd64LevelValue = 'v3' | 'v2' | 'v1'
type OptionalLinuxArchValue = LinuxArchValue | null
type OptionalAmd64LevelValue = Amd64LevelValue | null
const selectedLinuxArch = ref<OptionalLinuxArchValue>(null)
const selectedAmd64Level = ref<OptionalAmd64LevelValue>(null)
type LinuxTargetPreference = {
  arch?: string
  amd64Level?: string
  customUrl?: string
}
type CoreDownloadTarget = {
  os?: string
  arch?: string
  amd64Level?: string
}
const installedTarget = ref<CoreDownloadTarget | null>(null)
type RuntimeTargetOS = 'linux' | 'windows' | ''
const runtimeTargetOS = computed<RuntimeTargetOS>(() => {
  const installedOS = String(installedTarget.value?.os ?? '').trim().toLowerCase()
  if (installedOS === 'linux' || installedOS === 'windows') {
    return installedOS
  }
  const platformText = String(platform.value ?? '').trim().toLowerCase()
  if (platformText.startsWith('windows/')) {
    return 'windows'
  }
  if (platformText.startsWith('linux/')) {
    return 'linux'
  }
  return ''
})
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
  selectedLinuxArch.value === 'amd64'
))
const requiresAmd64LevelForDownload = computed(() => true)
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
let downloadProgressRequest: Promise<void> | null = null
let downloadProgressController: AbortController | null = null
let recoverDownloadProgressController: AbortController | null = null
const startingCore = ref(false)
const stoppingCore = ref(false)
const restartingCore = ref(false)
const deletingCore = ref(false)
const autoCheckSaving = ref(false)
const autoUpdateSaving = ref(false)
const intervalSaving = ref(false)
const feedbackMsg = ref('')
const feedbackType = ref<'success' | 'error' | 'info'>('info')
let downloadFeedbackTimer: number | null = null
const coreActionTimers = new Set<number>()

const scheduleCoreAction = (callback: () => void, delay: number) => {
  const timer = window.setTimeout(() => {
    coreActionTimers.delete(timer)
    callback()
  }, delay)
  coreActionTimers.add(timer)
}

const clearCoreActionTimers = () => {
  for (const timer of coreActionTimers) {
    window.clearTimeout(timer)
  }
  coreActionTimers.clear()
}

const autoCheckEnabled = ref(false)
const autoCheckIntervalInput = ref('12')
const latestStableVersion = ref('')
const latestAlphaVersion = ref('')
const pendingStableVersion = ref('')
const pendingAlphaVersion = ref('')
const lastCheckedAt = ref(0)
const autoUpdateEnabled = ref(false)
const autoUpdateDisabled = ref(false)
const autoUpdateDisableReason = ref('')
const autoUpdateLastAttemptAt = ref(0)
const autoUpdateLastSuccessAt = ref(0)
const autoUpdateError = ref('')
const autoUpdateErrorAt = ref(0)

type CoreDownloadProgress = {
  id: string
  core: string
  status: string
  state: string
  stage: string
  canCancel: boolean
  stopRequested: boolean
  deadlineExceeded: boolean
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
  state: 'idle',
  stage: '',
  canCancel: false,
  stopRequested: false,
  deadlineExceeded: false,
  runningBefore: false,
  percent: 0,
  approximate: false,
  downloadedBytes: 0,
  totalBytes: 0,
  error: '',
})

const versionItems = computed(() => {
  return versionList.value.map((item) => ({
    title: (item.version || item.tag_name || item.tagName || item.name || '').replace(/^v/, ''),
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
  return true
})

const canDownloadSelectedVersion = computed(() => (
  Boolean(selectedVersion.value) && hasCompleteLinuxTargetSelection.value
))
const coreReady = computed(() => installed.value && compatible.value)
const coreControlBusy = computed(() => (
  startingCore.value || stoppingCore.value || restartingCore.value || deletingCore.value
))

const latestRemoteVersion = computed(() => {
  if (versionList.value.length === 0) {
    return ''
  }
  const current = versionList.value[0]
  return (current.version || current.tag_name || current.tagName || '').replace(/^v/, '')
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
  const parts: string[] = [runtimeTargetOS.value || 'linux', selectedLinuxArch.value]
  if (showLinuxAmd64LevelSelector.value) {
    parts.push(selectedAmd64Level.value || t('coreManager.notDetected'))
  }
  return parts.join('/')
})
const archSelectorLabel = computed(() => (
  runtimeTargetOS.value === 'windows'
    ? t('coreManager.targetArchitecture')
    : t('coreManager.linuxArchitecture')
))
const downloadingVersionLabel = computed(() => (
  downloadingVersion.value === 'custom'
    ? t('coreManager.customBuild')
    : showLinuxArchSelector.value
      ? `${downloadingVersion.value} ${selectedLinuxPackageLabel.value}`
      : downloadingVersion.value
))

const coreDownloadTaskActive = computed(() => (
  ['queued', 'running', 'stopping'].includes(downloadProgress.value.state)
  || (downloadProgress.value.state === '' && downloadProgress.value.status === 'running')
))
const coreDownloadTaskStopping = computed(() => (
  downloadProgress.value.state === 'stopping' || downloadProgress.value.stopRequested
))
const coreDownloadTaskApplying = computed(() => (
  coreDownloadTaskActive.value
  && !coreDownloadTaskStopping.value
  && downloadProgress.value.canCancel === false
))
const coreDownloadStopLabel = computed(() => (
  coreDownloadTaskStopping.value ? '正在停止' : coreDownloadTaskApplying.value ? '正在应用' : '停止'
))
const hasActiveDownloadProgress = computed(() => (
  downloading.value || coreDownloadTaskActive.value || (
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
    case 'extracting':
      return '正在解压'
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
    case 'cancelled':
      return '已停止'
    case 'timed_out':
      return '下载超时，已停止'
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
  return formatPanelDateTime(lastCheckedAt.value * 1000)
})
const autoUpdateLastAttemptDisplay = computed(() => (
  autoUpdateLastAttemptAt.value > 0 ? formatPanelDateTime(autoUpdateLastAttemptAt.value * 1000) : ''
))
const autoUpdateLastSuccessDisplay = computed(() => (
  autoUpdateLastSuccessAt.value > 0 ? formatPanelDateTime(autoUpdateLastSuccessAt.value * 1000) : ''
))
const autoUpdateSwitchDisabled = computed(() => (
  !autoCheckEnabled.value || autoCheckSaving.value || autoUpdateSaving.value || intervalSaving.value || autoUpdateDisabled.value
))
const autoUpdateDisabledReasonText = computed(() => (
  autoUpdateDisabled.value ? autoUpdateDisableReason.value.trim() : ''
))
const autoUpdateErrorText = computed(() => {
  const message = autoUpdateError.value.trim()
  if (!message) {
    return ''
  }
  if (autoUpdateErrorAt.value > 0) {
    return `${message} (${formatPanelDateTime(autoUpdateErrorAt.value * 1000)})`
  }
  return message
})

const customUrlPlaceholder = computed(() => `https://github.com/.../${mihomoCore.coreName}-xxx.gz`)

const binaryPath = computed(() => {
  const suffix = platform.value.startsWith('windows') ? '.exe' : ''
  return `Promanager_data/core/mihomo/${mihomoCore.binaryBaseName}${suffix}`
})
const installedTargetLabel = computed(() => {
  const target = installedTarget.value
  if (!target?.arch) {
    return ''
  }
  const parts = [target.os || 'linux', target.arch]
  if (target.arch === 'amd64') {
    parts.push(target.amd64Level || t('coreManager.notDetected'))
  }
  return parts.join('/')
})
const getVersionTargetQuery = () => {
  const query: Record<string, string> = {}
  if (showLinuxArchSelector.value) {
    if (!selectedLinuxArch.value) {
      return query
    }
    if (runtimeTargetOS.value) {
      query.target_os = runtimeTargetOS.value
    }
    query.target_arch = selectedLinuxArch.value
    if (showLinuxAmd64LevelSelector.value && selectedAmd64Level.value) {
      query.target_amd64_level = selectedAmd64Level.value
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
  `core-manager-linux-target:${mihomoCore.binaryBaseName}:mihomo`
))

const normalizeLinuxArch = (value: unknown): OptionalLinuxArchValue => {
  if (value === 'amd64' || value === 'arm64') {
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

const normalizeInstalledChannel = (value: unknown): 'stable' | 'alpha' | '' => {
  if (value === 'stable' || value === 'alpha') {
    return value
  }
  return ''
}

const inferLinuxArchFromPlatform = (value: unknown): OptionalLinuxArchValue => {
  const platformText = String(value ?? '').trim().toLowerCase()
  if (platformText.endsWith('/amd64')) {
    return 'amd64'
  }
  if (platformText.endsWith('/arm64')) {
    return 'arm64'
  }
  return null
}

const clearLinuxTargetSelection = () => {
  selectedLinuxArch.value = null
  selectedAmd64Level.value = null
}

const applyTargetSelection = (target: CoreDownloadTarget | undefined | null) => {
  if (!target) {
    clearLinuxTargetSelection()
    return
  }
  const arch = normalizeLinuxArch(target.arch)
  selectedLinuxArch.value = arch
  selectedAmd64Level.value = arch === 'amd64'
    ? normalizeAmd64LevelValue(target.amd64Level)
    : null
}

const applyDefaultLinuxTargetSelection = () => {
  if (!selectedLinuxArch.value) {
    selectedLinuxArch.value = inferLinuxArchFromPlatform(platform.value)
  }
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
    if (runtimeTargetOS.value) {
      target.os = runtimeTargetOS.value
    }
    target.arch = selectedLinuxArch.value
    if (selectedLinuxArch.value === 'amd64' && selectedAmd64Level.value) {
      target.amd64Level = selectedAmd64Level.value
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
  installedChannel.value = normalizeInstalledChannel(status?.installedChannel)
  if (installedChannel.value) {
    selectedChannel.value = installedChannel.value
  } else if (versionList.value.length === 0 && !remoteLoaded.value) {
    selectedChannel.value = 'stable'
  }
  const installedTarget = status?.installedTarget as CoreDownloadTarget | undefined
  if (installedTarget && (installedTarget.arch || installedTarget.os)) {
    applyTargetSelection(installedTarget)
    return
  }
  const preferredTarget = status?.downloadPreference?.target as CoreDownloadTarget | undefined
  if (preferredTarget && (preferredTarget.arch || preferredTarget.os || preferredTarget.amd64Level)) {
    applyTargetSelection(preferredTarget)
    applyDefaultLinuxTargetSelection()
    return
  }
  if (status?.installed === true && status?.compatible !== true && (selectedLinuxArch.value || selectedAmd64Level.value)) {
    return
  }
  clearLinuxTargetSelection()
  applyDefaultLinuxTargetSelection()
}

const saveDownloadPreference = async (includeTarget = false) => {
  if (preferenceSaving.value) {
    return
  }
  preferenceSaving.value = true
  try {
    const data = await HttpUtils.post(
      mihomoCore.downloadPreferenceEndpoint,
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
      resetRemoteVersions()
      selectedChannel.value = 'stable'
      clearLinuxTargetSelection()
      if (downloadProgressSessionId.value) {
        startDownloadProgressPolling(downloadProgressSessionId.value)
      }
      void recoverCoreDownloadTask()
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

watch([selectedLinuxArch, selectedAmd64Level], () => {
  if (!remoteLoaded.value) {
    return
  }
  const shouldReload = true
  resetRemoteVersions()
  if (shouldReload) {
    void loadRemoteVersions(false)
  }
})

const close = () => {
  stopDownloadProgressPolling()
  recoverDownloadProgressController?.abort()
  recoverDownloadProgressController = null
  emit('close')
  emit('update:modelValue', false)
}

const normalizeCoreDownloadProgress = (raw: any): CoreDownloadProgress => ({
  id: String(raw?.id ?? '').trim(),
  core: String(raw?.core ?? '').trim(),
  status: String(raw?.status ?? '').trim().toLowerCase() || 'missing',
  state: String(raw?.state ?? '').trim().toLowerCase() || 'idle',
  stage: String(raw?.stage ?? '').trim().toLowerCase(),
  canCancel: raw?.canCancel === true,
  stopRequested: raw?.stopRequested === true,
  deadlineExceeded: raw?.deadlineExceeded === true,
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
    state: 'idle',
    stage: '',
    canCancel: false,
    stopRequested: false,
    deadlineExceeded: false,
    runningBefore: false,
    percent: 0,
    approximate: false,
    downloadedBytes: 0,
    totalBytes: 0,
    error: '',
  }
}

const clearDownloadFeedback = () => {
  if (downloadFeedbackTimer != null) {
    window.clearTimeout(downloadFeedbackTimer)
    downloadFeedbackTimer = null
  }
  feedbackMsg.value = ''
}

const showTransientDownloadFeedback = (type: 'success' | 'error' | 'info', message: string, duration = 4500) => {
  if (downloadFeedbackTimer != null) {
    window.clearTimeout(downloadFeedbackTimer)
  }
  feedbackType.value = type
  feedbackMsg.value = message
  downloadFeedbackTimer = window.setTimeout(() => {
    if (feedbackMsg.value === message) {
      feedbackMsg.value = ''
    }
    downloadFeedbackTimer = null
  }, duration)
}

const clearCompletedCoreDownloadTask = (id: string) => {
  if (downloadProgress.value.id === id && isTerminalCoreDownload(downloadProgress.value)) {
    resetDownloadProgress()
  }
  if (downloadProgressSessionId.value === id) {
    downloadProgressSessionId.value = ''
  }
  downloading.value = false
}

const makeDownloadSessionId = () => {
  const randomPart = typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function'
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(16).slice(2)}`
  return `core-download-${mihomoCore.binaryBaseName}-${randomPart}`
}

const stopDownloadProgressPolling = () => {
  if (downloadProgressTimerId.value != null) {
    window.clearTimeout(downloadProgressTimerId.value)
    downloadProgressTimerId.value = null
  }
	downloadProgressController?.abort()
	downloadProgressController = null
}

const isTerminalCoreDownload = (progress: CoreDownloadProgress) => (
  ['success', 'error', 'cancelled', 'timed_out'].includes(progress.state)
  || ['success', 'error', 'missing'].includes(progress.status)
)

let completedDownloadTaskId = ''
const completeCoreDownloadTask = async (progress: CoreDownloadProgress, allowTerminal = true) => {
  if (progress.id === '') return
  if (completedDownloadTaskId === progress.id) {
    clearCompletedCoreDownloadTask(progress.id)
    return
  }
  if (!allowTerminal) {
    clearCompletedCoreDownloadTask(progress.id)
    return
  }
  completedDownloadTaskId = progress.id
  downloading.value = false
  try {
    if (progress.state === 'success' || (progress.state === 'idle' && progress.status === 'success')) {
      showTransientDownloadFeedback('success', t('coreManager.downloadSuccess', { coreName: mihomoCore.coreName, version: '' }))
      await Promise.all([loadCoreStatus(), loadCoreUpdateInfo(false)])
      return
    }
    if (progress.state === 'cancelled' || progress.state === 'timed_out') {
      showTransientDownloadFeedback('info', progress.state === 'timed_out' ? '下载超时，任务已停止并清理临时文件' : '下载已停止并清理临时文件')
      return
    }
    showTransientDownloadFeedback('error', progress.error || t('coreManager.downloadFailed'), 5500)
  } finally {
    clearCompletedCoreDownloadTask(progress.id)
  }
}

const pollDownloadProgress = async (): Promise<void> => {
  if (downloadProgressRequest) return downloadProgressRequest
  if (!dialogVisible.value) return
  const sessionId = downloadProgressSessionId.value.trim()
  if (!sessionId) {
    return
  }
  const request = (async () => {
		const controller = new AbortController()
		downloadProgressController = controller
		const data = await HttpUtils.get(mihomoCore.progressEndpoint, { id: sessionId }, { silentAuthCheck: true, signal: controller.signal })
		if (controller.signal.aborted || downloadProgressController !== controller) return
    if (sessionId !== downloadProgressSessionId.value.trim()) return
    if (!data.success) {
      return
    }
    const nextProgress = normalizeCoreDownloadProgress(data.obj)
    if (nextProgress.status === 'missing' && coreDownloadTaskActive.value) {
      return
    }
    downloadProgress.value = nextProgress
    if (isTerminalCoreDownload(nextProgress)) {
      stopDownloadProgressPolling()
      void completeCoreDownloadTask(nextProgress)
    }
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

const scheduleDownloadProgressPolling = (delay = 800) => {
  if (!dialogVisible.value || !coreDownloadTaskActive.value || !downloadProgressSessionId.value.trim()) return
  if (downloadProgressTimerId.value != null) window.clearTimeout(downloadProgressTimerId.value)
  downloadProgressTimerId.value = window.setTimeout(async () => {
    downloadProgressTimerId.value = null
    try {
      await pollDownloadProgress()
    } catch {
      // The next scheduled pass will retry transient transport failures.
    } finally {
      if (dialogVisible.value && coreDownloadTaskActive.value && !isTerminalCoreDownload(downloadProgress.value)) {
        scheduleDownloadProgressPolling()
      }
    }
  }, delay)
}

const startDownloadProgressPolling = (sessionId: string) => {
  stopDownloadProgressPolling()
  downloadProgressSessionId.value = sessionId.trim()
  if (!downloadProgressSessionId.value || !dialogVisible.value) {
    return
  }
  void pollDownloadProgress().finally(() => scheduleDownloadProgressPolling())
}

const recoverCoreDownloadTask = async (allowTerminal = false) => {
	if (!dialogVisible.value) return
	recoverDownloadProgressController?.abort()
	const controller = new AbortController()
	recoverDownloadProgressController = controller
	const data = await HttpUtils.get(mihomoCore.progressEndpoint, {}, { silentAuthCheck: true, signal: controller.signal })
	if (controller.signal.aborted || recoverDownloadProgressController !== controller) return
  if (!data.success || !data.obj) return
  const nextProgress = normalizeCoreDownloadProgress(data.obj)
  if (nextProgress.id === '' || nextProgress.state === 'idle') return
  downloadProgress.value = nextProgress
  downloadProgressSessionId.value = nextProgress.id
  if (coreDownloadTaskActive.value) {
    if (dialogVisible.value) {
      startDownloadProgressPolling(nextProgress.id)
    }
  } else if (isTerminalCoreDownload(nextProgress)) {
    void completeCoreDownloadTask(nextProgress, allowTerminal)
  }
}

const handleVisibilityChange = () => {
  if (document.visibilityState === 'visible') {
    if (dialogVisible.value && downloadProgressSessionId.value) {
      startDownloadProgressPolling(downloadProgressSessionId.value)
    }
		if (dialogVisible.value) void recoverCoreDownloadTask()
    return
  }
  stopDownloadProgressPolling()
  recoverDownloadProgressController?.abort()
  recoverDownloadProgressController = null
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
  const requestId = ++statusRequestSeq.value
  try {
    const data = await HttpUtils.get(mihomoCore.statusEndpoint)
    if (requestId !== statusRequestSeq.value || !dialogVisible.value) return
    if (data.success && data.obj) {
      installed.value = data.obj.installed === true
      compatible.value = data.obj.compatible === true
      localVersion.value = data.obj.localVersion || ''
      versionInfo.value = data.obj.versionInfo || ''
      coreRunning.value = data.obj.running === true
      platform.value = data.obj.platform || ''
      installedTarget.value = data.obj.installedTarget || null
      installedChannel.value = normalizeInstalledChannel(data.obj.installedChannel)
      if (coreLogLevelItems.some((item) => item.value === data.obj.logLevel)) {
        coreLogLevel.value = data.obj.logLevel
        confirmedCoreLogLevel.value = data.obj.logLevel
      }
      applyStatusDownloadState(data.obj)
    }
  } catch (error) {
    console.error('Failed to load core status:', error)
  }
}

const saveCoreLogLevel = async (level: string | null) => {
  const previousLevel = confirmedCoreLogLevel.value
  if (!coreLogLevelItems.some((item) => item.value === level) || coreLogLevelSaving.value) {
    coreLogLevel.value = previousLevel
    return
  }

  coreLogLevelSaving.value = true
  try {
    const data = await HttpUtils.post(mihomoCore.logLevelEndpoint, { level })
    if (data.success && data.obj) {
      coreLogLevel.value = data.obj.level
      confirmedCoreLogLevel.value = data.obj.level
      feedbackMsg.value = t('coreManager.logLevelSaved')
      feedbackType.value = 'success'
      return
    }
    feedbackMsg.value = data.msg || t('coreManager.logLevelSaveFailed')
    feedbackType.value = 'error'
    coreLogLevel.value = previousLevel
    await loadCoreStatus()
  } catch (error: any) {
    feedbackMsg.value = t('coreManager.logLevelSaveFailedWithReason', {
      reason: error.message || t('coreManager.unknown'),
    })
    feedbackType.value = 'error'
    coreLogLevel.value = previousLevel
    await loadCoreStatus()
  } finally {
    coreLogLevelSaving.value = false
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
    const data = await HttpUtils.get(mihomoCore.versionsEndpoint, {
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
  autoUpdateEnabled.value = info.autoUpdateEnabled === true
  autoUpdateDisabled.value = info.autoUpdateDisabled === true
  autoUpdateDisableReason.value = String(info.autoUpdateDisableReason || '')
  autoUpdateLastAttemptAt.value = Number(info.autoUpdateLastAttemptAt || 0)
  autoUpdateLastSuccessAt.value = Number(info.autoUpdateLastSuccessAt || 0)
  autoUpdateError.value = String(info.autoUpdateError || '')
  autoUpdateErrorAt.value = Number(info.autoUpdateErrorAt || 0)
}

const loadCoreUpdateInfo = async (forceCheck: boolean) => {
  const requestId = ++updateInfoRequestSeq.value
  try {
    const data = await HttpUtils.get(
      mihomoCore.updateInfoEndpoint,
      forceCheck ? { force: 'true' } : {},
    )
    if (requestId !== updateInfoRequestSeq.value || !dialogVisible.value) return
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

const saveAutoCheckEnabled = async (enabled: boolean | null) => {
  autoCheckSaving.value = true
  clearDownloadFeedback()
  try {
    const data = await HttpUtils.post(mihomoCore.updateSettingsEndpoint, {
      action: 'auto_check',
      enabled: enabled === true ? 'true' : 'false',
    })
    if (data.success && data.obj) {
      applyCoreUpdateInfo(data.obj)
      feedbackMsg.value = t('coreManager.autoCheckSaved')
      feedbackType.value = 'success'
    } else {
      await loadCoreUpdateInfo(false)
      feedbackMsg.value = data.msg || t('coreManager.autoCheckSaveFailed')
      feedbackType.value = 'error'
    }
  } catch (error: any) {
    await loadCoreUpdateInfo(false)
    feedbackMsg.value = t('coreManager.autoCheckSaveFailedWithReason', {
      reason: error.message || t('coreManager.unknown'),
    })
    feedbackType.value = 'error'
  } finally {
    autoCheckSaving.value = false
  }
}

const saveAutoUpdateEnabled = async (enabled: boolean | null) => {
  if (!autoCheckEnabled.value) {
    // Keep the switch truthful when auto-check is disabled; no update request
    // is valid until the prerequisite setting is enabled.
    autoUpdateEnabled.value = false
    return
  }
  autoUpdateSaving.value = true
  clearDownloadFeedback()
  try {
    const data = await HttpUtils.post(mihomoCore.updateSettingsEndpoint, {
      action: 'auto_update',
      auto_update_enabled: enabled === true ? 'true' : 'false',
    })
    if (data.success && data.obj) {
      applyCoreUpdateInfo(data.obj)
      feedbackMsg.value = t('coreManager.autoCheckSaved')
      feedbackType.value = 'success'
    } else {
      await loadCoreUpdateInfo(false)
      feedbackMsg.value = data.msg || t('coreManager.autoCheckSaveFailed')
      feedbackType.value = 'error'
    }
  } catch (error: any) {
    await loadCoreUpdateInfo(false)
    feedbackMsg.value = t('coreManager.autoCheckSaveFailedWithReason', {
      reason: error.message || t('coreManager.unknown'),
    })
    feedbackType.value = 'error'
  } finally {
    autoUpdateSaving.value = false
  }
}

const saveAutoCheckInterval = async () => {
  const intervalHours = normalizeIntervalHours(autoCheckIntervalInput.value)
  if (intervalHours == null) {
    feedbackMsg.value = t('coreManager.intervalInvalid')
    feedbackType.value = 'error'
    return
  }

  intervalSaving.value = true
  clearDownloadFeedback()
  try {
    const data = await HttpUtils.post(mihomoCore.updateSettingsEndpoint, {
      action: 'interval',
      interval: String(intervalHours),
    })
    if (data.success && data.obj) {
      applyCoreUpdateInfo(data.obj)
      feedbackMsg.value = t('coreManager.autoCheckSaved')
      feedbackType.value = 'success'
    } else {
      await loadCoreUpdateInfo(false)
      feedbackMsg.value = data.msg || t('coreManager.autoCheckSaveFailed')
      feedbackType.value = 'error'
    }
  } catch (error: any) {
    await loadCoreUpdateInfo(false)
    feedbackMsg.value = t('coreManager.autoCheckSaveFailedWithReason', {
      reason: error.message || t('coreManager.unknown'),
    })
    feedbackType.value = 'error'
  } finally {
    intervalSaving.value = false
  }
}

const ackCoreUpdateNotice = async () => {
  pendingStableVersion.value = ''
  pendingAlphaVersion.value = ''
  try {
    const data = await HttpUtils.post(mihomoCore.updateAckEndpoint, {})
    if (data.success && data.obj) {
      applyCoreUpdateInfo(data.obj)
    }
  } catch (error) {
    console.error('Failed to acknowledge core update notice:', error)
  }
}

const ackCoreAutoUpdateError = async () => {
  autoUpdateError.value = ''
  autoUpdateErrorAt.value = 0
  try {
    const data = await HttpUtils.post('api/mihomo-core-auto-update-error-ack', {})
    if (data.success && data.obj) {
      applyCoreUpdateInfo(data.obj)
    }
  } catch (error) {
    console.error('Failed to acknowledge core auto update error:', error)
  }
}

const submitCoreDownload = async (formData: FormData, versionLabel: string) => {
  downloading.value = true
  downloadingVersion.value = versionLabel
  completedDownloadTaskId = ''
  resetDownloadProgress()
  feedbackMsg.value = ''
  try {
    const data = await HttpUtils.post(mihomoCore.downloadEndpoint, formData, { silentAuthCheck: true })
    if (data.success && data.obj) {
      const nextProgress = normalizeCoreDownloadProgress(data.obj)
      downloadProgress.value = nextProgress
      if (nextProgress.id !== '') startDownloadProgressPolling(nextProgress.id)
      return
    }
    await recoverCoreDownloadTask()
    if (coreDownloadTaskActive.value) return
    showTransientDownloadFeedback('error', data.msg || t('coreManager.downloadFailed'), 5500)
  } catch (error: any) {
    await recoverCoreDownloadTask()
    if (coreDownloadTaskActive.value) return
    showTransientDownloadFeedback('error', t('coreManager.downloadFailedWithReason', {
      reason: error?.message || t('coreManager.unknown'),
    }), 5500)
  } finally {
    downloading.value = false
  }
}

const downloadCore = async () => {
  if (!selectedVersion.value || downloading.value || coreDownloadTaskActive.value) return
  if (!hasCompleteLinuxTargetSelection.value) {
    feedbackMsg.value = t('coreManager.downloadTargetRequired')
    feedbackType.value = 'error'
    return
  }
  const formData = new FormData()
  formData.append('version', selectedVersion.value)
  if (showLinuxArchSelector.value && selectedLinuxArch.value) {
    if (runtimeTargetOS.value) {
      formData.append('target_os', runtimeTargetOS.value)
    }
    formData.append('target_arch', selectedLinuxArch.value)
    if (showLinuxAmd64LevelSelector.value && selectedAmd64Level.value) {
      formData.append('target_amd64_level', selectedAmd64Level.value)
    }
  }
  await submitCoreDownload(formData, selectedVersion.value.replace(/^v/, ''))
}

const downloadCoreFromCustomURL = async () => {
  const url = customDownloadURL.value.trim()
  if (!/^https?:\/\/.+/i.test(url) || downloading.value || coreDownloadTaskActive.value) return
  const formData = new FormData()
  formData.append('custom_url', url)
  await submitCoreDownload(formData, 'custom')
}

const stopCoreDownload = async () => {
  const id = downloadProgress.value.id.trim()
  if (!id || !downloadProgress.value.canCancel || coreDownloadTaskStopping.value) return
  downloadProgress.value = { ...downloadProgress.value, state: 'stopping', stopRequested: true, canCancel: false }
  try {
    const data = await HttpUtils.post('api/mihomo-core-download-stop', { id }, { silentAuthCheck: true })
    if (data.success && data.obj) {
      downloadProgress.value = normalizeCoreDownloadProgress(data.obj)
      startDownloadProgressPolling(id)
      return
    }
  } finally {
    await recoverCoreDownloadTask()
  }
}

const openReleasePage = () => {
  window.open(mihomoCore.repoUrl, '_blank')
}

const startCore = async () => {
  if (coreDownloadTaskActive.value || startingCore.value || !coreReady.value) {
    return
  }
  startingCore.value = true
  feedbackMsg.value = ''
  try {
    const data = await HttpUtils.post(mihomoCore.startEndpoint, {})
    if (data.success) {
      feedbackMsg.value = t('coreManager.startSuccess', { coreName: mihomoCore.coreName })
      feedbackType.value = 'success'
    } else {
      feedbackMsg.value = data.msg || t('coreManager.startFailed')
      feedbackType.value = 'error'
    }
    scheduleCoreAction(() => {
      void loadCoreStatus()
    }, 1500)
  } catch (error: any) {
    feedbackMsg.value = t('coreManager.startFailedWithReason', {
      reason: error.message || t('coreManager.unknown'),
    })
    feedbackType.value = 'error'
  } finally {
    scheduleCoreAction(() => {
      startingCore.value = false
    }, 1500)
  }
}

const stopCore = async () => {
  if (coreDownloadTaskActive.value || stoppingCore.value) return
  stoppingCore.value = true
  feedbackMsg.value = ''
  try {
    const data = await HttpUtils.post(mihomoCore.stopEndpoint, {})
    if (data.success) {
      feedbackMsg.value = t('coreManager.stopSuccess', { coreName: mihomoCore.coreName })
      feedbackType.value = 'info'
    } else {
      feedbackMsg.value = data.msg || t('coreManager.stopFailed')
      feedbackType.value = 'error'
    }
    scheduleCoreAction(() => {
      void loadCoreStatus()
    }, 1500)
  } catch (error: any) {
    feedbackMsg.value = t('coreManager.stopFailedWithReason', {
      reason: error.message || t('coreManager.unknown'),
    })
    feedbackType.value = 'error'
  } finally {
    scheduleCoreAction(() => {
      stoppingCore.value = false
    }, 1500)
  }
}

const restartCore = async () => {
  if (coreDownloadTaskActive.value || restartingCore.value || !coreReady.value) {
    return
  }
  restartingCore.value = true
  feedbackMsg.value = ''
  try {
    const data = await HttpUtils.post(mihomoCore.restartEndpoint, {})
    if (data.success) {
      feedbackMsg.value = t('coreManager.restartSuccess', { coreName: mihomoCore.coreName })
      feedbackType.value = 'success'
    } else {
      feedbackMsg.value = data.msg || t('coreManager.restartFailed')
      feedbackType.value = 'error'
    }
    scheduleCoreAction(() => {
      void loadCoreStatus()
    }, 2500)
  } catch (error: any) {
    feedbackMsg.value = t('coreManager.restartFailedWithReason', {
      reason: error.message || t('coreManager.unknown'),
    })
    feedbackType.value = 'error'
  } finally {
    scheduleCoreAction(() => {
      restartingCore.value = false
    }, 2500)
  }
}

const deleteCore = async () => {
  if (coreDownloadTaskActive.value || deletingCore.value || !installed.value) {
    return
  }

  const confirmDelete = await confirm({
    message: t('coreManager.deleteCoreConfirm', { coreName: mihomoCore.coreName }),
    severity: 'danger',
    confirmText: t('confirmDialog.actions.delete'),
  })
  if (!confirmDelete || coreDownloadTaskActive.value || deletingCore.value || !installed.value) {
    return
  }

  deletingCore.value = true
  feedbackMsg.value = ''
  try {
    const data = await HttpUtils.post(mihomoCore.deleteEndpoint, {})
    if (data.success) {
      feedbackMsg.value = t('coreManager.deleteSuccess', { coreName: mihomoCore.coreName })
      feedbackType.value = 'success'
      localVersion.value = ''
      versionInfo.value = ''
      coreRunning.value = false
      installed.value = false
      compatible.value = false
      installedTarget.value = null
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

onMounted(() => {
  if (typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', handleVisibilityChange)
  }
})

onBeforeUnmount(() => {
  stopDownloadProgressPolling()

  clearCoreActionTimers()
  statusRequestSeq.value += 1
  updateInfoRequestSeq.value += 1
	recoverDownloadProgressController?.abort()
	recoverDownloadProgressController = null
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
.core-modal-body {
  min-height: 450px;
  padding: 20px;
}

.core-path-chip {
  max-width: 100%;
  height: auto;
}

.core-path-chip :deep(.v-chip__content) {
  padding-block: 6px;
  white-space: normal;
  overflow-wrap: anywhere;
}

.core-status-chip {
  max-width: 100%;
  height: auto;
}

.core-status-chip :deep(.v-chip__content) {
  padding-block: 6px;
  white-space: normal;
  overflow-wrap: anywhere;
}

.core-incompatible-alert {
  max-width: 100%;
  overflow-wrap: anywhere;
}

.core-incompatible-path {
  margin-top: 6px;
  font-family: monospace;
  word-break: break-all;
}

@media (max-width: 600px) {
  .core-modal-body {
    min-height: 0;
    padding: 12px;
  }

  .core-modal-summary .v-spacer {
    flex-basis: 100%;
    height: 0;
  }

  .core-mobile-full,
  .core-action-col {
    flex: 0 0 100%;
    width: 100%;
    max-width: 100%;
  }

  .core-action-col > .v-btn {
    width: 100%;
  }

  .core-control-actions {
    justify-content: stretch;
  }

  .core-control-actions > .v-btn {
    flex: 1 1 80px;
    width: auto;
  }
}
</style>
