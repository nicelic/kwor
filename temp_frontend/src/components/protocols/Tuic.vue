<template>
  <v-card subtitle="TUIC">
    <v-row v-if="direction === 'out'">
      <v-col v-if="showMihomoOutboundFields" cols="12" sm="6" md="4">
        <v-text-field v-model="data.token" label="Token" hide-details></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-text-field v-model="data.uuid" label="Credential ID" hide-details></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-text-field v-model="data.password" :label="$t('types.pw')" hide-details></v-text-field>
      </v-col>
      <v-col v-if="showNetwork" cols="12" sm="6" md="4">
        <Network :data="data" />
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-select
          hide-details
          label="UDP Relay Mode"
          :items="['native', 'quic']"
          clearable
          @click:clear="delete data.udp_relay_mode"
          v-model="data.udp_relay_mode"
        ></v-select>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-switch color="primary" label="UDP Over Stream" v-model="data.udp_over_stream" hide-details></v-switch>
      </v-col>
      <v-col v-if="showMihomoOutboundFields && data.udp_over_stream" cols="12" sm="6" md="4">
        <v-text-field
          v-model.number="udp_over_stream_version"
          label="UDP Over Stream Version"
          hide-details
          type="number"
          min="1"
        ></v-text-field>
      </v-col>
    </v-row>

    <v-row>
      <v-col cols="12" sm="6" md="4">
        <v-select
          hide-details
          :label="$t('types.tuic.congControl')"
          :items="congestion_controls"
          v-model="data.congestion_control"
        ></v-select>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="showZeroRtt">
        <v-switch color="primary" label="Zero-RTT Handshake" v-model="data.zero_rtt_handshake" hide-details></v-switch>
      </v-col>
      <v-col v-if="showMihomoOutboundFields" cols="12" sm="6" md="4">
        <v-text-field
          v-model="ip"
          label="Resolved IP"
          hide-details
        ></v-text-field>
      </v-col>
    </v-row>

    <v-row>
      <v-col cols="12" sm="6" md="4" v-if="direction === 'in'">
        <v-text-field
          :label="$t('types.tuic.authTimeout')"
          hide-details
          type="number"
          suffix="ms"
          min="1"
          v-model.number="auth_timeout_ms"
        ></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="showHeartbeat">
        <v-text-field
          :label="$t('types.tuic.hb')"
          hide-details
          type="number"
          suffix="ms"
          min="1"
          v-model.number="heartbeat_ms"
        ></v-text-field>
      </v-col>
      <v-col v-if="showInboundMaxIdleTime" cols="12" sm="6" md="4">
        <v-text-field
          v-model.number="max_idle_time_ms"
          label="Max Idle Time"
          hide-details
          type="number"
          suffix="ms"
          min="1"
        ></v-text-field>
      </v-col>
      <v-col v-if="showMihomoOutboundFields" cols="12" sm="6" md="4">
        <v-text-field
          v-model.number="request_timeout_ms"
          label="Request Timeout"
          hide-details
          type="number"
          suffix="ms"
          min="1"
        ></v-text-field>
      </v-col>
    </v-row>

    <v-row v-if="showMihomoFields">
      <v-col cols="12" sm="6" md="4">
        <v-text-field
          v-model.number="max_udp_relay_packet_size"
          label="Max UDP Relay Packet Size"
          hide-details
          type="number"
          min="1"
        ></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-text-field
          v-model.number="cwnd"
          label="CWND"
          hide-details
          type="number"
          min="1"
        ></v-text-field>
      </v-col>
      <v-col v-if="showMihomoOutboundFields" cols="12" sm="6" md="4">
        <v-text-field
          v-model.number="max_open_streams"
          label="Max Open Streams"
          hide-details
          type="number"
          min="1"
        ></v-text-field>
      </v-col>
      <v-col v-if="showMihomoOutboundFields" cols="12" sm="6" md="4">
        <v-text-field
          v-model.number="max_datagram_frame_size"
          label="Max Datagram Frame Size"
          hide-details
          type="number"
          min="1"
        ></v-text-field>
      </v-col>
      <v-col v-if="showMihomoOutboundFields" cols="12" sm="6" md="4">
        <v-switch
          color="primary"
          label="Disable MTU Discovery"
          v-model="data.disable_mtu_discovery"
          hide-details
        ></v-switch>
      </v-col>
    </v-row>

  </v-card>
</template>

<script lang="ts">
import Network from '@/components/Network.vue'

function parsePositiveInteger(value: unknown): number | undefined {
  if (typeof value === 'number') {
    return Number.isSafeInteger(value) && value > 0 ? value : undefined
  }
  if (typeof value !== 'string') return undefined
  const normalized = value.trim()
  if (!/^\d+$/.test(normalized)) return undefined
  const parsed = Number(normalized)
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : undefined
}

function readMilliseconds(value: unknown): number | '' {
  if (typeof value === 'number') {
    return parsePositiveInteger(value) ?? ''
  }
  if (typeof value !== 'string') return ''
  const normalized = value.trim().toLowerCase()
  const matched = normalized.match(/^(\d+)(ms|s)?$/)
  if (!matched) return ''
  const numeric = parsePositiveInteger(matched[1])
  if (numeric === undefined) return ''
  const milliseconds = matched[2] === 's' ? numeric * 1000 : numeric
  return Number.isSafeInteger(milliseconds) ? milliseconds : ''
}

function writeMilliseconds(value: unknown): string | undefined {
  const normalized = parsePositiveInteger(value)
  return normalized === undefined ? undefined : `${normalized}ms`
}

export default {
  props: ['direction', 'data', 'namespace'],
  data() {
    return {
      congestion_controls: [
        'cubic', 'new_reno', 'bbr'
      ]
    }
  },
  methods: {
    sanitizeMihomoUnsupportedFields() {
      if (this.$props.namespace !== 'mihomo') return
      if (this.$props.direction === 'out') {
        delete this.$props.data.network
        return
      }
      if (this.$props.direction === 'in') {
        for (const key of [
          'fast_open', 'mihomo_fast_open', 'fast-open',
          'zero_rtt_handshake', 'heartbeat', 'network', 'udp_relay_mode',
          'udp_over_stream', 'udp_over_stream_version', 'ip', 'request_timeout',
          'max_open_streams', 'max_datagram_frame_size', 'disable_mtu_discovery',
        ]) {
          delete this.$props.data[key]
        }
      }
    },
  },
  mounted() {
    this.sanitizeMihomoUnsupportedFields()
  },
  computed: {
    isMihomoNamespace(): boolean {
      return this.$props.namespace === 'mihomo'
    },
    showMihomoFields(): boolean {
      return this.isMihomoNamespace
    },
    showMihomoOutboundFields(): boolean {
      return this.$props.direction === 'out' && this.isMihomoNamespace
    },
    showMihomoInboundFields(): boolean {
      return this.$props.direction === 'in' && this.isMihomoNamespace
    },
    showInboundMaxIdleTime(): boolean {
      return this.$props.direction === 'in'
    },
    showNetwork(): boolean {
      return this.$props.direction === 'out' && !this.showMihomoOutboundFields
    },
    auth_timeout_ms: {
      get() { return readMilliseconds(this.$props.data.auth_timeout) },
      set(newValue: unknown) { this.$props.data.auth_timeout = writeMilliseconds(newValue) }
    },
    max_idle_time_ms: {
      get() { return readMilliseconds(this.$props.data.max_idle_time) },
      set(newValue: unknown) { this.$props.data.max_idle_time = writeMilliseconds(newValue) }
    },
    request_timeout_ms: {
      get() { return readMilliseconds(this.$props.data.request_timeout) },
      set(newValue: unknown) { this.$props.data.request_timeout = writeMilliseconds(newValue) }
    },
    showZeroRtt(): boolean {
      return !(this.$props.direction === 'in' && this.isMihomoNamespace)
    },
    showHeartbeat(): boolean {
      return !(this.$props.direction === 'in' && this.isMihomoNamespace)
    },
    heartbeat_ms: {
      get() { return readMilliseconds(this.$props.data.heartbeat) },
      set(newValue: unknown) { this.$props.data.heartbeat = writeMilliseconds(newValue) }
    },
    max_open_streams: {
      get(): number | '' { return parsePositiveInteger(this.$props.data.max_open_streams) ?? '' },
      set(newValue: unknown) { this.$props.data.max_open_streams = parsePositiveInteger(newValue) }
    },
    max_udp_relay_packet_size: {
      get(): number | '' { return parsePositiveInteger(this.$props.data.max_udp_relay_packet_size) ?? '' },
      set(newValue: unknown) { this.$props.data.max_udp_relay_packet_size = parsePositiveInteger(newValue) }
    },
    cwnd: {
      get(): number | '' { return parsePositiveInteger(this.$props.data.cwnd) ?? '' },
      set(newValue: unknown) { this.$props.data.cwnd = parsePositiveInteger(newValue) }
    },
    max_datagram_frame_size: {
      get(): number | '' { return parsePositiveInteger(this.$props.data.max_datagram_frame_size) ?? '' },
      set(newValue: unknown) { this.$props.data.max_datagram_frame_size = parsePositiveInteger(newValue) }
    },
    udp_over_stream_version: {
      get(): number | '' { return parsePositiveInteger(this.$props.data.udp_over_stream_version) ?? '' },
      set(newValue: unknown) { this.$props.data.udp_over_stream_version = parsePositiveInteger(newValue) }
    },
    ip: {
      get(): string { return this.$props.data.ip ?? '' },
      set(newValue: string) { this.$props.data.ip = newValue?.trim() ? newValue.trim() : undefined }
    }
  },
  watch: {
    data() {
      this.sanitizeMihomoUnsupportedFields()
    },
    direction() {
      this.sanitizeMihomoUnsupportedFields()
    },
    namespace() {
      this.sanitizeMihomoUnsupportedFields()
    },
  },
  components: { Network }
}
</script>
