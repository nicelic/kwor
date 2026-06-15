<template>
  <v-card subtitle="Hysteria">
    <v-row>
      <v-col cols="12" sm="6" md="4">
        <v-text-field
        :label="$t('stats.upload')"
        hide-details
        type="number"
        :suffix="$t('stats.Mbps')"
        v-model.number="up_mbps">
        </v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-text-field
        :label="$t('stats.download')"
        hide-details
        type="number"
        :suffix="$t('stats.Mbps')"
        min="0"
        v-model.number="down_mbps">
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
    <v-row v-if="direction=='out'">
      <v-col cols="12" sm="6" md="4">
        <Network :data="data" />
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="6" md="4" v-if="data.stream_receive_window != undefined">
        <v-text-field
        label="Stream receive window"
        hide-details
        type="number"
        min="0"
        v-model.number="data.stream_receive_window">
        </v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="data.connection_receive_window != undefined">
        <v-text-field
        label="Connection receive window"
        hide-details
        type="number"
        min="0"
        v-model.number="data.connection_receive_window">
        </v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="data.max_concurrent_streams != undefined">
        <v-text-field
        label="Max concurrent streams"
        hide-details
        type="number"
        min="0"
        v-model.number="data.max_concurrent_streams">
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
              <v-switch v-model="optionMihomoFastOpen" color="primary" label="fast-open" hide-details></v-switch>
            </v-list-item>
            <v-list-item>
              <v-switch v-model="optionStreamReceiveWindow" color="primary" label="Stream receive window" hide-details></v-switch>
            </v-list-item>
            <v-list-item>
              <v-switch v-model="optionConnectionReceiveWindow" color="primary" label="Connection receive window" hide-details></v-switch>
            </v-list-item>
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
    }
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
    optionObfs: {
      get(): boolean { return this.$props.data.obfs != undefined },
      set(v:boolean) { this.$props.data.obfs = v ? '' : undefined }
    },
    optionStreamReceiveWindow: {
      get(): boolean { return this.$props.data.stream_receive_window != undefined },
      set(v:boolean) { this.$props.data.stream_receive_window = v ? 25000000 : undefined }
    },
    optionConnectionReceiveWindow: {
      get(): boolean { return this.$props.data.connection_receive_window != undefined },
      set(v:boolean) { this.$props.data.connection_receive_window = v ? 67108864 : undefined }
    },
    optionMaxConcurrentStreams: {
      get(): boolean { return this.$props.data.max_concurrent_streams != undefined },
      set(v:boolean) { this.$props.data.max_concurrent_streams = v ? 1024 : undefined }
    },
    down_mbps: {
      get() { return this.$props.data.server_down_mbps ?? 2000 },
      set(newValue:number) {
        this.$props.data.server_down_mbps = Number.isFinite(newValue) ? newValue : 2000
        delete this.$props.data.down
      }
    },
    up_mbps: {
      get() { return this.$props.data.server_up_mbps ?? 2000 },
      set(newValue:number) { this.$props.data.server_up_mbps = Number.isFinite(newValue) ? newValue : 2000 }
    },
  },
  components: { Network }
}
</script>
