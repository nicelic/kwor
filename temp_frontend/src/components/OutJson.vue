<template>
  <v-card :subtitle="type">
    <v-row>
      <v-col cols="12" sm="6" md="4" v-if="type == inTypes.SOCKS">
        <v-select
          hide-details
          :items="['4','4a','5']"
          :label="$t('version')"
          v-model="inData.out_json.version">
        </v-select>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="needNetwork">
        <Network :data="networkData" />
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="needUot">
        <UoT :data="uotData" />
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="type == inTypes.HTTP">
        <v-text-field
        :label="$t('transport.path')"
        hide-details
        v-model="inData.out_json.path">
        </v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="type == inTypes.VMess || type == inTypes.VLESS">
        <v-select
          hide-details
          :label="$t('types.vless.udpEnc')"
          :items="['none','packetaddr','xudp']"
          v-model="packet_encoding">
        </v-select>
      </v-col>
      <template v-if="type == inTypes.VMess">
        <v-col cols="12" sm="6" md="4">
          <v-select
            hide-details
            :label="$t('types.vmess.security')"
            :items="vmessSecurities"
            v-model="inData.out_json.security">
          </v-select>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-switch v-model="inData.out_json.global_padding" color="primary" :label="$t('types.vmess.globalPadding')" hide-details></v-switch>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-switch v-model="inData.out_json.authenticated_length" color="primary" :label="$t('types.vmess.authLen')" hide-details></v-switch>
        </v-col>
      </template>
      <template v-if="type == inTypes.TUIC">
        <v-col cols="12" sm="6" md="4">
          <v-select
            hide-details
            label="UDP Relay Mode"
            :items="['native', 'quic']"
            clearable
            @click:clear="delete inData.out_json.udp_relay_mode"
            v-model="inData.out_json.udp_relay_mode">
          </v-select>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-switch color="primary" label="UDP Over Stream" v-model="inData.out_json.udp_over_stream" hide-details></v-switch>
        </v-col>
      </template>
      <template v-if="type == inTypes.Snell">
        <v-col cols="12" sm="6" md="4">
          <v-select
            hide-details
            :label="$t('version')"
            :items="[1,2,3,4,5]"
            v-model="inData.out_json.version">
          </v-select>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-switch color="primary" label="Reuse" v-model="inData.out_json.reuse" hide-details></v-switch>
        </v-col>
      </template>
    </v-row>
    <v-row v-if="[inTypes.Hysteria, inTypes.Hysteria2].includes(type)">
      <v-col cols="12" sm="6" md="4">
        <v-text-field
          :label="$t('stats.upload')"
          hide-details
          type="number"
          min="0"
          :suffix="$t('stats.Mbps')"
          v-model.number="client_up_mbps">
        </v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-text-field
          :label="$t('stats.download')"
          hide-details
          type="number"
          min="0"
          :suffix="$t('stats.Mbps')"
          v-model.number="client_down_mbps">
        </v-text-field>
      </v-col>
    </v-row>
    <v-row v-if="[inTypes.Hysteria, inTypes.Hysteria2].includes(type)">
      <v-col cols="12" sm="6" md="4">
        <v-text-field
          :label="$t('rule.portRange') + ' ' + $t('commaSeparated')"
          @blur="onPortHopRangeBlur"
          v-model="server_ports">
        </v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="type == inTypes.Hysteria">
        <v-text-field
          :label="$t('ruleset.interval')"
          type="number"
          min="0"
          :suffix="$t('date.s')"
          v-model.number="hop_interval">
        </v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="type == inTypes.Hysteria2">
        <v-text-field
          :label="$t('ruleset.interval')"
          placeholder="30 | 30s | 30-60 | 30:60s"
          v-model="hy2HopIntervalInput"
          @blur="applyHy2HopIntervalInput"
          @keydown.enter.prevent="applyHy2HopIntervalInput">
        </v-text-field>
      </v-col>
    </v-row>
    <OutNaive v-if="type == inTypes.Naive" :data="inData.out_json" />
    <Headers :data="inData.out_json" v-if="type == inTypes.HTTP" />
    <TrustTunnel v-if="type == inTypes.TrustTunnel" :data="inData.out_json" direction="out_json" :namespace="namespace" />
    <AnyTls v-if="type == inTypes.AnyTls" :data="inData.out_json" direction="out_json" />
  </v-card>
</template>

<script lang="ts">
import { InTypes } from '@/types/inbounds'
import Network from './Network.vue'
import UoT from './UoT.vue'
import Headers from './Headers.vue'
import TrustTunnel from './protocols/TrustTunnel.vue'
import AnyTls from './protocols/AnyTls.vue'
import OutNaive from './protocols/OutNaive.vue'
import { formatHopIntervalInput, parseHopIntervalInput } from '@/plugins/hopInterval'
import { normalizePortRangeInput } from '@/plugins/portRange'

export default {
  emits: ['port-hop-range-blur'],
  props: {
    inData: { type: Object, required: true },
    type: { type: String, required: true },
    namespace: {
      type: String,
      default: 'default',
    },
  },
  data() {
    return {
      inTypes: InTypes,
      vmessSecurities: [
        "auto",
        "none",
        "zero",
        "aes-128-gcm",
        "aes-128-ctr",
        "chacha20-poly1305",
      ],
      haveNetwork: [
        InTypes.SOCKS,
        InTypes.Shadowsocks,
        InTypes.VMess,
        InTypes.Trojan,
        InTypes.Hysteria,
        InTypes.Hysteria2,
        InTypes.VLESS,
        InTypes.TUIC,
      ],
      havUoT: [
        InTypes.SOCKS,
        InTypes.Shadowsocks,
        InTypes.ShadowTLS,
      ],
      hy2HopIntervalInput: '',
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
      const value = this.$props.inData.out_json?.[key]
      if (value === '' || value === null || value === undefined) return null
      const normalized = Number(value)
      return Number.isFinite(normalized) ? Math.trunc(normalized) : null
    },
    writeOptionalMbpsValue(key: string, value: unknown) {
      if (!this.$props.inData.out_json) this.$props.inData.out_json = {}
      const normalized = this.normalizeOptionalMbpsValue(value)
      if (normalized === undefined) {
        delete this.$props.inData.out_json[key]
        return
      }
      this.$props.inData.out_json[key] = normalized
    },
    onPortHopRangeBlur() {
      if (!this.usesInboundPortHopBackend) return
      this.$emit('port-hop-range-blur', this.server_ports)
    },
    removeUnsupportedMihomoClientNetwork() {
      if (!this.isMihomoUnsupportedClientNetworkType) return
      if (!this.$props.inData.out_json) this.$props.inData.out_json = {}
      if (this.$props.type === this.inTypes.ShadowTLS) {
        const ssConfig = this.$props.inData.out_json.ss_config
        if (ssConfig && typeof ssConfig === 'object' && !Array.isArray(ssConfig)) {
          delete ssConfig.network
        }
        return
      }
      delete this.$props.inData.out_json.network
      if (this.$props.type === this.inTypes.Hysteria2) {
        delete this.$props.inData.bbr_profile
      }
    },
    syncHy2HopIntervalInput() {
      const lower = this.usesInboundPortHopBackend ? this.$props.inData.port_hop_interval : this.$props.inData.out_json.hop_interval
      const upper = this.usesInboundPortHopBackend ? this.$props.inData.port_hop_interval_max : this.$props.inData.out_json.hop_interval_max
      this.hy2HopIntervalInput = formatHopIntervalInput(lower, upper)
    },
    applyHy2HopIntervalInput() {
      if (this.$props.type !== this.inTypes.Hysteria2) return
 
      const parsed = parseHopIntervalInput(this.hy2HopIntervalInput)
      if (!parsed) {
        this.syncHy2HopIntervalInput()
        return
      }
 
      if (this.usesInboundPortHopBackend) {
        this.$props.inData.port_hop_interval = parsed.hopInterval
        this.$props.inData.port_hop_interval_max = parsed.hopIntervalMax
      } else {
        this.$props.inData.out_json.hop_interval = parsed.hopInterval
        this.$props.inData.out_json.hop_interval_max = parsed.hopIntervalMax
      }
      this.hy2HopIntervalInput = formatHopIntervalInput(parsed.hopInterval, parsed.hopIntervalMax)
    },
  },
  computed: {
    needNetwork():boolean {
      if (this.isMihomoUnsupportedClientNetworkType) {
        return false
      }
      return this.haveNetwork.includes(this.$props.type) || this.$props.type === InTypes.ShadowTLS
    },
    isMihomoUnsupportedClientNetworkType(): boolean {
      if (this.$props.namespace !== 'mihomo') return false
      return [
        InTypes.Hysteria2,
        InTypes.TUIC,
        InTypes.ShadowTLS,
        InTypes.Shadowsocks,
        InTypes.VMess,
        InTypes.VLESS,
        InTypes.Trojan,
      ].includes(this.$props.type)
    },
    networkData() {
      if (this.$props.type === InTypes.ShadowTLS) {
        if (!this.$props.inData.out_json) this.$props.inData.out_json = {}
        if (!this.$props.inData.out_json.ss_config) this.$props.inData.out_json.ss_config = {}
        return this.$props.inData.out_json.ss_config
      }
      return this.$props.inData.out_json
    },
    needUot():boolean { return this.havUoT.includes(this.$props.type) },
    uotData() {
      if (this.$props.type === InTypes.ShadowTLS) {
        if (!this.$props.inData.out_json) this.$props.inData.out_json = {}
        if (!this.$props.inData.out_json.ss_config) this.$props.inData.out_json.ss_config = {}
        return this.$props.inData.out_json.ss_config
      }
      return this.$props.inData.out_json
    },
    packet_encoding: {
      get() { return this.$props.inData.out_json.packet_encoding != undefined ? this.$props.inData.out_json.packet_encoding : 'none' },
      set(v:string) { this.$props.inData.out_json.packet_encoding = v != "none" ? v : undefined }
    },
    client_up_mbps: {
      get() {
        if (this.$props.type === this.inTypes.Hysteria2) {
          return this.readOptionalMbpsValue('up_mbps')
        }
        return this.$props.inData.out_json?.up_mbps ?? 2000
      },
      set(v:number | null) {
        this.writeOptionalMbpsValue('up_mbps', v)
      }
    },
    client_down_mbps: {
      get() {
        if (this.$props.type === this.inTypes.Hysteria2) {
          return this.readOptionalMbpsValue('down_mbps')
        }
        return this.$props.inData.out_json?.down_mbps ?? 2000
      },
      set(v:number | null) {
        this.writeOptionalMbpsValue('down_mbps', v)
      }
    },
    usesInboundPortHopBackend(): boolean {
      return [this.inTypes.Hysteria, this.inTypes.Hysteria2].includes(this.$props.type)
    },
    server_ports: {
      get() {
        if (this.usesInboundPortHopBackend) {
          return this.$props.inData.port_hop_range ?? ''
        }
        return this.$props.inData.out_json.server_ports?.join(',') ?? ''
      },
      set(v:string) {
        const normalized = normalizePortRangeInput(v)
        if (this.usesInboundPortHopBackend) {
          this.$props.inData.port_hop_range = normalized.length > 0 ? normalized.join(',') : undefined
          return
        }
        this.$props.inData.out_json.server_ports = normalized.length > 0 ? normalized : undefined
      }
    },
    hop_interval: {
      get() {
        const rawValue = this.usesInboundPortHopBackend ? this.$props.inData.port_hop_interval : this.$props.inData.out_json.hop_interval
        return rawValue ? parseInt(rawValue.replace('s','')) : 0
      },
      set(v:number) {
        if (this.usesInboundPortHopBackend) {
          this.$props.inData.port_hop_interval = v>0 ? v + 's' : undefined
          return
        }
        this.$props.inData.out_json.hop_interval = v>0 ? v + 's' : undefined
      }
    },
  },
  watch: {
    namespace: {
      handler() {
        this.removeUnsupportedMihomoClientNetwork()
        this.syncHy2HopIntervalInput()
      },
      immediate: true,
    },
    type: {
      handler() {
        this.removeUnsupportedMihomoClientNetwork()
        this.syncHy2HopIntervalInput()
      },
      immediate: true,
    },
    inData: {
      handler() {
        this.removeUnsupportedMihomoClientNetwork()
        this.syncHy2HopIntervalInput()
      },
      immediate: true,
    },
  },
  components: { Network, UoT, Headers, TrustTunnel, AnyTls, OutNaive }
}
</script>
