<template>
  <v-row>
    <v-col cols="12" sm="6" md="4">
      <v-text-field
      :label="$t('transport.path')"
      hide-details
      v-model="transport.path">
      </v-text-field>
    </v-col>
    <v-col cols="12" sm="6" md="4">
      <v-text-field
      :label="$t('transport.host')"
      hide-details
      v-model="host">
      </v-text-field>
    </v-col>
  </v-row>
  <v-row>
    <v-col cols="12" sm="6" md="4">
      <v-text-field
      label="Max Early Data"
      hide-details
      type="number"
      min="0"
      v-model.number="max_early_data">
      </v-text-field>
    </v-col>
    <v-col cols="12" sm="6" md="4">
      <v-text-field
      label="Early Data Header Name"
      hide-details
      v-model="transport.early_data_header_name">
      </v-text-field>
    </v-col>
  </v-row>
  <v-row v-if="isMihomo">
    <v-col cols="12" sm="6" md="4">
      <v-switch
      color="primary"
      label="v2ray-http-upgrade"
      hide-details
      v-model="transport.v2ray_http_upgrade">
      </v-switch>
    </v-col>
    <v-col cols="12" sm="6" md="4">
      <v-switch
      color="primary"
      label="v2ray-http-upgrade-fast-open"
      hide-details
      v-model="transport.v2ray_http_upgrade_fast_open">
      </v-switch>
    </v-col>
  </v-row>
  <Headers :data="transport" />
</template>

<script lang="ts">
import { WebSocket } from '../../types/transport'
import Headers from '../Headers.vue'
export default {
  props: {
    transport: {
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
    }
  },
  computed: {
    isMihomo(): boolean {
      return this.$props.namespace === 'mihomo'
    },
    WS(): WebSocket {
      return <WebSocket> this.$props.transport
    },
    max_early_data: {
      get() { return this.WS.max_early_data ? this.WS.max_early_data : '' },
      set(newValue:number) { this.$props.transport.max_early_data = newValue != 0 ? newValue : undefined }
    },
    host: {
      get() {
        const headers = this.WS.headers
        if (!headers || typeof headers !== 'object') return ''
        const host = (headers as Record<string, unknown>)['Host']
        return typeof host === 'string' ? host : ''
      },
      set(newValue:string) {
        this.writeHostHeader(newValue)
      }
    },
  },
  methods: {
    ensureHeadersMap(): Record<string, unknown> {
      const rawHeaders = this.$props.transport.headers
      if (!rawHeaders || typeof rawHeaders !== 'object' || Array.isArray(rawHeaders)) {
        this.$props.transport.headers = {}
      }
      return this.$props.transport.headers as Record<string, unknown>
    },
    cleanupHeadersMap() {
      const rawHeaders = this.$props.transport.headers
      if (!rawHeaders || typeof rawHeaders !== 'object' || Array.isArray(rawHeaders)) {
        delete this.$props.transport.headers
        return
      }
      if (Object.keys(rawHeaders as Record<string, unknown>).length === 0) {
        delete this.$props.transport.headers
      }
    },
    writeHostHeader(rawHost: string) {
      const host = rawHost.trim()
      if (host.length === 0) {
        const headers = this.$props.transport.headers as Record<string, unknown> | undefined
        if (headers && typeof headers === 'object' && !Array.isArray(headers)) {
          delete headers['Host']
        }
        this.cleanupHeadersMap()
        return
      }
      const headers = this.ensureHeadersMap()
      headers['Host'] = host
      this.$props.transport.headers = headers
    },
  },
  mounted() {
    this.WS.early_data_header_name ??= 'Sec-WebSocket-Protocol'
    this.WS.path ??= '/'
  },
  components: { Headers }
}
</script>
