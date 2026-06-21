<template>
  <v-row>
    <v-col cols="12" justify="center" align="center">
      <v-btn variant="outlined" color="warning" @click="saveConfig" :loading="loading" :disabled="isPristine">
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
        hide-details
      ></v-combobox>
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
        hide-details
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
        hide-details
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
        hide-details
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
        hide-details
      ></v-combobox>
    </v-col>
  </v-row>
  <v-row>
    <v-col cols="12" sm="4" md="3">
      <v-select
        v-model="form.globalIpv6"
        :items="optionalBoolOptions"
        label="IPv6 总开关"
        hide-details
      ></v-select>
    </v-col>
  </v-row>
  <v-row>
    <v-col cols="12" sm="3" md="2">
      <v-switch
        v-model="form.dnsIpv6"
        color="primary"
        label="DNS_IPv6"
        hide-details
      ></v-switch>
    </v-col>
    <v-col cols="12" sm="5" md="3" v-if="form.dnsIpv6">
      <v-text-field
        v-model="form.ipv6Timeout"
        label="ipv6-timeout"
        placeholder="ipv6-timeout"
        hide-details
        @blur="normalizeIpv6TimeoutField"
      ></v-text-field>
    </v-col>
    <v-col cols="12" sm="3" md="2">
      <v-switch
        v-model="form.preferH3"
        color="primary"
        label="prefer-h3"
        hide-details
      ></v-switch>
    </v-col>
  </v-row>
  <v-row>
    <v-col cols="12" sm="3" md="2">
      <v-switch
        v-model="form.tcpConcurrent"
        color="primary"
        label="TCP并发"
        hide-details
      ></v-switch>
    </v-col>
  </v-row>
</template>

<script lang="ts" setup>
import { computed, ref, watch } from 'vue'
import MihomoData from '@/store/modules/mihomoData'
import { FindDiff } from '@/plugins/utils'

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

const defaultMihomoNameserver = [
  'tls://1.1.1.1#disable-ipv6=true',
  'tls://1.0.0.1#disable-ipv6=true',
]

interface MihomoDnsForm {
  globalIpv6: boolean | null
  directNameserver: string[]
  proxyServerNameserver: string[]
  nameserver: string[]
  defaultNameserver: string[]
  fallback: string[]
  dnsIpv6: boolean
  preferH3: boolean
  tcpConcurrent: boolean
  ipv6Timeout: string
}

const store = MihomoData()
const loading = ref(false)
const initialized = ref(false)
const form = ref<MihomoDnsForm>(createEmptyForm())
const oldForm = ref<MihomoDnsForm>(createEmptyForm())
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
    dnsIpv6: false,
    preferH3: false,
    tcpConcurrent: false,
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

function normalizeIpv6TimeoutInput(value: unknown): string {
  if (typeof value === 'number' && Number.isFinite(value)) {
    const normalized = Math.trunc(value)
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
    dnsIpv6: value.dnsIpv6 === true,
    preferH3: value.preferH3 === true,
    tcpConcurrent: value.tcpConcurrent === true,
    ipv6Timeout: normalizeIpv6TimeoutInput(value.ipv6Timeout),
  }
}

function parseForm(config: any): MihomoDnsForm {
  const dns = config?.dns
  const emptyForm = createEmptyForm()
  emptyForm.globalIpv6 = normalizeOptionalBoolean(config?.['ipv6'])
  emptyForm.tcpConcurrent = normalizeOptionalBoolean(config?.['tcp-concurrent']) === true
  if (!dns || typeof dns !== 'object' || Array.isArray(dns)) {
    return emptyForm
  }

  return {
    globalIpv6: normalizeOptionalBoolean(config?.['ipv6']),
    tcpConcurrent: normalizeOptionalBoolean(config?.['tcp-concurrent']) === true,
    directNameserver: normalizeStringList(dns['direct-nameserver']),
    proxyServerNameserver: normalizeStringList(dns['proxy-server-nameserver']),
    nameserver: normalizeStringList(dns['nameserver']),
    defaultNameserver: normalizeStringList(dns['default-nameserver']),
    fallback: normalizeStringList(dns['fallback']),
    dnsIpv6: dns['ipv6'] === true,
    preferH3: dns['prefer-h3'] === true,
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

  dns['ipv6'] = normalized.dnsIpv6 === true
  dns['prefer-h3'] = normalized.preferH3 === true
  if (normalized.dnsIpv6 && normalized.ipv6Timeout.length > 0) {
    dns['ipv6-timeout'] = Number.parseInt(normalized.ipv6Timeout, 10)
  }
  return dns
}

function normalizeIpv6TimeoutField() {
  form.value.ipv6Timeout = normalizeIpv6TimeoutInput(form.value.ipv6Timeout)
}

const isPristine = computed(() => {
  return FindDiff.deepCompare(normalizeForm(form.value), oldForm.value)
})

watch(
  () => store.config,
  (config) => {
    if (!initialized.value || isPristine.value) {
      const nextForm = parseForm(config)
      form.value = cloneForm(nextForm)
      oldForm.value = cloneForm(nextForm)
      initialized.value = true
    }
  },
  { deep: true, immediate: true },
)

const saveConfig = async () => {
  const payload = JSON.parse(JSON.stringify(store.config ?? {}))
  const normalizedForm = normalizeForm(form.value)
  const dnsConfig = buildDnsConfig(form.value)

  if (normalizedForm.globalIpv6 === null) {
    delete payload.ipv6
  } else {
    payload.ipv6 = normalizedForm.globalIpv6
  }

  payload['tcp-concurrent'] = normalizedForm.tcpConcurrent === true

  if (dnsConfig) {
    payload.dns = dnsConfig
  } else {
    delete payload.dns
  }

  loading.value = true
  const success = await store.save('config', 'set', payload)
  if (success) {
    const nextForm = parseForm(store.config)
    form.value = cloneForm(nextForm)
    oldForm.value = cloneForm(nextForm)
  }
  loading.value = false
}
</script>
