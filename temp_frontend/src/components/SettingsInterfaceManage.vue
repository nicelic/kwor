<template>
  <div class="settings-interface-manage">
    <!-- Card 1: 网络与访问配置 -->
    <v-card rounded="xl" variant="outlined" class="mb-4 card-cyan">
      <v-card-title class="d-flex align-center justify-space-between py-3 px-4">
        <div class="text-subtitle-1 font-weight-medium d-flex align-center ga-2">
          <v-icon size="small" color="primary">mdi-web</v-icon>
          <span>{{ $t('setting.networkAndAccessConfig') || '网络与访问配置' }}</span>
        </div>
        <v-chip size="x-small" color="info" variant="outlined">
          {{ $t('setting.restartRequiredBadge') || '保存后需重启生效' }}
        </v-chip>
      </v-card-title>
      <v-divider />
      <v-card-text class="pt-4">
        <v-row>
          <v-col cols="12" sm="6" md="4">
            <v-text-field
              v-model="settings.webListen"
              :label="$t('setting.addr')"
              hide-details
              density="comfortable"
            />
          </v-col>
          <v-col cols="12" sm="6" md="4">
            <v-text-field
              v-model="settings.webPort"
              min="1"
              type="number"
              :label="$t('setting.port')"
              hide-details
              density="comfortable"
            />
          </v-col>
          <v-col cols="12" sm="6" md="4">
            <v-text-field
              v-model="settings.webPath"
              :label="$t('setting.webPath')"
              hide-details
              density="comfortable"
            />
          </v-col>
          <v-col cols="12" sm="6" md="6">
            <v-text-field
              v-model="settings.webDomain"
              :label="$t('setting.domain')"
              hide-details
              density="comfortable"
            />
          </v-col>
          <v-col cols="12" sm="12" md="6">
            <v-text-field
              v-model="settings.webURI"
              :label="$t('setting.webUri')"
              hide-details
              density="comfortable"
            />
          </v-col>
        </v-row>

        <v-alert v-if="!panelCanRestart" type="warning" variant="tonal" density="compact" class="mt-4">
          {{ panelRestartHint }}
        </v-alert>
        <v-alert type="info" variant="tonal" density="compact" class="mt-3">
          {{ $t('setting.panelRestartRequiredHint') }}
        </v-alert>
      </v-card-text>
    </v-card>

    <!-- Card 2: 安全会话与运行环境 -->
    <v-card rounded="xl" variant="outlined" class="mb-4 card-green">
      <v-card-title class="py-3 px-4">
        <div class="text-subtitle-1 font-weight-medium d-flex align-center ga-2">
          <v-icon size="small" color="primary">mdi-shield-clock-outline</v-icon>
          <span>{{ $t('setting.sessionAndEnvironment') || '会话安全与环境时区' }}</span>
        </div>
      </v-card-title>
      <v-divider />
      <v-card-text class="pt-4">
        <v-row>
          <!-- 会话超时限（移动端弹性适配，单位下拉固定宽度防止截断） -->
          <v-col cols="12" sm="6" md="4">
            <div class="d-flex ga-2 align-center">
              <v-text-field
                type="number"
                v-model="sessionAgeInput.value"
                min="0"
                :label="$t('setting.sessionAge')"
                hide-details
                density="comfortable"
                class="flex-grow-1"
                @update:model-value="onSessionAgeInputChanged"
              />
              <v-select
                v-model="sessionAgeInput.unit"
                :items="sessionAgeUnitItems"
                hide-details
                density="comfortable"
                class="session-age-unit-select"
                style="width: 100px; flex-shrink: 0;"
                @update:model-value="onSessionAgeInputChanged"
              />
            </div>
            <div class="text-caption text-medium-emphasis mt-1">{{ $t('setting.sessionAgeHint') }}</div>
          </v-col>

          <!-- 面板时区 -->
          <v-col cols="12" sm="6" md="4">
            <v-select
              v-model="settings.timeLocation"
              :items="timeZoneOptions"
              item-title="title"
              item-value="value"
              item-props="props"
              :label="$t('setting.panelTimeLoc')"
              hide-details
              density="comfortable"
              :menu-props="{ maxHeight: 360 }"
            />
            <div v-if="hiddenPanelTimeLocation" class="text-caption text-warning mt-1">
              {{ $t('setting.panelTimeUnknownHint', { value: hiddenPanelTimeLocation }) }}
            </div>
            <div class="text-caption text-medium-emphasis mt-1">{{ $t('setting.panelTimeScope') }}</div>
          </v-col>

          <!-- 系统时区 -->
          <v-col cols="12" sm="12" md="4">
            <v-select
              :model-value="systemTimeLocation"
              :items="timeZoneOptions"
              item-title="title"
              item-value="value"
              item-props="props"
              :label="$t('setting.systemTimeLoc')"
              hide-details
              density="comfortable"
              :menu-props="{ maxHeight: 360 }"
              :disabled="systemTimeZoneLoadState === 'loading' || systemTimeZoneLoadState === 'error'"
              @update:model-value="onSystemTimeLocationSelected"
            />
            <div v-if="systemTimeZoneLoadState === 'error'" class="text-caption text-error mt-1">
              <div>{{ $t('setting.systemTimeReadFailed') }}：{{ systemTimeZoneLoadError || $t('setting.requestFailed') }}</div>
              <v-btn class="mt-1" size="small" variant="text" color="primary" @click="emit('retrySystemTimezone')">
                {{ $t('setting.systemTimeReload') }}
              </v-btn>
            </div>
            <div v-else-if="systemTimeZoneStatus.reason" class="text-caption text-medium-emphasis mt-1">
              {{ systemTimeZoneStatus.reason }}
            </div>
            <div v-else class="text-caption text-medium-emphasis mt-1">{{ $t('setting.systemTimeScope') }}</div>
          </v-col>
        </v-row>
      </v-card-text>
    </v-card>

    <!-- Card 3: 面板软件版本与在线更新 -->
    <v-card rounded="xl" variant="outlined" class="mb-4 card-cyan">
      <v-card-title class="py-3 px-4">
        <div class="text-subtitle-1 font-weight-medium d-flex align-center ga-2">
          <v-icon size="small" color="primary">mdi-update</v-icon>
          <span>{{ $t('setting.panelUpdateTitle') || '面板版本与更新管理' }}</span>
        </div>
      </v-card-title>
      <v-divider />
      <v-card-text class="pt-4">
        <!-- 版本状态指示条 -->
        <div class="d-flex align-center flex-wrap ga-2 mb-4">
          <v-chip variant="outlined" color="success" size="small" label>
            <v-progress-circular
              v-if="panelStatusLoading"
              indeterminate
              size="12"
              width="2"
              class="mr-1"
            />
            {{ $t('setting.panelLocal') }}: {{ panelLocalVersionLabel }}
          </v-chip>
          <v-chip variant="outlined" color="info" size="small" label>
            <v-progress-circular
              v-if="panelRemoteLoading"
              indeterminate
              size="12"
              width="2"
              class="mr-1"
            />
            {{ $t('setting.panelRemote') }}: {{ panelRemoteVersionLabel }}
          </v-chip>
          <v-chip v-if="panelBinaryName" variant="tonal" size="small" label>
            {{ $t('setting.panelFile') }}: {{ panelBinaryName }}
            <v-tooltip
              v-if="panelUpdateStatus?.binaryPath"
              activator="parent"
              location="top"
              :text="panelUpdateStatus.binaryPath"
            />
          </v-chip>
        </div>

        <!-- 版本选择与操作按钮栏 -->
        <v-row align="center">
          <v-col cols="12" sm="6" md="5">
            <v-select
              v-model="panelSelectedVersion"
              v-model:menu="panelVersionMenuVisible"
              :items="panelVersionItems"
              item-title="title"
              item-value="value"
              :label="$t('setting.panelVersion')"
              variant="outlined"
              density="comfortable"
              hide-details
              :loading="panelRemoteLoading"
              :disabled="panelRemoteLoading || panelLoadingMoreVersions || isBusy"
              :menu-props="{ maxHeight: 260 }"
              :no-data-text="panelVersionNoDataText"
              @update:menu="onPanelVersionMenuUpdate"
            >
              <template #item="{ props: itemProps, item }">
                <v-list-item
                  v-bind="itemProps"
                  :subtitle="item.raw.assetName || undefined"
                >
                  <template #append>
                    <v-chip
                      v-if="item.raw.prerelease"
                      size="x-small"
                      color="warning"
                      variant="flat"
                    >
                      {{ $t('setting.panelPrerelease') }}
                    </v-chip>
                  </template>
                </v-list-item>
              </template>
              <template #append-item>
                <v-divider v-if="panelVersionItems.length > 0" class="mt-1" />
                <div
                  v-if="panelVersionItems.length > 0"
                  class="panel-version-footer px-3 py-3 d-flex align-center justify-space-between flex-wrap"
                  style="gap: 10px;"
                >
                  <span class="text-caption panel-version-footer__summary">
                    {{ $t('setting.panelVersionsLoaded', { count: panelVersionItems.length }) }}
                  </span>
                  <div class="d-flex align-center flex-wrap" style="gap: 8px;">
                    <v-btn
                      size="small"
                      color="primary"
                      variant="tonal"
                      class="panel-version-footer__action"
                      :loading="panelLoadingMoreVersions"
                      :disabled="isBusy || panelRemoteLoading || panelAllVersionsLoaded"
                      @mousedown.prevent
                      @click.stop="loadMorePanelVersions"
                    >
                      {{ panelAllVersionsLoaded ? $t('setting.panelNoMoreVersions') : $t('setting.panelLoadMoreVersions') }}
                    </v-btn>
                  </div>
                </div>
              </template>
            </v-select>
          </v-col>

          <v-col cols="12" sm="auto" class="d-flex align-center flex-wrap ga-2">
            <v-btn
              color="secondary"
              variant="tonal"
              prepend-icon="mdi-refresh"
              :loading="panelRemoteLoading"
              :disabled="panelRemoteLoading || panelLoadingMoreVersions || isBusy"
              @click="checkPanelUpdates"
            >
              {{ $t('setting.panelCheckUpdates') }}
            </v-btn>

            <v-btn
              color="primary"
              variant="flat"
              :prepend-icon="panelUpdateTaskActive ? (panelUpdateTaskApplying ? 'mdi-progress-wrench' : 'mdi-stop') : 'mdi-download'"
              :disabled="panelUpdateTaskActive
                ? panelUpdateStopRequestPending || !panelUpdateTaskCanCancel
                : !panelSelectedVersion || isBusy || panelRemoteLoading || !panelCanInstall"
              @click="panelUpdateTaskActive ? stopPanelUpdateTask() : openPanelInstallDialog()"
            >
              {{ panelUpdateTaskButtonText }}
            </v-btn>
          </v-col>
        </v-row>

        <!-- 任务状态提示 -->
        <v-alert
          v-if="panelManagedUpdateTask"
          :type="panelUpdateTaskAlertType"
          variant="tonal"
          density="compact"
          class="mt-3 panel-update-task-status"
        >
          <div class="d-flex align-center justify-space-between flex-wrap ga-2">
            <span class="panel-update-task-status__text">{{ panelUpdateTaskStatusText }}</span>
            <span v-if="panelManagedUpdateTask.id" class="text-caption text-medium-emphasis panel-update-task-status__id">
              {{ panelManagedUpdateTask.id }}
            </span>
          </div>
        </v-alert>

        <!-- 更新反馈与日志入口 -->
        <v-alert
          v-if="panelUpdateFeedback"
          :type="panelUpdateFeedbackType"
          variant="tonal"
          density="compact"
          closable
          class="mt-3"
          @click:close="panelUpdateFeedback = ''"
        >
          <div class="d-flex align-center justify-space-between flex-wrap ga-2">
            <span>{{ panelUpdateFeedback }}</span>
            <v-btn
              v-if="panelUpdateStatus?.lastUpdateLogPath"
              size="small"
              variant="text"
              color="primary"
              :loading="panelUpdateLogLoading"
              @click="openPanelUpdateLogDialog"
            >
              {{ $t('setting.panelViewLog') }}
            </v-btn>
          </div>
        </v-alert>

        <v-alert v-if="panelInstallHint" type="warning" variant="tonal" density="compact" class="mt-3">
          {{ panelInstallHint }}
        </v-alert>
      </v-card-text>
    </v-card>

    <!-- Card 4: 危险操作区 (Danger Zone) -->
    <v-card rounded="xl" variant="outlined" color="error" class="mb-4 danger-zone-card">
      <v-card-title class="py-3 px-4">
        <div class="text-subtitle-1 font-weight-medium d-flex align-center ga-2 text-error">
          <v-icon size="small" color="error">mdi-alert-octagon-outline</v-icon>
          <span>{{ $t('setting.dangerZone') || '危险操作区' }}</span>
        </div>
      </v-card-title>
      <v-divider style="opacity: 0.25;" />
      <v-card-text class="pt-4">
        <v-row align="center">
          <v-col cols="12" md="8">
            <v-text-field
              type="number"
              v-model="settings.trafficAge"
              min="0"
              :label="$t('setting.trafficAge')"
              :suffix="$t('date.d')"
              hide-details
              density="comfortable"
            />
            <div class="text-caption text-medium-emphasis mt-1">{{ $t('setting.trafficAgeHint') }}</div>
          </v-col>

          <v-col cols="12" md="4" class="d-flex justify-md-end">
            <v-tooltip
              :disabled="panelCanUseUninstallAction || !panelUninstallHint"
              location="top"
              :text="panelUninstallHint"
            >
              <template #activator="{ props: tooltipProps }">
                <span v-bind="tooltipProps" class="panel-uninstall-trigger">
                  <v-btn
                    class="panel-uninstall-button"
                    color="error"
                    variant="outlined"
                    prepend-icon="mdi-delete-forever"
                    :loading="panelUninstalling"
                    :disabled="panelStatusLoading || disabled || isBusy || !panelCanUseUninstallAction"
                    @click="requestPanelUninstall"
                  >
                    {{ panelUninstallButtonText }}
                  </v-btn>
                </span>
              </template>
            </v-tooltip>
          </v-col>
        </v-row>

        <!-- 卸载失败提示 -->
        <v-alert v-if="panelUninstallFailed" type="error" variant="tonal" density="compact" class="mt-3">
          <div class="d-flex align-start justify-space-between flex-wrap ga-2">
            <div class="panel-uninstall-failure">
              <div class="font-weight-medium">
                {{ $t('setting.uninstallPanelFailed') }}
                <span v-if="panelUninstallPhase">: {{ panelUninstallPhase }}</span>
              </div>
              <div v-if="panelUninstallError" class="text-body-2 mt-1">{{ panelUninstallError }}</div>
              <ul v-if="panelUninstallFailures.length > 0" class="panel-uninstall-message-list mt-2">
                <li v-for="failure in panelUninstallFailures" :key="failure">{{ failure }}</li>
              </ul>
              <ul v-if="panelUninstallWarnings.length > 0" class="panel-uninstall-message-list text-medium-emphasis mt-2">
                <li v-for="warning in panelUninstallWarnings" :key="warning">{{ warning }}</li>
              </ul>
            </div>
            <v-btn
              v-if="panelUninstallCanRetry"
              color="error"
              variant="outlined"
              prepend-icon="mdi-reload"
              :disabled="panelStatusLoading || disabled || isBusy"
              @click="requestPanelUninstall"
            >
              {{ $t('setting.uninstallPanelRetry') }}
            </v-btn>
          </div>
        </v-alert>
      </v-card-text>
    </v-card>

    <!-- 安装确认 Dialog -->
    <v-dialog v-model="panelInstallDialogVisible" max-width="460">
      <v-card>
        <v-card-title>{{ $t('setting.panelInstallConfirmTitle') }}</v-card-title>
        <v-card-text>
          {{ $t('setting.panelInstallConfirmMessage', { version: panelSelectedVersion || '-' }) }}
        </v-card-text>
        <v-card-actions>
          <v-spacer></v-spacer>
          <v-btn variant="text" :disabled="panelInstalling" @click="panelInstallDialogVisible = false">
            {{ $t('setting.panelCancel') }}
          </v-btn>
          <v-btn color="primary" variant="flat" :disabled="panelInstalling" @click="installPanelVersion">
            {{ panelInstalling ? '正在提交' : $t('setting.panelConfirmInstall') }}
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- Docker 卸载指引 Dialog -->
    <v-dialog v-model="panelDockerUninstallDialogVisible" max-width="860">
      <v-card>
        <v-card-title class="text-subtitle-1 font-weight-medium">{{ $t('setting.uninstallDockerGuideTitle') }}</v-card-title>
        <v-divider />
        <v-card-text>
          <div class="text-body-2 text-medium-emphasis mb-4">{{ $t('setting.uninstallDockerGuideDesc') }}</div>
          <section
            v-for="instruction in panelDockerUninstallCommands"
            :key="instruction.id || instruction.command"
            class="docker-uninstall-command mb-4"
          >
            <div class="d-flex align-center justify-space-between" style="gap: 8px;">
              <div class="text-subtitle-2">{{ panelDockerUninstallCommandLabel(instruction.id) }}</div>
              <v-btn
                icon="mdi-content-copy"
                size="small"
                variant="text"
                :aria-label="$t('copyToClipboard')"
                @click="copyDockerUninstallCommand(instruction.command)"
              >
                <v-tooltip activator="parent" location="top" :text="$t('copyToClipboard')" />
              </v-btn>
            </div>
            <pre class="docker-uninstall-command__content"><code>{{ instruction.command }}</code></pre>
          </section>
        </v-card-text>
        <v-divider />
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="panelDockerUninstallDialogVisible = false">{{ $t('actions.close') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- 卸载中 Overlay 遮罩 -->
    <v-overlay :model-value="panelUninstallOverlay" class="align-center justify-center" persistent>
      <v-card class="panel-uninstall-overlay-card">
        <v-card-text class="text-center py-8">
          <v-progress-circular indeterminate size="52" width="5" color="error" class="mb-4" />
          <div class="text-subtitle-1 font-weight-medium">{{ $t('setting.uninstallPanelPendingTitle') }}</div>
          <div class="text-caption text-medium-emphasis mt-2">{{ $t('setting.uninstallPanelPendingDesc') }}</div>
        </v-card-text>
      </v-card>
    </v-overlay>

    <!-- 查看更新日志 Dialog -->
    <v-dialog v-model="panelUpdateLogDialogVisible" max-width="960">
      <v-card rounded="xl" :loading="panelUpdateLogLoading">
        <v-card-title class="text-subtitle-1 font-weight-medium">{{ $t('setting.panelUpdateLogTitle') }}</v-card-title>
        <v-divider />
        <v-card-text>
          <div class="text-body-2 text-medium-emphasis mb-2">
            {{ $t('setting.panelLogPath') }}：{{ panelUpdateStatus?.lastUpdateLogPath || '-' }}
          </div>
          <div class="text-body-2 text-medium-emphasis mb-4" v-if="panelUpdateLogModifiedText">
            {{ $t('setting.panelLogUpdatedAt') }}：{{ panelUpdateLogModifiedText }}
          </div>
          <pre class="panel-update-log-box">{{ panelUpdateLogContent }}</pre>
        </v-card-text>
        <v-divider />
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="panelUpdateLogDialogVisible = false">{{ $t('actions.close') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </div>
</template>

<script lang="ts" setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { i18n } from '@/locales'
import HttpUtils from '@/plugins/httputil'
import { confirm } from '@/plugins/confirm'
import { push } from 'notivue'
import { formatPanelDateTime } from '@/plugins/panelTime'

export type SessionAgeUnit = 'm' | 'h' | 'd'

export type TimeZoneOption = {
  title: string
  value: string
  props?: {
    disabled?: boolean
  }
}

export type SystemTimeZoneStatus = {
  timeLocation?: string
  displayable?: boolean
  canModify?: boolean
  reason?: string
}

export type PanelVersionItem = {
  title: string
  value: string
  tagName: string
  name?: string
  prerelease?: boolean
  publishedAt?: string
  assetName?: string
  assetSize?: number
}

export type PanelManagedUpdateTask = {
  id: string
  state: string
  phase: string
  canCancel: boolean
  stopRequested: boolean
  deadlineExceeded: boolean
  startedAt: number
  updatedAt: number
  deadlineAt: number
  finishedAt: number
  error: string
}

export type PanelUpdateStatus = {
  localVersion?: string
  binaryPath?: string
  binaryName?: string
  installDir?: string
  serviceFilePath?: string
  serviceBinaryPath?: string
  runningBinaryPath?: string
  installSource?: string
  platform?: string
  canRestart?: boolean
  restartHint?: string
  canInstall?: boolean
  installHint?: string
  canUninstall?: boolean
  uninstallHint?: string
  uninstallMode?: 'native' | 'docker-guide' | 'unsupported'
  uninstallState?: string
  uninstallPhase?: string
  uninstallError?: string
  uninstallFailures?: string[]
  uninstallWarnings?: string[]
  uninstallCanRetry?: boolean
  dockerUninstallCommands?: Array<{
    id?: string
    command?: string
  }>
  lastUpdateLogPath?: string
  lastUpdateError?: string
  updateTask?: PanelManagedUpdateTask
}

export type PanelUpdateLogView = {
  path?: string
  exists?: boolean
  lines?: string[]
  tooLong?: boolean
  modified?: number
}

const props = withDefaults(defineProps<{
  settings: Record<string, any>
  systemTimeLocation?: string
  systemTimeZoneStatus?: SystemTimeZoneStatus
  systemTimeZoneLoadState?: 'idle' | 'loading' | 'ready' | 'error'
  systemTimeZoneLoadError?: string
  timeZoneOptions?: TimeZoneOption[]
  hiddenPanelTimeLocation?: string
  sessionAgeUnitItems?: Array<{ title: string; value: string }>
  panelCanRestart?: boolean
  panelRestartHint?: string
  disabled?: boolean
}>(), {
  systemTimeLocation: '',
  systemTimeZoneStatus: () => ({}),
  systemTimeZoneLoadState: 'idle',
  systemTimeZoneLoadError: '',
  timeZoneOptions: () => [],
  hiddenPanelTimeLocation: '',
  sessionAgeUnitItems: () => [],
  panelCanRestart: false,
  panelRestartHint: '',
  disabled: false,
})

const emit = defineEmits<{
  (e: 'update:systemTimeLocation', value: string): void
  (e: 'retrySystemTimezone'): void
  (e: 'systemTimeLocationSelected', value: string): void
  (e: 'busyChange', busy: boolean): void
  (e: 'canRestartChange', canRestart: boolean, hint: string): void
  (e: 'startReconnect', options?: { targetLoginURL?: string; trackUpdateStatus?: boolean }): void
}>()

let componentMounted = false

// --- 会话超时限单位与换算 ---
const SESSION_MAX_AGE_MAX_MINUTES = 72 * 60
const SESSION_MAX_AGE_DEFAULT_UNIT: SessionAgeUnit = 'd'

const normalizeSessionAgeUnit = (value: unknown): SessionAgeUnit | '' => {
  const normalized = String(value ?? '').trim().toLowerCase()
  if (normalized === 'm' || normalized === 'h' || normalized === 'd') {
    return normalized
  }
  return ''
}

const sessionAgeUnitFactor = (unit: SessionAgeUnit) => {
  switch (unit) {
    case 'd':
      return 24 * 60
    case 'h':
      return 60
    default:
      return 1
  }
}

const deriveSessionAgeDisplay = (minutesValue: unknown, preferredUnit: unknown): { value: string; unit: SessionAgeUnit } => {
  const rawValue = String(minutesValue ?? '').trim()
  if (!/^\d+$/.test(rawValue)) {
    return { value: '3', unit: SESSION_MAX_AGE_DEFAULT_UNIT }
  }
  const parsedValue = Number.parseInt(rawValue, 10)
  if (!Number.isSafeInteger(parsedValue) || parsedValue <= 0) {
    return { value: '3', unit: SESSION_MAX_AGE_DEFAULT_UNIT }
  }
  const effectiveMinutes = Math.min(parsedValue, SESSION_MAX_AGE_MAX_MINUTES)
  const normalizedPreferred = normalizeSessionAgeUnit(preferredUnit)
  if (normalizedPreferred) {
    const factor = sessionAgeUnitFactor(normalizedPreferred)
    const displayValue = effectiveMinutes / factor
    if (Number.isInteger(displayValue) && displayValue >= 1) {
      return { value: String(displayValue), unit: normalizedPreferred }
    }
  }
  if (effectiveMinutes % (24 * 60) === 0) {
    return { value: String(effectiveMinutes / (24 * 60)), unit: 'd' }
  }
  if (effectiveMinutes % 60 === 0) {
    return { value: String(effectiveMinutes / 60), unit: 'h' }
  }
  return { value: String(effectiveMinutes), unit: 'm' }
}

const sessionAgeInput = ref<{ value: string; unit: SessionAgeUnit }>({
  value: '3',
  unit: SESSION_MAX_AGE_DEFAULT_UNIT,
})

const syncSessionAgeFromSettings = () => {
  sessionAgeInput.value = deriveSessionAgeDisplay(props.settings.sessionMaxAge, props.settings.sessionMaxAgeUnit)
}

const onSessionAgeInputChanged = () => {
  const rawValue = String(sessionAgeInput.value.value ?? '').trim()
  const normalizedUnit = normalizeSessionAgeUnit(sessionAgeInput.value.unit) || SESSION_MAX_AGE_DEFAULT_UNIT
  if (/^\d+$/.test(rawValue)) {
    const parsedValue = Number.parseInt(rawValue, 10)
    if (Number.isSafeInteger(parsedValue) && parsedValue >= 0) {
      if (parsedValue === 0) {
        props.settings.sessionMaxAge = String(SESSION_MAX_AGE_MAX_MINUTES)
        props.settings.sessionMaxAgeUnit = SESSION_MAX_AGE_DEFAULT_UNIT
        return
      }
      props.settings.sessionMaxAge = String(parsedValue * sessionAgeUnitFactor(normalizedUnit))
      props.settings.sessionMaxAgeUnit = normalizedUnit
      return
    }
  }
  props.settings.sessionMaxAge = rawValue
  props.settings.sessionMaxAgeUnit = normalizedUnit
}

watch(
  () => [props.settings.sessionMaxAge, props.settings.sessionMaxAgeUnit],
  () => {
    syncSessionAgeFromSettings()
  },
  { immediate: true },
)

const onSystemTimeLocationSelected = (val: string) => {
  emit('update:systemTimeLocation', val)
  emit('systemTimeLocationSelected', val)
}

// --- 面板更新与生命周期状态 ---
const panelStatusLoading = ref(false)
const panelRemoteLoading = ref(false)
const panelLoadingMoreVersions = ref(false)
const panelInstalling = ref(false)
const panelUninstalling = ref(false)
const panelVersionMenuVisible = ref(false)
const panelInstallDialogVisible = ref(false)
const panelUninstallOverlay = ref(false)
const panelDockerUninstallDialogVisible = ref(false)
const panelUpdateLogDialogVisible = ref(false)
const panelUpdateLogLoading = ref(false)
const panelUpdateStatus = ref<PanelUpdateStatus | null>(null)
const panelUpdateLog = ref<PanelUpdateLogView | null>(null)
const panelSelectedVersion = ref('')
const panelVersionItems = ref<PanelVersionItem[]>([])
const panelHasMoreVersions = ref(false)
const panelAllVersionsLoaded = ref(false)
const panelUpdateFeedback = ref('')
const panelUpdateFeedbackType = ref<'success' | 'error' | 'info' | 'warning'>('info')
let panelVersionsRequest: Promise<void> | null = null
let panelUpdateStatusRequestSequence = 0
let panelUpdatePollingGeneration = 0
const panelUninstallPollTimerId = ref<number | null>(null)
const panelUpdateTaskPollTimerId = ref<number | null>(null)
const panelUpdateStopRequestPending = ref(false)
let panelUpdateTaskRequest: Promise<void> | null = null

const isPanelUpdatePollingAllowed = () => componentMounted && typeof document !== 'undefined' && document.visibilityState === 'visible'

const normalizePanelVersionTag = (value: unknown): string => {
  const normalized = String(value ?? '').trim()
  if (!/^v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/.test(normalized)) return ''
  return normalized
}

const normalizePanelManagedUpdateTask = (raw: any): PanelManagedUpdateTask | undefined => {
  if (raw == null || typeof raw !== 'object') return undefined
  const id = String(raw.id ?? '').trim()
  const state = String(raw.state ?? '').trim().toLowerCase()
  const phase = String(raw.phase ?? '').trim()
  if (!id || !state) return undefined
  return {
    id,
    state,
    phase,
    canCancel: raw.canCancel === true,
    stopRequested: raw.stopRequested === true,
    deadlineExceeded: raw.deadlineExceeded === true,
    startedAt: Number(raw.startedAt ?? 0),
    updatedAt: Number(raw.updatedAt ?? 0),
    deadlineAt: Number(raw.deadlineAt ?? 0),
    finishedAt: Number(raw.finishedAt ?? 0),
    error: String(raw.error ?? '').trim(),
  }
}

const normalizePanelUpdateStatus = (raw: any): PanelUpdateStatus | null => {
  if (raw == null || typeof raw !== 'object') return null
  const dockerUninstallCommands = Array.isArray(raw.dockerUninstallCommands)
    ? raw.dockerUninstallCommands
        .map((item: any) => ({
          id: String(item?.id ?? '').trim(),
          command: String(item?.command ?? '').trim(),
        }))
        .filter((item: any) => item.command !== '')
    : undefined
  return {
    localVersion: normalizePanelVersionTag(raw.localVersion) || String(raw.localVersion ?? '').trim(),
    binaryPath: String(raw.binaryPath ?? '').trim(),
    binaryName: String(raw.binaryName ?? '').trim(),
    installDir: String(raw.installDir ?? '').trim(),
    serviceFilePath: String(raw.serviceFilePath ?? '').trim(),
    serviceBinaryPath: String(raw.serviceBinaryPath ?? '').trim(),
    runningBinaryPath: String(raw.runningBinaryPath ?? '').trim(),
    installSource: String(raw.installSource ?? '').trim(),
    platform: String(raw.platform ?? '').trim(),
    canRestart: raw.canRestart === true,
    restartHint: String(raw.restartHint ?? '').trim(),
    canInstall: raw.canInstall === true,
    installHint: String(raw.installHint ?? '').trim(),
    canUninstall: raw.canUninstall === true,
    uninstallHint: String(raw.uninstallHint ?? '').trim(),
    uninstallMode: raw.uninstallMode === 'docker-guide' ? 'docker-guide' : raw.uninstallMode === 'unsupported' ? 'unsupported' : 'native',
    uninstallState: String(raw.uninstallState ?? '').trim(),
    uninstallPhase: String(raw.uninstallPhase ?? '').trim(),
    uninstallError: String(raw.uninstallError ?? '').trim(),
    uninstallFailures: Array.isArray(raw.uninstallFailures) ? raw.uninstallFailures.map((item: any) => String(item ?? '').trim()).filter(Boolean) : [],
    uninstallWarnings: Array.isArray(raw.uninstallWarnings) ? raw.uninstallWarnings.map((item: any) => String(item ?? '').trim()).filter(Boolean) : [],
    uninstallCanRetry: raw.uninstallCanRetry === true,
    dockerUninstallCommands,
    lastUpdateLogPath: String(raw.lastUpdateLogPath ?? '').trim(),
    lastUpdateError: String(raw.lastUpdateError ?? '').trim(),
    updateTask: normalizePanelManagedUpdateTask(raw.updateTask),
  }
}

const panelLocalVersionLabel = computed(() => {
  const version = String(panelUpdateStatus.value?.localVersion ?? '').trim()
  return version || i18n.global.t('setting.panelLocalUnknown')
})

const panelBinaryName = computed(() => String(panelUpdateStatus.value?.binaryName ?? '').trim())
const panelCanInstall = computed(() => panelUpdateStatus.value?.canInstall === true)
const panelInstallHint = computed(() => String(panelUpdateStatus.value?.installHint ?? '').trim())
const panelUninstallMode = computed(() => String(panelUpdateStatus.value?.uninstallMode ?? '').trim())
const panelCanUninstall = computed(() => panelUninstallMode.value === 'native' && panelUpdateStatus.value?.canUninstall === true)
const panelUninstallHint = computed(() => String(panelUpdateStatus.value?.uninstallHint ?? '').trim())
const panelUninstallState = computed(() => String(panelUpdateStatus.value?.uninstallState ?? '').trim())
const panelUninstallPhase = computed(() => String(panelUpdateStatus.value?.uninstallPhase ?? '').trim())
const panelUninstallError = computed(() => String(panelUpdateStatus.value?.uninstallError ?? '').trim())
const panelUninstallFailures = computed(() => Array.isArray(panelUpdateStatus.value?.uninstallFailures)
  ? panelUpdateStatus.value!.uninstallFailures!.map(value => String(value).trim()).filter(Boolean)
  : [])
const panelUninstallWarnings = computed(() => Array.isArray(panelUpdateStatus.value?.uninstallWarnings)
  ? panelUpdateStatus.value!.uninstallWarnings!.map(value => String(value).trim()).filter(Boolean)
  : [])
const panelUninstallCanRetry = computed(() => panelUpdateStatus.value?.uninstallCanRetry === true)
const panelDockerUninstallCommands = computed(() => Array.isArray(panelUpdateStatus.value?.dockerUninstallCommands)
  ? panelUpdateStatus.value!.dockerUninstallCommands!.filter(item => String(item?.command ?? '').trim() !== '')
  : [])
const panelHasDockerUninstallGuide = computed(() => panelUninstallMode.value === 'docker-guide' && panelDockerUninstallCommands.value.length > 0)
const panelCanUseUninstallAction = computed(() => panelCanUninstall.value || panelHasDockerUninstallGuide.value)
const panelUninstallFailed = computed(() => panelUninstallState.value === 'failed')
const panelUninstallButtonText = computed(() => panelHasDockerUninstallGuide.value
  ? i18n.global.t('setting.uninstallDockerGuide')
  : i18n.global.t('setting.uninstallPanel'))
const panelManagedUpdateTask = computed(() => panelUpdateStatus.value?.updateTask ?? null)
const panelUpdateTaskActive = computed(() => {
  const state = String(panelManagedUpdateTask.value?.state ?? '').trim().toLowerCase()
  return state === 'queued' || state === 'running' || state === 'stopping'
})
const panelUpdateTaskStopping = computed(() => (
  panelUpdateTaskActive.value && (
    panelManagedUpdateTask.value?.stopRequested === true
    || String(panelManagedUpdateTask.value?.state ?? '').trim().toLowerCase() === 'stopping'
  )
))
const panelUpdateTaskCanCancel = computed(() => (
  panelUpdateTaskActive.value
  && panelManagedUpdateTask.value?.canCancel === true
  && !panelUpdateTaskStopping.value
))
const panelUpdateTaskApplying = computed(() => (
  panelUpdateTaskActive.value
  && !panelUpdateTaskStopping.value
  && panelManagedUpdateTask.value?.canCancel === false
))
const panelUpdateTaskButtonText = computed(() => {
  if (panelUpdateStopRequestPending.value || panelUpdateTaskStopping.value) return '正在停止'
  if (panelUpdateTaskActive.value) {
    return panelUpdateTaskCanCancel.value ? '停止' : '正在应用'
  }
  return i18n.global.t('setting.panelInstall')
})
const panelUpdateTaskAlertType = computed<'info' | 'success' | 'warning' | 'error'>(() => {
  const state = String(panelManagedUpdateTask.value?.state ?? '').trim().toLowerCase()
  if (state === 'success') return 'success'
  if (state === 'error') return 'error'
  if (state === 'cancelled' || state === 'timed_out') return 'warning'
  return 'info'
})
const panelUpdateTaskStatusText = computed(() => {
  const task = panelManagedUpdateTask.value
  if (task == null) return ''
  const state = task.state.trim().toLowerCase()
  const phase = task.phase.trim()
  if (panelUpdateTaskStopping.value) return '正在停止面板更新任务'
  if (panelUpdateTaskActive.value && !task.canCancel) return phase ? `正在应用：${phase}` : '正在应用面板更新'
  if (panelUpdateTaskActive.value) return phase ? `正在准备更新：${phase}` : '正在准备面板更新'
  if (state === 'success') return phase === 'handoff' ? '更新 worker 已接手，面板将自动重启' : '面板更新准备已完成'
  if (state === 'cancelled') return '面板更新已停止'
  if (state === 'timed_out') return '面板更新准备超时，已停止并清理临时文件'
  if (state === 'error') return task.error ? `面板更新失败：${task.error}` : '面板更新失败'
  return phase || '面板更新状态未知'
})

const isBusy = computed(() => panelInstalling.value || panelUninstalling.value || panelUpdateTaskActive.value)

watch(isBusy, (val) => {
  emit('busyChange', val)
}, { immediate: true })

watch(
  () => panelUpdateStatus.value,
  (status) => {
    emit('canRestartChange', status?.canRestart === true, status?.restartHint || '')
  },
  { immediate: true },
)

const panelRemoteVersionLabel = computed(() => {
  if (panelRemoteLoading.value) return i18n.global.t('setting.panelLoading')
  if (panelVersionItems.value.length > 0) return panelVersionItems.value[0].value
  return i18n.global.t('setting.panelNotLoaded')
})

const panelLastUpdateError = computed(() => String(panelUpdateStatus.value?.lastUpdateError ?? '').trim())
const panelVersionNoDataText = computed(() => {
  if (panelRemoteLoading.value) return i18n.global.t('setting.panelVersionsLoading')
  if (panelVersionItems.value.length > 0) return i18n.global.t('setting.panelNoMoreVersions')
  return i18n.global.t('setting.panelOpenToLoad')
})
const panelUpdateLogContent = computed(() => {
  const lines = Array.isArray(panelUpdateLog.value?.lines) ? panelUpdateLog.value?.lines : []
  return lines.length > 0 ? lines.join('\n') : i18n.global.t('setting.panelNoLogs')
})
const panelUpdateLogModifiedText = computed(() => {
  const unix = Number(panelUpdateLog.value?.modified ?? 0)
  if (!Number.isFinite(unix) || unix <= 0) return ''
  return formatPanelDateTime(unix * 1000)
})

const describePanelUninstallFailure = (status: PanelUpdateStatus | null): string => {
  if (!status) return i18n.global.t('setting.uninstallPanelFailed')
  if (status.uninstallError) return status.uninstallError
  if (status.uninstallFailures && status.uninstallFailures.length > 0) {
    return status.uninstallFailures[0]
  }
  return i18n.global.t('setting.uninstallPanelFailed')
}

const clearPanelUpdateTaskPolling = () => {
  if (panelUpdateTaskPollTimerId.value !== null) {
    window.clearTimeout(panelUpdateTaskPollTimerId.value)
    panelUpdateTaskPollTimerId.value = null
  }
}

const clearPanelUninstallStatusTimer = () => {
  if (panelUninstallPollTimerId.value !== null) {
    window.clearTimeout(panelUninstallPollTimerId.value)
    panelUninstallPollTimerId.value = null
  }
}

const schedulePanelUpdateTaskPolling = () => {
  clearPanelUpdateTaskPolling()
  if (!isPanelUpdatePollingAllowed()) return
  panelUpdateTaskPollTimerId.value = window.setTimeout(() => {
    void pollPanelUpdateTask()
  }, 2500)
}

const handlePanelUpdateTaskTerminal = () => {
  clearPanelUpdateTaskPolling()
  panelUpdateStopRequestPending.value = false
  const task = panelManagedUpdateTask.value
  if (!task) return
  if (task.state === 'success') {
    if (task.phase === 'handoff') {
      emit('startReconnect', { trackUpdateStatus: true })
      return
    }
    panelUpdateFeedback.value = '面板更新准备已完成'
    panelUpdateFeedbackType.value = 'success'
  } else if (task.state === 'cancelled') {
    panelUpdateFeedback.value = '面板更新已停止'
    panelUpdateFeedbackType.value = 'warning'
  } else if (task.state === 'timed_out') {
    panelUpdateFeedback.value = '面板更新准备超时，已停止'
    panelUpdateFeedbackType.value = 'warning'
  } else if (task.state === 'error') {
    panelUpdateFeedback.value = task.error ? `面板更新失败：${task.error}` : '面板更新失败'
    panelUpdateFeedbackType.value = 'error'
  }
}

const pollPanelUpdateTask = async (): Promise<void> => {
  if (panelUpdateTaskRequest) return panelUpdateTaskRequest
  if (!isPanelUpdatePollingAllowed()) return
  const pollingGeneration = panelUpdatePollingGeneration
  const request = (async () => {
    const msg = await HttpUtils.get('api/panel-update-status', {}, { silentAuthCheck: true })
    if (pollingGeneration !== panelUpdatePollingGeneration || !isPanelUpdatePollingAllowed()) return
    if (!msg.success) {
      schedulePanelUpdateTaskPolling()
      return
    }
    applyPanelUpdateStatus(msg.obj)
    if (panelUpdateTaskActive.value) {
      schedulePanelUpdateTaskPolling()
      return
    }
    handlePanelUpdateTaskTerminal()
  })()
  panelUpdateTaskRequest = request
  try {
    await request
  } finally {
    if (panelUpdateTaskRequest === request) {
      panelUpdateTaskRequest = null
    }
  }
}

const startPanelUpdateTaskPolling = () => {
  clearPanelUpdateTaskPolling()
  if (!panelUpdateTaskActive.value || !isPanelUpdatePollingAllowed()) return
  void pollPanelUpdateTask()
}

const applyPanelUpdateStatus = (raw: any) => {
  panelUpdateStatus.value = normalizePanelUpdateStatus(raw)
}

const loadPanelUpdateStatus = async () => {
  if (!isPanelUpdatePollingAllowed()) return
  const requestSequence = ++panelUpdateStatusRequestSequence
  const pollingGeneration = panelUpdatePollingGeneration
  panelStatusLoading.value = true
  try {
    const msg = await HttpUtils.get('api/panel-update-status', {}, { silentAuthCheck: true })
    if (requestSequence !== panelUpdateStatusRequestSequence
      || pollingGeneration !== panelUpdatePollingGeneration
      || !isPanelUpdatePollingAllowed()) return
    if (msg.success) {
      applyPanelUpdateStatus(msg.obj)
      if (panelUninstallFailed.value) {
        panelUpdateFeedback.value = describePanelUninstallFailure(panelUpdateStatus.value)
        panelUpdateFeedbackType.value = 'error'
      } else if (panelLastUpdateError.value) {
        panelUpdateFeedback.value = `${i18n.global.t('setting.panelPreviousUpdateFailed')}：${panelLastUpdateError.value}`
        panelUpdateFeedbackType.value = 'warning'
      }
      if (panelUpdateTaskActive.value) {
        schedulePanelUpdateTaskPolling()
      } else {
        handlePanelUpdateTaskTerminal()
      }
    }
  } finally {
    if (requestSequence === panelUpdateStatusRequestSequence && componentMounted) {
      panelStatusLoading.value = false
    }
  }
}

const openPanelUpdateLogDialog = async () => {
  panelUpdateLogDialogVisible.value = true
  panelUpdateLogLoading.value = true
  try {
    const msg = await HttpUtils.get('api/panel-update-log', {}, { silentAuthCheck: true })
    if (msg.success) {
      panelUpdateLog.value = msg.obj ?? null
    } else {
      panelUpdateLog.value = {
        lines: [String(msg.msg || i18n.global.t('setting.panelLogReadFailed'))],
      }
    }
  } finally {
    panelUpdateLogLoading.value = false
  }
}

const buildPanelVersionItems = (versions: any[]): PanelVersionItem[] => {
  const items: PanelVersionItem[] = []
  versions.forEach(item => {
    const tagName = normalizePanelVersionTag(item?.tag_name ?? item?.tagName ?? '')
    if (!tagName) return
    items.push({
      title: tagName,
      value: tagName,
      tagName,
      name: item?.name ?? '',
      prerelease: item?.prerelease === true,
      publishedAt: item?.published_at ?? item?.publishedAt ?? '',
      assetName: item?.asset_name ?? item?.assetName ?? '',
      assetSize: item?.asset_size ?? item?.assetSize ?? 0,
    })
  })
  return items
}

const applyPanelVersionResponse = (obj: any, append: boolean) => {
  const nextItems = buildPanelVersionItems(Array.isArray(obj?.versions) ? obj.versions : [])
  const existing = append ? [...panelVersionItems.value] : []
  const seen = new Set(existing.map(item => item.value))
  nextItems.forEach(item => {
    if (!seen.has(item.value)) {
      existing.push(item)
      seen.add(item.value)
    }
  })
  panelVersionItems.value = existing
  panelHasMoreVersions.value = obj?.has_more === true || obj?.hasMore === true
  panelAllVersionsLoaded.value = append && !panelHasMoreVersions.value
  if (!append && panelVersionItems.value.length > 0) {
    panelSelectedVersion.value = panelVersionItems.value[0].value
  } else if (!panelSelectedVersion.value && panelVersionItems.value.length > 0) {
    panelSelectedVersion.value = panelVersionItems.value[0].value
  }
}

const loadPanelVersions = async (append = false) => {
  if (panelVersionsRequest) return panelVersionsRequest

  const request = (async () => {
    if (append) {
      panelLoadingMoreVersions.value = true
    } else {
      panelRemoteLoading.value = true
      panelAllVersionsLoaded.value = false
    }

    try {
      const msg = await HttpUtils.get('api/panel-update-versions', {
        offset: append ? panelVersionItems.value.length : 0,
        limit: 5,
      }, { silentAuthCheck: true })

      if (msg.success) {
        applyPanelVersionResponse(msg.obj, append)
        if (append) {
          panelUpdateFeedback.value = panelAllVersionsLoaded.value
            ? i18n.global.t('setting.panelNoMoreVersions')
            : i18n.global.t('setting.panelMoreLoaded')
          panelUpdateFeedbackType.value = panelAllVersionsLoaded.value ? 'info' : 'success'
          return
        }
        panelUpdateFeedback.value = i18n.global.t('setting.panelUpdateDone')
        panelUpdateFeedbackType.value = 'success'
      } else if (msg.msg) {
        panelUpdateFeedback.value = msg.msg
        panelUpdateFeedbackType.value = 'error'
      }
    } finally {
      if (append) {
        panelLoadingMoreVersions.value = false
      } else {
        panelRemoteLoading.value = false
      }
    }
  })()

  panelVersionsRequest = request
  try {
    await request
  } finally {
    if (panelVersionsRequest === request) {
      panelVersionsRequest = null
    }
  }
}

const checkPanelUpdates = async () => {
  await loadPanelVersions(false)
}

const ensurePanelVersionsLoaded = async () => {
  if (panelRemoteLoading.value || panelLoadingMoreVersions.value) return
  if (panelVersionItems.value.length > 0) return
  await loadPanelVersions(false)
}

const onPanelVersionMenuUpdate = (opened: boolean) => {
  if (!opened) return
  void ensurePanelVersionsLoaded()
}

const loadMorePanelVersions = async () => {
  if (panelAllVersionsLoaded.value) {
    panelUpdateFeedback.value = i18n.global.t('setting.panelNoMoreVersions')
    panelUpdateFeedbackType.value = 'info'
    return
  }
  await loadPanelVersions(true)
}

const openPanelInstallDialog = () => {
  if (panelUninstalling.value) return
  if (!panelCanInstall.value) {
    panelUpdateFeedback.value = panelInstallHint.value || i18n.global.t('setting.panelInstallUnsupported')
    panelUpdateFeedbackType.value = 'warning'
    return
  }
  if (!panelSelectedVersion.value) return
  panelInstallDialogVisible.value = true
}

const stopPanelUpdateTask = async () => {
  const task = panelManagedUpdateTask.value
  if (task == null || !panelUpdateTaskCanCancel.value || panelUpdateStopRequestPending.value) return
  panelUpdateStopRequestPending.value = true
  panelUpdateStatus.value = {
    ...(panelUpdateStatus.value ?? {}),
    updateTask: {
      ...task,
      state: 'stopping',
      phase: 'stopping',
      canCancel: false,
      stopRequested: true,
    },
  }
  try {
    const msg = await HttpUtils.post('api/panel-update-stop', { id: task.id }, { silentAuthCheck: true })
    if (msg.success && msg.obj) {
      panelUpdateStatus.value = {
        ...(panelUpdateStatus.value ?? {}),
        updateTask: normalizePanelManagedUpdateTask(msg.obj),
      }
    }
  } catch {
    // 由紧随其后的状态轮询确认停止请求是否已被后端受理
  } finally {
    startPanelUpdateTaskPolling()
  }
}

const installPanelVersion = async () => {
  if (!panelSelectedVersion.value || panelUninstalling.value) return
  panelInstalling.value = true
  const version = panelSelectedVersion.value
  try {
    const msg = await HttpUtils.post('api/panel-update-install', { version }, { silentAuthCheck: true, timeout: 35000 })
    panelInstallDialogVisible.value = false

    if (msg.success && msg.obj) {
      const task = normalizePanelManagedUpdateTask(msg.obj)
      panelUpdateStatus.value = {
        ...(panelUpdateStatus.value ?? {}),
        updateTask: task,
      }
      panelUpdateFeedback.value = ''
      panelUpdateFeedbackType.value = 'info'
      startPanelUpdateTaskPolling()
      void loadPanelUpdateStatus()
    } else {
      await loadPanelUpdateStatus()
      if (!panelUpdateTaskActive.value) {
        panelUpdateFeedback.value = msg.msg || i18n.global.t('setting.panelInstallFailed')
        panelUpdateFeedbackType.value = 'error'
      }
    }
  } catch {
    await loadPanelUpdateStatus()
    if (!panelUpdateTaskActive.value) {
      panelUpdateFeedback.value = i18n.global.t('setting.panelInstallFailed')
      panelUpdateFeedbackType.value = 'error'
    }
  } finally {
    panelInstalling.value = false
  }
}

const startPanelUninstallStatusPolling = () => {
  clearPanelUninstallStatusTimer()
  if (!isPanelUpdatePollingAllowed()) return
  const pollingGeneration = panelUpdatePollingGeneration

  const poll = async () => {
    if (pollingGeneration !== panelUpdatePollingGeneration || !isPanelUpdatePollingAllowed()) return
    try {
      const msg = await HttpUtils.get('api/panel-update-status', {}, { silentAuthCheck: true })
      if (pollingGeneration !== panelUpdatePollingGeneration || !isPanelUpdatePollingAllowed()) return
      if (msg.success) {
        panelUpdateStatus.value = msg.obj ?? null
        if (panelUninstallFailed.value) {
          panelUninstallOverlay.value = false
          panelUninstalling.value = false
          panelUpdateFeedback.value = describePanelUninstallFailure(panelUpdateStatus.value)
          panelUpdateFeedbackType.value = 'error'
          clearPanelUninstallStatusTimer()
          return
        }
      }
    } catch {
      // 原生卸载成功后连接会中断；遮罩保持，避免已确认的操作被误判为失败。
    }

    if (pollingGeneration === panelUpdatePollingGeneration && isPanelUpdatePollingAllowed()) {
      panelUninstallPollTimerId.value = window.setTimeout(poll, 2000)
    }
  }

  panelUninstallPollTimerId.value = window.setTimeout(poll, 1200)
}

const requestPanelUninstall = async () => {
  if (panelStatusLoading.value || props.disabled || isBusy.value) return
  if (panelHasDockerUninstallGuide.value) {
    panelDockerUninstallDialogVisible.value = true
    return
  }
  if (!panelCanUninstall.value) return

  const confirmed = await confirm({
    severity: 'danger',
    title: i18n.global.t('setting.uninstallPanelConfirmTitle'),
    message: i18n.global.t('setting.uninstallPanelConfirm'),
    confirmText: i18n.global.t('confirmDialog.actions.uninstall'),
  })
  if (!confirmed || panelStatusLoading.value || props.disabled || isBusy.value || !panelCanUninstall.value) return

  panelUninstalling.value = true
  let accepted = false
  try {
    const msg = await HttpUtils.post('api/panel-uninstall', {}, { silentAuthCheck: true, timeout: 10000 })
    if (msg.success) {
      accepted = true
      panelUninstallOverlay.value = true
      startPanelUninstallStatusPolling()
      return
    }
    push.warning({
      title: i18n.global.t('failed'),
      duration: 6000,
      message: msg.msg || i18n.global.t('setting.uninstallPanelStartFailed'),
    })
  } finally {
    if (!accepted) {
      panelUninstalling.value = false
    }
  }
}

const panelDockerUninstallCommandLabel = (id?: string) => id === 'compose'
  ? i18n.global.t('setting.uninstallDockerCompose')
  : i18n.global.t('setting.uninstallDockerRun')

const copyDockerUninstallCommand = async (command?: string) => {
  const text = String(command ?? '').trim()
  if (!text) return
  try {
    if (navigator.clipboard?.writeText && window.isSecureContext) {
      await navigator.clipboard.writeText(text)
    } else {
      const textarea = document.createElement('textarea')
      textarea.value = text
      textarea.setAttribute('readonly', '')
      textarea.style.position = 'fixed'
      textarea.style.opacity = '0'
      document.body.appendChild(textarea)
      textarea.select()
      const copied = document.execCommand('copy')
      document.body.removeChild(textarea)
      if (!copied) throw new Error('copy command was rejected')
    }
    push.success({
      title: i18n.global.t('success'),
      duration: 3000,
      message: i18n.global.t('copyToClipboard'),
    })
  } catch {
    push.error({
      title: i18n.global.t('failed'),
      duration: 5000,
      message: i18n.global.t('copyToClipboard'),
    })
  }
}

const handleVisibilityChange = () => {
  if (typeof document === 'undefined') return
  if (document.visibilityState !== 'visible') {
    panelUpdatePollingGeneration += 1
    panelUpdateStatusRequestSequence += 1
    panelStatusLoading.value = false
    clearPanelUpdateTaskPolling()
    clearPanelUninstallStatusTimer()
    return
  }
  if (panelUninstallOverlay.value && panelUninstalling.value) {
    startPanelUninstallStatusPolling()
  } else {
    void loadPanelUpdateStatus()
  }
}

onMounted(() => {
  componentMounted = true
  syncSessionAgeFromSettings()
  void loadPanelUpdateStatus()
  if (typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', handleVisibilityChange)
  }
})

onBeforeUnmount(() => {
  componentMounted = false
  panelUpdatePollingGeneration += 1
  panelUpdateStatusRequestSequence += 1
  clearPanelUpdateTaskPolling()
  clearPanelUninstallStatusTimer()
  if (typeof document !== 'undefined') {
    document.removeEventListener('visibilitychange', handleVisibilityChange)
  }
  emit('busyChange', false)
})
</script>

<style scoped>
.danger-zone-card {
  border-style: dashed !important;
  border-width: 1px !important;
}

.panel-version-footer__summary {
  color: rgba(255, 255, 255, 0.92);
}

.panel-version-footer__action,
.panel-version-footer__action :deep(.v-btn__content) {
  color: #fff !important;
}

.panel-version-footer__action.v-btn--disabled,
.panel-version-footer__action.v-btn--disabled :deep(.v-btn__content) {
  color: rgba(255, 255, 255, 0.88) !important;
  opacity: 1;
}

.panel-uninstall-trigger,
.panel-uninstall-button {
  width: 100%;
}

.panel-uninstall-trigger {
  display: block;
}

.panel-uninstall-overlay-card {
  width: calc(100vw - 32px);
  max-width: 420px;
}

.panel-uninstall-failure {
  min-width: 0;
  flex: 1 1 260px;
}

.panel-uninstall-message-list {
  margin-bottom: 0;
  padding-left: 20px;
  overflow-wrap: anywhere;
}

.panel-update-task-status__text,
.panel-update-task-status__id {
  min-width: 0;
  overflow-wrap: anywhere;
}

.docker-uninstall-command {
  border: 1px solid rgba(0, 0, 0, 0.14);
  border-radius: 6px;
  padding: 12px;
}

.docker-uninstall-command__content {
  max-width: 100%;
  margin: 10px 0 0;
  padding: 12px;
  overflow-x: auto;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  background: #f4f6f7;
  color: #1d252c;
  border-radius: 4px;
}

.panel-update-log-box {
  max-height: 480px;
  overflow-y: auto;
  white-space: pre-wrap;
  background: rgba(0, 0, 0, 0.2);
  padding: 12px;
  border-radius: 8px;
  font-size: 13px;
  line-height: 1.5;
}

@media (min-width: 600px) {
  .panel-uninstall-trigger,
  .panel-uninstall-button {
    width: auto;
  }

  .panel-uninstall-trigger {
    display: inline-flex;
  }
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
