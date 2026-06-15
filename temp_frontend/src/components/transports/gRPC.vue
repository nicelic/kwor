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
      v-model.number="ping_interval">
      </v-text-field>
    </v-col>
    <v-col cols="12" sm="6" md="4">
      <v-text-field
      label="max-connections"
      hide-details
      type="number"
      min="1"
      v-model.number="max_connections">
      </v-text-field>
    </v-col>
    <v-col cols="12" sm="6" md="4">
      <v-text-field
      label="min-streams"
      hide-details
      type="number"
      min="0"
      v-model.number="min_streams">
      </v-text-field>
    </v-col>
    <v-col cols="12" sm="6" md="4">
      <v-text-field
      label="max-streams"
      hide-details
      type="number"
      min="0"
      v-model.number="max_streams">
      </v-text-field>
    </v-col>
  </v-row>
</template>

<script lang="ts">
import { gRPC } from '../../types/transport'
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
    GRPC(): gRPC {
      return <gRPC> this.$props.transport?? {}
    },
    idle_timeout: {
      get() { return this.GRPC.idle_timeout ? parseInt(this.GRPC.idle_timeout.replace('s','')) : '' },
      set(newValue:number) { this.$props.transport.idle_timeout = newValue ? newValue + 's' : '' }
    },
    ping_timeout: {
      get() { return this.GRPC.ping_timeout ? parseInt(this.GRPC.ping_timeout.replace('s','')) : '' },
      set(newValue:number) { this.$props.transport.ping_timeout = newValue ? newValue + 's' : '' }
    },
    ping_interval: {
      get() { return this.GRPC.ping_interval ?? '' },
      set(newValue:number) {
        if (newValue && newValue > 0) {
          this.$props.transport.ping_interval = Math.floor(newValue)
        } else {
          delete this.$props.transport.ping_interval
        }
      },
    },
    max_connections: {
      get() { return this.GRPC.max_connections ?? '' },
      set(newValue:number) {
        if (newValue && newValue > 0) {
          this.$props.transport.max_connections = Math.floor(newValue)
        } else {
          delete this.$props.transport.max_connections
        }
      },
    },
    min_streams: {
      get() { return this.GRPC.min_streams ?? '' },
      set(newValue:number) {
        if (newValue === 0 || (newValue && newValue > 0)) {
          this.$props.transport.min_streams = Math.floor(newValue)
        } else {
          delete this.$props.transport.min_streams
        }
      },
    },
    max_streams: {
      get() { return this.GRPC.max_streams ?? '' },
      set(newValue:number) {
        if (newValue === 0 || (newValue && newValue > 0)) {
          this.$props.transport.max_streams = Math.floor(newValue)
        } else {
          delete this.$props.transport.max_streams
        }
      },
    }
  }
}
</script>
