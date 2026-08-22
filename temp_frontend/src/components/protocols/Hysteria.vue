<template>
  <v-card subtitle="Hysteria">
    <v-row>
      <v-col cols="12" sm="6" md="4">
        <v-text-field
        :label="$t('stats.upload')"
        hide-details
        type="number"
        min="0"
        step="1"
        :suffix="$t('stats.Mbps')"
        v-model="up_mbps">
        </v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-text-field
        :label="$t('stats.download')"
        hide-details
        type="number"
        :suffix="$t('stats.Mbps')"
        min="0"
        step="1"
        v-model="down_mbps">
        </v-text-field>
      </v-col>
    </v-row>
    <v-row v-if="optionObfs || direction=='out'">
      <v-col cols="12" sm="6" md="4" v-if="optionObfs">
       <v-text-field
       :label="$t('types.hy.obfs')"
        hide-details
        v-model="data.obfs">
        </v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="direction=='out'">
        <v-text-field
        :label="$t('types.hy.auth')"
        hide-details
        v-model="data.auth_str">
        </v-text-field>
      </v-col>
    </v-row>
    <v-row v-if="direction=='out' && namespace !== 'mihomo'">
      <v-col cols="12" sm="6" md="4">
        <Network :data="data" />
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="6" md="4" v-if="data.stream_receive_window != undefined">
        <v-text-field
        label="Stream receive window"
        :placeholder="receiveWindowPlaceholders.stream"
        hide-details
        type="number"
        min="0"
        step="1"
        v-model="streamReceiveWindow">
        </v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="data.connection_receive_window != undefined">
        <v-text-field
        label="Connection receive window"
        :placeholder="receiveWindowPlaceholders.connection"
        hide-details
        type="number"
        min="0"
        step="1"
        v-model="connectionReceiveWindow">
        </v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="data.max_concurrent_streams != undefined">
        <v-text-field
        label="Max concurrent streams"
        hide-details
        type="number"
        min="0"
        step="1"
        v-model="maxConcurrentStreams">
        </v-text-field>
      </v-col>
    </v-row>
    <v-card-actions>
      <v-spacer></v-spacer>
      <v-menu v-model="menu" :close-on-content-click="false" location="start">
        <template v-slot:activator="{ props }">
          <v-btn v-bind="props" hide-details variant="tonal">{{ $t('types.hy.hyOptions') }}</v-btn>
        </template>
        <v-card>
          <v-list>
            <v-list-item v-if="showMihomoFastOpenOption">
              <v-switch v-model="optionMihomoFastOpen" color="primary" label="fast-open(mihomo)" hide-details></v-switch>
            </v-list-item>
            <v-list-item v-if="direction == 'in'">
              <v-switch v-model="optionQuicGo" color="primary" label="quic-go" hide-details></v-switch>
            </v-list-item>
            <template v-else>
              <v-list-item>
                <v-switch v-model="optionStreamReceiveWindow" color="primary" label="Stream receive window" hide-details></v-switch>
              </v-list-item>
              <v-list-item>
                <v-switch v-model="optionConnectionReceiveWindow" color="primary" label="Connection receive window" hide-details></v-switch>
              </v-list-item>
            </template>
            <v-list-item>
              <v-switch v-model="optionMaxConcurrentStreams" color="primary" label="Max concurrent streams" hide-details></v-switch>
            </v-list-item>
            <v-list-item>
              <v-switch v-model="optionObfs" color="primary" :label="$t('types.hy.obfs')" hide-details></v-switch>
            </v-list-item>
            <v-list-item>
              <v-switch v-model="data.disable_path_mtu_discovery" color="primary" label="Disable path MTU discovery" hide-details></v-switch>
            </v-list-item>
          </v-list>
        </v-card>
      </v-menu>
    </v-card-actions>
  </v-card>
</template>

<script lang="ts">
import Network from '@/components/Network.vue'
import { parseSingboxInteger } from '@/plugins/singboxInteger'

const hysteriaReceiveWindowDefaults = {
  stream: 38000000,
  connection: 150000000,
} as const

function formatBytePlaceholder(value: number): string {
  return `${value}_${value / 1000000}MB`
}

export default {
  props: {
    direction: {
      type: String,
      required: true,
    },
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
      menu: false,
      receiveWindowPlaceholders: {
        stream: formatBytePlaceholder(hysteriaReceiveWindowDefaults.stream),
        connection: formatBytePlaceholder(hysteriaReceiveWindowDefaults.connection),
      },
    }
  },
  methods: {
    bandwidthKey(direction: 'up' | 'down'): string {
      return this.$props.direction === 'out' ? `${direction}_mbps` : `server_${direction}_mbps`
    },
    sanitizeOutboundState() {
      if (this.$props.direction !== 'out' || !this.$props.data) return

      for (const direction of ['up', 'down'] as const) {
        const legacyKey = `server_${direction}_mbps`
        const canonicalKey = `${direction}_mbps`
        const legacyValue = parseSingboxInteger(this.$props.data[legacyKey], { min: 0 })
        if (legacyValue !== undefined) {
          this.$props.data[canonicalKey] = legacyValue
        }
        delete this.$props.data[legacyKey]
      }

      if (this.$props.namespace === 'mihomo') {
        delete this.$props.data.network
      }
    },
  },
  computed: {
    showMihomoFastOpenOption(): boolean {
      return this.$props.direction === 'in' || (this.$props.direction === 'out' && this.$props.namespace === 'mihomo')
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
        return this.mihomoFastOpenStore.mihomo_fast_open !== false
      },
      set(v:boolean) {
        if (this.showMihomoFastOpenOption) {
          this.mihomoFastOpenStore.mihomo_fast_open = v
        }
      }
    },
    optionQuicGo: {
      get(): boolean {
        return this.$props.direction === 'in' && (
          this.$props.data.stream_receive_window != undefined ||
          this.$props.data.connection_receive_window != undefined
        )
      },
      set(v: boolean) {
        if (this.$props.direction !== 'in') return
        if (v) {
          if (this.$props.data.stream_receive_window == undefined) {
            this.$props.data.stream_receive_window = hysteriaReceiveWindowDefaults.stream
          }
          if (this.$props.data.connection_receive_window == undefined) {
            this.$props.data.connection_receive_window = hysteriaReceiveWindowDefaults.connection
          }
        } else {
          this.$props.data.stream_receive_window = undefined
          this.$props.data.connection_receive_window = undefined
        }
      }
    },
    optionObfs: {
      get(): boolean { return this.$props.data.obfs != undefined },
      set(v:boolean) { this.$props.data.obfs = v ? '' : undefined }
    },
    optionStreamReceiveWindow: {
      get(): boolean { return this.$props.data.stream_receive_window != undefined },
      set(v:boolean) { this.$props.data.stream_receive_window = v ? hysteriaReceiveWindowDefaults.stream : undefined }
    },
    optionConnectionReceiveWindow: {
      get(): boolean { return this.$props.data.connection_receive_window != undefined },
      set(v:boolean) { this.$props.data.connection_receive_window = v ? hysteriaReceiveWindowDefaults.connection : undefined }
    },
    optionMaxConcurrentStreams: {
      get(): boolean { return this.$props.data.max_concurrent_streams != undefined },
      set(v:boolean) { this.$props.data.max_concurrent_streams = v ? 1024 : undefined }
    },
    down_mbps: {
      get() { return parseSingboxInteger(this.$props.data[this.bandwidthKey('down')], { min: 0 }) ?? 500 },
      set(newValue:unknown) {
        this.$props.data[this.bandwidthKey('down')] = parseSingboxInteger(newValue, { min: 0 }) ?? 500
        delete this.$props.data.down
      }
    },
    up_mbps: {
      get() { return parseSingboxInteger(this.$props.data[this.bandwidthKey('up')], { min: 0 }) ?? 500 },
      set(newValue:unknown) {
        this.$props.data[this.bandwidthKey('up')] = parseSingboxInteger(newValue, { min: 0 }) ?? 500
      }
    },
    streamReceiveWindow: {
      get() { return parseSingboxInteger(this.$props.data.stream_receive_window, { min: 0 }) ?? '' },
      set(value:unknown) { this.$props.data.stream_receive_window = parseSingboxInteger(value, { min: 0 }) }
    },
    connectionReceiveWindow: {
      get() { return parseSingboxInteger(this.$props.data.connection_receive_window, { min: 0 }) ?? '' },
      set(value:unknown) { this.$props.data.connection_receive_window = parseSingboxInteger(value, { min: 0 }) }
    },
    maxConcurrentStreams: {
      get() { return parseSingboxInteger(this.$props.data.max_concurrent_streams, { min: 0 }) ?? '' },
      set(value:unknown) { this.$props.data.max_concurrent_streams = parseSingboxInteger(value, { min: 0 }) }
    },
  },
  mounted() {
    this.sanitizeOutboundState()
  },
  watch: {
    data() {
      this.sanitizeOutboundState()
    },
    direction() {
      this.sanitizeOutboundState()
    },
    namespace() {
      this.sanitizeOutboundState()
    },
  },
  components: { Network }
}
</script>
