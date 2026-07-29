<template>
  <v-card subtitle="ShadowQUIC">
    <template v-if="isInbound">
      <v-card subtitle="jls-upstream">
        <v-row>
          <v-col cols="12" sm="6" md="4">
            <v-text-field
              :model-value="jlsAddr"
              label="addr"
              placeholder="www.example.com:443"
              hint="required, supports [IPv6]:port"
              persistent-hint
              required
              @update:model-value="jlsAddr = $event"
            />
          </v-col>
          <v-col v-if="hasMissingJlsProxy" cols="12">
            <v-alert density="compact" type="warning" variant="tonal">
              The selected jls-upstream proxy no longer exists. Select a valid target or clear it before saving.
            </v-alert>
          </v-col>
          <v-col
            v-for="option in enabledJlsOptions"
            :key="`jls-${option.key}`"
            cols="12"
            sm="6"
            md="4"
          >
            <v-switch
              v-if="option.kind === 'boolean'"
              color="primary"
              hide-details
              :label="option.label"
              :model-value="readBoolean('jls', option.key)"
              @update:model-value="setField('jls', option.key, $event, option.kind)"
            />
            <v-text-field
              v-else-if="option.kind === 'number'"
              hide-details
              min="0"
              type="number"
              :suffix="option.unit"
              :label="option.label"
              :model-value="readField('jls', option.key)"
              @update:model-value="setField('jls', option.key, $event, option.kind)"
            />
            <v-select
              v-else-if="option.kind === 'proxy'"
              clearable
              hide-details
              :items="mihomoProxyTargets"
              :label="option.label"
              :model-value="jlsProxy"
              @update:model-value="jlsProxy = $event"
            />
            <v-text-field
              v-else
              hide-details
              :label="option.label"
              :model-value="readField('jls', option.key)"
              @update:model-value="setField('jls', option.key, $event, option.kind)"
            />
          </v-col>
        </v-row>
      </v-card>
    </template>

    <v-row v-if="isClientTemplate">
      <v-col cols="12" sm="6" md="4">
        <v-select
          hide-details
          label="udp-over-stream"
          :items="boolItems"
          :model-value="readBoolean('root', 'udp_over_stream')"
          @update:model-value="setField('root', 'udp_over_stream', $event, 'boolean')"
        />
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-text-field
          hide-details
          min="0"
          type="number"
          label="keep-alive-interval"
          suffix="ms"
          :model-value="readField('root', 'keep_alive_interval')"
          @update:model-value="setField('root', 'keep_alive_interval', $event, 'number')"
        />
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-text-field
          hide-details
          min="0"
          type="number"
          label="max-open-streams"
          :model-value="readField('root', 'max_open_streams')"
          @update:model-value="setField('root', 'max_open_streams', $event, 'number')"
        />
      </v-col>
    </v-row>

    <v-row v-else-if="!isInbound">
      <v-col cols="12" sm="6" md="4">
        <v-text-field v-model="value.username" hide-details label="用户名" />
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-text-field v-model="value.password" hide-details label="密码" />
      </v-col>
    </v-row>

    <v-row v-if="!isClientTemplate && enabledRootOptions.length > 0">
      <v-col
        v-for="option in enabledRootOptions"
        :key="`root-${option.key}`"
        cols="12"
        sm="6"
        md="4"
      >
        <v-switch
          v-if="option.kind === 'boolean'"
          color="primary"
          hide-details
          :label="option.label"
          :model-value="readBoolean('root', option.key)"
          @update:model-value="setField('root', option.key, $event, option.kind)"
        />
        <v-select
          v-else-if="option.kind === 'select'"
          clearable
          hide-details
          :items="option.items"
          :label="option.label"
          :model-value="readStringField('root', option.key)"
          @update:model-value="setField('root', option.key, $event, option.kind)"
        />
        <v-select
          v-else-if="option.kind === 'list' && isInbound"
          chips
          clearable
          closable-chips
          hide-details
          multiple
          :items="option.items"
          :label="option.label"
          :model-value="readList('root', option.key)"
          @update:model-value="setField('root', option.key, $event, option.kind)"
        />
        <v-combobox
          v-else-if="option.kind === 'list'"
          chips
          clearable
          closable-chips
          hide-details
          multiple
          :items="option.items"
          :label="option.label"
          :model-value="readList('root', option.key)"
          @update:model-value="setField('root', option.key, $event, option.kind)"
        />
        <v-select
          v-else-if="option.kind === 'single-list'"
          clearable
          hide-details
          :items="option.items"
          :label="option.label"
          :model-value="readSingleList('root', option.key, option.items)"
          @update:model-value="setField('root', option.key, $event, option.kind)"
        />
        <v-text-field
          v-else-if="option.kind === 'number'"
          hide-details
          min="0"
          type="number"
          :suffix="option.unit"
          :label="option.label"
          :model-value="readField('root', option.key)"
          @update:model-value="setField('root', option.key, $event, option.kind)"
        />
        <v-text-field
          v-else
          hide-details
          :label="option.label"
          :model-value="readField('root', option.key)"
          @update:model-value="setField('root', option.key, $event, option.kind)"
        />
      </v-col>
    </v-row>

    <v-card-actions v-if="!isClientTemplate" class="pt-0">
      <v-spacer />
      <v-menu v-model="optionMenu" :close-on-content-click="false" location="start">
        <template #activator="{ props }">
          <v-btn v-bind="props" variant="tonal">ShadowQUIC options</v-btn>
        </template>
        <v-card min-width="270">
          <v-list>
            <v-list-subheader v-if="isInbound">jls-upstream</v-list-subheader>
            <template v-if="isInbound">
              <v-list-item v-for="option in jlsOptionDefinitions" :key="`toggle-jls-${option.key}`">
                <v-switch
                  color="primary"
                  hide-details
                  :label="option.label"
                  :model-value="hasField('jls', option.key)"
                  @update:model-value="setOptionEnabled('jls', option, Boolean($event))"
                />
              </v-list-item>
            </template>
            <v-list-subheader>protocol</v-list-subheader>
            <v-list-item v-for="option in rootOptionDefinitions" :key="`toggle-root-${option.key}`">
              <v-switch
                color="primary"
                hide-details
                :label="option.label"
                :model-value="hasField('root', option.key)"
                @update:model-value="setOptionEnabled('root', option, Boolean($event))"
              />
            </v-list-item>
          </v-list>
        </v-card>
      </v-menu>
    </v-card-actions>
  </v-card>
</template>

<script lang="ts">
import { getNamespaceStore } from '@/store/uiNamespace'
import { normalizeShadowQuicBBRProfile, shadowQuicBBRProfileItems } from '@/plugins/shadowQuic'

type ShadowQuicFieldKind = 'string' | 'number' | 'boolean' | 'list' | 'single-list' | 'select' | 'proxy'
type ShadowQuicOption = {
  key: string
  label: string
  kind: ShadowQuicFieldKind
  items?: string[]
  defaultValue?: string | number | boolean | string[]
  defaultEnabled?: boolean
  unit?: string
}

const inboundRootOptionDefinitions: ShadowQuicOption[] = [
  { key: 'alpn', label: 'alpn', kind: 'list', items: ['h3', 'h2', 'http/1.1'], defaultValue: ['h3'], defaultEnabled: true },
  { key: 'quic_versions', label: 'quic-versions', kind: 'list', items: ['v1', 'v2'], defaultValue: ['v2'], defaultEnabled: true },
  { key: 'zero_rtt', label: 'zero-rtt', kind: 'boolean', defaultValue: true, defaultEnabled: true },
  { key: 'congestion_controller', label: 'congestion-controller', kind: 'select', items: ['cubic', 'new_reno', 'bbr'], defaultValue: 'bbr' },
  { key: 'up', label: 'up', kind: 'string' },
  { key: 'down', label: 'down', kind: 'string' },
  { key: 'ignore_client_bandwidth', label: 'ignore-client-bandwidth', kind: 'boolean', defaultValue: false },
  { key: 'cwnd', label: 'cwnd', kind: 'number', defaultValue: 32 },
  { key: 'bbr_profile', label: 'bbr-profile', kind: 'select', items: ['standard', 'conservative', 'aggressive'], defaultValue: 'standard' },
  { key: 'max_idle_time', label: 'max-idle-time', kind: 'number', defaultValue: 120000, unit: 'ms' },
  { key: 'max_datagram_frame_size', label: 'max-datagram-frame-size', kind: 'number', defaultValue: 1400, defaultEnabled: true },
  { key: 'recv_window_conn', label: 'recv-window-conn', kind: 'number', defaultValue: 0 },
  { key: 'recv_window', label: 'recv-window', kind: 'number', defaultValue: 0 },
  { key: 'disable_mtu_discovery', label: 'disable-mtu-discovery', kind: 'boolean', defaultValue: false },
]

const clientTemplateOptionDefinitions: ShadowQuicOption[] = [
  { key: 'udp_over_stream', label: 'udp-over-stream', kind: 'boolean', defaultValue: false },
  { key: 'keep_alive_interval', label: 'keep-alive-interval', kind: 'number', defaultValue: 10000, unit: 'ms' },
  { key: 'max_open_streams', label: 'max-open-streams', kind: 'number', defaultValue: 1024 },
]

const outboundRootOptionDefinitions: ShadowQuicOption[] = [
  { key: 'sni', label: 'SNI', kind: 'string' },
  { key: 'alpn', label: 'ALPN', kind: 'list' },
  { key: 'quic_versions', label: 'QUIC 版本', kind: 'list', items: ['v1', 'v2'] },
  { key: 'udp_over_stream', label: 'udp-over-stream', kind: 'boolean', defaultValue: false },
  { key: 'zero_rtt', label: 'zero-rtt', kind: 'boolean', defaultValue: false },
  { key: 'keep_alive_interval', label: 'keep-alive-interval（毫秒）', kind: 'number', defaultValue: 0 },
  { key: 'congestion_controller', label: 'congestion-controller', kind: 'string' },
  { key: 'up', label: '上行带宽', kind: 'string' },
  { key: 'down', label: '下行带宽', kind: 'string' },
  { key: 'cwnd', label: 'cwnd', kind: 'number', defaultValue: 0 },
  { key: 'bbr_profile', label: 'bbr-profile', kind: 'select', items: [...shadowQuicBBRProfileItems], defaultValue: 'aggressive' },
  { key: 'max_datagram_frame_size', label: 'max-datagram-frame-size', kind: 'number', defaultValue: 0 },
  { key: 'max_open_streams', label: 'max-open-streams', kind: 'number', defaultValue: 0 },
  { key: 'recv_window_conn', label: 'recv-window-conn', kind: 'number', defaultValue: 0 },
  { key: 'recv_window', label: 'recv-window', kind: 'number', defaultValue: 0 },
  { key: 'disable_mtu_discovery', label: 'disable-mtu-discovery', kind: 'boolean', defaultValue: false },
]

const jlsOptionDefinitions: ShadowQuicOption[] = [
  { key: 'sni', label: 'sni', kind: 'string' },
  { key: 'proxy', label: 'proxy', kind: 'proxy' },
  { key: 'rate_limit', label: 'rate-limit', kind: 'number', defaultValue: 0 },
]

export default {
  props: {
    direction: { type: String, required: true },
    data: { type: Object, required: true },
  },
  data() {
    return {
      optionMenu: false,
      boolItems: [
        { title: 'true', value: true },
        { title: 'false', value: false },
      ],
      inboundRootOptionDefinitions,
      clientTemplateOptionDefinitions,
      outboundRootOptionDefinitions,
      jlsOptionDefinitions,
    }
  },
  computed: {
    value(): Record<string, any> {
      return this.$props.data as Record<string, any>
    },
    isInbound(): boolean {
      return this.direction === 'in'
    },
    isClientTemplate(): boolean {
      return this.direction === 'out_json'
    },
    rootOptionDefinitions(): ShadowQuicOption[] {
      if (this.isInbound) return this.inboundRootOptionDefinitions
      if (this.isClientTemplate) return this.clientTemplateOptionDefinitions
      return this.outboundRootOptionDefinitions
    },
    enabledRootOptions(): ShadowQuicOption[] {
      return this.rootOptionDefinitions.filter((option) => this.hasField('root', option.key))
    },
    enabledJlsOptions(): ShadowQuicOption[] {
      return this.jlsOptionDefinitions.filter((option) => this.hasField('jls', option.key))
    },
    jlsAddr: {
      get(): string {
        const upstream = this.readJlsUpstream()
        return typeof upstream?.addr === 'string' ? upstream.addr : ''
      },
      set(raw: unknown) {
        const value = this.normalizeString(raw)
        if (value === '') {
          const upstream = this.readJlsUpstream()
          if (upstream) {
            delete upstream.addr
            this.cleanupJlsUpstream()
          }
          return
        }
        this.ensureJlsUpstream().addr = value
      },
    },
    jlsProxy: {
      get(): string | undefined {
        const upstream = this.readJlsUpstream()
        const value = this.normalizeJlsProxyTarget(upstream?.proxy)
        return value === '' ? undefined : value
      },
      set(raw: unknown) {
        const value = this.normalizeJlsProxyTarget(raw)
        if (value === '') {
          const upstream = this.readJlsUpstream()
          if (upstream) {
            delete upstream.proxy
            this.cleanupJlsUpstream()
          }
          return
        }
        this.ensureJlsUpstream().proxy = value
      },
    },
    mihomoProxyTargets(): string[] {
      const store = getNamespaceStore('mihomo') as any
      const tags = [
        'DIRECT',
        ...(store.outbounds?.map((outbound: any) => (
          this.normalizeJlsProxyTarget(outbound?.type === 'direct' ? 'DIRECT' : outbound?.tag)
        )) ?? []),
        ...(store.outboundgroups?.map((group: any) => group?.tag ?? group?.name) ?? []),
      ]
      const seen = new Set<string>()
      return tags
        .map((tag: unknown) => this.normalizeJlsProxyTarget(tag))
        .filter((tag: string) => tag !== '' && !seen.has(tag) && Boolean(seen.add(tag)))
    },
    hasMissingJlsProxy(): boolean {
      const proxy = this.jlsProxy
      return this.isInbound && typeof proxy === 'string' && proxy !== '' && !this.mihomoProxyTargets.includes(proxy)
    },
  },
  methods: {
    normalizeString(raw: unknown): string {
      return typeof raw === 'string' ? raw.trim() : ''
    },
    normalizeJlsProxyTarget(raw: unknown): string {
      const value = this.normalizeString(raw)
      return value.toLowerCase() === 'direct' ? 'DIRECT' : value
    },
    normalizeList(raw: unknown): string[] {
      const values = Array.isArray(raw)
        ? raw
        : typeof raw === 'string'
          ? raw.split(/[\n,]/)
          : []
      const seen = new Set<string>()
      return values
        .map((item) => this.normalizeString(item))
        .filter((item) => item !== '' && !seen.has(item) && Boolean(seen.add(item)))
    },
    readJlsUpstream(): Record<string, any> | undefined {
      const upstream = this.value.jls_upstream
      return upstream && typeof upstream === 'object' && !Array.isArray(upstream) ? upstream : undefined
    },
    ensureJlsUpstream(): Record<string, any> {
      const existing = this.readJlsUpstream()
      if (existing) return existing
      this.value.jls_upstream = {}
      return this.value.jls_upstream
    },
    cleanupJlsUpstream() {
      const upstream = this.readJlsUpstream()
      if (!upstream) return
      if (Object.keys(upstream).length === 0) {
        delete this.value.jls_upstream
      }
    },
    fieldTarget(scope: 'root' | 'jls', create: boolean = false): Record<string, any> | undefined {
      if (scope === 'root') return this.value
      return create ? this.ensureJlsUpstream() : this.readJlsUpstream()
    },
    hasField(scope: 'root' | 'jls', key: string): boolean {
      const target = this.fieldTarget(scope)
      return target != null && key in target
    },
    readField(scope: 'root' | 'jls', key: string): string | number | undefined {
      const target = this.fieldTarget(scope)
      const value = target?.[key]
      return typeof value === 'string' || typeof value === 'number' ? value : undefined
    },
    readStringField(scope: 'root' | 'jls', key: string): string | undefined {
      const value = this.readField(scope, key)
      return typeof value === 'string' ? value : undefined
    },
    readBoolean(scope: 'root' | 'jls', key: string): boolean {
      return this.fieldTarget(scope)?.[key] === true
    },
    readList(scope: 'root' | 'jls', key: string): string[] {
      return this.normalizeList(this.fieldTarget(scope)?.[key])
    },
    readSingleList(scope: 'root' | 'jls', key: string, items?: string[]): string | undefined {
      const values = this.readList(scope, key)
      return values.find((value) => !items || items.includes(value))
    },
    setOptionEnabled(scope: 'root' | 'jls', option: ShadowQuicOption, enabled: boolean) {
      const target = this.fieldTarget(scope, enabled)
      if (!target) return
      if (!enabled) {
        delete target[option.key]
        if (scope === 'jls') this.cleanupJlsUpstream()
        return
      }
      if (option.kind === 'list' || option.kind === 'single-list') {
        target[option.key] = Array.isArray(option.defaultValue)
          ? [...option.defaultValue]
          : []
      } else if (option.defaultValue !== undefined) {
        target[option.key] = option.defaultValue
      } else {
        target[option.key] = ''
      }
    },
    setField(scope: 'root' | 'jls', key: string, raw: unknown, kind: ShadowQuicFieldKind) {
      const target = this.fieldTarget(scope)
      if (!target) return
      if (kind === 'boolean') {
        target[key] = raw === true
        return
      }
      if (kind === 'list') {
        const values = this.normalizeList(raw)
        if (values.length === 0) {
          delete target[key]
        } else {
          target[key] = values
        }
      } else if (kind === 'single-list') {
        const value = this.normalizeString(raw)
        if (value === '') {
          delete target[key]
        } else {
          target[key] = [value]
        }
      } else if (kind === 'number') {
        const text = typeof raw === 'string' ? raw.trim() : raw
        if (text === '' || text === null || text === undefined) {
          delete target[key]
        } else {
          const numberValue = Number(text)
          if (!Number.isFinite(numberValue) || numberValue < 0) {
            delete target[key]
          } else {
            target[key] = Math.trunc(numberValue)
          }
        }
      } else {
        let value = this.normalizeString(raw)
        if (kind === 'select' && key === 'bbr_profile') {
          value = normalizeShadowQuicBBRProfile(value)
        }
        if (value === '') {
          delete target[key]
        } else {
          target[key] = value
        }
      }
      if (scope === 'jls') this.cleanupJlsUpstream()
    },
    normalizeOptionValue(target: Record<string, any>, option: ShadowQuicOption) {
      if (!Object.prototype.hasOwnProperty.call(target, option.key)) return
      const value = target[option.key]
      if (option.kind === 'boolean') {
        if (typeof value !== 'boolean') delete target[option.key]
        return
      }
      if (option.kind === 'list') {
        const normalized = this.normalizeList(value).filter((item) => !option.items || option.items.includes(item))
        if (normalized.length === 0) delete target[option.key]
        else target[option.key] = normalized
        return
      }
      if (option.kind === 'single-list') {
        const normalized = this.normalizeList(value).filter((item) => !option.items || option.items.includes(item))
        if (normalized.length === 0) delete target[option.key]
        else target[option.key] = [normalized[0]]
        return
      }
      if (option.kind === 'number') {
        const numberValue = Number(value)
        if (!Number.isFinite(numberValue) || numberValue < 0) delete target[option.key]
        else target[option.key] = Math.trunc(numberValue)
        return
      }
      if (option.kind === 'select') {
        let normalized = this.normalizeString(value)
        if (option.key === 'bbr_profile') {
          normalized = normalizeShadowQuicBBRProfile(value)
        }
        if (normalized === '' || (option.items && !option.items.includes(normalized))) {
          delete target[option.key]
        } else {
          target[option.key] = normalized
        }
        return
      }
      const normalized = this.normalizeString(value)
      if (normalized === '') delete target[option.key]
      else target[option.key] = normalized
    },
    initializeDefaults() {
      if (this.isInbound && Number(this.value.id ?? 0) === 0) {
        for (const option of this.inboundRootOptionDefinitions) {
          if (!option.defaultEnabled || this.hasField('root', option.key)) continue
          const target = this.fieldTarget('root', true)
          if (!target) continue
          target[option.key] = Array.isArray(option.defaultValue)
            ? [...option.defaultValue]
            : option.defaultValue
        }
      }
      if (this.isClientTemplate) {
        for (const option of this.clientTemplateOptionDefinitions) {
          if (this.hasField('root', option.key)) continue
          const target = this.fieldTarget('root', true)
          if (!target) continue
          target[option.key] = Array.isArray(option.defaultValue)
            ? [...option.defaultValue]
            : option.defaultValue
        }
      }
    },
    sanitize() {
      const legacyUpstream = this.value['jls-upstream']
      if (legacyUpstream && typeof legacyUpstream === 'object' && !Array.isArray(legacyUpstream)) {
        this.value.jls_upstream = { ...legacyUpstream, ...(this.readJlsUpstream() ?? {}) }
      }
      delete this.value['jls-upstream']
      delete this.value.quic_version_probe
      delete this.value['quic-version-probe']

      for (const option of this.rootOptionDefinitions) {
        const hyphenKey = option.key.replaceAll('_', '-')
        if (!Object.prototype.hasOwnProperty.call(this.value, option.key) && Object.prototype.hasOwnProperty.call(this.value, hyphenKey)) {
          this.value[option.key] = this.value[hyphenKey]
        }
        delete this.value[hyphenKey]
        this.normalizeOptionValue(this.value, option)
      }

      const upstream = this.readJlsUpstream()
      if (upstream) {
        for (const option of this.jlsOptionDefinitions) {
          const hyphenKey = option.key.replaceAll('_', '-')
          if (!Object.prototype.hasOwnProperty.call(upstream, option.key) && Object.prototype.hasOwnProperty.call(upstream, hyphenKey)) {
            upstream[option.key] = upstream[hyphenKey]
          }
          delete upstream[hyphenKey]
          this.normalizeOptionValue(upstream, option)
        }
        delete upstream.quic_version_probe
        delete upstream['quic-version-probe']
        if (Object.prototype.hasOwnProperty.call(upstream, 'proxy')) {
          const proxy = this.normalizeJlsProxyTarget(upstream.proxy)
          if (proxy === '') delete upstream.proxy
          else upstream.proxy = proxy
        }
        for (const key of ['addr', 'sni', 'proxy']) {
          if (!Object.prototype.hasOwnProperty.call(upstream, key)) continue
          const normalized = this.normalizeString(upstream[key])
          if (normalized === '') delete upstream[key]
          else upstream[key] = normalized
        }
        this.cleanupJlsUpstream()
      }

      delete this.value.tls
      if (this.isInbound) {
        this.value.tls_id = 0
        for (const key of ['routing_mark', 'routing-mark', 'rule', 'proxy', 'detour', 'tcp_fast_open', 'tcp_multi_path', 'udp_fragment', 'udp_timeout']) {
          delete this.value[key]
        }
      } else {
        for (const key of ['detour', 'bind_interface', 'routing_mark', 'routing-mark', 'tcp_fast_open', 'tcp_multi_path', 'udp_fragment', 'udp_timeout']) {
          delete this.value[key]
        }
      }
    },
  },
  mounted() {
    this.initializeDefaults()
    this.sanitize()
  },
  watch: {
    data() {
      this.initializeDefaults()
      this.sanitize()
    },
  },
}
</script>
