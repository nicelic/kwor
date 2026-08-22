<template>
  <v-card :subtitle="$t('objects.listen')">
    <v-row>
      <v-col cols="12" sm="6" md="4">
        <v-text-field
        :label="$t('in.addr')"
        hide-details
        required
        v-model="data.listen">
        </v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-text-field
        :label="$t('in.port')"
        hide-details
        type="number"
        min="1"
        max="65535"
        step="1"
        required
        v-model="listenPort"
        @blur="onListenPortBlur"></v-text-field>
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="6" md="4" v-if="!disableDetourOption && optionDetour">
        <v-select
        :label="$t('listen.detourText')"
        hide-details
        :items="detourTags"
        v-model="data.detour">
        </v-select>
      </v-col>
    </v-row>
    <v-row v-if="!disableTcpOptions && optionTCP">
      <v-col cols="12" sm="6" md="4">
        <v-switch v-model="data.tcp_fast_open" color="primary" label="TCP Fast Open" hide-details></v-switch>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-switch v-model="data.tcp_multi_path" color="primary" label="TCP Multi Path" hide-details></v-switch>
      </v-col>
    </v-row>
    <v-row v-if="!disableUdpOptions && optionUDP">
      <v-col cols="12" sm="6" md="4">
        <v-switch v-model="data.udp_fragment" color="primary" label="UDP Fragment" hide-details></v-switch>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-text-field
        label="UDP NAT expiration"
        hide-details
        type="number"
        min="1"
        :suffix="$t('date.m')"
        v-model.number="udpTimeout"></v-text-field>
      </v-col>
    </v-row>
    <v-card-actions class="pt-0">
      <v-spacer></v-spacer>
      <v-menu v-if="showOptionsMenu" v-model="menu" :close-on-content-click="false" location="start">
        <template v-slot:activator="{ props }">
          <v-btn v-bind="props" hide-details variant="tonal">{{ $t('listen.options') }}</v-btn>
        </template>
        <v-card>
          <v-list>
            <v-list-item v-if="!disableDetourOption">
              <v-switch
                v-model="optionDetour"
                color="primary"
                :label="$t('listen.detour')"
                :disabled="!optionDetour && !canEnableDetour"
                hide-details
              ></v-switch>
            </v-list-item>
            <v-list-item v-if="!disableTcpOptions">
              <v-switch v-model="optionTCP" color="primary" :label="$t('listen.tcpOptions')" hide-details></v-switch>
            </v-list-item>
            <v-list-item v-if="!disableUdpOptions">
              <v-switch v-model="optionUDP" color="primary" :label="$t('listen.udpOptions')" hide-details></v-switch>
            </v-list-item>
          </v-list>
        </v-card>
      </v-menu>
    </v-card-actions>
  </v-card>
</template>

<script lang="ts">
import { readSingboxDuration, writeSingboxDuration } from '@/plugins/singboxDuration'
import { parseSingboxInteger } from '@/plugins/singboxInteger'

export default {
  props: {
    data: { type: Object, required: true },
    inTags: { type: Array, default: () => [] },
    disableDetourOption: { type: Boolean, default: false },
    disableTcpOptions: { type: Boolean, default: false },
    disableUdpOptions: { type: Boolean, default: false },
  },
  emits: ['listen-port-blur'],
  data() {
    return {
      menu: false
    }
  },
  methods: {
    enforceDisabledOptions() {
      if (this.disableDetourOption) {
        delete this.$props.data.detour
      }
      if (this.disableTcpOptions) {
        delete this.$props.data.tcp_fast_open
        delete this.$props.data.tcp_multi_path
      }
      if (this.disableUdpOptions) {
        delete this.$props.data.udp_fragment
        delete this.$props.data.udp_timeout
      }
    },
    onListenPortBlur() {
      this.$emit('listen-port-blur', this.$props.data.listen_port)
    }
  },
  mounted() {
    this.enforceDisabledOptions()
  },
  computed: {
    showOptionsMenu(): boolean {
      return !this.disableDetourOption || !this.disableTcpOptions || !this.disableUdpOptions
    },
    eligibleDetourTags(): string[] {
      const currentTag = typeof this.$props.data?.tag === 'string'
        ? this.$props.data.tag.trim()
        : ''
      const source = Array.isArray(this.$props.inTags) ? this.$props.inTags : []
      return [...new Set(source
        .filter((tag: unknown): tag is string => typeof tag === 'string')
        .map((tag: string) => tag.trim())
        .filter((tag: string) => tag.length > 0 && tag !== currentTag))]
    },
    detourTags(): string[] {
      const currentDetour = typeof this.$props.data?.detour === 'string'
        ? this.$props.data.detour.trim()
        : ''
      if (currentDetour && !this.eligibleDetourTags.includes(currentDetour)) {
        return [currentDetour, ...this.eligibleDetourTags]
      }
      return this.eligibleDetourTags
    },
    canEnableDetour(): boolean {
      return this.eligibleDetourTags.length > 0
    },
    udpTimeout: {
      get() { return readSingboxDuration(this.$props.data.udp_timeout, 'm') ?? 5 },
      set(newValue:unknown) { this.$props.data.udp_timeout = writeSingboxDuration(newValue, 'm', { minimum: 1 }) ?? '5m' }
    },
    listenPort: {
      get() { return parseSingboxInteger(this.$props.data.listen_port, { min: 1, max: 65535 }) ?? '' },
      set(value:unknown) { this.$props.data.listen_port = parseSingboxInteger(value, { min: 1, max: 65535 }) }
    },
    optionTCP: {
      get(): boolean { 
        return this.$props.data.tcp_fast_open != undefined ||
               this.$props.data.tcp_multi_path != undefined
      },
      set(v:boolean) {
        if (v) {
          this.$props.data.tcp_fast_open = false
          this.$props.data.tcp_multi_path = false
          return
        }
        delete this.$props.data.tcp_fast_open
        delete this.$props.data.tcp_multi_path
      }
    },
    optionUDP: {
      get(): boolean { 
        return this.$props.data.udp_fragment != undefined ||
               this.$props.data.udp_timeout != undefined
      },
      set(v:boolean) {
        if (v) {
          this.$props.data.udp_fragment = false
          this.$props.data.udp_timeout = '5m'
          return
        }
        delete this.$props.data.udp_fragment
        delete this.$props.data.udp_timeout
      }
    },
    optionDetour: {
      get(): boolean { return this.$props.data.detour != undefined },
      set(v:boolean) {
        if (!v) {
          delete this.$props.data.detour
          return
        }
        const detour = this.eligibleDetourTags[0]
        if (detour) this.$props.data.detour = detour
      }
    }
  },
  watch: {
    disableDetourOption() {
      this.enforceDisabledOptions()
    },
    disableTcpOptions() {
      this.enforceDisabledOptions()
    },
    disableUdpOptions() {
      this.enforceDisabledOptions()
    },
    data: {
      deep: true,
      handler() {
        this.enforceDisabledOptions()
      },
    },
  }
}
</script>
