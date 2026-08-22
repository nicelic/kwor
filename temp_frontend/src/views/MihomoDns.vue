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
  <v-row>
    <v-col cols="12" justify="center" align="center">
      <v-btn variant="outlined" color="warning" @click="saveConfig" :loading="loading" :disabled="loading || !initialized || (isPristine && !runtimeRefreshFailed) || hasDnsInputError">
        {{ $t('actions.save') }}
      </v-btn>
    </v-col>
  </v-row>
  <v-row>
    <v-col cols="12" class="v-card-subtitle">
      仅设置 mihomo 服务端自身使用的 DNS，不监听任何 DNS 端口。
    </v-col>
  </v-row>
  <v-row>
    <v-col cols="12" md="8">
      <v-combobox
        v-model="form.directNameserver"
        :items="mihomoDnsOptions"
        label="(direct-nameserver)"
        multiple
        chips
        closable-chips
        :disabled="loading"
        :error-messages="dnsListError(form.directNameserver)"
        :hide-details="dnsListError(form.directNameserver).length === 0"
      ></v-combobox>
    </v-col>
  </v-row>
  <v-row v-if="dnsTotalError(form)">
    <v-col cols="12" md="8">
      <v-alert type="error" density="compact" variant="tonal">
        {{ dnsTotalError(form) }}
      </v-alert>
    </v-col>
  </v-row>
  <v-row>
    <v-col cols="12" md="8">
      <v-combobox
        v-model="form.proxyServerNameserver"
        :items="mihomoDnsOptions"
        label="(proxy-server-nameserver)"
        multiple
        chips
        closable-chips
        :disabled="loading"
        :error-messages="dnsListError(form.proxyServerNameserver)"
        :hide-details="dnsListError(form.proxyServerNameserver).length === 0"
      ></v-combobox>
    </v-col>
  </v-row>
  <v-row>
    <v-col cols="12" md="8">
      <v-combobox
        v-model="form.nameserver"
        :items="mihomoDnsOptions"
        label="(nameserver)"
        multiple
        chips
        closable-chips
        :disabled="loading"
        :error-messages="dnsListError(form.nameserver)"
        :hide-details="dnsListError(form.nameserver).length === 0"
      ></v-combobox>
    </v-col>
  </v-row>
  <v-row>
    <v-col cols="12" md="8">
      <v-combobox
        v-model="form.defaultNameserver"
        :items="mihomoDnsOptions"
        label="(default-nameserver)"
        multiple
        chips
        closable-chips
        :disabled="loading"
        :error-messages="dnsListError(form.defaultNameserver)"
        :hide-details="dnsListError(form.defaultNameserver).length === 0"
      ></v-combobox>
    </v-col>
  </v-row>
  <v-row>
    <v-col cols="12" md="8">
      <v-combobox
        v-model="form.fallback"
        :items="mihomoDnsOptions"
        label="(fallback)"
        multiple
        chips
        closable-chips
        :disabled="loading"
        :error-messages="dnsListError(form.fallback)"
        :hide-details="dnsListError(form.fallback).length === 0"
      ></v-combobox>
    </v-col>
  </v-row>
  <v-row>
    <v-col cols="12" sm="4" md="3">
      <v-select
        v-model="form.globalIpv6"
        :items="optionalBoolOptions"
        label="IPv6 总开关"
        :disabled="loading"
        hide-details
      ></v-select>
    </v-col>
  </v-row>
  <v-row>
    <v-col cols="12" sm="4" md="3">
      <v-select
        v-model="form.dnsIpv6"
        label="DNS_IPv6"
        :items="optionalBoolOptions"
        :disabled="loading || !hasDnsServers"
        hide-details
      ></v-select>
    </v-col>
    <v-col cols="12" sm="5" md="3" v-if="form.dnsIpv6 === true && hasDnsServers">
      <v-text-field
        v-model="form.ipv6Timeout"
        label="ipv6-timeout"
        placeholder="ipv6-timeout"
        :disabled="loading"
        :error-messages="ipv6TimeoutError ? [ipv6TimeoutError] : []"
        :hide-details="ipv6TimeoutError === ''"
        @blur="normalizeIpv6TimeoutField"
      ></v-text-field>
    </v-col>
  </v-row>
  <v-row>
    <v-col cols="12" sm="4" md="3">
      <v-select
        v-model="form.preferH3"
        label="prefer-h3"
        :items="optionalBoolOptions"
        :disabled="loading || !hasDnsServers"
        hide-details
      ></v-select>
    </v-col>
  </v-row>
  <v-row>
    <v-col cols="12" sm="4" md="3">
      <v-select
        v-model="form.tcpConcurrent"
        label="TCP 并发（全局）"
        :items="optionalBoolOptions"
        :disabled="loading"
        hide-details
      ></v-select>
    </v-col>
  </v-row>
  </template>
</template>

<script lang="ts" setup>
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import MihomoData from '@/store/modules/mihomoData'
import { FindDiff } from '@/plugins/utils'
import { push } from 'notivue'

const mihomoDnsOptions = [
  'udp://127.0.0.1',
  'udp://8.8.8.8',
  'udp://8.8.4.4',
  'udp://[2001:4860:4860::8888]',
  'udp://[2001:4860:4860::8844]',
  'udp://1.1.1.1',
  'udp://1.0.0.1',
  'udp://[2606:4700:4700::1111]',
  'udp://[2606:4700:4700::1001]',
  'tls://8.8.8.8',
  'tls://8.8.4.4',
  'tls://[2001:4860:4860::8888]',
  'tls://[2001:4860:4860::8844]',
  'tls://1.1.1.1',
  'tls://1.0.0.1',
  'tls://[2606:4700:4700::1111]',
  'tls://[2606:4700:4700::1001]',
  'tls://[2001:4860:4860::8888]#disable-ipv6=true',
  'tls://[2001:4860:4860::8844]#disable-ipv6=true',
  'tls://1.1.1.1#disable-ipv6=true',
  'tls://1.0.0.1#disable-ipv6=true',
  'tls://[2606:4700:4700::1111]#disable-ipv6=true',
  'tls://[2606:4700:4700::1001]#disable-ipv6=true',
  'tls://[2001:4860:4860::8888]#disable-ipv4=true',
  'tls://[2001:4860:4860::8844]#disable-ipv4=true',
  'tls://1.1.1.1#disable-ipv4=true',
  'tls://1.0.0.1#disable-ipv4=true',
  'tls://[2606:4700:4700::1111]#disable-ipv4=true',
  'tls://[2606:4700:4700::1001]#disable-ipv4=true',
]

const maxMihomoDNSAddressesPerList = 8
const maxMihomoDNSAddressesTotal = 32
const maxMihomoDNSAddressBytes = 1024
const maxMihomoDNSAddressesBytes = 16 * 1024
const textEncoder = new TextEncoder()

interface MihomoDnsForm {
  globalIpv6: boolean | null
  directNameserver: string[]
  proxyServerNameserver: string[]
  nameserver: string[]
  defaultNameserver: string[]
  fallback: string[]
  dnsIpv6: boolean | null
  preferH3: boolean | null
  tcpConcurrent: boolean | null
  ipv6Timeout: string
}

const store = MihomoData()
const loading = ref(false)
const initializing = ref(true)
const loadFailed = ref(false)
const initialized = ref(false)
const form = ref<MihomoDnsForm>(createEmptyForm())
const oldForm = ref<MihomoDnsForm>(createEmptyForm())
const runtimeRefreshFailed = ref(false)
const revision = ref(0)
let componentActive = true
let refreshTimer: number | undefined
const optionalBoolOptions = [
  { title: '', value: null },
  { title: 'true', value: true },
  { title: 'false', value: false },
]

function createEmptyForm(): MihomoDnsForm {
  return {
    globalIpv6: null,
    directNameserver: [],
    proxyServerNameserver: [],
    nameserver: [],
    defaultNameserver: [],
    fallback: [],
    dnsIpv6: null,
    preferH3: null,
    tcpConcurrent: null,
    ipv6Timeout: '',
  }
}

function cloneForm(value: MihomoDnsForm): MihomoDnsForm {
  return JSON.parse(JSON.stringify(value))
}

function normalizeStringList(value: unknown): string[] {
  const source = Array.isArray(value)
    ? value
    : typeof value === 'string'
      ? [value]
      : []

  const result: string[] = []
  const seen = new Set<string>()
  for (const entry of source) {
    if (typeof entry !== 'string') continue
    const trimmed = entry.trim()
    if (trimmed.length === 0 || seen.has(trimmed)) continue
    seen.add(trimmed)
    result.push(trimmed)
  }
  return result
}

function dnsAddressError(value: string): string {
  if (textEncoder.encode(value).byteLength > maxMihomoDNSAddressBytes) {
    return `单个 DNS 地址不能超过 ${maxMihomoDNSAddressBytes} 字节`
  }
  if (/[\u0000-\u001F\u007F\s]/.test(value)) {
    return 'DNS 地址不能包含空白或控制字符'
  }
  if (value.includes('://')) {
    try {
      const parsed = new URL(value)
      if (!parsed.protocol || !parsed.host) return 'DNS URI 格式无效'
    } catch {
      return 'DNS URI 格式无效'
    }
  }
  return ''
}

function dnsListError(value: unknown): string[] {
  const normalized = normalizeStringList(value)
  if (normalized.length > maxMihomoDNSAddressesPerList) {
    return [`每个 DNS 列表最多允许 ${maxMihomoDNSAddressesPerList} 个地址`]
  }
  for (const item of normalized) {
    const error = dnsAddressError(item)
    if (error) return [error]
  }
  return []
}

function dnsTotalError(value: MihomoDnsForm): string {
  const lists = [
    value.directNameserver,
    value.proxyServerNameserver,
    value.nameserver,
    value.defaultNameserver,
    value.fallback,
  ].map(normalizeStringList)
  const count = lists.reduce((total, list) => total + list.length, 0)
  if (count > maxMihomoDNSAddressesTotal) {
    return `DNS 地址总数不能超过 ${maxMihomoDNSAddressesTotal}`
  }
  const bytes = lists.flat().reduce((total, item) => total + textEncoder.encode(item).byteLength, 0)
  if (bytes > maxMihomoDNSAddressesBytes) {
    return `DNS 地址总大小不能超过 ${maxMihomoDNSAddressesBytes} 字节`
  }
  return ''
}

function normalizeIpv6TimeoutInput(value: unknown): string {
  if (typeof value === 'number' && Number.isFinite(value) && Number.isInteger(value)) {
    const normalized = value
    return normalized > 0 ? String(normalized) : ''
  }

  if (typeof value !== 'string') {
    return ''
  }

  let normalized = value.trim().toLowerCase().replace(/\s+/g, '')
  if (normalized.endsWith('ms')) {
    normalized = normalized.slice(0, -2)
  }

  if (!/^\d+$/.test(normalized)) {
    return ''
  }

  const parsed = Number.parseInt(normalized, 10)
  return parsed > 0 ? String(parsed) : ''
}

function normalizeOptionalBoolean(value: unknown): boolean | null {
  if (value === null || value === undefined) {
    return null
  }
  if (typeof value === 'boolean') {
    return value
  }
  if (typeof value === 'number') {
    return value !== 0
  }
  if (typeof value === 'string') {
    const normalized = value.trim().toLowerCase()
    if (normalized.length === 0) {
      return null
    }
    if (normalized === 'true' || normalized === '1') {
      return true
    }
    if (normalized === 'false' || normalized === '0') {
      return false
    }
  }
  return null
}

function normalizeForm(value: MihomoDnsForm): MihomoDnsForm {
  return {
    globalIpv6: normalizeOptionalBoolean(value.globalIpv6),
    directNameserver: normalizeStringList(value.directNameserver),
    proxyServerNameserver: normalizeStringList(value.proxyServerNameserver),
    nameserver: normalizeStringList(value.nameserver),
    defaultNameserver: normalizeStringList(value.defaultNameserver),
    fallback: normalizeStringList(value.fallback),
    dnsIpv6: normalizeOptionalBoolean(value.dnsIpv6),
    preferH3: normalizeOptionalBoolean(value.preferH3),
    tcpConcurrent: normalizeOptionalBoolean(value.tcpConcurrent),
    ipv6Timeout: normalizeIpv6TimeoutInput(value.ipv6Timeout),
  }
}

function parseForm(config: any): MihomoDnsForm {
  const dns = config?.dns
  const emptyForm = createEmptyForm()
  emptyForm.globalIpv6 = normalizeOptionalBoolean(config?.['ipv6'])
  emptyForm.tcpConcurrent = normalizeOptionalBoolean(config?.['tcp-concurrent'])
  if (!dns || typeof dns !== 'object' || Array.isArray(dns)) {
    return emptyForm
  }

  return {
    globalIpv6: normalizeOptionalBoolean(config?.['ipv6']),
    tcpConcurrent: normalizeOptionalBoolean(config?.['tcp-concurrent']),
    directNameserver: normalizeStringList(dns['direct-nameserver']),
    proxyServerNameserver: normalizeStringList(dns['proxy-server-nameserver']),
    nameserver: normalizeStringList(dns['nameserver']),
    defaultNameserver: normalizeStringList(dns['default-nameserver']),
    fallback: normalizeStringList(dns['fallback']),
    dnsIpv6: normalizeOptionalBoolean(dns['ipv6']),
    preferH3: normalizeOptionalBoolean(dns['prefer-h3']),
    ipv6Timeout: normalizeIpv6TimeoutInput(dns['ipv6-timeout']),
  }
}

function buildDnsConfig(value: MihomoDnsForm): Record<string, unknown> | null {
  const normalized = normalizeForm(value)
  const dns: Record<string, unknown> = {}

  if (normalized.directNameserver.length > 0) {
    dns['direct-nameserver'] = normalized.directNameserver
  }
  if (normalized.proxyServerNameserver.length > 0) {
    dns['proxy-server-nameserver'] = normalized.proxyServerNameserver
  }
  if (normalized.nameserver.length > 0) {
    dns['nameserver'] = normalized.nameserver
  }
  if (normalized.defaultNameserver.length > 0) {
    dns['default-nameserver'] = normalized.defaultNameserver
  }
  if (normalized.fallback.length > 0) {
    dns['fallback'] = normalized.fallback
  }

  if (Object.keys(dns).length === 0) {
    return null
  }

  if (normalized.dnsIpv6 !== null) {
    dns['ipv6'] = normalized.dnsIpv6
  }
  if (normalized.preferH3 !== null) {
    dns['prefer-h3'] = normalized.preferH3
  }
  if (normalized.dnsIpv6 === true && normalized.ipv6Timeout.length > 0) {
    dns['ipv6-timeout'] = Number.parseInt(normalized.ipv6Timeout, 10)
  }
  return dns
}

function normalizeIpv6TimeoutField() {
  const raw = form.value.ipv6Timeout
  if (typeof raw !== 'string' || raw.trim() === '') {
    form.value.ipv6Timeout = ''
    return
  }
  const normalized = normalizeIpv6TimeoutInput(raw)
  if (normalized !== '') {
    form.value.ipv6Timeout = normalized
  }
}

const hasDnsServers = computed(() => [
  form.value.directNameserver,
  form.value.proxyServerNameserver,
  form.value.nameserver,
  form.value.defaultNameserver,
  form.value.fallback,
].some(list => normalizeStringList(list).length > 0))

const ipv6TimeoutError = computed(() => {
  if (!hasDnsServers.value || form.value.dnsIpv6 !== true) return ''
  const raw = form.value.ipv6Timeout
  if (typeof raw === 'string' && raw.trim() === '') return ''
  return normalizeIpv6TimeoutInput(raw) === ''
    ? 'ipv6-timeout 必须是大于 0 的整数毫秒'
    : ''
})

watch(hasDnsServers, enabled => {
  if (enabled) return
  form.value.dnsIpv6 = null
  form.value.preferH3 = null
  form.value.ipv6Timeout = ''
}, { immediate: true })

const isPristine = computed(() => {
  return FindDiff.deepCompare(normalizeForm(form.value), oldForm.value)
})

const hasDnsInputError = computed(() => {
  const lists = [
    form.value.directNameserver,
    form.value.proxyServerNameserver,
    form.value.nameserver,
    form.value.defaultNameserver,
    form.value.fallback,
  ]
  return lists.some(list => dnsListError(list).length > 0)
    || dnsTotalError(form.value) !== ''
    || ipv6TimeoutError.value !== ''
})

const initialize = async () => {
  initializing.value = true
  loadFailed.value = false
  initialized.value = false
  loading.value = true
  try {
    const config = await store.loadConfig()
    if (!componentActive) return
    if (config === null) {
      loadFailed.value = true
      return
    }
    const nextForm = parseForm(config)
    form.value = cloneForm(nextForm)
    oldForm.value = cloneForm(nextForm)
    revision.value = store.lastLoad
    runtimeRefreshFailed.value = false
    initialized.value = true
  } catch {
    if (componentActive) loadFailed.value = true
  } finally {
    if (componentActive) {
      initializing.value = false
      loading.value = false
    }
  }
}

const refreshWhenClean = async () => {
  if (!componentActive || (typeof document !== 'undefined' && document.visibilityState !== 'visible') || !initialized.value || loading.value || !isPristine.value) return
  const config = await store.loadConfig()
  if (!componentActive || config === null || !isPristine.value) return
  const nextForm = parseForm(config)
  form.value = cloneForm(nextForm)
  oldForm.value = cloneForm(nextForm)
  revision.value = store.lastLoad
  runtimeRefreshFailed.value = false
}

const handleVisibilityChange = () => {
  if (typeof document !== 'undefined' && document.visibilityState !== 'visible') {
    stopRefreshTimer()
    return
  }
  startRefreshTimer()
  void refreshWhenClean()
}

const stopRefreshTimer = () => {
  if (refreshTimer === undefined) return
  window.clearTimeout(refreshTimer)
  refreshTimer = undefined
}

const scheduleRefresh = (delay = 30_000) => {
  if ((typeof document !== 'undefined' && document.visibilityState !== 'visible') || !componentActive) return
  if (refreshTimer !== undefined) window.clearTimeout(refreshTimer)
  refreshTimer = window.setTimeout(async () => {
    refreshTimer = undefined
    try {
      await refreshWhenClean()
    } catch {
      // Keep the next refresh alive after a transient load failure.
    }
    scheduleRefresh()
  }, delay)
}

const startRefreshTimer = () => {
  if ((typeof document !== 'undefined' && document.visibilityState !== 'visible') || refreshTimer !== undefined) return
  scheduleRefresh()
}

onMounted(() => {
  void initialize()
  startRefreshTimer()
  if (typeof document !== 'undefined') document.addEventListener('visibilitychange', handleVisibilityChange)
})
onUnmounted(() => {
  componentActive = false
  stopRefreshTimer()
  if (typeof document !== 'undefined') document.removeEventListener('visibilitychange', handleVisibilityChange)
})

const saveConfig = async () => {
  if (!initialized.value || loading.value) return
  const normalizedForm = normalizeForm(form.value)
  if (hasDnsInputError.value) return

  loading.value = true
  try {
    const result = await store.saveDnsConfig({
      expectedRevision: revision.value,
      ipv6: normalizedForm.globalIpv6,
      tcpConcurrent: normalizedForm.tcpConcurrent,
      dns: buildDnsConfig(normalizedForm),
      retryRuntime: runtimeRefreshFailed.value,
    })
    if (result.saved) {
      const nextForm = parseForm(store.config)
      form.value = cloneForm(nextForm)
      oldForm.value = cloneForm(nextForm)
      revision.value = result.revision ?? store.lastLoad
      runtimeRefreshFailed.value = result.runtimeRefreshFailed
    } else if (result.conflict) {
      push.warning({
        title: 'DNS 配置已变更',
        message: '其他页面或窗口已更新 Mihomo 配置，请重新加载 DNS 页面后再保存。',
      })
    }
  } finally {
    loading.value = false
  }
}
</script>
