<template>
  <v-card subtitle="ShadowQUIC">
    <template v-if="isInbound">
      <v-card subtitle="jls-upstream">
        <v-row>
          <v-col cols="12">
            <v-text-field
              class="jls-upstream-field"
              :model-value="jlsAddr"
              label="addr"
              placeholder="www.example.com:443"
              hint="addr（域名/IP:端口，回落转发的上游域名、地址）；必填，支持 [IPv6]:端口"
              persistent-hint
              required
              @update:model-value="jlsAddr = $event"
            />
          </v-col>
          <v-col cols="12">
            <v-text-field
              class="jls-upstream-field"
              :model-value="jlsSni"
              label="sni"
              hint="sni（客户端tls握手sni，不填写时由addr决定）"
              persistent-hint
              @update:model-value="jlsSni = $event"
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
              :placeholder="option.placeholder"
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
              :placeholder="option.placeholder"
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
      <v-col cols="12" sm="6" md="4">
        <v-text-field
          hide-details
          min="0"
          type="number"
          label="up"
          placeholder="Mbps"
          :model-value="readField('root', 'up')"
          @update:model-value="setField('root', 'up', $event, 'number')"
        />
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-text-field
          hide-details
          min="0"
          type="number"
          label="down"
          placeholder="Mbps"
          :model-value="readField('root', 'down')"
          @update:model-value="setField('root', 'down', $event, 'number')"
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
          v-else-if="option.kind === 'list' && (isInbound || option.items)"
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
          :placeholder="option.placeholder"
          :suffix="option.unit"
          :label="option.label"
          :model-value="readField('root', option.key)"
          @update:model-value="setField('root', option.key, $event, option.kind)"
        />
        <v-text-field
          v-else
          hide-details
          :placeholder="option.placeholder"
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
import HttpUtils from '@/plugins/httputil'
import {
  createShadowQuicInboundDefaultOptions,
  normalizeShadowQuicBBRProfile,
  normalizeShadowQuicJlsUpstreamAddr,
  shadowQuicJlsSniFromAddr,
  shadowQuicBBRProfileItems,
} from '@/plugins/shadowQuic'

type ShadowQuicFieldKind = 'string' | 'number' | 'boolean' | 'list' | 'single-list' | 'select' | 'proxy'
type ShadowQuicOption = {
  key: string
  label: string
  kind: ShadowQuicFieldKind
  items?: string[]
  defaultValue?: string | number | boolean | string[]
  defaultEnabled?: boolean
  placeholder?: string
  unit?: string
}

const shadowQuicInboundDefaults = createShadowQuicInboundDefaultOptions()
const initializedShadowQuicInboundDefaults = new WeakSet<object>()
const initializedShadowQuicClientDefaults = new WeakSet<object>()
const shadowQuicCongestionControllerItems = ['cubic', 'new_reno', 'bbr']
const shadowQuicOutboundReceiveWindowDefaults = {
  recv_window_conn: 25000000,
  recv_window: 99000000,
} as const

function formatBytePlaceholder(value: number): string {
  return `${value}_${value / 1000000}MB`
}

const inboundRootOptionDefinitions: ShadowQuicOption[] = [
  { key: 'alpn', label: 'alpn', kind: 'list', items: ['h3', 'h2', 'http/1.1'], defaultValue: [...shadowQuicInboundDefaults.alpn], defaultEnabled: true },
  { key: 'quic_versions', label: 'quic-versions', kind: 'list', items: ['v1', 'v2'], defaultValue: [...shadowQuicInboundDefaults.quic_versions], defaultEnabled: true },
  { key: 'zero_rtt', label: 'zero-rtt', kind: 'boolean', defaultValue: shadowQuicInboundDefaults.zero_rtt, defaultEnabled: true },
  { key: 'congestion_controller', label: 'congestion-controller', kind: 'select', items: shadowQuicCongestionControllerItems, defaultValue: shadowQuicInboundDefaults.congestion_controller, defaultEnabled: true },
  { key: 'up', label: 'up', kind: 'number', defaultValue: shadowQuicInboundDefaults.up, defaultEnabled: true, placeholder: 'Mbps' },
  { key: 'down', label: 'down', kind: 'number', defaultValue: shadowQuicInboundDefaults.down, defaultEnabled: true, placeholder: 'Mbps' },
  { key: 'ignore_client_bandwidth', label: 'ignore-client-bandwidth', kind: 'boolean', defaultValue: false },
  { key: 'cwnd', label: 'cwnd', kind: 'number', defaultValue: shadowQuicInboundDefaults.cwnd, defaultEnabled: true },
  { key: 'bbr_profile', label: 'bbr-profile', kind: 'select', items: ['standard', 'conservative', 'aggressive'], defaultValue: 'standard' },
  { key: 'max_idle_time', label: 'max-idle-time', kind: 'number', defaultValue: shadowQuicInboundDefaults.max_idle_time, defaultEnabled: true, unit: 'ms' },
  { key: 'max_datagram_frame_size', label: 'max-datagram-frame-size', kind: 'number', defaultValue: shadowQuicInboundDefaults.max_datagram_frame_size, defaultEnabled: true },
  {
    key: 'recv_window_conn',
    label: 'recv-window-conn',
    kind: 'number',
    defaultValue: shadowQuicInboundDefaults.recv_window_conn,
    defaultEnabled: true,
    placeholder: formatBytePlaceholder(shadowQuicInboundDefaults.recv_window_conn),
  },
  {
    key: 'recv_window',
    label: 'recv-window',
    kind: 'number',
    defaultValue: shadowQuicInboundDefaults.recv_window,
    defaultEnabled: true,
    placeholder: formatBytePlaceholder(shadowQuicInboundDefaults.recv_window),
  },
  { key: 'disable_mtu_discovery', label: 'disable-mtu-discovery', kind: 'boolean', defaultValue: false },
]

const clientTemplateOptionDefinitions: ShadowQuicOption[] = [
  { key: 'udp_over_stream', label: 'udp-over-stream', kind: 'boolean', defaultValue: false },
  { key: 'keep_alive_interval', label: 'keep-alive-interval', kind: 'number', defaultValue: 10000, unit: 'ms' },
  { key: 'max_open_streams', label: 'max-open-streams', kind: 'number', defaultValue: 1024 },
  { key: 'up', label: 'up', kind: 'number', defaultValue: shadowQuicInboundDefaults.up, placeholder: 'Mbps' },
  { key: 'down', label: 'down', kind: 'number', defaultValue: shadowQuicInboundDefaults.down, placeholder: 'Mbps' },
]

const outboundRootOptionDefinitions: ShadowQuicOption[] = [
  { key: 'sni', label: 'SNI', kind: 'string' },
  { key: 'alpn', label: 'ALPN', kind: 'list' },
  { key: 'quic_versions', label: 'QUIC 版本', kind: 'list', items: ['v1', 'v2'] },
  { key: 'udp_over_stream', label: 'udp-over-stream', kind: 'boolean', defaultValue: false },
  { key: 'zero_rtt', label: 'zero-rtt', kind: 'boolean', defaultValue: false },
  { key: 'keep_alive_interval', label: 'keep-alive-interval（毫秒）', kind: 'number', defaultValue: 0 },
  { key: 'congestion_controller', label: 'congestion-controller', kind: 'select', items: shadowQuicCongestionControllerItems },
  { key: 'up', label: '上行带宽', kind: 'number', placeholder: 'Mbps' },
  { key: 'down', label: '下行带宽', kind: 'number', placeholder: 'Mbps' },
  { key: 'cwnd', label: 'cwnd', kind: 'number', defaultValue: 0 },
  { key: 'bbr_profile', label: 'bbr-profile', kind: 'select', items: [...shadowQuicBBRProfileItems], defaultValue: 'aggressive' },
  { key: 'max_datagram_frame_size', label: 'max-datagram-frame-size', kind: 'number', defaultValue: 0 },
  { key: 'max_open_streams', label: 'max-open-streams', kind: 'number', defaultValue: 0 },
  {
    key: 'recv_window_conn',
    label: 'recv-window-conn',
    kind: 'number',
    defaultValue: shadowQuicOutboundReceiveWindowDefaults.recv_window_conn,
    placeholder: formatBytePlaceholder(shadowQuicOutboundReceiveWindowDefaults.recv_window_conn),
  },
  {
    key: 'recv_window',
    label: 'recv-window',
    kind: 'number',
    defaultValue: shadowQuicOutboundReceiveWindowDefaults.recv_window,
    placeholder: formatBytePlaceholder(shadowQuicOutboundReceiveWindowDefaults.recv_window),
  },
  { key: 'disable_mtu_discovery', label: 'disable-mtu-discovery', kind: 'boolean', defaultValue: false },
]

const jlsOptionDefinitions: ShadowQuicOption[] = [
  { key: 'proxy', label: 'proxy', kind: 'proxy' },
  {
    key: 'rate_limit',
    label: 'rate-limit',
    kind: 'number',
    defaultValue: shadowQuicInboundDefaults.jls_upstream.rate_limit,
    defaultEnabled: true,
    placeholder: '204800即25KB/s',
  },
]

export default {
  props: {
    direction: { type: String, required: true },
    data: { type: Object, required: true },
    namespace: { type: String, default: 'default' },
    initializeClientDefaults: { type: Boolean, default: false },
  },
  data() {
    return {
      optionMenu: false,
      mihomoRouteTargets: <string[]>['DIRECT'],
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
        this.ensureJlsUpstream().addr = value
      },
    },
    jlsSni: {
      get(): string {
        const upstream = this.readJlsUpstream()
        if (upstream && Object.prototype.hasOwnProperty.call(upstream, 'sni')) {
          return typeof upstream.sni === 'string' ? upstream.sni : ''
        }
        return shadowQuicJlsSniFromAddr(upstream?.addr)
      },
      set(raw: unknown) {
        const value = this.normalizeString(raw)
        this.ensureJlsUpstream().sni = value
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
        this.ensureJlsUpstream().proxy = value
      },
    },
    mihomoProxyTargets(): string[] {
      const seen = new Set<string>()
      return this.mihomoRouteTargets
        .map((tag: unknown) => this.normalizeJlsProxyTarget(tag))
        .filter((tag: string) => tag !== '' && tag !== 'REJECT' && tag !== 'REJECT-DROP' && !seen.has(tag) && Boolean(seen.add(tag)))
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
    async loadMihomoProxyTargets() {
      if (!this.isInbound || this.$props.namespace !== 'mihomo') return
      const msg = await HttpUtils.get('api/mihomo-route-targets', {}, { silentErrorToast: true })
      if (!msg.success) return
      this.mihomoRouteTargets = Array.isArray(msg.obj?.routeTargets) ? msg.obj.routeTargets : ['DIRECT']
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
    emptyValueForKind(kind: ShadowQuicFieldKind): string | string[] {
      return kind === 'list' || kind === 'single-list' ? [] : ''
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
        // 只有这里的显式选项开关才会关闭控件；清空输入由 setField 保留空占位。
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
        target[key] = values
      } else if (kind === 'single-list') {
        const value = this.normalizeString(raw)
        target[key] = value === '' ? [] : [value]
      } else if (kind === 'number') {
        const text = typeof raw === 'string' ? raw.trim() : raw
        if (text === '' || text === null || text === undefined) {
          target[key] = this.emptyValueForKind(kind)
        } else {
          const numberValue = Number(text)
          if (!Number.isSafeInteger(numberValue) || numberValue < 0) {
            target[key] = this.emptyValueForKind(kind)
          } else {
            target[key] = numberValue
          }
        }
      } else {
        let value = this.normalizeString(raw)
        if (kind === 'select' && key === 'bbr_profile') {
          value = normalizeShadowQuicBBRProfile(value)
        }
        if (value === '') {
          target[key] = this.emptyValueForKind(kind)
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
        if (typeof value !== 'boolean') target[option.key] = false
        return
      }
      if (option.kind === 'list') {
        const normalized = this.normalizeList(value).filter((item) => !option.items || option.items.includes(item))
        target[option.key] = normalized
        return
      }
      if (option.kind === 'single-list') {
        const normalized = this.normalizeList(value).filter((item) => !option.items || option.items.includes(item))
        target[option.key] = normalized.length === 0 ? [] : [normalized[0]]
        return
      }
      if (option.kind === 'number') {
        if (value === null || value === undefined || (typeof value === 'string' && value.trim() === '')) {
          target[option.key] = this.emptyValueForKind(option.kind)
          return
        }
        const numberValue = Number(value)
        if (!Number.isSafeInteger(numberValue) || numberValue < 0) target[option.key] = this.emptyValueForKind(option.kind)
        else target[option.key] = numberValue
        return
      }
      if (option.kind === 'select') {
        let normalized = this.normalizeString(value)
        if (option.key === 'bbr_profile') {
          normalized = normalizeShadowQuicBBRProfile(value)
        }
        target[option.key] = normalized !== '' && (!option.items || option.items.includes(normalized))
          ? normalized
          : this.emptyValueForKind(option.kind)
        return
      }
      const normalized = this.normalizeString(value)
      target[option.key] = normalized
    },
    initializeDefaults() {
      if (
        this.isInbound &&
        Number(this.value.id ?? 0) === 0 &&
        !initializedShadowQuicInboundDefaults.has(this.value)
      ) {
		initializedShadowQuicInboundDefaults.add(this.value)
        for (const option of this.inboundRootOptionDefinitions) {
          if (!option.defaultEnabled || this.hasField('root', option.key)) continue
          const target = this.fieldTarget('root', true)
          if (!target) continue
          target[option.key] = Array.isArray(option.defaultValue)
            ? [...option.defaultValue]
            : option.defaultValue
        }
        for (const option of this.jlsOptionDefinitions) {
          if (!option.defaultEnabled || this.hasField('jls', option.key)) continue
          const target = this.fieldTarget('jls', true)
          if (!target) continue
          target[option.key] = Array.isArray(option.defaultValue)
            ? [...option.defaultValue]
            : option.defaultValue
        }
      }
      if (this.isClientTemplate && this.$props.initializeClientDefaults && !initializedShadowQuicClientDefaults.has(this.value)) {
        initializedShadowQuicClientDefaults.add(this.value)
        for (const option of this.clientTemplateOptionDefinitions) {
          if (option.defaultValue === undefined || this.hasField('root', option.key)) continue
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
        if (hyphenKey !== option.key) {
          if (!Object.prototype.hasOwnProperty.call(this.value, option.key) && Object.prototype.hasOwnProperty.call(this.value, hyphenKey)) {
            this.value[option.key] = this.value[hyphenKey]
          }
          delete this.value[hyphenKey]
        }
        this.normalizeOptionValue(this.value, option)
      }

      const upstream = this.readJlsUpstream()
      if (upstream) {
        for (const option of this.jlsOptionDefinitions) {
          const hyphenKey = option.key.replaceAll('_', '-')
          if (hyphenKey !== option.key) {
            if (!Object.prototype.hasOwnProperty.call(upstream, option.key) && Object.prototype.hasOwnProperty.call(upstream, hyphenKey)) {
              upstream[option.key] = upstream[hyphenKey]
            }
            delete upstream[hyphenKey]
          }
          this.normalizeOptionValue(upstream, option)
        }
        delete upstream.quic_version_probe
        delete upstream['quic-version-probe']
        if (Object.prototype.hasOwnProperty.call(upstream, 'proxy')) {
          const proxy = this.normalizeJlsProxyTarget(upstream.proxy)
          upstream.proxy = proxy
        }
        for (const key of ['addr', 'sni', 'proxy']) {
          if (!Object.prototype.hasOwnProperty.call(upstream, key)) continue
          const normalized = this.normalizeString(upstream[key])
          upstream[key] = normalized
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
    void this.loadMihomoProxyTargets()
  },
  watch: {
    data() {
      this.initializeDefaults()
      this.sanitize()
    },
    initializeClientDefaults() {
      this.initializeDefaults()
    },
  },
}
</script>

<style scoped>
.jls-upstream-field :deep(.v-messages__message) {
  overflow-wrap: anywhere;
  white-space: normal;
}
</style>
