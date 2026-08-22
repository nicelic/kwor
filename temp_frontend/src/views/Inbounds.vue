<template>
  <template v-if="initializing">
    <v-row align="center" justify="center" style="min-height: 240px;">
      <v-col cols="12" class="text-center">
        <v-progress-circular indeterminate color="primary" />
        <div class="mt-3">{{ $t('loading') }}</div>
      </v-col>
    </v-row>
  </template>
  <template v-else-if="loadFailed">
    <v-row align="center" justify="center" style="min-height: 240px;">
      <v-col cols="12" sm="8" md="6">
        <v-alert type="error" variant="tonal" :title="$t('failed')" class="text-center">
          <v-btn color="primary" class="mt-2" prepend-icon="mdi-refresh" @click="initialize">
            {{ $t('actions.update') }}
          </v-btn>
        </v-alert>
      </v-col>
    </v-row>
  </template>
  <template v-else>
    <SingboxCore
      v-if="namespaceApi.showCoreControlsOnInbounds && props.namespace !== 'mihomo'"
      v-model="coreModal.visible"
      :visible="coreModal.visible"
      @close="closeCoreModal"
    />
    <MihomoCore
      v-else-if="namespaceApi.showCoreControlsOnInbounds"
      v-model="coreModal.visible"
      :visible="coreModal.visible"
      @close="closeCoreModal"
    />
    <InboundVue
      v-model="modal.visible"
      :visible="modal.visible"
      :id="modal.id"
      :namespace="props.namespace"
      :inTags="inTags"
      :tlsConfigs="tlsConfigs"
      @close="closeModal"
    />
    <Stats
      v-model="stats.visible"
      :visible="stats.visible"
      :resource="stats.resource"
      :tag="stats.tag"
      :namespace="props.namespace"
      @close="closeStats"
    />
    <PortLogs
      v-model="portLogModal.visible"
      :visible="portLogModal.visible"
      :logs="portLogs"
      @close="closePortLog"
      @clear="clearPortLogs"
    />

  <v-row v-if="namespaceApi.showCoreControlsOnInbounds" align="center" class="mb-1">
    <v-col cols="auto" class="d-flex align-center" style="gap: 6px;">
      <v-chip
        :color="coreRunning ? 'success' : 'error'"
        variant="flat"
        size="small"
        :prepend-icon="coreRunning ? 'mdi-check-circle' : 'mdi-close-circle'"
      >
        {{ coreRunning ? t('coreManager.running') : t('coreManager.stopped') }}
      </v-chip>
      <v-tooltip location="top" :text="t('coreManager.start')">
        <template #activator="{ props: tooltipProps }">
          <v-btn
            v-bind="tooltipProps"
            color="success"
            variant="flat"
            size="x-small"
            icon="mdi-play"
            :disabled="coreDownloadTaskActive || coreControlBusy || coreRunning || !coreReady"
            :loading="startingCore"
            @click="startCore"
          />
        </template>
      </v-tooltip>
      <v-tooltip location="top" :text="t('coreManager.stop')">
        <template #activator="{ props: tooltipProps }">
          <v-btn
            v-bind="tooltipProps"
            color="error"
            variant="flat"
            size="x-small"
            icon="mdi-stop"
            :disabled="coreDownloadTaskActive || coreControlBusy || !coreRunning"
            :loading="stoppingCore"
            @click="stopCore"
          />
        </template>
      </v-tooltip>
      <v-tooltip location="top" :text="t('coreManager.restart')">
        <template #activator="{ props: tooltipProps }">
          <v-btn
            v-bind="tooltipProps"
            color="warning"
            variant="flat"
            size="x-small"
            icon="mdi-restart"
            :disabled="coreDownloadTaskActive || coreControlBusy || !coreRunning || !coreReady"
            :loading="restartingCore"
            @click="restartCore"
          />
        </template>
      </v-tooltip>
    </v-col>
    <v-spacer></v-spacer>
    <v-col cols="auto" class="d-flex align-center" style="gap: 8px;">
      <v-badge
        :model-value="coreUpdateCount > 0"
        :content="coreUpdateCount"
        color="error"
        offset-x="4"
        offset-y="6"
      >
        <v-btn color="warning" size="small" prepend-icon="mdi-engine" @click="openCoreModal">
          {{ t(namespaceApi.core.modalButtonLabel) }}
        </v-btn>
      </v-badge>
    </v-col>
    <v-col v-if="coreDownloadTaskActive" cols="12" class="pt-0">
      <v-alert type="info" variant="tonal" density="compact" class="core-download-hint">
        <div class="d-flex align-center justify-space-between flex-wrap" style="gap: 8px;">
          <span class="core-download-hint__text">{{ coreDownloadTaskHint }}</span>
          <v-btn size="small" variant="text" color="primary" prepend-icon="mdi-engine" @click="openCoreModal">
            查看
          </v-btn>
        </div>
      </v-alert>
    </v-col>
  </v-row>

    <v-row>
      <v-col cols="12" justify="center" align="center">
        <v-btn color="primary" :disabled="inboundWriteBusy" @click="showModal(0)">{{ $t('actions.add') }}</v-btn>
        <v-btn color="primary" variant="tonal" class="ml-3" @click="openPortLog">{{ t('portLogs.open') }}</v-btn>
      </v-col>
    </v-row>

    <v-row>
      <v-col cols="12" sm="4" md="3" lg="2" v-for="item in <any[]>inbounds" :key="item.id">
      <v-card rounded="xl" elevation="5" min-width="200" :title="item.tag">
        <v-card-subtitle style="margin-top: -20px;">
          <v-row>
            <v-col>{{ item.type }}</v-col>
          </v-row>
        </v-card-subtitle>
        <v-card-text>
          <v-row>
            <v-col>{{ $t('in.addr') }}</v-col>
            <v-col>
              {{ item.listen }}
            </v-col>
          </v-row>
          <v-row>
            <v-col>{{ $t('in.port') }}</v-col>
            <v-col>
              {{ item.listen_port }}
            </v-col>
          </v-row>
          <v-row>
            <v-col>{{ $t('objects.tls') }}</v-col>
            <v-col>
              {{ item.tls_id > 0 ? $t('enable') : $t('disable') }}
            </v-col>
          </v-row>
          <v-row>
            <v-col>{{ $t('pages.clients') }}</v-col>
            <v-col>
              <template v-if="item.user_management?.selectable ?? !!item.users">
                <v-tooltip activator="parent" dir="ltr" location="bottom" v-if="(item.users?.length ?? 0) > 0">
                  <span v-for="u in item.users">{{ u }}<br /></span>
                </v-tooltip>
                {{ item.users?.length ?? 0 }}
              </template>
              <template v-else>-</template>
            </v-col>
          </v-row>
          <v-row>
            <v-col>{{ $t('online') }}</v-col>
            <v-col>
              <template v-if="onlines.includes(item.tag)">
                <v-chip density="comfortable" size="small" color="success" variant="flat">{{ $t('online') }}</v-chip>
              </template>
              <template v-else>-</template>
            </v-col>
          </v-row>
        </v-card-text>
        <v-divider></v-divider>
        <v-card-actions style="padding: 0;">
          <v-tooltip location="top" :text="$t('actions.edit')">
            <template #activator="{ props: tooltipProps }">
              <v-btn v-bind="tooltipProps" icon="mdi-file-edit" :disabled="inboundWriteBusy" @click="showModal(item.id)" />
            </template>
          </v-tooltip>
          <v-tooltip location="top" :text="$t('actions.del')">
            <template #activator="{ props: tooltipProps }">
              <v-btn v-bind="tooltipProps" icon="mdi-file-remove" style="margin-inline-start:0;" color="warning" :disabled="inboundWriteBusy" @click="requestDeleteConfirm(item.id)" />
            </template>
          </v-tooltip>
          <v-overlay
            :model-value="deleteConfirmId === item.id"
            contained
            :persistent="inboundWriteBusy"
            class="align-center justify-center"
            @update:model-value="value => { if (!value) closeDeleteConfirm(item.id) }"
          >
            <v-card :title="$t('actions.del')" rounded="lg">
              <v-divider></v-divider>
              <v-card-text>{{ $t('confirm') }}</v-card-text>
              <v-card-actions>
                <v-btn color="error" variant="outlined" :loading="isDeletingInbound(item.id)" :disabled="inboundWriteBusy" @click="delInbound(item.id)">{{ $t('yes') }}</v-btn>
                <v-btn color="success" variant="outlined" :disabled="inboundWriteBusy" @click="closeDeleteConfirm(item.id)">{{ $t('no') }}</v-btn>
              </v-card-actions>
            </v-card>
          </v-overlay>
          <v-tooltip v-if="enableTraffic" location="top" :text="$t('stats.graphTitle')">
            <template #activator="{ props: tooltipProps }">
              <v-btn v-bind="tooltipProps" icon="mdi-chart-line" :disabled="inboundWriteBusy" @click="showStats(item.tag)" />
            </template>
          </v-tooltip>
        </v-card-actions>
      </v-card>
      </v-col>
    </v-row>
  </template>
</template>

<script lang="ts" setup>
import SingboxCore from '@/layouts/modals/SingboxCore.vue'
import MihomoCore from '@/layouts/modals/MihomoCore.vue'
import InboundVue from '@/layouts/modals/Inbound.vue'
import Stats from '@/layouts/modals/Stats.vue'
import PortLogs from '@/layouts/modals/PortLogs.vue'
import HttpUtils from '@/plugins/httputil'
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { Inbound } from '@/types/inbounds'
import { push } from 'notivue'
import { PORT_RANGE_TEMPLATE, PortRangeCheckItem, UDPRangeStatus, checkPortOccupancy } from '@/plugins/portCheck'
import { getNamespaceApi, getNamespaceStore, type UiNamespace } from '@/store/uiNamespace'
import { useI18n } from 'vue-i18n'

interface PortLogEntry {
  id: string
  timestamp: number
  tag: string
  range: string
  message: string
}

interface CoreDownloadTaskStatus {
  id: string
  state: string
  stage: string
  canCancel: boolean
  stopRequested: boolean
}

const props = withDefaults(defineProps<{ namespace?: UiNamespace }>(), {
  namespace: 'default',
})

const { t } = useI18n()
const store = getNamespaceStore(props.namespace)
const namespaceApi = getNamespaceApi(props.namespace)
const PORT_LOG_STORAGE_KEY = namespaceApi.portLogStorageKey
const initializing = ref(true)
const loadFailed = ref(false)
let componentActive = true

const initialize = async () => {
  const hadLoadedData = store.hasFullData
  initializing.value = true
  loadFailed.value = false
  try {
    const success = await store.loadData()
    if (!componentActive) return
    if (!success && !hadLoadedData) {
      loadFailed.value = true
    }
  } catch {
    if (componentActive && !hadLoadedData) loadFailed.value = true
  } finally {
    if (componentActive) {
      initializing.value = false
      if (!loadFailed.value) startBackgroundPolling()
    }
  }
}

const inbounds = computed((): Inbound[] => {
  return <Inbound[]>store.inbounds
})

const tlsConfigs = computed((): any[] => {
  return <any[]>store.tlsConfigs
})

const inTags = computed((): string[] => {
  return [...inbounds.value?.map(i => i.tag), ...store.endpoints?.filter((e: any) => e.listen_port > 0).map((e: any) => e.tag)]
})

const onlines = computed(() => {
  return store.onlines.inbound ?? []
})

const enableTraffic = computed(() => {
  return store.enableTraffic
})

const modal = ref({
  visible: false,
  id: 0,
})

const deleteConfirmId = ref<number | null>(null)
const deletingInboundIds = ref<number[]>([])
const inboundWriteBusy = computed(() => deletingInboundIds.value.length > 0)
const coreModal = ref({
  visible: false,
})
const startingCore = ref(false)
const stoppingCore = ref(false)
const restartingCore = ref(false)
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

const coreUpdateCount = ref(0)
const coreUpdateTimerId = ref<number | 0>(0)
const coreDownloadTask = ref<CoreDownloadTaskStatus | null>(null)
const coreDownloadTimerId = ref<number | null>(null)
const coreInstalled = ref(false)
const coreCompatible = ref(false)

const showModal = (id: number) => {
  if (inboundWriteBusy.value) return
  modal.value.id = id
  modal.value.visible = true
}

const closeModal = () => {
  modal.value.visible = false
}

const openCoreModal = () => {
  coreModal.value.visible = true
}

const closeCoreModal = () => {
  coreModal.value.visible = false
  void loadCoreStatus()
  void loadCoreUpdateMarker()
}

const requestDeleteConfirm = (id: number) => {
  if (inboundWriteBusy.value) return
  deleteConfirmId.value = id
}

const closeDeleteConfirm = (id: number) => {
  if (inboundWriteBusy.value) return
  if (deleteConfirmId.value === id) deleteConfirmId.value = null
}

const delInbound = async (id: number) => {
  if (inboundWriteBusy.value) return
  const inbound = inbounds.value.find(item => item.id === id)
  if (!inbound) {
    deleteConfirmId.value = null
    return
  }
  deletingInboundIds.value = [...deletingInboundIds.value, id]
  try {
    const success = await store.save('inbounds', 'del', inbound.tag)
    if (success && deleteConfirmId.value === id) deleteConfirmId.value = null
  } finally {
    deletingInboundIds.value = deletingInboundIds.value.filter(value => value !== id)
  }
}

const isDeletingInbound = (id: number) => deletingInboundIds.value.includes(id)

const stats = ref({
  visible: false,
  resource: 'inbound',
  tag: '',
})

const showStats = (tag: string) => {
  if (inboundWriteBusy.value) return
  stats.value.tag = tag
  stats.value.visible = true
}

const closeStats = () => {
  stats.value.visible = false
}

const portLogModal = ref({
  visible: false,
})

const openPortLog = () => {
  portLogModal.value.visible = true
}

const closePortLog = () => {
  portLogModal.value.visible = false
}

const portLogs = ref(<PortLogEntry[]>[])
const normalizePortLogs = (raw: unknown): PortLogEntry[] => {
  if (!Array.isArray(raw)) return []
  return raw.map((item: any) => {
    const timestamp = Number(item?.timestamp)
    const id = String(item?.id ?? '').trim()
    const tag = String(item?.tag ?? '')
    const range = String(item?.range ?? '')
    const message = String(item?.message ?? '')
    if (!id || !Number.isFinite(timestamp) || timestamp <= 0 || !message) return null
    return { id, timestamp, tag, range, message }
  }).filter((item): item is PortLogEntry => item !== null).slice(0, 1000)
}
const clearPortLogs = () => {
  portLogs.value = []
  try {
    localStorage.removeItem(PORT_LOG_STORAGE_KEY)
  } catch {
    // Storage may be unavailable in a restricted browser context.
  }
}

const monitorState = ref(<Record<string, string>>{})
const monitorIntervalId = ref(<number | 0>0)
const portCheckUnsupportedHinted = ref(false)
const portMonitorLimitHinted = ref(false)
const coreRunning = ref(false)
let portRangeMonitorRequest: Promise<void> | null = null
let coreStatusRequest: Promise<boolean> | null = null
let coreUpdateMarkerRequest: Promise<void> | null = null
let coreDownloadTaskRequest: Promise<void> | null = null
let portRangeMonitorController: AbortController | null = null
let coreStatusController: AbortController | null = null
let coreUpdateMarkerController: AbortController | null = null
let coreDownloadTaskController: AbortController | null = null

const coreDownloadTaskActive = computed(() => {
  const state = String(coreDownloadTask.value?.state ?? '').trim().toLowerCase()
  return state === 'queued' || state === 'running' || state === 'stopping'
})
const coreControlBusy = computed(() => (
  startingCore.value || stoppingCore.value || restartingCore.value
))
const coreReady = computed(() => coreInstalled.value && coreCompatible.value)

const coreDownloadTaskStageText = computed(() => {
  const stage = String(coreDownloadTask.value?.stage ?? '').trim().toLowerCase()
  switch (stage) {
    case 'downloading':
      return t('coreManager.stageDownloading')
    case 'extracting':
      return t('coreManager.stageExtracting')
    case 'validating':
      return t('coreManager.stageValidating')
    case 'stopping':
      return t('coreManager.stageStopping')
    case 'replacing':
      return t('coreManager.stageReplacing')
    case 'starting':
      return t('coreManager.stageStarting')
    default:
      return stage || '准备中'
  }
})

const coreDownloadTaskHint = computed(() => {
  if (coreDownloadTask.value?.stopRequested) return '核心下载任务正在停止'
  if (coreDownloadTask.value?.canCancel === false) return `核心下载任务正在应用：${coreDownloadTaskStageText.value}`
  return `核心下载任务进行中：${coreDownloadTaskStageText.value}`
})

const summarizePorts = (ports: number[]): string => {
  if (!ports || ports.length === 0) return '-'
  if (ports.length <= 20) return ports.join(',')
  return `${ports.slice(0, 20).join(',')} ...`
}

const showUnsupportedHint = () => {
  if (portCheckUnsupportedHinted.value) return
  portCheckUnsupportedHinted.value = true
  push.warning({
    title: t('portMonitor.noticeTitle'),
    duration: 5000,
    message: t('portMonitor.linuxOnly'),
  })
}

const appendPortLog = (tag: string, range: string, message: string) => {
  portLogs.value.unshift({
    id: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
    timestamp: Date.now(),
    tag,
    range,
    message,
  })
  if (portLogs.value.length > 1000) {
    portLogs.value = portLogs.value.slice(0, 1000)
  }
  try {
    localStorage.setItem(PORT_LOG_STORAGE_KEY, JSON.stringify(portLogs.value))
  } catch {
    // Keep the in-memory log even when persistence is unavailable/full.
  }
}

const refreshCoreRunning = (): Promise<boolean> => {
  if (coreStatusRequest) return coreStatusRequest
  const request = (async () => {
    const controller = new AbortController()
    coreStatusController = controller
    try {
      const data = await HttpUtils.get(namespaceApi.core.statusEndpoint, {}, { silentAuthCheck: true, signal: controller.signal })
      if (controller.signal.aborted || coreStatusController !== controller) return coreRunning.value
      if (data.success && data.obj) {
        coreRunning.value = data.obj.running === true
        coreInstalled.value = data.obj.installed === true
        coreCompatible.value = data.obj.compatible === true
      }
    } catch {
      // Keep last known state to avoid noisy monitor flapping.
    } finally {
      if (coreStatusController === controller) coreStatusController = null
    }
    return coreRunning.value
  })()
  coreStatusRequest = request
  void request.finally(() => {
    if (coreStatusRequest === request) {
      coreStatusRequest = null
    }
  })
  return request
}

const getMonitorTargets = (): PortRangeCheckItem[] => {
	const maxTargets = 32
  const targets: PortRangeCheckItem[] = []
  for (const inbound of inbounds.value) {
    if (!namespaceApi.portHopTypes.includes(inbound.type)) continue
    const portHopRange = (<any>inbound).port_hop_range
    if (typeof portHopRange !== 'string') continue
    const normalizedRange = portHopRange.trim()
    if (normalizedRange === '') continue
    targets.push({
      id: String(inbound.id ?? 0),
      tag: inbound.tag ?? '',
      range: normalizedRange,
	  })
	}
	if (targets.length <= maxTargets) return targets
	if (!portMonitorLimitHinted.value) {
		portMonitorLimitHinted.value = true
		push.warning({
			title: t('portMonitor.monitorTitle'),
			duration: 7000,
			message: `端口跳跃监控最多同时检查 ${maxTargets} 个入站，当前仅监控前 ${maxTargets} 个。`,
		})
	}
	return targets.slice(0, maxTargets)
}

const getStateKey = (status: UDPRangeStatus): string => {
  return `${status.id}:${status.normalized || status.input}`
}

const handleRangeStatus = (status: UDPRangeStatus) => {
  const stateKey = getStateKey(status)
  const previous = monitorState.value[stateKey]

  if (!status.valid) {
    const next = `invalid:${status.error ?? 'invalid'}`
    const invalidRangeMessage = t('portMonitor.invalidRange', { example: PORT_RANGE_TEMPLATE })
    if (previous !== next) {
      appendPortLog(status.tag, status.input, invalidRangeMessage)
      push.warning({
        title: t('portMonitor.monitorTitle'),
        duration: 7000,
        message: `[${status.tag}] ${invalidRangeMessage}`,
      })
    }
    monitorState.value[stateKey] = next
    return
  }

  if (status.occupied_count > 0) {
    const next = `occupied:${status.occupied_ports.join(',')}`
    if (previous !== next) {
      const occupiedText = summarizePorts(status.occupied_ports)
      const occupiedMessage = t('portMonitor.udpOccupied', { ports: occupiedText })
      appendPortLog(status.tag, status.normalized || status.input, occupiedMessage)
      push.warning({
        title: t('portMonitor.monitorTitle'),
        duration: 7000,
        message: `[${status.tag}] ${occupiedMessage}`,
      })
    }
    monitorState.value[stateKey] = next
    return
  }

  if (previous && previous.startsWith('occupied:')) {
    const recoveredMessage = t('portMonitor.udpRecovered')
    appendPortLog(status.tag, status.normalized || status.input, recoveredMessage)
    push.success({
      title: t('portMonitor.monitorTitle'),
      duration: 5000,
      message: `[${status.tag}] ${recoveredMessage}`,
    })
  }
  monitorState.value[stateKey] = 'free'
}

const runPortRangeMonitor = (): Promise<void> => {
  if (portRangeMonitorRequest) return portRangeMonitorRequest
  const request = (async () => {
		portRangeMonitorController?.abort()
		const controller = new AbortController()
		portRangeMonitorController = controller
    if (typeof document !== 'undefined' && document.visibilityState !== 'visible') return
    const targets = getMonitorTargets()
    if (targets.length === 0) {
      monitorState.value = {}
      return
    }

    const isCoreRunning = await refreshCoreRunning()
    if (controller.signal.aborted || portRangeMonitorController !== controller) return
    if (!isCoreRunning) {
      monitorState.value = {}
      return
    }

		const response = await checkPortOccupancy({
      udp_ranges: targets,
		}, { signal: controller.signal })
		if (controller.signal.aborted || portRangeMonitorController !== controller) return
    if (!response) return
    if (!response.supported) {
      showUnsupportedHint()
      return
    }

    const activeKeys = new Set<string>()
    for (const status of response.udp_ranges ?? []) {
      handleRangeStatus(status)
      activeKeys.add(getStateKey(status))
    }

    for (const key of Object.keys(monitorState.value)) {
      if (!activeKeys.has(key)) {
        delete monitorState.value[key]
      }
    }
	})()
  portRangeMonitorRequest = request
  void request.finally(() => {
    if (portRangeMonitorRequest === request) {
      portRangeMonitorRequest = null
    }
  })
  return request
}

const loadCoreStatus = async () => {
  await refreshCoreRunning()
}

const loadCoreUpdateMarker = (): Promise<void> => {
  if (coreUpdateMarkerRequest) return coreUpdateMarkerRequest
  const request = (async () => {
		coreUpdateMarkerController?.abort()
		const controller = new AbortController()
		coreUpdateMarkerController = controller
    if (!namespaceApi.showCoreControlsOnInbounds) {
      coreUpdateCount.value = 0
      return
    }
    if (typeof document !== 'undefined' && document.visibilityState !== 'visible') return
    try {
		const data = await HttpUtils.get(namespaceApi.core.updateInfoEndpoint, {}, { silentAuthCheck: true, signal: controller.signal })
		if (controller.signal.aborted || coreUpdateMarkerController !== controller) return
      if (data.success && data.obj) {
        const stable = data.obj.pendingStable ? 1 : 0
        const alpha = namespaceApi.core.supportsPrereleaseChannel && data.obj.pendingAlpha ? 1 : 0
        coreUpdateCount.value = stable + alpha
      } else {
        coreUpdateCount.value = 0
      }
    } catch {
      coreUpdateCount.value = 0
    }
  })()
  coreUpdateMarkerRequest = request
  void request.finally(() => {
    if (coreUpdateMarkerRequest === request) {
      coreUpdateMarkerRequest = null
    }
  })
  return request
}

const clearCoreDownloadTaskPolling = () => {
  if (coreDownloadTimerId.value !== null) {
    window.clearTimeout(coreDownloadTimerId.value)
    coreDownloadTimerId.value = null
  }
}

const normalizeCoreDownloadTaskStatus = (raw: any): CoreDownloadTaskStatus | null => {
  const id = String(raw?.id ?? '').trim()
  const state = String(raw?.state ?? '').trim().toLowerCase()
  if (!id || !state || state === 'idle') return null
  return {
    id,
    state,
    stage: String(raw?.stage ?? raw?.phase ?? '').trim(),
    canCancel: raw?.canCancel === true,
    stopRequested: raw?.stopRequested === true,
  }
}

const scheduleCoreDownloadTaskPolling = () => {
  clearCoreDownloadTaskPolling()
  if (!coreDownloadTaskActive.value || (typeof document !== 'undefined' && document.visibilityState !== 'visible')) return
  coreDownloadTimerId.value = window.setTimeout(() => {
    void loadCoreDownloadTask()
  }, 1500)
}

const loadCoreDownloadTask = async (): Promise<void> => {
  if (coreDownloadTaskRequest || !namespaceApi.showCoreControlsOnInbounds) return
  if (typeof document !== 'undefined' && document.visibilityState !== 'visible') return
  const request = (async () => {
		coreDownloadTaskController?.abort()
		const controller = new AbortController()
		coreDownloadTaskController = controller
    try {
		const data = await HttpUtils.get(namespaceApi.core.progressEndpoint, {}, { silentAuthCheck: true, signal: controller.signal })
		if (controller.signal.aborted || coreDownloadTaskController !== controller) return
      if (!data.success || !data.obj) {
        scheduleCoreDownloadTaskPolling()
        return
      }
      const task = normalizeCoreDownloadTaskStatus(data.obj)
      coreDownloadTask.value = task
      if (coreDownloadTaskActive.value) {
        scheduleCoreDownloadTaskPolling()
      } else {
        clearCoreDownloadTaskPolling()
      }
    } catch {
      scheduleCoreDownloadTaskPolling()
    }
  })()
  coreDownloadTaskRequest = request
  try {
    await request
  } finally {
    if (coreDownloadTaskRequest === request) {
      coreDownloadTaskRequest = null
    }
  }
}

const startCore = async () => {
  if (coreDownloadTaskActive.value || coreControlBusy.value || !coreReady.value) return
  startingCore.value = true
  try {
    await HttpUtils.post(namespaceApi.core.startEndpoint, {})
    scheduleCoreAction(() => {
      void loadCoreStatus()
      startingCore.value = false
    }, 1500)
  } catch {
    startingCore.value = false
  }
}

const stopCore = async () => {
  if (coreDownloadTaskActive.value || coreControlBusy.value) return
  stoppingCore.value = true
  try {
    await HttpUtils.post(namespaceApi.core.stopEndpoint, {})
    scheduleCoreAction(() => {
      void loadCoreStatus()
      stoppingCore.value = false
    }, 1500)
  } catch {
    stoppingCore.value = false
  }
}

const restartCore = async () => {
  if (coreDownloadTaskActive.value || coreControlBusy.value || !coreReady.value) return
  restartingCore.value = true
  try {
    await HttpUtils.post(namespaceApi.core.restartEndpoint, {})
    scheduleCoreAction(() => {
      void loadCoreStatus()
      restartingCore.value = false
    }, 2500)
  } catch {
    restartingCore.value = false
  }
}

const stopBackgroundPolling = () => {
  if (monitorIntervalId.value !== 0) {
    clearTimeout(monitorIntervalId.value)
    monitorIntervalId.value = 0
  }
  if (coreUpdateTimerId.value !== 0) {
    clearTimeout(coreUpdateTimerId.value)
    coreUpdateTimerId.value = 0
  }
  clearCoreDownloadTaskPolling()
	portRangeMonitorController?.abort()
	portRangeMonitorController = null
	portRangeMonitorRequest = null
	coreStatusController?.abort()
	coreStatusController = null
	coreStatusRequest = null
	coreUpdateMarkerController?.abort()
	coreUpdateMarkerController = null
	coreUpdateMarkerRequest = null
	coreDownloadTaskController?.abort()
	coreDownloadTaskController = null
	coreDownloadTaskRequest = null
}

const scheduleCoreUpdateMarkerPolling = (delay = 60000) => {
  if (!namespaceApi.showCoreControlsOnInbounds || typeof document !== 'undefined' && document.visibilityState !== 'visible') return
  if (coreUpdateTimerId.value !== 0) clearTimeout(coreUpdateTimerId.value)
  coreUpdateTimerId.value = window.setTimeout(async () => {
    coreUpdateTimerId.value = 0
    try {
      await loadCoreUpdateMarker()
    } catch {
      // Retry on the next marker pass.
    } finally {
      scheduleCoreUpdateMarkerPolling()
    }
  }, delay)
}

const schedulePortRangeMonitorPolling = (delay = 30000) => {
  if (typeof document !== 'undefined' && document.visibilityState !== 'visible') return
  if (getMonitorTargets().length === 0) return
  if (monitorIntervalId.value !== 0) clearTimeout(monitorIntervalId.value)
  monitorIntervalId.value = window.setTimeout(async () => {
    monitorIntervalId.value = 0
    try {
      await runPortRangeMonitor()
    } catch {
      // Retry on the next visible monitor pass.
    } finally {
      schedulePortRangeMonitorPolling()
    }
  }, delay)
}

const startBackgroundPolling = () => {
  if (typeof document !== 'undefined' && document.visibilityState !== 'visible') return
  if (namespaceApi.showCoreControlsOnInbounds) {
    void loadCoreStatus()
    void loadCoreUpdateMarker().finally(() => scheduleCoreUpdateMarkerPolling())
    void loadCoreDownloadTask()
  }
  void runPortRangeMonitor().finally(() => schedulePortRangeMonitorPolling())
}

const handleVisibilityChange = () => {
  if (document.visibilityState === 'visible') {
    startBackgroundPolling()
    return
  }
  stopBackgroundPolling()
}

onMounted(() => {
  void initialize()
  let rawLogs: string | null = null
  try {
    rawLogs = localStorage.getItem(PORT_LOG_STORAGE_KEY)
  } catch {
    rawLogs = null
  }
  if (rawLogs) {
    try {
      const parsed = JSON.parse(rawLogs)
      portLogs.value = normalizePortLogs(parsed)
    } catch {
      try {
        localStorage.removeItem(PORT_LOG_STORAGE_KEY)
      } catch {
        // Ignore unavailable storage.
      }
    }
  }


  if (typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', handleVisibilityChange)
  }
})

onUnmounted(() => {
  componentActive = false
  clearCoreActionTimers()
  stopBackgroundPolling()
  if (typeof document !== 'undefined') {
    document.removeEventListener('visibilitychange', handleVisibilityChange)
  }
})
</script>

<style scoped>
.core-download-hint__text {
  min-width: 0;
  overflow-wrap: anywhere;
}
</style>
