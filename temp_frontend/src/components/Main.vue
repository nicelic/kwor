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
          <v-card class="rounded-lg" variant="outlined" :height="i === 'i-sbd' ? 244 : i.startsWith('i-') ? 228 : 210"
                  :title="menuItems.flatMap(cat => cat.value).find(m => m.value === i)?.title">
            <v-card-text style="padding: 0 16px;" align="center" justify="center">
              <Gauge v-if="i.charAt(0) === 'g'" :tilesData="tilesData" :type="i" />
              <History v-if="i.charAt(0) === 'h'" :tilesData="tilesData" :type="i" />
              <template v-if="i === 'i-sys'">
                <v-row class="home-info-grid">
                  <v-col cols="3" class="home-info-label">{{ $t('main.info.host') }}</v-col>
                  <v-col cols="9" class="home-info-value">
                    <div class="home-copy-row">
                      <span class="home-copy-value">{{ hostNameLabel }}</span>
                      <v-btn
                        v-if="hostNameLabel !== '-'"
                        icon
                        size="x-small"
                        variant="text"
                        class="home-copy-btn"
                        :aria-label="$t('copyToClipboard')"
                        @click.stop="void copyPlainText(hostNameLabel)"
                      >
                        <v-icon icon="mdi-content-copy" />
                        <v-tooltip activator="parent" location="top" :text="$t('copyToClipboard')" />
                      </v-btn>
                    </div>
                  </v-col>
                  <v-col cols="3" class="home-info-label">{{ $t('main.info.cpu') }}</v-col>
                  <v-col cols="9" class="home-info-value">
                    <v-menu
                      v-if="cpuTypeLabel !== '-'"
                      open-on-hover
                      open-on-click
                      location="end"
                      :close-on-content-click="false"
                      :open-delay="80"
                      :close-delay="220"
                    >
                      <template #activator="{ props }">
                        <v-chip v-bind="props" density="compact" variant="flat">
                          {{ cpuCountLabel }}
                        </v-chip>
                      </template>
                      <v-card class="home-detail-card" rounded="lg">
                        <div class="home-detail-row">
                          <span class="home-detail-text">{{ cpuTypeLabel }}</span>
                          <v-btn
                            icon
                            size="x-small"
                            variant="text"
                            class="home-copy-btn"
                            :aria-label="$t('copyToClipboard')"
                            @click.stop="void copyPlainText(cpuTypeLabel)"
                          >
                            <v-icon icon="mdi-content-copy" />
                            <v-tooltip activator="parent" location="top" :text="$t('copyToClipboard')" />
                          </v-btn>
                        </div>
                      </v-card>
                    </v-menu>
                    <v-chip v-else density="compact" variant="flat">
                      {{ cpuCountLabel }}
                    </v-chip>
                  </v-col>
                  <v-col cols="3" class="home-info-label">IP</v-col>
                  <v-col cols="9" class="home-info-value">
                    <div class="d-flex flex-wrap home-ip-chip-row">
                      <v-menu
                        v-if="ipv4List.length > 0"
                        open-on-hover
                        open-on-click
                        location="end"
                        :close-on-content-click="false"
                        :open-delay="80"
                        :close-delay="220"
                      >
                        <template #activator="{ props }">
                          <v-chip v-bind="props" density="compact" color="primary" variant="flat">
                            IPv4
                          </v-chip>
                        </template>
                        <v-card class="home-detail-card" rounded="lg">
                          <div
                            v-for="(ip, index) in ipv4List"
                            :key="`ipv4-${ip}-${index}`"
                            class="home-detail-row"
                          >
                            <span class="home-detail-text">{{ ip }}</span>
                            <v-btn
                              icon
                              size="x-small"
                              variant="text"
                              class="home-copy-btn"
                              :aria-label="$t('copyToClipboard')"
                              @click.stop="void copyIPAddress(ip)"
                            >
                              <v-icon icon="mdi-content-copy" />
                              <v-tooltip activator="parent" location="top" :text="$t('copyToClipboard')" />
                            </v-btn>
                          </div>
                        </v-card>
                      </v-menu>
                      <v-menu
                        v-if="ipv6List.length > 0"
                        open-on-hover
                        open-on-click
                        location="end"
                        :close-on-content-click="false"
                        :open-delay="80"
                        :close-delay="220"
                      >
                        <template #activator="{ props }">
                          <v-chip v-bind="props" density="compact" color="primary" variant="flat">
                            IPv6
                          </v-chip>
                        </template>
                        <v-card class="home-detail-card" rounded="lg">
                          <div
                            v-for="(ip, index) in ipv6List"
                            :key="`ipv6-${ip}-${index}`"
                            class="home-detail-row"
                          >
                            <span class="home-detail-text">{{ ip }}</span>
                            <v-btn
                              icon
                              size="x-small"
                              variant="text"
                              class="home-copy-btn"
                              :aria-label="$t('copyToClipboard')"
                              @click.stop="void copyIPAddress(ip)"
                            >
                              <v-icon icon="mdi-content-copy" />
                              <v-tooltip activator="parent" location="top" :text="$t('copyToClipboard')" />
                            </v-btn>
                          </div>
                        </v-card>
                      </v-menu>
                      <span v-if="!hasIPAddresses">-</span>
                    </div>
                  </v-col>
                  <v-col cols="3" class="home-info-label">kwor</v-col>
                  <v-col cols="9" class="home-info-value">
                    <v-chip density="compact" color="blue">
                      {{ appVersionLabel }}
                    </v-chip>
                  </v-col>
                  <v-col cols="3" class="home-info-label">{{ $t('main.info.uptime') }}</v-col>
                  <v-col cols="9" class="home-info-value">{{ HumanReadable.formatSecond(tilesData.uptime) }}</v-col>
                </v-row>
              </template>
              <template v-if="i === 'i-sbd'">
                <v-row class="home-info-grid">
                  <v-col cols="4" class="home-info-label">{{ $t('main.info.running') }}</v-col>
                  <v-col cols="8" class="home-info-value d-flex flex-column runtime-status-cell">
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
                  <v-col cols="4" class="home-info-label">{{ $t('main.info.memory') }}</v-col>
                  <v-col cols="8" class="home-info-value">
                    <v-chip class="runtime-metric-chip" density="compact" color="primary" variant="flat">
                      {{ formatTripleMemory(tilesData.sbd?.stats) }}
                    </v-chip>
                  </v-col>
                  <v-col cols="4" class="home-info-label">{{ $t('main.info.threads') }}</v-col>
                  <v-col cols="8" class="home-info-value">
                    <v-chip class="runtime-metric-chip" density="compact" color="primary" variant="flat">
                      {{ formatTripleThreads(tilesData.sbd?.stats) }}
                    </v-chip>
                  </v-col>
                  <v-col cols="4" class="home-info-label">{{ $t('main.info.uptime') }}</v-col>
                  <v-col cols="8" class="home-info-value">
                    <v-chip class="runtime-metric-chip" density="compact" color="primary" variant="flat">
                      {{ formatTripleUptime(tilesData.sbd?.stats) }}
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
import { push } from 'notivue'

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

const normalizeStringList = (value: unknown): string[] => {
  if (!Array.isArray(value)) return []
  return value
    .map(item => String(item ?? '').trim())
    .filter(Boolean)
}

const normalizeCopyText = (value: unknown): string => String(value ?? '').trim()

const normalizeIPAddressForCopy = (value: unknown): string => {
  const text = normalizeCopyText(value)
  if (!text) return ''
  const firstToken = text.split(/\s+/)[0] ?? ''
  const [address] = firstToken.split('/')
  return address.trim().replace(/^\[/, '').replace(/\]$/, '')
}

const fallbackCopyText = (text: string): boolean => {
  if (typeof document === 'undefined') return false
  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.setAttribute('readonly', 'readonly')
  textarea.style.position = 'fixed'
  textarea.style.opacity = '0'
  textarea.style.pointerEvents = 'none'
  document.body.appendChild(textarea)
  textarea.focus()
  textarea.select()
  textarea.setSelectionRange(0, textarea.value.length)
  let copied = false
  try {
    copied = document.execCommand('copy')
  } finally {
    document.body.removeChild(textarea)
  }
  return copied
}

const notifyCopyResult = (copied: boolean) => {
  if (copied) {
    push.success({
      message: `${i18n.global.t('success')}: ${i18n.global.t('copyToClipboard')}`,
      duration: 2200,
    })
    return
  }
  push.error({
    title: i18n.global.t('failed'),
    message: i18n.global.t('copyToClipboard'),
    duration: 3000,
  })
}

const copyText = async (value: unknown) => {
  const text = normalizeCopyText(value)
  if (!text) {
    notifyCopyResult(false)
    return
  }

  let copied = false
  if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text)
      copied = true
    } catch {
      copied = false
    }
  }

  if (!copied) {
    copied = fallbackCopyText(text)
  }
  notifyCopyResult(copied)
}

const copyPlainText = async (value: unknown) => copyText(value)

const copyIPAddress = async (value: unknown) => copyText(normalizeIPAddressForCopy(value))

const appVersionLabel = computed(() => {
  const version = tilesData.value?.sys?.appVersion
  return typeof version === 'string' && version.trim() ? `v${version.trim()}` : '-'
})

const hostNameLabel = computed(() => {
  const hostName = String(tilesData.value?.sys?.hostName ?? '').trim()
  return hostName || '-'
})

const cpuTypeLabel = computed(() => {
  const cpuType = String(tilesData.value?.sys?.cpuType ?? '').trim()
  return cpuType || '-'
})

const toNumber = (value: unknown): number => {
  if (typeof value === 'number' && Number.isFinite(value)) return value
  if (typeof value === 'string') {
    const parsed = Number(value)
    if (Number.isFinite(parsed)) return parsed
  }
  return 0
}

const cpuCountLabel = computed(() => {
  const cpuCount = Math.floor(toNumber(tilesData.value?.sys?.cpuCount))
  return cpuCount > 0 ? `${cpuCount} ${i18n.global.t('main.info.core')}` : '-'
})

const ipv4List = computed(() => normalizeStringList(tilesData.value?.sys?.ipv4))

const ipv6List = computed(() => normalizeStringList(tilesData.value?.sys?.ipv6))

const hasIPAddresses = computed(() => ipv4List.value.length > 0 || ipv6List.value.length > 0)

const toMB = (bytes: unknown): number => {
  return Math.max(0, Math.round(toNumber(bytes) / (1024 * 1024)))
}

const toMBWithOneDecimal = (bytes: unknown): string => {
  const value = Math.max(0, toNumber(bytes) / (1024 * 1024)).toFixed(1)
  return value.endsWith('.0') ? value.slice(0, -2) : value
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

const readMetricValue = (stats: any, keys: string[]): number => {
  if (!stats || typeof stats !== 'object') return 0
  let fallbackValue = 0
  for (const key of keys) {
    if (!Object.prototype.hasOwnProperty.call(stats, key)) continue
    const value = toNumber(stats[key])
    if (value > 0) return value
    fallbackValue = value
  }
  return fallbackValue
}

const formatTripleMemory = (stats: any): string => {
  const appMemory = readMetricValue(stats, ['AppMemoryActual', 'AppMemoryRSS', 'AppMemory'])
  const singboxMemory = readMetricValue(stats, ['SingboxMemoryActual', 'CoreMemoryRSS', 'CoreMemory'])
  const mihomoMemory = readMetricValue(stats, ['MihomoMemoryActual', 'MihomoMemoryRSS', 'MihomoMemory'])
  return `${toMBWithOneDecimal(appMemory)}+${toMBWithOneDecimal(singboxMemory)}(s)+${toMBWithOneDecimal(mihomoMemory)}(m) MB`
}

const formatTripleThreads = (stats: any): string => {
  const appThreads = Math.floor(readMetricValue(stats, ['AppThreadsActual', 'AppThreads']))
  const singboxThreads = Math.floor(readMetricValue(stats, ['SingboxThreadsActual', 'CoreThreads']))
  const mihomoThreads = Math.floor(readMetricValue(stats, ['MihomoThreadsActual']))
  return `${appThreads}+${singboxThreads}(s)+${mihomoThreads}(m)`
}

const formatTripleUptime = (stats: any): string => {
  const appUptime = readMetricValue(stats, ['AppUptimeActual', 'AppUptime'])
  const singboxUptime = readMetricValue(stats, ['SingboxUptimeActual', 'CoreUptime'])
  const mihomoUptime = readMetricValue(stats, ['MihomoUptimeActual'])
  return `${formatRuntimeMin(appUptime)}+${formatRuntimeMin(singboxUptime)}(s)+${formatRuntimeMin(mihomoUptime)}(m)`
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

.home-info-label {
  display: flex;
  align-items: center;
  justify-content: center;
  text-align: center;
}

.home-info-value {
  min-width: 0;
  text-align: center;
}

.home-copy-row {
  display: flex;
  align-items: center;
  gap: 4px;
  min-width: 0;
  width: fit-content;
  max-width: 100%;
  margin-inline: auto;
}

.home-copy-value {
  flex: 1 1 auto;
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.home-copy-btn {
  flex: 0 0 auto;
}

.home-ip-chip-row {
  gap: 6px;
  justify-content: center;
}

.home-detail-card {
  padding: 8px 10px;
  min-width: 180px;
  max-width: min(440px, calc(100vw - 32px));
}

.home-detail-row {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.home-detail-row + .home-detail-row {
  margin-top: 4px;
  padding-top: 4px;
  border-top: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
}

.home-detail-text {
  flex: 1 1 auto;
  min-width: 0;
  text-align: left;
  direction: ltr;
  overflow-wrap: anywhere;
}

.runtime-status-cell {
  gap: 3px;
  align-items: center;
}

.runtime-status-row {
  gap: 3px;
  min-height: 22px;
  justify-content: center;
}

.runtime-metric-chip {
  max-width: 100%;
  height: auto;
  min-height: 24px;
  line-height: 1.2;
}

:deep(.runtime-metric-chip .v-chip__content) {
  white-space: normal;
  overflow-wrap: anywhere;
}
</style>
