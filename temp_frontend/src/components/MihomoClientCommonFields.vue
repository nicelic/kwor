<template>
  <v-card :subtitle="$t('mihomoCommon.title')" style="background-color: inherit;">
    <v-row>
      <v-col cols="12" sm="6" md="4" v-if="showUDPOption && optionUDP">
        <v-select
          hide-details
          label="UDP"
          :items="udpItems"
          v-model="udpValue">
        </v-select>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="showIPVersionOption && optionIPVersion">
        <v-select
          hide-details
          :label="$t('rule.ipVer')"
          :items="ipVersionItems"
          v-model="ipVersionValue">
        </v-select>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="showTFOOption && optionTFO">
        <v-select
          hide-details
          label="TFO"
          :items="boolItems"
          v-model="tfoValue">
        </v-select>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="showMPTCPOption && optionMPTCP">
        <v-select
          hide-details
          label="MPTCP"
          :items="boolItems"
          v-model="mptcpValue">
        </v-select>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="showRoutingMarkOption && optionRoutingMark">
        <v-text-field
          hide-details
          type="number"
          min="0"
          step="1"
          label="Routing Mark"
          v-model.number="routingMarkValue">
        </v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="showBBRProfileOption && optionBBRProfile">
        <v-select
          hide-details
          label="bbr-profile"
          :items="bbrProfileItems"
          v-model="bbrProfileValue">
        </v-select>
      </v-col>
    </v-row>
    <template v-if="showMuxOption && optionMux">
      <v-row>
        <v-col cols="12" sm="6" md="4">
          <v-select
            hide-details
            clearable
            :label="$t('protocol')"
            :items="muxProtocols"
            @click:clear="muxProtocol = undefined"
            v-model="muxProtocol">
          </v-select>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-text-field
            hide-details
            type="number"
            min="0"
            step="1"
            :label="$t('mux.maxConn')"
            v-model.number="muxMaxConnections">
          </v-text-field>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-text-field
            hide-details
            type="number"
            min="0"
            step="1"
            :label="$t('mux.minStr')"
            v-model.number="muxMinStreams">
          </v-text-field>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-text-field
            hide-details
            type="number"
            min="0"
            step="1"
            :label="$t('mux.maxStr')"
            v-model.number="muxMaxStreams">
          </v-text-field>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-select
            hide-details
            label="Statistic"
            :items="boolItems"
            v-model="muxStatisticValue">
          </v-select>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-select
            hide-details
            label="Only TCP"
            :items="boolItems"
            v-model="muxOnlyTCPValue">
          </v-select>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-switch
            color="primary"
            :label="$t('mux.padding')"
            v-model="muxPadding"
            hide-details>
          </v-switch>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-switch
            color="primary"
            :label="$t('mux.enableBrutal')"
            v-model="muxBrutalEnabled"
            hide-details>
          </v-switch>
        </v-col>
      </v-row>
      <v-row v-if="muxBrutalEnabled">
        <v-col cols="12" sm="6" md="4">
          <v-text-field
            hide-details
            type="number"
            min="0"
            step="1"
            :label="$t('stats.upload')"
            :suffix="$t('stats.Mbps')"
            v-model.number="muxBrutalUpMbps">
          </v-text-field>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-text-field
            hide-details
            type="number"
            min="0"
            step="1"
            :label="$t('stats.download')"
            :suffix="$t('stats.Mbps')"
            v-model.number="muxBrutalDownMbps">
          </v-text-field>
        </v-col>
      </v-row>
    </template>
    <v-card-actions class="pt-0">
      <v-spacer></v-spacer>
      <v-menu v-model="menu" :close-on-content-click="false" location="start">
        <template v-slot:activator="{ props }">
          <v-btn v-bind="props" hide-details variant="tonal">{{ $t('mihomoCommon.options') }}</v-btn>
        </template>
        <v-card>
          <v-list>
            <v-list-item v-if="showUDPOption">
              <v-switch v-model="optionUDP" color="primary" label="UDP" hide-details></v-switch>
            </v-list-item>
            <v-list-item v-if="showIPVersionOption">
              <v-switch v-model="optionIPVersion" color="primary" :label="$t('rule.ipVer')" hide-details></v-switch>
            </v-list-item>
            <v-list-item v-if="showTFOOption">
              <v-switch v-model="optionTFO" color="primary" label="TFO" hide-details></v-switch>
            </v-list-item>
            <v-list-item v-if="showMPTCPOption">
              <v-switch v-model="optionMPTCP" color="primary" label="MPTCP" hide-details></v-switch>
            </v-list-item>
            <v-list-item v-if="showRoutingMarkOption">
              <v-switch v-model="optionRoutingMark" color="primary" label="Routing Mark" hide-details></v-switch>
            </v-list-item>
            <v-list-item v-if="showMuxOption">
              <v-switch v-model="optionMux" color="primary" :label="$t('objects.multiplex')" hide-details></v-switch>
            </v-list-item>
            <v-list-item v-if="showBBRProfileOption">
              <v-switch v-model="optionBBRProfile" color="primary" label="bbr-profile" hide-details></v-switch>
            </v-list-item>
          </v-list>
        </v-card>
      </v-menu>
    </v-card-actions>
  </v-card>
</template>

<script lang="ts">
import { oMultiplex } from '@/types/multiplex'

type GenericData = Record<string, any>
type MuxProtocol = NonNullable<oMultiplex['protocol']>
type MihomoSMux = oMultiplex & {
  statistic?: boolean
  only_tcp?: boolean
}

const mihomoIPVersionItems = ['dual', 'ipv4', 'ipv6', 'ipv4-prefer', 'ipv6-prefer'] as const
type MihomoIPVersion = typeof mihomoIPVersionItems[number]

export default {
  props: {
    data: {
      type: Object,
      required: true,
    },
    protocol: {
      type: String,
      default: '',
    },
  },
  data() {
    return {
      menu: false,
      boolItems: [
        { title: 'true', value: true },
        { title: 'false', value: false },
      ],
      udpItems: [
        { title: '', value: null },
        { title: 'true', value: true },
        { title: 'false', value: false },
      ],
      ipVersionItems: mihomoIPVersionItems,
      muxProtocols: ['smux', 'yamux', 'h2mux'],
      bbrProfileItems: [
        { title: 'conservative（保守）', value: 'conservative' },
        { title: 'standard（标准）', value: 'standard' },
        { title: 'aggressive（激进）', value: 'aggressive' },
      ],
    }
  },
  methods: {
    isRecord(value: unknown): value is GenericData {
      return value != null && typeof value === 'object' && !Array.isArray(value)
    },
    isMuxProtocol(value: unknown): value is MuxProtocol {
      return value === 'smux' || value === 'yamux' || value === 'h2mux'
    },
    supportsMihomoBBRProfileProtocol(value: unknown): boolean {
      const protocol = typeof value === 'string' ? value.trim().toLowerCase() : ''
      return ['hysteria2', 'tuic', 'trusttunnel', 'masque'].includes(protocol)
    },
    normalizeMihomoBBRProfile(value: unknown): '' | 'conservative' | 'standard' | 'aggressive' {
      const profile = typeof value === 'string' ? value.trim().toLowerCase() : ''
      if (profile === 'conservative' || profile === 'standard' || profile === 'aggressive') {
        return profile
      }
      return ''
    },
    normalizeMihomoIPVersion(value: unknown): MihomoIPVersion | undefined {
      const normalized = typeof value === 'string' ? value.trim().toLowerCase() : ''
      return mihomoIPVersionItems.includes(normalized as MihomoIPVersion)
        ? normalized as MihomoIPVersion
        : undefined
    },
    normalizeRoutingMark(value: unknown): number | undefined {
      if (value === '' || value === null || value === undefined) return undefined
      const normalized = Number(value)
      if (!Number.isSafeInteger(normalized) || normalized < 0) return undefined
      return normalized
    },
    normalizeBoolean(value: unknown): boolean | undefined {
      return value === true || value === false ? value : undefined
    },
    sanitizeSMuxFields(data: GenericData) {
      const legacyMux = this.isRecord(data.mux) ? data.mux : undefined
      const mux = this.isRecord(data.smux) ? data.smux : legacyMux
      delete data.mux
      if (!mux || mux.enabled === false) {
        delete data.smux
        return
      }

      data.smux = mux
      if (mux.enabled !== undefined && this.normalizeBoolean(mux.enabled) === undefined) {
        delete mux.enabled
      }
      if (mux.protocol !== undefined && !this.isMuxProtocol(mux.protocol)) {
        delete mux.protocol
      }
      for (const key of ['max_connections', 'min_streams', 'max_streams']) {
        if (mux[key] === undefined) continue
        const normalized = this.normalizeRoutingMark(mux[key])
        if (normalized === undefined) delete mux[key]
        else mux[key] = normalized
      }

      for (const key of ['statistic', 'only_tcp', 'padding']) {
        if (mux[key] === undefined) continue
        const normalized = this.normalizeBoolean(mux[key])
        if (normalized === undefined) delete mux[key]
        else mux[key] = normalized
      }
      if (mux['only-tcp'] !== undefined) {
        if (mux.only_tcp === undefined) {
          const normalized = this.normalizeBoolean(mux['only-tcp'])
          if (normalized !== undefined) mux.only_tcp = normalized
        }
        delete mux['only-tcp']
      }

      if (!this.isRecord(mux.brutal)) {
        if (mux.brutal !== undefined) delete mux.brutal
      } else if (mux.brutal.enabled !== true) {
        delete mux.brutal
      } else {
        for (const key of ['up_mbps', 'down_mbps']) {
          if (mux.brutal[key] === undefined) continue
          const normalized = this.normalizeRoutingMark(mux.brutal[key])
          if (normalized === undefined) delete mux.brutal[key]
          else mux.brutal[key] = normalized
        }
      }
      if (Object.keys(mux).length === 0) {
        delete data.smux
      }
    },
    sanitizeCurrentData() {
      const data = this.$props.data as GenericData
      if (!this.isRecord(data)) return

      if (data.udp !== undefined && typeof data.udp !== 'boolean') {
        delete data.udp
      }
      if (data.ip_version !== undefined) {
        const ipVersion = this.normalizeMihomoIPVersion(data.ip_version)
        if (ipVersion === undefined) delete data.ip_version
        else data.ip_version = ipVersion
      }
      if (data.routing_mark !== undefined) {
        const routingMark = this.normalizeRoutingMark(data.routing_mark)
        if (routingMark === undefined) delete data.routing_mark
        else data.routing_mark = routingMark
      }
      for (const key of ['tcp_fast_open', 'tcp_multi_path']) {
        if (data[key] === undefined) continue
        const normalized = this.normalizeBoolean(data[key])
        if (normalized === undefined) delete data[key]
        else data[key] = normalized
      }
      if (this.showBBRProfileOption) {
        const bbrProfile = this.normalizeMihomoBBRProfile(data.bbr_profile)
        if (bbrProfile === '') delete data.bbr_profile
        else data.bbr_profile = bbrProfile
      } else {
        delete data.bbr_profile
      }
      delete data['bbr-profile']
      this.sanitizeSMuxFields(data)

      if (this.isShadowQUIC) {
        for (const key of ['tcp_fast_open', 'tcp_multi_path', 'smux', 'bbr_profile', 'bbr-profile']) {
          delete data[key]
        }
      }
    },
    ensureSMuxBooleanDefaults(mux: MihomoSMux) {
      if (typeof mux.statistic !== 'boolean') {
        mux.statistic = false
      }
      if (typeof mux.only_tcp !== 'boolean') {
        const legacyOnlyTCP = (mux as GenericData)['only-tcp']
        mux.only_tcp = legacyOnlyTCP === true
      }
    },
    ensureSMux(): MihomoSMux {
      if (!this.isRecord(this.$props.data.smux)) {
        this.$props.data.smux = { enabled: true }
      }
      if (this.$props.data.smux.enabled !== true) {
        this.$props.data.smux.enabled = true
      }
      const mux = this.$props.data.smux as MihomoSMux
      this.ensureSMuxBooleanDefaults(mux)
      return mux
    },
    hasActiveSMux(): boolean {
      const mux = this.$props.data.smux
      if (!this.isRecord(mux)) return false
      if (typeof mux.enabled === 'boolean') return mux.enabled
      return Object.keys(mux).length > 0
    },
    ensureSMuxBrutal(): GenericData {
      const mux = this.ensureSMux()
      if (!this.isRecord(mux.brutal)) {
        mux.brutal = { enabled: true, up_mbps: 100, down_mbps: 100 }
      }
      if (mux.brutal.enabled !== true) {
        mux.brutal.enabled = true
      }
      return mux.brutal as GenericData
    },
  },
  mounted() {
    this.sanitizeCurrentData()
    if (this.hasActiveSMux()) {
      this.ensureSMux()
    }
  },
  computed: {
    isShadowQUIC(): boolean {
      return typeof this.$props.protocol === 'string' && this.$props.protocol.trim().toLowerCase() === 'shadowquic'
    },
    showUDPOption(): boolean {
      return true
    },
    showIPVersionOption(): boolean {
      return true
    },
    showTFOOption(): boolean {
      return !this.isShadowQUIC
    },
    showMPTCPOption(): boolean {
      return !this.isShadowQUIC
    },
    showRoutingMarkOption(): boolean {
      return true
    },
    showMuxOption(): boolean {
      return !this.isShadowQUIC
    },
    optionUDP: {
      get(): boolean {
        return this.$props.data.udp === true || this.$props.data.udp === false
      },
      set(v: boolean) {
        if (v) {
          this.$props.data.udp = true
          return
        }
        delete this.$props.data.udp
      },
    },
    udpValue: {
      get(): boolean | null {
        const value = this.$props.data.udp
        return value === true || value === false ? value : null
      },
      set(v: boolean | null) {
        if (v === true || v === false) {
          this.$props.data.udp = v
          return
        }
        delete this.$props.data.udp
      },
    },
    optionIPVersion: {
      get(): boolean {
        return this.normalizeMihomoIPVersion(this.$props.data.ip_version) !== undefined
      },
      set(v: boolean) {
        if (v) {
          this.$props.data.ip_version = 'dual'
          return
        }
        delete this.$props.data.ip_version
      },
    },
    ipVersionValue: {
      get(): MihomoIPVersion {
        return this.normalizeMihomoIPVersion(this.$props.data.ip_version) ?? 'dual'
      },
      set(v: MihomoIPVersion | null) {
        this.$props.data.ip_version = this.normalizeMihomoIPVersion(v) ?? 'dual'
      },
    },
    optionTFO: {
      get(): boolean {
        return this.$props.data.tcp_fast_open !== undefined
      },
      set(v: boolean) {
        if (v) {
          this.$props.data.tcp_fast_open = false
          return
        }
        delete this.$props.data.tcp_fast_open
      },
    },
    tfoValue: {
      get(): boolean {
        return this.$props.data.tcp_fast_open === true
      },
      set(v: boolean) {
        this.$props.data.tcp_fast_open = v === true
      },
    },
    optionMPTCP: {
      get(): boolean {
        return this.$props.data.tcp_multi_path !== undefined
      },
      set(v: boolean) {
        if (v) {
          this.$props.data.tcp_multi_path = false
          return
        }
        delete this.$props.data.tcp_multi_path
      },
    },
    mptcpValue: {
      get(): boolean {
        return this.$props.data.tcp_multi_path === true
      },
      set(v: boolean) {
        this.$props.data.tcp_multi_path = v === true
      },
    },
    optionRoutingMark: {
      get(): boolean {
        return this.normalizeRoutingMark(this.$props.data.routing_mark) !== undefined
      },
      set(v: boolean) {
        if (v) {
          this.$props.data.routing_mark = 0
          return
        }
        delete this.$props.data.routing_mark
      },
    },
    routingMarkValue: {
      get(): number {
        return this.normalizeRoutingMark(this.$props.data.routing_mark) ?? 0
      },
      set(v: number) {
        const normalized = this.normalizeRoutingMark(v)
        if (normalized === undefined) {
          delete this.$props.data.routing_mark
          return
        }
        this.$props.data.routing_mark = normalized
      },
    },
    showBBRProfileOption(): boolean {
      return this.supportsMihomoBBRProfileProtocol(this.$props.protocol)
    },
    bbrProfileValue: {
      get(): 'conservative' | 'standard' | 'aggressive' {
        return this.normalizeMihomoBBRProfile(this.$props.data.bbr_profile) || 'aggressive'
      },
      set(v: string) {
        if (!this.showBBRProfileOption) return
        this.$props.data.bbr_profile = this.normalizeMihomoBBRProfile(v) || 'aggressive'
      },
    },
    optionBBRProfile: {
      get(): boolean {
        if (!this.showBBRProfileOption) return false
        return this.normalizeMihomoBBRProfile(this.$props.data.bbr_profile) !== ''
      },
      set(v: boolean) {
        if (!this.showBBRProfileOption) {
          delete this.$props.data.bbr_profile
          return
        }
        if (v) {
          this.$props.data.bbr_profile = this.normalizeMihomoBBRProfile(this.$props.data.bbr_profile) || 'aggressive'
          return
        }
        delete this.$props.data.bbr_profile
      },
    },
    optionMux: {
      get(): boolean {
        return this.hasActiveSMux()
      },
      set(v: boolean) {
        if (v) {
          this.$props.data.smux = { enabled: true, statistic: false, only_tcp: false }
          return
        }
        delete this.$props.data.smux
      },
    },
    muxProtocol: {
      get(): MuxProtocol | undefined {
        const mux = this.$props.data.smux
        if (!this.isRecord(mux)) return undefined
        return this.isMuxProtocol(mux.protocol) ? mux.protocol : undefined
      },
      set(v: MuxProtocol | undefined) {
        const mux = this.ensureSMux()
        if (this.isMuxProtocol(v)) mux.protocol = v
        else delete mux.protocol
      },
    },
    muxMaxConnections: {
      get(): number {
        const mux = this.$props.data.smux
        if (!this.isRecord(mux)) return 0
        return this.normalizeRoutingMark(mux.max_connections) ?? 0
      },
      set(v: number) {
        const mux = this.ensureSMux()
        const normalized = this.normalizeRoutingMark(v)
        if (normalized === undefined) delete mux.max_connections
        else mux.max_connections = normalized
      },
    },
    muxMinStreams: {
      get(): number {
        const mux = this.$props.data.smux
        if (!this.isRecord(mux)) return 0
        return this.normalizeRoutingMark(mux.min_streams) ?? 0
      },
      set(v: number) {
        const mux = this.ensureSMux()
        const normalized = this.normalizeRoutingMark(v)
        if (normalized === undefined) delete mux.min_streams
        else mux.min_streams = normalized
      },
    },
    muxMaxStreams: {
      get(): number {
        const mux = this.$props.data.smux
        if (!this.isRecord(mux)) return 0
        return this.normalizeRoutingMark(mux.max_streams) ?? 0
      },
      set(v: number) {
        const mux = this.ensureSMux()
        const normalized = this.normalizeRoutingMark(v)
        if (normalized === undefined) delete mux.max_streams
        else mux.max_streams = normalized
      },
    },
    muxStatisticValue: {
      get(): boolean {
        const mux = this.$props.data.smux
        return this.isRecord(mux) && mux.statistic === true
      },
      set(v: boolean) {
        const mux = this.ensureSMux()
        mux.statistic = v === true
      },
    },
    muxOnlyTCPValue: {
      get(): boolean {
        const mux = this.$props.data.smux
        return this.isRecord(mux) && mux.only_tcp === true
      },
      set(v: boolean) {
        const mux = this.ensureSMux()
        mux.only_tcp = v === true
      },
    },
    muxPadding: {
      get(): boolean {
        const mux = this.$props.data.smux
        return this.isRecord(mux) && mux.padding === true
      },
      set(v: boolean) {
        const mux = this.ensureSMux()
        if (v === true) mux.padding = true
        else delete mux.padding
      },
    },
    muxBrutalEnabled: {
      get(): boolean {
        const mux = this.$props.data.smux
        if (!this.isRecord(mux) || !this.isRecord(mux.brutal)) return false
        return mux.brutal.enabled === true
      },
      set(v: boolean) {
        if (v) {
          this.ensureSMuxBrutal()
          return
        }
        const mux = this.ensureSMux()
        delete mux.brutal
      },
    },
    muxBrutalUpMbps: {
      get(): number {
        const mux = this.$props.data.smux
        if (!this.isRecord(mux) || !this.isRecord(mux.brutal)) return 100
        return this.normalizeRoutingMark(mux.brutal.up_mbps) ?? 100
      },
      set(v: number) {
        const brutal = this.ensureSMuxBrutal()
        brutal.up_mbps = this.normalizeRoutingMark(v) ?? 100
      },
    },
    muxBrutalDownMbps: {
      get(): number {
        const mux = this.$props.data.smux
        if (!this.isRecord(mux) || !this.isRecord(mux.brutal)) return 100
        return this.normalizeRoutingMark(mux.brutal.down_mbps) ?? 100
      },
      set(v: number) {
        const brutal = this.ensureSMuxBrutal()
        brutal.down_mbps = this.normalizeRoutingMark(v) ?? 100
      },
    },
  },
  watch: {
    data() {
      this.sanitizeCurrentData()
      if (this.hasActiveSMux()) {
        this.ensureSMux()
      }
    },
    protocol() {
      this.sanitizeCurrentData()
      if (this.hasActiveSMux()) {
        this.ensureSMux()
      }
    },
  },
}
</script>
