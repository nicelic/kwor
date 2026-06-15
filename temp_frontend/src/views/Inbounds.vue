<template>
  <SingboxCore
    v-if="namespaceApi.showCoreControlsOnInbounds"
    v-model="coreModal.visible"
    :visible="coreModal.visible"
    :namespace="props.namespace"
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
      <v-btn
        color="success"
        variant="flat"
        size="x-small"
        icon="mdi-play"
        :disabled="coreRunning"
        :loading="startingCore"
        @click="startCore"
      >
        <v-icon />
        <v-tooltip activator="parent" location="top" :text="t('coreManager.start')"></v-tooltip>
      </v-btn>
      <v-btn
        color="error"
        variant="flat"
        size="x-small"
        icon="mdi-stop"
        :disabled="!coreRunning"
        :loading="stoppingCore"
        @click="stopCore"
      >
        <v-icon />
        <v-tooltip activator="parent" location="top" :text="t('coreManager.stop')"></v-tooltip>
      </v-btn>
      <v-btn
        color="warning"
        variant="flat"
        size="x-small"
        icon="mdi-restart"
        :disabled="!coreRunning"
        :loading="restartingCore"
        @click="restartCore"
      >
        <v-icon />
        <v-tooltip activator="parent" location="top" :text="t('coreManager.restart')"></v-tooltip>
      </v-btn>
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
  </v-row>

  <v-row>
    <v-col cols="12" justify="center" align="center">
      <v-btn color="primary" @click="showModal(0)">{{ $t('actions.add') }}</v-btn>
      <v-btn color="primary" variant="tonal" class="ml-3" @click="openPortLog">{{ t('portLogs.open') }}</v-btn>
    </v-col>
  </v-row>

  <v-row>
    <v-col cols="12" sm="4" md="3" lg="2" v-for="(item, index) in <any[]>inbounds" :key="item.tag">
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
          <v-btn icon="mdi-file-edit" @click="showModal(item.id)">
            <v-icon />
            <v-tooltip activator="parent" location="top" :text="$t('actions.edit')"></v-tooltip>
          </v-btn>
          <v-btn icon="mdi-file-remove" style="margin-inline-start:0;" color="warning" @click="delOverlay[index] = true">
            <v-icon />
            <v-tooltip activator="parent" location="top" :text="$t('actions.del')"></v-tooltip>
          </v-btn>
          <v-overlay
            v-model="delOverlay[index]"
            contained
            class="align-center justify-center"
          >
            <v-card :title="$t('actions.del')" rounded="lg">
              <v-divider></v-divider>
              <v-card-text>{{ $t('confirm') }}</v-card-text>
              <v-card-actions>
                <v-btn color="error" variant="outlined" @click="delInbound(item.id)">{{ $t('yes') }}</v-btn>
                <v-btn color="success" variant="outlined" @click="delOverlay[index] = false">{{ $t('no') }}</v-btn>
              </v-card-actions>
            </v-card>
          </v-overlay>
          <v-btn icon="mdi-chart-line" @click="showStats(item.tag)" v-if="enableTraffic">
            <v-icon />
            <v-tooltip activator="parent" location="top" :text="$t('stats.graphTitle')"></v-tooltip>
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-col>
  </v-row>
</template>

<script lang="ts" setup>
import SingboxCore from '@/layouts/modals/SingboxCore.vue'
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

const props = withDefaults(defineProps<{ namespace?: UiNamespace }>(), {
  namespace: 'default',
})

const { t } = useI18n()
const store = getNamespaceStore(props.namespace)
const namespaceApi = getNamespaceApi(props.namespace)
const PORT_LOG_STORAGE_KEY = namespaceApi.portLogStorageKey

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

const delOverlay = ref(new Array<boolean>())
const coreModal = ref({
  visible: false,
})
const startingCore = ref(false)
const stoppingCore = ref(false)
const restartingCore = ref(false)
const coreUpdateCount = ref(0)
const coreUpdateTimerId = ref<ReturnType<typeof setInterval> | 0>(0)

const showModal = (id: number) => {
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

const delInbound = async (id: number) => {
  const index = inbounds.value.findIndex(i => i.id == id)
  const tag = inbounds.value[index].tag
  const success = await store.save('inbounds', 'del', tag)
  if (success) delOverlay.value[index] = false
}

const stats = ref({
  visible: false,
  resource: 'inbound',
  tag: '',
})

const showStats = (tag: string) => {
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
const clearPortLogs = () => {
  portLogs.value = []
  localStorage.removeItem(PORT_LOG_STORAGE_KEY)
}

const monitorState = ref(<Record<string, string>>{})
const monitorIntervalId = ref(<ReturnType<typeof setInterval> | 0>0)
const portCheckUnsupportedHinted = ref(false)
const coreRunning = ref(false)

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
  localStorage.setItem(PORT_LOG_STORAGE_KEY, JSON.stringify(portLogs.value))
}

const refreshCoreRunning = async (): Promise<boolean> => {
  try {
    const data = await HttpUtils.get(namespaceApi.core.statusEndpoint)
    if (data.success && data.obj) {
      coreRunning.value = data.obj.running === true
    }
  } catch {
    // Keep last known state to avoid noisy monitor flapping.
  }
  return coreRunning.value
}

const getMonitorTargets = (): PortRangeCheckItem[] => {
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
  return targets
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

const runPortRangeMonitor = async () => {
  const targets = getMonitorTargets()
  if (targets.length === 0) {
    monitorState.value = {}
    return
  }

  const isCoreRunning = await refreshCoreRunning()
  if (!isCoreRunning) {
    monitorState.value = {}
    return
  }

  const response = await checkPortOccupancy({
    udp_ranges: targets,
  })
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
}

const loadCoreStatus = async () => {
  await refreshCoreRunning()
}

const loadCoreUpdateMarker = async () => {
  if (!namespaceApi.showCoreControlsOnInbounds) {
    coreUpdateCount.value = 0
    return
  }
  try {
    const data = await HttpUtils.get(namespaceApi.core.updateInfoEndpoint)
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
}

const startCore = async () => {
  startingCore.value = true
  try {
    await HttpUtils.post(namespaceApi.core.startEndpoint, {})
    setTimeout(() => {
      void loadCoreStatus()
      startingCore.value = false
    }, 1500)
  } catch {
    startingCore.value = false
  }
}

const stopCore = async () => {
  stoppingCore.value = true
  try {
    await HttpUtils.post(namespaceApi.core.stopEndpoint, {})
    setTimeout(() => {
      void loadCoreStatus()
      stoppingCore.value = false
    }, 1500)
  } catch {
    stoppingCore.value = false
  }
}

const restartCore = async () => {
  restartingCore.value = true
  try {
    await HttpUtils.post(namespaceApi.core.restartEndpoint, {})
    setTimeout(() => {
      void loadCoreStatus()
      restartingCore.value = false
    }, 2500)
  } catch {
    restartingCore.value = false
  }
}

onMounted(() => {
  const rawLogs = localStorage.getItem(PORT_LOG_STORAGE_KEY)
  if (rawLogs) {
    try {
      const parsed = JSON.parse(rawLogs)
      if (Array.isArray(parsed)) {
        portLogs.value = parsed
      }
    } catch {
      localStorage.removeItem(PORT_LOG_STORAGE_KEY)
    }
  }

  if (namespaceApi.showCoreControlsOnInbounds) {
    void loadCoreStatus()
    void loadCoreUpdateMarker()
    coreUpdateTimerId.value = setInterval(() => {
      void loadCoreUpdateMarker()
    }, 60000)
  }

  void runPortRangeMonitor()
  monitorIntervalId.value = setInterval(() => {
    void runPortRangeMonitor()
  }, 30000)
})

onUnmounted(() => {
  if (monitorIntervalId.value !== 0) {
    clearInterval(monitorIntervalId.value)
    monitorIntervalId.value = 0
  }
  if (coreUpdateTimerId.value !== 0) {
    clearInterval(coreUpdateTimerId.value)
    coreUpdateTimerId.value = 0
  }
})
</script>
