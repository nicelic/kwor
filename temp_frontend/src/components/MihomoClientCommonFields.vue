<template>
  <v-card :subtitle="$t('mihomoCommon.title')" style="background-color: inherit;">
    <v-row>
      <v-col cols="12" sm="6" md="4" v-if="optionUDP">
        <v-select
          hide-details
          label="UDP"
          :items="boolItems"
          v-model="udpValue">
        </v-select>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="optionIPVersion">
        <v-select
          hide-details
          :label="$t('rule.ipVer')"
          :items="ipVersionItems"
          v-model="ipVersionValue">
        </v-select>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="optionTFO">
        <v-select
          hide-details
          label="TFO"
          :items="boolItems"
          v-model="tfoValue">
        </v-select>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="optionMPTCP">
        <v-select
          hide-details
          label="MPTCP"
          :items="boolItems"
          v-model="mptcpValue">
        </v-select>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="optionRoutingMark">
        <v-text-field
          hide-details
          type="number"
          min="0"
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
    <template v-if="optionMux">
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
            :label="$t('mux.maxConn')"
            v-model.number="muxMaxConnections">
          </v-text-field>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-text-field
            hide-details
            type="number"
            min="0"
            :label="$t('mux.minStr')"
            v-model.number="muxMinStreams">
          </v-text-field>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-text-field
            hide-details
            type="number"
            min="0"
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
            <v-list-item>
              <v-switch v-model="optionUDP" color="primary" label="UDP" hide-details></v-switch>
            </v-list-item>
            <v-list-item>
              <v-switch v-model="optionIPVersion" color="primary" :label="$t('rule.ipVer')" hide-details></v-switch>
            </v-list-item>
            <v-list-item>
              <v-switch v-model="optionTFO" color="primary" label="TFO" hide-details></v-switch>
            </v-list-item>
            <v-list-item>
              <v-switch v-model="optionMPTCP" color="primary" label="MPTCP" hide-details></v-switch>
            </v-list-item>
            <v-list-item>
              <v-switch v-model="optionRoutingMark" color="primary" label="Routing Mark" hide-details></v-switch>
            </v-list-item>
            <v-list-item>
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
      ipVersionItems: ['dual', 'ipv4', 'ipv6', 'ipv4-prefer', 'ipv6-prefer'],
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
    if (this.hasActiveSMux()) {
      this.ensureSMux()
    }
  },
  computed: {
    optionUDP: {
      get(): boolean {
        return this.$props.data.udp !== undefined
      },
      set(v: boolean) {
        if (v) {
          this.$props.data.udp = false
          return
        }
        delete this.$props.data.udp
      },
    },
    udpValue: {
      get(): boolean {
        return this.$props.data.udp === true
      },
      set(v: boolean) {
        this.$props.data.udp = v === true
      },
    },
    optionIPVersion: {
      get(): boolean {
        return typeof this.$props.data.ip_version === 'string'
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
      get(): string {
        return typeof this.$props.data.ip_version === 'string' && this.$props.data.ip_version.trim() !== ''
          ? this.$props.data.ip_version
          : 'dual'
      },
      set(v: string) {
        this.$props.data.ip_version = typeof v === 'string' && v.trim() !== '' ? v : 'dual'
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
        return this.$props.data.routing_mark !== undefined
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
        return typeof this.$props.data.routing_mark === 'number' ? this.$props.data.routing_mark : 0
      },
      set(v: number) {
        this.$props.data.routing_mark = Number.isFinite(v) ? v : 0
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
        mux.protocol = this.isMuxProtocol(v) ? v : undefined
      },
    },
    muxMaxConnections: {
      get(): number {
        const mux = this.$props.data.smux
        if (!this.isRecord(mux) || typeof mux.max_connections !== 'number') return 0
        return mux.max_connections
      },
      set(v: number) {
        const mux = this.ensureSMux()
        mux.max_connections = Number.isFinite(v) && v >= 0 ? v : undefined
      },
    },
    muxMinStreams: {
      get(): number {
        const mux = this.$props.data.smux
        if (!this.isRecord(mux) || typeof mux.min_streams !== 'number') return 0
        return mux.min_streams
      },
      set(v: number) {
        const mux = this.ensureSMux()
        mux.min_streams = Number.isFinite(v) && v >= 0 ? v : undefined
      },
    },
    muxMaxStreams: {
      get(): number {
        const mux = this.$props.data.smux
        if (!this.isRecord(mux) || typeof mux.max_streams !== 'number') return 0
        return mux.max_streams
      },
      set(v: number) {
        const mux = this.ensureSMux()
        mux.max_streams = Number.isFinite(v) && v >= 0 ? v : undefined
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
        mux.padding = v === true ? true : undefined
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
        mux.brutal = undefined
      },
    },
    muxBrutalUpMbps: {
      get(): number {
        const mux = this.$props.data.smux
        if (!this.isRecord(mux) || !this.isRecord(mux.brutal) || typeof mux.brutal.up_mbps !== 'number') return 100
        return mux.brutal.up_mbps
      },
      set(v: number) {
        const brutal = this.ensureSMuxBrutal()
        brutal.up_mbps = Number.isFinite(v) ? v : 100
      },
    },
    muxBrutalDownMbps: {
      get(): number {
        const mux = this.$props.data.smux
        if (!this.isRecord(mux) || !this.isRecord(mux.brutal) || typeof mux.brutal.down_mbps !== 'number') return 100
        return mux.brutal.down_mbps
      },
      set(v: number) {
        const brutal = this.ensureSMuxBrutal()
        brutal.down_mbps = Number.isFinite(v) ? v : 100
      },
    },
  },
}
</script>
