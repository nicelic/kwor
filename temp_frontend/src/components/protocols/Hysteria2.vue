<template>
  <v-card subtitle="Hysteria2">
    <v-row v-if="direction == 'in' && !data.ignore_client_bandwidth">
      <v-col cols="12" sm="6" md="4">
        <v-text-field
        :label="$t('stats.upload')"
        hide-details
        type="number"
        :suffix="$t('stats.Mbps')"
        min="0"
        v-model.number="up_mbps">
        </v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-text-field
        :label="$t('stats.download')"
        hide-details
        type="number"
        :suffix="$t('stats.Mbps')"
        min="0"
        v-model.number="down_mbps">
        </v-text-field>
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="6" md="4" v-if="data.obfs != undefined">
      <v-text-field
        :label="$t('types.hy.obfs')"
        hide-details
        v-model="data.obfs.password">
        </v-text-field>
      </v-col>
    </v-row>
    <v-row v-if="direction == 'in' && isSingboxNamespace && optionBBRProfile">
      <v-col cols="12" sm="6" md="4">
        <v-select
          hide-details
          label="bbr_profile"
          :items="bbrProfileItems"
          v-model="bbrProfileValue">
        </v-select>
      </v-col>
    </v-row>
    <template v-if="direction == 'in'">
      <v-card subtitle="Hysteria2 Masquerade" v-if="data.masquerade != undefined">
        <template v-if="isSingboxInbound">
          <v-row v-if="optionMasq">
            <v-col cols="12" sm="8">
              <v-text-field
              label="HTTP3 server on auth fails"
              placeholder="file:///var/www | http://127.0.0.1:8080 | https://127.0.0.1:8443"
              v-model="masqueradeString"
              hide-details>
              </v-text-field>
            </v-col>
          </v-row>
          <template v-else-if="optionMasqType">
            <v-row>
              <v-col cols="12" sm="6" md="4">
                <v-select v-model="masqueradeType" hide-details :label="$t('type')" :items="availableMasqTypes"></v-select>
              </v-col>
              <v-col cols="12" sm="8" v-if="masqueradeType == 'file'">
                <v-text-field
                label="File server root directory"
                placeholder="/var/www"
                v-model="data.masquerade.directory"
                hide-details>
                </v-text-field>
              </v-col>
              <v-col cols="12" sm="6" md="4" v-if="masqueradeType == 'string'">
                <v-text-field
                label="HTTP Code"
                type="number"
                min="100"
                max="599"
                v-model.number="data.masquerade.status_code"
                hide-details>
                </v-text-field>
              </v-col>
            </v-row>
            <v-row v-if="masqueradeType == 'proxy'">
              <v-col cols="12" sm="6">
                <v-text-field
                label="Target URL"
                placeholder="http://example.com:8080"
                v-model="data.masquerade.url"
                hide-details>
                </v-text-field>
              </v-col>
              <v-col cols="12" sm="6" md="4">
                <v-switch
                label="Rewrite Host"
                v-model="data.masquerade.rewrite_host"
                color="primary"
                hide-details>
                </v-switch>
              </v-col>
            </v-row>
            <template v-if="masqueradeType == 'string'">
              <v-row>
                <v-col cols="12" sm="8">
                  <v-text-field
                  label="Content"
                  v-model="data.masquerade.content"
                  hide-details>
                  </v-text-field>
                </v-col>
              </v-row>
              <Headers :data="data.masquerade" />
            </template>
          </template>
        </template>
        <template v-else>
          <v-row>
            <v-col cols="12" sm="6" md="4">
              <v-select v-model="masqueradeType" hide-details :label="$t('type')" :items="availableMasqTypes"></v-select>
            </v-col>
            <v-col cols="12" sm="8" v-if="masqueradeType == ''">
              <v-text-field
              :label="isMihomoInbound ? 'Masquerade URL (file/http/https)' : 'HTTP3 server on auth fails'"
              placeholder="file:///var/www | http://127.0.0.1:8080 | https://127.0.0.1:8443"
              v-model="data.masquerade"
              hide-details>
              </v-text-field>
            </v-col>
            <v-col cols="12" sm="8" v-if="masqueradeType == 'file'">
              <v-text-field
              label="File server root directory"
              placeholder="/var/www"
              v-model="data.masquerade.directory"
              hide-details>
              </v-text-field>
            </v-col>
            <v-col cols="12" sm="6" md="4" v-if="masqueradeType == 'string'">
              <v-text-field
              label="HTTP Code"
              type="number"
              min="100"
              max="599"
              v-model.number="data.masquerade.status_code"
              hide-details>
              </v-text-field>
            </v-col>
          </v-row>
          <v-row v-if="masqueradeType == 'proxy'">
            <v-col cols="12" sm="6">
              <v-text-field
              :label="isMihomoInbound ? 'Target URL (http/https)' : 'Target URL'"
              placeholder="http://example.com:8080"
              v-model="data.masquerade.url"
              hide-details>
              </v-text-field>
            </v-col>
            <v-col cols="12" sm="6" md="4" v-if="!isMihomoInbound">
              <v-switch
              label="Rewrite Host"
              v-model="data.masquerade.rewrite_host"
              color="primary"
              hide-details>
              </v-switch>
            </v-col>
          </v-row>
          <template v-if="masqueradeType == 'string'">
            <v-row>
              <v-col cols="12" sm="8">
                <v-text-field
                label="Content"
                v-model="data.masquerade.content"
                hide-details>
                </v-text-field>
              </v-col>
            </v-row>
            <Headers :data="data.masquerade" />
          </template>
        </template>
      </v-card>
    </template>
    <template v-else>
      <v-row>
        <v-col cols="12" sm="6" md="4">
          <v-text-field
          :label="$t('types.pw')"
          hide-details
          v-model="data.password">
          </v-text-field>
        </v-col>
        <v-col cols="12" sm="6" md="4" v-if="!hideNetworkSelectorForOut">
          <Network :data="data" />
        </v-col>
        <v-col cols="12" sm="8" v-if="optionMPort && !hidePortHopEditorsForOut">
          <v-text-field
            :label="$t('rule.portRange') + ' ' + $t('commaSeparated')"
            v-model="server_ports">
          </v-text-field>
        </v-col>
        <v-col cols="12" sm="6" md="4" v-if="optionMPort && !hidePortHopEditorsForOut">
          <v-text-field
            :label="$t('ruleset.interval')"
            placeholder="30 | 30s | 30-60 | 30:60s"
            v-model="hopIntervalInputDraft"
            @blur="applyHopIntervalInputDraft"
            @keydown.enter.prevent="applyHopIntervalInputDraft">
          </v-text-field>
        </v-col>
      </v-row>
    </template>
    <!-- mihomo QUIC receive window options -->
    <template v-if="optionMihomo">
      <v-card subtitle="mihomo QUIC receive window" style="margin-top: 0.5rem;">
        <v-row>
          <v-col cols="12" sm="6">
            <v-text-field
              label="initial-stream-receive-window"
              hide-details
              type="number"
              min="0"
              v-model.number="mihomoInitialStreamRecvWindow">
            </v-text-field>
          </v-col>
          <v-col cols="12" sm="6">
            <v-text-field
              label="max-stream-receive-window"
              hide-details
              type="number"
              min="0"
              v-model.number="mihomoMaxStreamRecvWindow">
            </v-text-field>
          </v-col>
        </v-row>
        <v-row>
          <v-col cols="12" sm="6">
            <v-text-field
              label="initial-connection-receive-window"
              hide-details
              type="number"
              min="0"
              v-model.number="mihomoInitialConnRecvWindow">
            </v-text-field>
          </v-col>
          <v-col cols="12" sm="6">
            <v-text-field
              label="max-connection-receive-window"
              hide-details
              type="number"
              min="0"
              v-model.number="mihomoMaxConnRecvWindow">
            </v-text-field>
          </v-col>
        </v-row>
      </v-card>
    </template>

    <v-card-actions>
      <v-spacer></v-spacer>
      <v-menu v-model="menu" :close-on-content-click="false" location="start">
        <template v-slot:activator="{ props }">
          <v-btn v-bind="props" hide-details variant="tonal">{{ $t('types.hy.hy2Options') }}</v-btn>
        </template>
        <v-card>
          <v-list>
            <v-list-item>
              <v-switch v-model="optionMihomo" color="primary" label="mihomo" hide-details></v-switch>
            </v-list-item>
            <v-list-item v-if="showMihomoFastOpenOption">
              <v-switch v-model="optionMihomoFastOpen" color="primary" label="fast-open" hide-details></v-switch>
            </v-list-item>
            <v-list-item>
              <v-switch v-model="optionObfs" color="primary" :label="$t('types.hy.obfs')" hide-details></v-switch>
            </v-list-item>
            <v-list-item v-if="isSingboxNamespace">
              <v-switch v-model="optionBBRProfile" color="primary" label="bbr_profile" hide-details></v-switch>
            </v-list-item>
            <template v-if="direction == 'in'">
              <v-list-item>
                <v-switch v-model="optionMasq" color="primary" label="Masquerade" hide-details></v-switch>
              </v-list-item>
              <v-list-item v-if="isSingboxInbound">
                <v-switch v-model="optionMasqType" color="primary" label="Masquerade.type" hide-details></v-switch>
              </v-list-item>
              <v-list-item>
                <v-switch v-model="data.ignore_client_bandwidth" color="primary" :label="$t('types.hy.ignoreBw')" hide-details></v-switch>
              </v-list-item>
            </template>
            <template v-else>
              <v-list-item v-if="!hidePortHopEditorsForOut">
                <v-switch v-model="optionMPort" color="primary" :label="$t('rule.portRange')" hide-details></v-switch>
              </v-list-item>
            </template>
          </v-list>
        </v-card>
      </v-menu>
    </v-card-actions>
  </v-card>
</template>

<script lang="ts">
import Network from '@/components/Network.vue'
import Headers from '@/components/Headers.vue'
import { applyHopIntervalInput, formatHopIntervalInput, parseHopIntervalInput } from '@/plugins/hopInterval'
import { normalizePortRangeInput } from '@/plugins/portRange'

export default {
  props: ['direction', 'data', 'hidePortHopEditors', 'namespace'],
  data() {
    return {
      menu: false,
      hopIntervalInputDraft: '',
    }
  },
  methods: {
    normalizeOptionalMbpsValue(value: unknown): number | undefined {
      if (value === '' || value === null || value === undefined) return undefined
      const normalized = Number(value)
      if (!Number.isFinite(normalized)) return undefined
      if (normalized > 0) return Math.trunc(normalized)
      return 0
    },
    readOptionalMbpsValue(key: string): number | null {
      const value = this.$props.data?.[key]
      if (value === '' || value === null || value === undefined) return null
      const normalized = Number(value)
      return Number.isFinite(normalized) ? Math.trunc(normalized) : null
    },
    writeOptionalMbpsValue(key: string, value: unknown) {
      const normalized = this.normalizeOptionalMbpsValue(value)
      if (normalized === undefined) {
        delete this.$props.data[key]
        return
      }
      this.$props.data[key] = normalized
    },
    normalizeHy2BBRProfile(value: unknown): string {
      const raw = typeof value === 'string' ? value.trim().toLowerCase() : ''
      switch (raw) {
        case 'conservative':
        case 'standard':
        case 'aggressive':
          return raw
        default:
          return ''
      }
    },
    stripMihomoFileScheme(value: string) {
      const trimmed = value.trim()
      return /^file:\/\//i.test(trimmed) ? trimmed.replace(/^file:\/\//i, '') : trimmed
    },
    looksLikeMihomoProxyURL(value: unknown) {
      if (typeof value !== 'string') return false
      const trimmed = value.trim().toLowerCase()
      return trimmed.startsWith('http://') || trimmed.startsWith('https://')
    },
    looksLikeMihomoFilePath(value: unknown) {
      if (typeof value !== 'string') return false
      const trimmed = value.trim()
      if (trimmed === '') return false
      if (/^file:\/\//i.test(trimmed)) return true
      if (trimmed.startsWith('/') || trimmed.startsWith('./') || trimmed.startsWith('../')) return true
      return /^[a-zA-Z]:[\\/]/.test(trimmed)
    },
    extractMihomoMasqueradeDirectory() {
      const masquerade = this.$props.data.masquerade
      if (typeof masquerade === 'string') {
        return this.looksLikeMihomoFilePath(masquerade) ? this.stripMihomoFileScheme(masquerade) : ''
      }
      if (!masquerade || typeof masquerade !== 'object') return ''
      if (typeof masquerade.directory === 'string') {
        return this.stripMihomoFileScheme(masquerade.directory)
      }
      return ''
    },
    extractMihomoMasqueradeURL() {
      const masquerade = this.$props.data.masquerade
      if (typeof masquerade === 'string') {
        return this.looksLikeMihomoProxyURL(masquerade) ? masquerade.trim() : ''
      }
      if (!masquerade || typeof masquerade !== 'object') return ''
      return this.looksLikeMihomoProxyURL(masquerade.url) ? masquerade.url.trim() : ''
    },
    sanitizeMihomoMasquerade() {
      if (!this.isMihomoInbound) return
      const masquerade = this.$props.data.masquerade
      if (!masquerade || typeof masquerade !== 'object') return
      if (masquerade.type === 'string') {
        this.$props.data.masquerade = { type: 'file', directory: this.extractMihomoMasqueradeDirectory() }
        return
      }
      if (masquerade.type === 'file') {
        masquerade.directory = this.extractMihomoMasqueradeDirectory()
        delete masquerade.url
      } else if (masquerade.type === 'proxy') {
        masquerade.url = this.extractMihomoMasqueradeURL()
        delete masquerade.directory
      }
      delete masquerade.rewrite_host
    },
    removeUnsupportedMihomoHy2Network() {
      if (!this.hideNetworkSelectorForOut) return
      delete this.$props.data.network
    },
    removeUnsupportedMihomoFastOpen() {
      if (this.$props.direction === 'in' && this.$props.data.out_json) {
        delete this.$props.data.out_json.fast_open
      }
      delete this.$props.data.fast_open
    },
    removeUnsupportedMihomoHy2Fields() {
      if (this.$props.namespace !== 'mihomo') return
      delete this.$props.data.bbr_profile
    },
    syncHopIntervalInputDraft() {
      this.hopIntervalInputDraft = formatHopIntervalInput(this.$props.data.hop_interval, this.$props.data.hop_interval_max)
    },
    applyHopIntervalInputDraft() {
      if (this.$props.direction !== 'out') return
      const parsed = parseHopIntervalInput(this.hopIntervalInputDraft)
      if (!parsed) {
        this.syncHopIntervalInputDraft()
        return
      }
      applyHopIntervalInput(this.$props.data, this.hopIntervalInputDraft)
      this.hopIntervalInputDraft = formatHopIntervalInput(parsed.hopInterval, parsed.hopIntervalMax)
    },
    isMasqueradeObject(value: unknown): boolean {
      return typeof value === 'object' && value != null && !Array.isArray(value)
    },
    withFileScheme(path: string): string {
      const trimmed = path.trim()
      if (trimmed === '') return ''
      if (/^file:\/\//i.test(trimmed)) return trimmed
      if (/^[a-zA-Z]:[\\/]/.test(trimmed)) {
        return `file:///${trimmed.replace(/\\/g, '/')}`
      }
      if (trimmed.startsWith('/')) {
        return `file://${trimmed}`
      }
      return `file:///${trimmed.replace(/^\/+/, '')}`
    },
    extractSingboxMasqueradeString() {
      const masquerade = this.$props.data.masquerade
      if (typeof masquerade === 'string') return masquerade
      if (!this.isMasqueradeObject(masquerade)) return ''
      const type = String(masquerade.type ?? '').trim().toLowerCase()
      if (type === 'proxy') {
        return typeof masquerade.url === 'string' ? masquerade.url : ''
      }
      if (type === 'file') {
        return typeof masquerade.directory === 'string' ? this.withFileScheme(masquerade.directory) : ''
      }
      return ''
    },
    extractSingboxMasqueradeObject() {
      const masquerade = this.$props.data.masquerade
      if (this.isMasqueradeObject(masquerade)) {
        return masquerade
      }
      if (typeof masquerade === 'string') {
        const trimmed = masquerade.trim()
        if (trimmed.toLowerCase().startsWith('http://') || trimmed.toLowerCase().startsWith('https://')) {
          return { type: 'proxy', url: trimmed }
        }
        if (trimmed.toLowerCase().startsWith('file://')) {
          return { type: 'file', directory: trimmed.replace(/^file:\/\//i, '') }
        }
      }
      return { type: 'file', directory: '' }
    },
    sanitizeSingboxMasquerade() {
      if (!this.isSingboxInbound) return
      const masquerade = this.$props.data.masquerade
      if (masquerade == undefined) return
      if (typeof masquerade === 'string') return
      if (!this.isMasqueradeObject(masquerade)) {
        this.$props.data.masquerade = undefined
        return
      }
      const type = String(masquerade.type ?? '').trim().toLowerCase()
      if (!['file', 'proxy', 'string'].includes(type)) {
        masquerade.type = 'file'
      } else {
        masquerade.type = type
      }
      if (masquerade.type === 'file') {
        delete masquerade.url
        delete masquerade.rewrite_host
        delete masquerade.status_code
        delete masquerade.headers
        delete masquerade.content
        if (typeof masquerade.directory !== 'string') masquerade.directory = ''
      } else if (masquerade.type === 'proxy') {
        delete masquerade.directory
        delete masquerade.status_code
        delete masquerade.headers
        delete masquerade.content
      } else if (masquerade.type === 'string') {
        delete masquerade.directory
        delete masquerade.url
        delete masquerade.rewrite_host
      }
    }
  },
  computed: {
    down_mbps: {
      get() { return this.readOptionalMbpsValue('server_down_mbps') },
      set(v:number | null) { this.writeOptionalMbpsValue('server_down_mbps', v) }
    },
    up_mbps: {
      get() { return this.readOptionalMbpsValue('server_up_mbps') },
      set(v:number | null) { this.writeOptionalMbpsValue('server_up_mbps', v) }
    },
    server_ports: {
      get() { return this.$props.data.server_ports?.join(',') ?? '' },
      set(v:string) {
        const normalized = normalizePortRangeInput(v)
        this.$props.data.server_ports = normalized.length > 0 ? normalized : undefined
      }
    },
    masqueradeString: {
      get() {
        if (typeof this.$props.data.masquerade === 'string') {
          return this.$props.data.masquerade
        }
        return ''
      },
      set(v: string) {
        this.$props.data.masquerade = v
      }
    },
    masqueradeType: {
      get() {
        if (this.isSingboxInbound) {
          if (this.isMasqueradeObject(this.$props.data.masquerade)) {
            const value = String(this.$props.data.masquerade.type ?? '').trim().toLowerCase()
            return ['file', 'proxy', 'string'].includes(value) ? value : 'file'
          }
          return 'file'
        }
        if (this.isMasqueradeObject(this.$props.data.masquerade)) {
          return this.$props.data.masquerade.type ?? ''
        }
        if (!this.isMihomoInbound || typeof this.$props.data.masquerade !== 'string') {
          return ''
        }
        if (this.looksLikeMihomoFilePath(this.$props.data.masquerade)) {
          return 'file'
        }
        if (this.looksLikeMihomoProxyURL(this.$props.data.masquerade)) {
          return 'proxy'
        }
        return ''
      },
      set(v:string) {
        if (this.isSingboxInbound) {
          const masquerade = this.extractSingboxMasqueradeObject()
          masquerade.type = ['file', 'proxy', 'string'].includes(v) ? v : 'file'
          this.$props.data.masquerade = masquerade
          this.sanitizeSingboxMasquerade()
          return
        }
        if (this.isMihomoInbound) {
          if (v === 'file') {
            this.$props.data.masquerade = {
              type: 'file',
              directory: this.extractMihomoMasqueradeDirectory(),
            }
            return
          }
          if (v === 'proxy') {
            this.$props.data.masquerade = {
              type: 'proxy',
              url: this.extractMihomoMasqueradeURL(),
            }
            return
          }
        }
        if (v == '') {
          this.$props.data.masquerade = ''
        } else {
          this.$props.data.masquerade = { type: v }
        }
      }
    },
    availableMasqTypes() {
      if (this.isSingboxInbound) {
        return [
          { title: "file", value: "file" },
          { title: "proxy", value: "proxy" },
          { title: "string", value: "string" },
        ]
      }
      if (this.isMihomoInbound) {
        return [
          { title: 'file', value: 'file' },
          { title: 'http/https', value: 'proxy' },
        ]
      }
      return []
    },
    bbrProfileItems() {
      return [
        { title: 'conservative（保守）', value: 'conservative' },
        { title: 'standard（标准）', value: 'standard' },
        { title: 'aggressive（激进）', value: 'aggressive' },
      ]
    },
    bbrProfileValue: {
      get(): string {
        const value = this.normalizeHy2BBRProfile(this.$props.data.bbr_profile)
        return value !== '' ? value : 'standard'
      },
      set(v: string) {
        const normalized = this.normalizeHy2BBRProfile(v)
        this.$props.data.bbr_profile = normalized !== '' ? normalized : 'standard'
      }
    },
    optionBBRProfile: {
      get(): boolean { return this.normalizeHy2BBRProfile(this.$props.data.bbr_profile) !== '' },
      set(v: boolean) {
        if (!v) {
          delete this.$props.data.bbr_profile
          return
        }
        const normalized = this.normalizeHy2BBRProfile(this.$props.data.bbr_profile)
        this.$props.data.bbr_profile = normalized !== '' ? normalized : 'standard'
      }
    },
    optionObfs: {
      get(): boolean { return this.$props.data.obfs != undefined },
      set(v:boolean) { this.$props.data.obfs = v ? { type: "salamander", password: "" } : undefined }
    },
    optionMasq: {
      get(): boolean {
        if (this.isSingboxInbound) {
          return typeof this.$props.data.masquerade === 'string'
        }
        return this.$props.data.masquerade != undefined
      },
      set(v:boolean) {
        if (this.isSingboxInbound) {
          if (!v) {
            if (typeof this.$props.data.masquerade === 'string') {
              this.$props.data.masquerade = undefined
            }
            return
          }
          this.$props.data.masquerade = this.extractSingboxMasqueradeString()
          return
        }
        if (!v) {
          this.$props.data.masquerade = undefined
          return
        }
        this.$props.data.masquerade = this.isMihomoInbound ? { type: 'file', directory: '' } : ""
      }
    },
    optionMasqType: {
      get(): boolean {
        if (!this.isSingboxInbound) return false
        return this.isMasqueradeObject(this.$props.data.masquerade)
      },
      set(v: boolean) {
        if (!this.isSingboxInbound) return
        if (!v) {
          if (this.isMasqueradeObject(this.$props.data.masquerade)) {
            this.$props.data.masquerade = undefined
          }
          return
        }
        this.$props.data.masquerade = this.extractSingboxMasqueradeObject()
        this.sanitizeSingboxMasquerade()
      }
    },
    optionMPort: {
      get(): boolean { return this.$props.data.server_ports != undefined },
      set(v:boolean) { this.$props.data.server_ports = v ? [] : undefined }
    },
    isSingboxInbound(): boolean {
      return this.$props.direction === 'in' && this.$props.namespace !== 'mihomo'
    },
    isMihomoInbound(): boolean {
      return this.$props.direction === 'in' && this.$props.namespace === 'mihomo'
    },
    hideNetworkSelectorForOut(): boolean {
      return this.$props.direction === 'out' && this.$props.namespace === 'mihomo'
    },
    hidePortHopEditorsForOut(): boolean {
      return this.$props.direction === 'out' && this.$props.hidePortHopEditors === true
    },
    isSingboxNamespace(): boolean {
      return this.$props.namespace !== 'mihomo'
    },
    showMihomoFastOpenOption(): boolean {
      return this.$props.namespace === 'mihomo' || (this.$props.direction === 'in' && this.$props.namespace !== 'mihomo')
    },
    mihomoFastOpenStore(): any {
      if (this.$props.direction === 'in') {
        if (!this.$props.data.out_json) this.$props.data.out_json = {}
        return this.$props.data.out_json
      }
      return this.$props.data
    },
    optionMihomoFastOpen: {
      get(): boolean {
        if (!this.showMihomoFastOpenOption) return false
        return this.mihomoFastOpenStore.mihomo_fast_open === true
      },
      set(v:boolean) {
        if (this.showMihomoFastOpenOption) {
          this.mihomoFastOpenStore.mihomo_fast_open = v
        }
      }
    },
    // mihomo QUIC receive window options
    mihomoStore(): any {
      // Inbound stores values under data.out_json.mihomo_hy2.
      // Outbound (SubOutbound) stores values under data.mihomo_hy2.
      if (this.$props.direction === 'in') {
        if (!this.$props.data.out_json) this.$props.data.out_json = {}
        return this.$props.data.out_json
      }
      return this.$props.data
    },
    optionMihomo: {
      get(): boolean { return this.mihomoStore.mihomo_hy2 != undefined },
      set(v: boolean) {
        if (v) {
          this.mihomoStore.mihomo_hy2 = {
            initial_stream_receive_window: 25000000,
            max_stream_receive_window: 88000000,
            initial_connection_receive_window: 99000000,
            max_connection_receive_window: 166000000,
          }
        } else {
          delete this.mihomoStore.mihomo_hy2
        }
      }
    },
    mihomoInitialStreamRecvWindow: {
      get(): number { return this.mihomoStore.mihomo_hy2?.initial_stream_receive_window ?? 0 },
      set(v: number) { if (this.mihomoStore.mihomo_hy2) this.mihomoStore.mihomo_hy2.initial_stream_receive_window = v > 0 ? v : undefined }
    },
    mihomoMaxStreamRecvWindow: {
      get(): number { return this.mihomoStore.mihomo_hy2?.max_stream_receive_window ?? 0 },
      set(v: number) { if (this.mihomoStore.mihomo_hy2) this.mihomoStore.mihomo_hy2.max_stream_receive_window = v > 0 ? v : undefined }
    },
    mihomoInitialConnRecvWindow: {
      get(): number { return this.mihomoStore.mihomo_hy2?.initial_connection_receive_window ?? 0 },
      set(v: number) { if (this.mihomoStore.mihomo_hy2) this.mihomoStore.mihomo_hy2.initial_connection_receive_window = v > 0 ? v : undefined }
    },
    mihomoMaxConnRecvWindow: {
      get(): number { return this.mihomoStore.mihomo_hy2?.max_connection_receive_window ?? 0 },
      set(v: number) { if (this.mihomoStore.mihomo_hy2) this.mihomoStore.mihomo_hy2.max_connection_receive_window = v > 0 ? v : undefined }
    },
  },
  watch: {
    data: {
      handler() {
        this.sanitizeSingboxMasquerade()
        this.sanitizeMihomoMasquerade()
        this.removeUnsupportedMihomoHy2Network()
        this.removeUnsupportedMihomoFastOpen()
        this.removeUnsupportedMihomoHy2Fields()
        this.syncHopIntervalInputDraft()
      },
      immediate: true,
    },
  },
  components: { Network, Headers }
}
</script>


