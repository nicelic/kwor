<template>
    <v-card :subtitle="$t('objects.transport')">
    <v-row>
      <v-col cols="12" sm="6" md="4">
        <v-switch color="primary" :label="$t('transport.enable')" v-model="tpEnable" hide-details></v-switch>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="tpEnable">
        <v-select
          hide-details
          :label="$t('type')"
          :items="transportTypeItems"
          v-model="transportType">
        </v-select>
      </v-col>
    </v-row>
    <Http v-if="Transport.type == trspTypes.HTTP" :transport="Transport" :namespace="namespace" />
    <H2 v-if="Transport.type == trspTypes.H2" :transport="Transport" />
    <WebSocket v-if="Transport.type == trspTypes.WebSocket" :transport="Transport" :namespace="namespace" />
    <GRPC v-if="Transport.type == trspTypes.gRPC" :transport="Transport" :namespace="namespace" />
    <HttpUpgrade v-if="Transport.type == trspTypes.HTTPUpgrade" :transport="Transport" />
    <XHTTP v-if="Transport.type == trspTypes.XHTTP" :transport="Transport" />
  </v-card>
</template>

<script lang="ts">
import { TrspTypes, Transport } from '@/types/transport'
import Http from './transports/Http.vue'
import H2 from './transports/H2.vue'
import WebSocket from './transports/WebSocket.vue'
import GRPC from './transports/gRPC.vue'
import HttpUpgrade from './transports/HttpUpgrade.vue'
import XHTTP from './transports/XHTTP.vue'

type HeaderMap = Record<string, unknown>

export default {
  props: {
    data: {
      type: Object,
      required: true,
    },
    namespace: {
      type: String,
      default: 'default',
    },
  },
  data() {
    return {
      trspTypes: TrspTypes,
      defaultTransportOrder: [
        TrspTypes.HTTP,
        TrspTypes.WebSocket,
        TrspTypes.QUIC,
        TrspTypes.gRPC,
        TrspTypes.HTTPUpgrade,
      ],
      mihomoTransportOrder: [
        TrspTypes.HTTP,
        TrspTypes.H2,
        TrspTypes.WebSocket,
        TrspTypes.gRPC,
      ],
    }
  },
  computed: {
    isMihomo(): boolean {
      return this.$props.namespace === 'mihomo'
    },
    supportsXHTTP(): boolean {
      const proto = typeof this.$props.data?.type === 'string'
        ? this.$props.data.type.toLowerCase()
        : ''
      return proto === 'vless'
    },
    availableTransportTypes(): string[] {
      if (!this.isMihomo) {
        return [...this.defaultTransportOrder]
      }

      const types = [...this.mihomoTransportOrder]
      if (this.supportsXHTTP) {
        types.push(TrspTypes.XHTTP)
      }
      return types
    },
    transportTypeItems(): { title: string; value: string }[] {
      return this.availableTransportTypes.map((value: string) => ({
        title: this.transportTitle(value),
        value,
      }))
    },
    defaultTransportType(): string {
      if (this.availableTransportTypes.length === 0) {
        return TrspTypes.HTTP
      }
      return this.availableTransportTypes[0]
    },
    Transport() {
      return <Transport>this.$props.data.transport
    },
    tpEnable: {
      get() { return Object.hasOwn(this.$props.data.transport, 'type') },
      set(newValue: boolean) { this.$props.data.transport = newValue ? { type: this.defaultTransportType } : {} }
    },
    transportType: {
      get() { return this.Transport.type },
      set(newValue: string) { this.$props.data.transport = { type: newValue } }
    },
  },
  methods: {
    transportTitle(value: string): string {
      switch (value) {
        case TrspTypes.HTTP:
          return 'HTTP'
        case TrspTypes.H2:
          return 'H2'
        case TrspTypes.WebSocket:
          return 'WebSocket'
        case TrspTypes.QUIC:
          return 'QUIC'
        case TrspTypes.gRPC:
          return 'gRPC'
        case TrspTypes.HTTPUpgrade:
          return 'HTTPUpgrade'
        case TrspTypes.XHTTP:
          return 'XHTTP'
        default:
          return String(value)
      }
    },
    normalizeHeaderValues(raw: unknown): string[] {
      if (typeof raw === 'string') {
        return raw
          .split(',')
          .map((value: string) => value.trim())
          .filter((value: string) => value.length > 0)
      }
      if (Array.isArray(raw)) {
        return raw
          .filter((value: unknown) => typeof value === 'string')
          .map((value: unknown) => String(value).trim())
          .filter((value: string) => value.length > 0)
      }
      return []
    },
    sanitizeMihomoTransportCompatibility() {
      if (!this.isMihomo) return

      const transport = this.$props.data.transport as Record<string, unknown>
      if (!transport || typeof transport !== 'object') return

      const rawType = typeof transport.type === 'string' ? transport.type.toLowerCase() : ''
      if (rawType === TrspTypes.QUIC) {
        this.$props.data.transport = { type: this.defaultTransportType }
        return
      }

      if (rawType === TrspTypes.HTTPUpgrade) {
        const wsTransport = { ...transport, type: TrspTypes.WebSocket } as Record<string, unknown>
        wsTransport['v2ray_http_upgrade'] = true

        const host = typeof transport.host === 'string' ? transport.host.trim() : ''
        const headersRaw = transport.headers
        const headers: HeaderMap = (headersRaw && typeof headersRaw === 'object' && !Array.isArray(headersRaw))
          ? { ...(headersRaw as HeaderMap) }
          : {}
        if (host.length > 0 && typeof headers.Host === 'undefined') {
          headers.Host = host
        }
        if (Object.keys(headers).length > 0) {
          wsTransport.headers = headers
        } else {
          delete wsTransport.headers
        }
        delete wsTransport.host
        this.$props.data.transport = wsTransport
        return
      }

      if (rawType === TrspTypes.H2) {
        const hostValues = this.normalizeHeaderValues((transport.headers as HeaderMap | undefined)?.Host)
        if ((!Array.isArray(transport.host) || transport.host.length === 0) && hostValues.length > 0) {
          transport.host = hostValues
        }
      }
    },
    ensureAllowedTransportType() {
      const transport = this.$props.data.transport as Record<string, unknown>
      if (!transport || typeof transport !== 'object') return

      const currentType = typeof transport.type === 'string' ? transport.type : ''
      if (!currentType) return

      if (!this.availableTransportTypes.includes(currentType)) {
        this.$props.data.transport = { type: this.defaultTransportType }
      }
    },
    sanitizeTransportState() {
      this.sanitizeMihomoTransportCompatibility()
      this.ensureAllowedTransportType()
    },
  },
  mounted() {
    this.sanitizeTransportState()
  },
  watch: {
    namespace() {
      this.sanitizeTransportState()
    },
    data: {
      handler() {
        this.sanitizeTransportState()
      },
      deep: false,
    },
    'data.type'() {
      this.sanitizeTransportState()
    },
    'data.transport.type'() {
      this.sanitizeTransportState()
    },
  },
  components: { Http, H2, WebSocket, GRPC, HttpUpgrade, XHTTP }
}
</script>
