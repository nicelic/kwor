<template>
  <LogVue v-model="logModal.visible" :control="logModal" :visible="logModal.visible" />
  <Backup v-model="backupModal.visible" :control="backupModal" :visible="backupModal.visible" />
  <v-container class="fill-height" :loading="loading">
    <v-responsive :class="reloadItems.length > 0 ? 'fill-height text-center' : 'align-center'">
      <v-row class="d-flex align-center justify-center">
        <v-col cols="auto">
          <v-img src="@/assets/logo.svg" :width="reloadItems.length > 0 ? 100 : 200"></v-img>
        </v-col>
      </v-row>
      <v-row class="d-flex align-center justify-center">
        <v-col cols="auto">
          <v-dialog v-model="menu" :close-on-content-click="false" transition="scale-transition" max-width="800">
            <template v-slot:activator="{ props }">
              <v-btn v-bind="props" hide-details variant="tonal">{{ $t('main.tiles') }} <v-icon icon="mdi-star-plus" /></v-btn>
            </template>
            <v-card rounded="xl">
              <v-card-title>
                <v-row>
                  <v-col>
                    {{ $t('main.tiles') }}
                  </v-col>
                  <v-spacer></v-spacer>
                  <v-col cols="auto"><v-icon icon="mdi-close" @click="menu = false"></v-icon></v-col>
                </v-row>
              </v-card-title>
              <v-divider></v-divider>
              <v-row v-for="items in menuItems" :key="items.title" no-gutters>
                <v-col cols="12">
                  <v-card :subtitle="items.title" variant="flat">
                    <v-card-text>
                      <v-row no-gutters>
                        <v-col v-for="item in items.value" :key="item.value" cols="12" md="6" lg="3">
                          <v-switch
                            density="compact"
                            v-model="reloadItems"
                            :value="item.value"
                            color="primary"
                            :label="item.title"
                            hide-details
                          ></v-switch>
                        </v-col>
                      </v-row>
                    </v-card-text>
                  </v-card>
                </v-col>
              </v-row>
            </v-card>
          </v-dialog>
          <v-btn variant="tonal" hide-details style="margin-inline-start: 10px;" @click="backupModal.visible = true">{{ $t('main.backup.title') }} <v-icon icon="mdi-backup-restore" /></v-btn>
          <v-btn variant="tonal" hide-details style="margin-inline-start: 10px;" @click="logModal.visible = true">{{ $t('basic.log.title') }} <v-icon icon="mdi-list-box-outline" /></v-btn>
        </v-col>
      </v-row>
      <v-row>
        <v-col v-for="i in reloadItems" :key="i" cols="12" sm="6" md="3">
          <v-card class="rounded-lg" variant="outlined" :height="i.startsWith('i-') ? 228 : 210"
                  :title="menuItems.flatMap(cat => cat.value).find(m => m.value === i)?.title">
            <v-card-text style="padding: 0 16px;" align="center" justify="center">
              <Gauge v-if="i.charAt(0) === 'g'" :tilesData="tilesData" :type="i" />
              <History v-if="i.charAt(0) === 'h'" :tilesData="tilesData" :type="i" />
              <template v-if="i === 'i-sys'">
                <v-row class="home-info-grid">
                  <v-col cols="3">{{ $t('main.info.host') }}</v-col>
                  <v-col cols="9" style="text-wrap: nowrap; overflow: hidden">{{ tilesData.sys?.hostName }}</v-col>
                  <v-col cols="3">{{ $t('main.info.cpu') }}</v-col>
                  <v-col cols="9">
                    <v-chip density="compact" variant="flat">
                      <v-tooltip activator="parent" location="top" style="direction: ltr;">
                        {{ tilesData.sys?.cpuType }}
                      </v-tooltip>
                      {{ tilesData.sys?.cpuCount }} {{ $t('main.info.core') }}
                    </v-chip>
                  </v-col>
                  <v-col cols="3">IP</v-col>
                  <v-col cols="9">
                    <v-chip density="compact" color="primary" variant="flat" v-if="tilesData.sys?.ipv4?.length > 0">
                      <v-tooltip activator="parent" location="top" style="direction: ltr;">
                        <span v-html="tilesData.sys?.ipv4?.join('<br />')"></span>
                      </v-tooltip>
                      IPv4
                    </v-chip>
                    <v-chip density="compact" color="primary" variant="flat" v-if="tilesData.sys?.ipv6?.length > 0">
                      <v-tooltip activator="parent" location="top" style="direction: ltr;">
                        <span v-html="tilesData.sys?.ipv6?.join('<br />')"></span>
                      </v-tooltip>
                      IPv6
                    </v-chip>
                  </v-col>
                  <v-col cols="3">kwor</v-col>
                  <v-col cols="9">
                    <v-chip density="compact" color="blue">
                      {{ appVersionLabel }}
                    </v-chip>
                  </v-col>
                  <v-col cols="3">{{ $t('main.info.uptime') }}</v-col>
                  <v-col cols="9">{{ HumanReadable.formatSecond(tilesData.uptime) }}</v-col>
                </v-row>
              </template>
              <template v-if="i === 'i-sbd'">
                <v-row class="home-info-grid">
                  <v-col cols="4">{{ $t('main.info.running') }}</v-col>
                  <v-col cols="8" class="d-flex flex-column align-start runtime-status-cell">
                    <div class="d-flex align-center flex-wrap runtime-status-row">
                      <span class="text-caption">Sing-Box</span>
                      <v-chip density="compact" :color="singboxRunning ? 'success' : 'error'" variant="flat">
                        {{ $t(singboxRunning ? 'coreManager.running' : 'coreManager.stopped') }}
                      </v-chip>
                      <v-btn
                        v-if="!singboxRunning"
                        icon
                        size="x-small"
                        variant="text"
                        color="success"
                        :loading="singboxLoading"
                        @click="startSingboxCore"
                      >
                        <v-icon icon="mdi-play" />
                        <v-tooltip activator="parent" location="top" :text="$t('coreManager.start')" />
                      </v-btn>
                      <v-btn
                        v-if="singboxRunning"
                        icon
                        size="x-small"
                        variant="text"
                        color="error"
                        :loading="singboxLoading"
                        @click="stopSingboxCore"
                      >
                        <v-icon icon="mdi-stop" />
                        <v-tooltip activator="parent" location="top" :text="$t('coreManager.stop')" />
                      </v-btn>
                      <v-btn
                        v-if="singboxRunning"
                        icon
                        size="x-small"
                        variant="text"
                        color="warning"
                        :loading="singboxLoading"
                        @click="restartSingboxCore"
                      >
                        <v-icon icon="mdi-restart" />
                        <v-tooltip activator="parent" location="top" :text="$t('coreManager.restart')" />
                      </v-btn>
                    </div>
                    <div class="d-flex align-center flex-wrap runtime-status-row">
                      <span class="text-caption">Mihomo</span>
                      <v-chip density="compact" :color="mihomoRunning ? 'success' : 'error'" variant="flat">
                        {{ $t(mihomoRunning ? 'coreManager.running' : 'coreManager.stopped') }}
                      </v-chip>
                      <v-btn
                        v-if="!mihomoRunning"
                        icon
                        size="x-small"
                        variant="text"
                        color="success"
                        :loading="mihomoLoading"
                        @click="startMihomoCore"
                      >
                        <v-icon icon="mdi-play" />
                        <v-tooltip activator="parent" location="top" :text="$t('coreManager.start')" />
                      </v-btn>
                      <v-btn
                        v-if="mihomoRunning"
                        icon
                        size="x-small"
                        variant="text"
                        color="error"
                        :loading="mihomoLoading"
                        @click="stopMihomoCore"
                      >
                        <v-icon icon="mdi-stop" />
                        <v-tooltip activator="parent" location="top" :text="$t('coreManager.stop')" />
                      </v-btn>
                      <v-btn
                        v-if="mihomoRunning"
                        icon
                        size="x-small"
                        variant="text"
                        color="warning"
                        :loading="mihomoLoading"
                        @click="restartMihomoCore"
                      >
                        <v-icon icon="mdi-restart" />
                        <v-tooltip activator="parent" location="top" :text="$t('coreManager.restart')" />
                      </v-btn>
                    </div>
                  </v-col>
                  <v-col cols="4">{{ $t('main.info.memory') }}</v-col>
                  <v-col cols="8">
                    <v-chip density="compact" color="primary" variant="flat">
                      {{ formatDualMemory(tilesData.sbd?.stats) }}
                    </v-chip>
                  </v-col>
                  <v-col cols="4">{{ $t('main.info.threads') }}</v-col>
                  <v-col cols="8">
                    <v-chip density="compact" color="primary" variant="flat">
                      {{ formatDualThreads(tilesData.sbd?.stats) }}
                    </v-chip>
                  </v-col>
                  <v-col cols="4">{{ $t('main.info.uptime') }}</v-col>
                  <v-col cols="8">
                    <v-chip density="compact" color="primary" variant="flat">
                      {{ formatDualUptime(tilesData.sbd?.stats) }}
                    </v-chip>
                  </v-col>
                </v-row>
              </template>
            </v-card-text>
          </v-card>
        </v-col>
      </v-row>
    </v-responsive>
  </v-container>
</template>

<script lang="ts" setup>
import HttpUtils from '@/plugins/httputil'
import { HumanReadable } from '@/plugins/utils'
import Data from '@/store/modules/data'
import Gauge from '@/components/tiles/Gauge.vue'
import History from '@/components/tiles/History.vue'
import { computed, onBeforeUnmount, onMounted, ref, type Ref } from 'vue'
import { i18n } from '@/locales'
import LogVue from '@/layouts/modals/Logs.vue'
import Backup from '@/layouts/modals/Backup.vue'

const loading = ref(false)
const singboxRunning = ref(false)
const singboxLoading = ref(false)
const mihomoRunning = ref(false)
const mihomoLoading = ref(false)
const menu = ref(false)
const menuItems = [
  { title: i18n.global.t('main.gauges'), value: [
    { title: i18n.global.t('main.gauge.cpu'), value: 'g-cpu' },
    { title: i18n.global.t('main.gauge.mem'), value: 'g-mem' },
    { title: i18n.global.t('main.gauge.dsk'), value: 'g-dsk' },
    { title: i18n.global.t('main.gauge.swp'), value: 'g-swp' },
  ] },
  { title: i18n.global.t('main.charts'), value: [
    { title: i18n.global.t('main.chart.cpu'), value: 'h-cpu' },
    { title: i18n.global.t('main.chart.mem'), value: 'h-mem' },
    { title: i18n.global.t('main.chart.net'), value: 'h-net' },
    { title: i18n.global.t('main.chart.pnet'), value: 'hp-net' },
    { title: i18n.global.t('main.chart.dio'), value: 'h-dio' },
  ] },
  { title: i18n.global.t('main.infos'), value: [
    { title: i18n.global.t('main.info.sys'), value: 'i-sys' },
    { title: i18n.global.t('main.info.sbd'), value: 'i-sbd' },
  ] },
]

const tilesData = ref(<any>{})

const appVersionLabel = computed(() => {
  const version = tilesData.value?.sys?.appVersion
  return typeof version === 'string' && version.trim() ? `v${version.trim()}` : '-'
})

const toNumber = (value: unknown): number => {
  if (typeof value === 'number' && Number.isFinite(value)) return value
  if (typeof value === 'string') {
    const parsed = Number(value)
    if (Number.isFinite(parsed)) return parsed
  }
  return 0
}

const toMB = (bytes: unknown): number => {
  return Math.max(0, Math.round(toNumber(bytes) / (1024 * 1024)))
}

const toMBWithOneDecimal = (bytes: unknown): string => {
  return Math.max(0, toNumber(bytes) / (1024 * 1024)).toFixed(1)
}

const formatRuntimeMin = (seconds: unknown): string => {
  const totalSeconds = Math.max(0, Math.floor(toNumber(seconds)))
  if (totalSeconds <= 0) return '0min'

  const totalMinutes = Math.max(1, Math.floor(totalSeconds / 60))
  if (totalMinutes < 60) {
    return `${totalMinutes}min`
  }
  if (totalMinutes < 60 * 24) {
    return `${Math.floor(totalMinutes / 60)}h`
  }
  const day = Math.floor(totalMinutes / (60 * 24))
  const remainHour = Math.floor((totalMinutes % (60 * 24)) / 60)
  return remainHour > 0 ? `${day}d ${remainHour}h` : `${day}d`
}

const formatDualMemory = (stats: any): string => {
  const coreCombinedLegacy = stats?.CoreCombinedMemory ?? (toNumber(stats?.CoreMemory) + toNumber(stats?.MihomoMemory))
  const totalLegacy = stats?.TotalMemory ?? (toNumber(stats?.AppMemory) + coreCombinedLegacy)
  const coreCombinedRSS = stats?.CoreCombinedMemoryRSS ?? (toNumber(stats?.CoreMemoryRSS) + toNumber(stats?.MihomoMemoryRSS))
  const totalRSS = stats?.TotalMemoryRSS ?? (toNumber(stats?.AppMemoryRSS) + coreCombinedRSS)
  const legacy = `${toMB(totalLegacy)}+${toMB(coreCombinedLegacy)} MB`
  const vmRSS = `${toMBWithOneDecimal(totalRSS)}+${toMBWithOneDecimal(coreCombinedRSS)} MB`
  return `${legacy} (${vmRSS})`
}

const formatDualThreads = (stats: any): string => {
  return `${Math.floor(toNumber(stats?.AppThreads))}+${Math.floor(toNumber(stats?.CoreThreads))}`
}

const formatDualUptime = (stats: any): string => {
  return `${formatRuntimeMin(stats?.AppUptime)}+${formatRuntimeMin(stats?.CoreUptime)}`
}

const reloadItems = computed({
  get() { return Data().reloadItems },
  set(v: string[]) {
    if (Data().reloadItems.length === 0 && v.length > 0) startTimer()
    if (Data().reloadItems.length > 0 && v.length === 0) stopTimer()
    Data().reloadItems = v
    v.length > 0 ? localStorage.setItem('reloadItems', v.join(',')) : localStorage.removeItem('reloadItems')
  }
})

const reloadData = async () => {
  const request = [...new Set(reloadItems.value.map(r => r.split('-')[1]))]
  const data = await HttpUtils.get('api/status', { r: request.join(',') })
  if (data.success) {
    tilesData.value = data.obj
  }
  await loadCoreStatuses()
}

const loadSingboxCoreStatus = async () => {
  try {
    const data = await HttpUtils.get('api/core-status')
    singboxRunning.value = data.success && data.obj ? data.obj.running === true : false
  } catch {
    singboxRunning.value = false
  }
}

const loadMihomoCoreStatus = async () => {
  try {
    const data = await HttpUtils.get('api/mihomo-core-status')
    mihomoRunning.value = data.success && data.obj ? data.obj.running === true : false
  } catch {
    mihomoRunning.value = false
  }
}

const loadCoreStatuses = async () => {
  await Promise.allSettled([
    loadSingboxCoreStatus(),
    loadMihomoCoreStatus(),
  ])
}

const runCoreAction = async (endpoint: string, loadingState: Ref<boolean>, reloadDelayMs: number) => {
  loadingState.value = true
  try {
    await HttpUtils.post(endpoint, {})
  } finally {
    setTimeout(() => {
      void loadCoreStatuses()
      loadingState.value = false
    }, reloadDelayMs)
  }
}

const startSingboxCore = async () => runCoreAction('api/coreStart', singboxLoading, 1500)

const stopSingboxCore = async () => runCoreAction('api/coreStop', singboxLoading, 1500)

const restartSingboxCore = async () => runCoreAction('api/coreRestart', singboxLoading, 2500)

const startMihomoCore = async () => runCoreAction('api/mihomo-coreStart', mihomoLoading, 1500)

const stopMihomoCore = async () => runCoreAction('api/mihomo-coreStop', mihomoLoading, 1500)

const restartMihomoCore = async () => runCoreAction('api/mihomo-coreRestart', mihomoLoading, 2500)

let intervalId: ReturnType<typeof setInterval> | null = null

const startTimer = () => {
  intervalId = setInterval(() => {
    void reloadData()
  }, 2000)
}

const stopTimer = () => {
  if (intervalId) {
    clearInterval(intervalId)
    intervalId = null
  }
}

onMounted(() => {
  void loadCoreStatuses()
  if (Data().reloadItems.length !== 0) {
    void reloadData()
    startTimer()
  }
})

onBeforeUnmount(() => {
  stopTimer()
})

const logModal = ref({ visible: false })

const backupModal = ref({ visible: false })

const restartSingbox = async () => {
  loading.value = true
  await HttpUtils.post('api/restartSb', {})
  loading.value = false
}
</script>

<style scoped>
.home-info-grid {
  margin: 0;
  row-gap: 2px;
  line-height: 1.15;
}

.home-info-grid > .v-col {
  padding-top: 4px;
  padding-bottom: 4px;
}

.runtime-status-cell {
  gap: 3px;
}

.runtime-status-row {
  gap: 3px;
  min-height: 22px;
}
</style>
