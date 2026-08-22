<template>
  <v-row>
    <v-col cols="12" sm="6" md="4">
      <v-text-field
      :label="$t('transport.grpcServiceName')"
      hide-details
      v-model="transport.service_name">
      </v-text-field>
    </v-col>
    <v-col cols="12" sm="6" md="4" v-if="isMihomo">
      <v-text-field
      label="gRPC User Agent"
      hide-details
      v-model="transport.grpc_user_agent">
      </v-text-field>
    </v-col>
    <v-col cols="12" sm="6" md="4" v-else>
      <v-switch
        color="primary"
        v-model="transport.permit_without_stream"
        :label="$t('transport.grpcPws')"
        hide-details>
      </v-switch>
    </v-col>
  </v-row>
  <v-row v-if="!isMihomo">
    <v-col cols="12" sm="6" md="4">
      <v-text-field
      :label="$t('transport.idleTimeout')"
      hide-details
      type="number"
      suffix="s"
      min="1"
      step="any"
      v-model.number="idle_timeout">
      </v-text-field>
    </v-col>
    <v-col cols="12" sm="6" md="4">
      <v-text-field
      :label="$t('transport.pingTimeout')"
      hide-details
      type="number"
      suffix="s"
      min="1"
      step="any"
      v-model.number="ping_timeout">
      </v-text-field>
    </v-col>
  </v-row>
  <v-row v-else>
    <v-col cols="12" sm="6" md="4">
      <v-text-field
      label="ping-interval (s)"
      hide-details
      type="number"
      min="1"
      step="1"
      v-model.number="ping_interval">
      </v-text-field>
    </v-col>
    <v-col cols="12" sm="6" md="4">
      <v-text-field
      label="max-connections"
      hide-details
      type="number"
      min="1"
      step="1"
      v-model.number="max_connections">
      </v-text-field>
    </v-col>
    <v-col cols="12" sm="6" md="4">
      <v-text-field
      label="min-streams"
      hide-details
      type="number"
      min="0"
      step="1"
      v-model.number="min_streams">
      </v-text-field>
    </v-col>
    <v-col cols="12" sm="6" md="4">
      <v-text-field
      label="max-streams"
      hide-details
      type="number"
      min="0"
      step="1"
      v-model.number="max_streams">
      </v-text-field>
    </v-col>
  </v-row>
</template>

<script lang="ts">
import { gRPC } from '../../types/transport'
import { readSingboxDuration, writeSingboxDuration } from '@/plugins/singboxDuration'
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
  methods: {
    normalizePositiveInteger(value: unknown): number | undefined {
      if (value === '' || value === null || value === undefined) return undefined
      const normalized = Number(value)
      return Number.isSafeInteger(normalized) && normalized > 0 ? normalized : undefined
    },
    normalizeNonNegativeInteger(value: unknown): number | undefined {
      if (value === '' || value === null || value === undefined) return undefined
      const normalized = Number(value)
      return Number.isSafeInteger(normalized) && normalized >= 0 ? normalized : undefined
    },
    sanitizeMihomoGRPCNumericFields() {
      if (!this.isMihomo || String(this.$props.transport?.type ?? '').trim().toLowerCase() !== 'grpc') return
      const positiveKeys = ['ping_interval', 'max_connections'] as const
      const nonNegativeKeys = ['min_streams', 'max_streams'] as const
      for (const key of positiveKeys) {
        const normalized = this.normalizePositiveInteger(this.$props.transport[key])
        if (normalized === undefined) delete this.$props.transport[key]
        else this.$props.transport[key] = normalized
      }
      for (const key of nonNegativeKeys) {
        const normalized = this.normalizeNonNegativeInteger(this.$props.transport[key])
        if (normalized === undefined) delete this.$props.transport[key]
        else this.$props.transport[key] = normalized
      }
    },
  },
  computed: {
    isMihomo(): boolean {
      return this.$props.namespace === 'mihomo'
    },
    GRPC(): gRPC {
      return <gRPC> this.$props.transport?? {}
    },
    idle_timeout: {
      get() { return readSingboxDuration(this.GRPC.idle_timeout, 's') ?? '' },
      set(newValue:number) {
        const normalized = writeSingboxDuration(newValue, 's', { minimum: 1 })
        if (normalized === undefined) delete this.$props.transport.idle_timeout
        else this.$props.transport.idle_timeout = normalized
      }
    },
    ping_timeout: {
      get() { return readSingboxDuration(this.GRPC.ping_timeout, 's') ?? '' },
      set(newValue:number) {
        const normalized = writeSingboxDuration(newValue, 's', { minimum: 1 })
        if (normalized === undefined) delete this.$props.transport.ping_timeout
        else this.$props.transport.ping_timeout = normalized
      }
    },
    ping_interval: {
      get() { return this.GRPC.ping_interval ?? '' },
      set(newValue:number) {
        const normalized = this.normalizePositiveInteger(newValue)
        if (normalized === undefined) delete this.$props.transport.ping_interval
        else this.$props.transport.ping_interval = normalized
      },
    },
    max_connections: {
      get() { return this.GRPC.max_connections ?? '' },
      set(newValue:number) {
        const normalized = this.normalizePositiveInteger(newValue)
        if (normalized === undefined) delete this.$props.transport.max_connections
        else this.$props.transport.max_connections = normalized
      },
    },
    min_streams: {
      get() { return this.GRPC.min_streams ?? '' },
      set(newValue:number) {
        const normalized = this.normalizeNonNegativeInteger(newValue)
        if (normalized === undefined) delete this.$props.transport.min_streams
        else this.$props.transport.min_streams = normalized
      },
    },
    max_streams: {
      get() { return this.GRPC.max_streams ?? '' },
      set(newValue:number) {
        const normalized = this.normalizeNonNegativeInteger(newValue)
        if (normalized === undefined) delete this.$props.transport.max_streams
        else this.$props.transport.max_streams = normalized
      },
    }
  },
  watch: {
    transport: {
      handler() {
        this.sanitizeMihomoGRPCNumericFields()
      },
      immediate: true,
    },
  },
}
</script>
