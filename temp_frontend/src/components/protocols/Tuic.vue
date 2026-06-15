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
          :suffix="$t('date.s')"
          min="1"
          v-model.number="heartbeat_seconds"
        ></v-text-field>
      </v-col>
      <v-col v-if="showMihomoInboundFields" cols="12" sm="6" md="4">
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
      <v-col cols="12" sm="6" md="4">
        <v-switch
          color="primary"
          label="fast-open"
          v-model="optionMihomoFastOpen"
          hide-details
        ></v-switch>
      </v-col>
    </v-row>

  </v-card>
</template>

<script lang="ts">
import Network from '@/components/Network.vue'

function readMilliseconds(value: unknown): number | '' {
  if (typeof value !== 'string' || value.trim() === '') return ''
  const normalized = value.trim().toLowerCase()
  if (normalized.endsWith('ms')) {
    const parsed = parseInt(normalized.slice(0, -2), 10)
    return Number.isFinite(parsed) ? parsed : ''
  }
  if (normalized.endsWith('s')) {
    const parsed = parseInt(normalized.slice(0, -1), 10)
    return Number.isFinite(parsed) ? parsed * 1000 : ''
  }
  const parsed = parseInt(normalized, 10)
  return Number.isFinite(parsed) ? parsed : ''
}

function writeMilliseconds(value: number): string {
  return value ? `${value}ms` : ''
}

function readSeconds(value: unknown): number | '' {
  if (typeof value !== 'string' || value.trim() === '') return ''
  const normalized = value.trim().toLowerCase()
  if (normalized.endsWith('ms')) {
    const parsed = parseInt(normalized.slice(0, -2), 10)
    return Number.isFinite(parsed) ? Math.floor(parsed / 1000) : ''
  }
  if (normalized.endsWith('s')) {
    const parsed = parseInt(normalized.slice(0, -1), 10)
    return Number.isFinite(parsed) ? parsed : ''
  }
  const parsed = parseInt(normalized, 10)
  return Number.isFinite(parsed) ? parsed : ''
}

function writeSeconds(value: number): string {
  return value ? `${value}s` : ''
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
    showNetwork(): boolean {
      return this.$props.direction === 'out' && !this.showMihomoOutboundFields
    },
    fastOpenStore(): any {
      if (this.$props.direction === 'in') {
        if (!this.$props.data.out_json || typeof this.$props.data.out_json !== 'object') {
          this.$props.data.out_json = {}
        }
        return this.$props.data.out_json
      }
      return this.$props.data
    },
    optionMihomoFastOpen: {
      get(): boolean {
        if (!this.isMihomoNamespace) return false
        return this.fastOpenStore.mihomo_fast_open === true
      },
      set(newValue: boolean) {
        if (!this.isMihomoNamespace) return
        this.fastOpenStore.mihomo_fast_open = newValue
      }
    },
    auth_timeout_ms: {
      get() { return readMilliseconds(this.$props.data.auth_timeout) },
      set(newValue: number) { this.$props.data.auth_timeout = writeMilliseconds(newValue) }
    },
    max_idle_time_ms: {
      get() { return readMilliseconds(this.$props.data.max_idle_time) },
      set(newValue: number) { this.$props.data.max_idle_time = writeMilliseconds(newValue) }
    },
    request_timeout_ms: {
      get() { return readMilliseconds(this.$props.data.request_timeout) },
      set(newValue: number) { this.$props.data.request_timeout = writeMilliseconds(newValue) }
    },
    showZeroRtt(): boolean {
      return !(this.$props.direction === 'in' && this.isMihomoNamespace)
    },
    showHeartbeat(): boolean {
      return !(this.$props.direction === 'in' && this.isMihomoNamespace)
    },
    heartbeat_seconds: {
      get() { return readSeconds(this.$props.data.heartbeat) },
      set(newValue: number) { this.$props.data.heartbeat = writeSeconds(newValue) }
    },
    max_open_streams: {
      get(): number | '' { return this.$props.data.max_open_streams ?? '' },
      set(newValue: number) { this.$props.data.max_open_streams = newValue || undefined }
    },
    max_udp_relay_packet_size: {
      get(): number | '' { return this.$props.data.max_udp_relay_packet_size ?? '' },
      set(newValue: number) { this.$props.data.max_udp_relay_packet_size = newValue || undefined }
    },
    cwnd: {
      get(): number | '' { return this.$props.data.cwnd ?? '' },
      set(newValue: number) { this.$props.data.cwnd = newValue || undefined }
    },
    max_datagram_frame_size: {
      get(): number | '' { return this.$props.data.max_datagram_frame_size ?? '' },
      set(newValue: number) { this.$props.data.max_datagram_frame_size = newValue || undefined }
    },
    udp_over_stream_version: {
      get(): number | '' { return this.$props.data.udp_over_stream_version ?? '' },
      set(newValue: number) { this.$props.data.udp_over_stream_version = newValue || undefined }
    },
    ip: {
      get(): string { return this.$props.data.ip ?? '' },
      set(newValue: string) { this.$props.data.ip = newValue?.trim() ? newValue.trim() : undefined }
    }
  },
  components: { Network }
}
</script>
