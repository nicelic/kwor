<template>
  <v-row>
    <v-col cols="12" sm="6" md="4">
      <v-text-field
      label="Host"
      hide-details
      v-model="transport.host">
      </v-text-field>
    </v-col>
    <v-col cols="12" sm="6" md="4">
      <v-text-field
      :label="$t('transport.path')"
      hide-details
      v-model="transport.path">
      </v-text-field>
    </v-col>
    <v-col cols="12" sm="6" md="4">
      <v-select
      label="Mode"
      hide-details
      clearable
      :items="modeList"
      v-model="transport.mode">
      </v-select>
    </v-col>
  </v-row>
  <v-row>
    <v-col cols="12" sm="6" md="4">
      <v-switch
      color="primary"
      label="no-grpc-header"
      hide-details
      v-model="transport.no_grpc_header">
      </v-switch>
    </v-col>
    <v-col cols="12" sm="6" md="4">
      <v-text-field
      label="x-padding-bytes"
      hide-details
      v-model="transport.x_padding_bytes">
      </v-text-field>
    </v-col>
    <v-col cols="12" sm="6" md="4">
      <v-text-field
      label="sc-max-each-post-bytes"
      hide-details
      type="number"
      min="1"
      v-model.number="scMaxEachPostBytes">
      </v-text-field>
    </v-col>
  </v-row>
  <v-card style="margin-bottom: 8px;">
    <v-card-subtitle>reuse-settings (XMUX)</v-card-subtitle>
    <v-row>
      <v-col cols="12" sm="6" md="4">
        <v-switch
        color="primary"
        label="Enable reuse-settings"
        hide-details
        v-model="reuseEnabled">
        </v-switch>
      </v-col>
    </v-row>
    <v-row v-if="reuseEnabled">
      <v-col cols="12" sm="6" md="4">
        <v-text-field
        label="max-connections"
        hide-details
        v-model="reuseMaxConnections">
        </v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-text-field
        label="max-concurrency"
        hide-details
        v-model="reuseMaxConcurrency">
        </v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-text-field
        label="c-max-reuse-times"
        hide-details
        v-model="reuseCMaxReuseTimes">
        </v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-text-field
        label="h-max-request-times"
        hide-details
        v-model="reuseHMaxRequestTimes">
        </v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-text-field
        label="h-max-reusable-secs"
        hide-details
        v-model="reuseHMaxReusableSecs">
        </v-text-field>
      </v-col>
    </v-row>
  </v-card>
  <Headers :data="transport" />
</template>

<script lang="ts">
import { XHTTP } from '../../types/transport'
import Headers from '../Headers.vue'

type ReuseSettings = {
  max_connections?: string
  max_concurrency?: string
  c_max_reuse_times?: string
  h_max_request_times?: string
  h_max_reusable_secs?: string
}

export default {
  props: ['transport'],
  data() {
    return {
      modeList: ['auto', 'stream-one', 'stream-up', 'packet-up'],
    }
  },
  computed: {
    xhttp(): XHTTP {
      return <XHTTP>this.$props.transport ?? {}
    },
    scMaxEachPostBytes: {
      get() {
        return this.xhttp.sc_max_each_post_bytes ?? ''
      },
      set(newValue: number) {
        if (newValue && newValue > 0) {
          this.$props.transport.sc_max_each_post_bytes = newValue
        } else {
          delete this.$props.transport.sc_max_each_post_bytes
        }
      },
    },
    reuseEnabled: {
      get(): boolean {
        const reuse = this.xhttp.reuse_settings
        return !!reuse && typeof reuse === 'object'
      },
      set(enabled: boolean) {
        if (enabled) {
          this.ensureReuseSettings()
          return
        }
        delete this.$props.transport.reuse_settings
      },
    },
    reuseMaxConnections: {
      get() { return this.readReuseValue('max_connections') },
      set(newValue: string) { this.writeReuseValue('max_connections', newValue) },
    },
    reuseMaxConcurrency: {
      get() { return this.readReuseValue('max_concurrency') },
      set(newValue: string) { this.writeReuseValue('max_concurrency', newValue) },
    },
    reuseCMaxReuseTimes: {
      get() { return this.readReuseValue('c_max_reuse_times') },
      set(newValue: string) { this.writeReuseValue('c_max_reuse_times', newValue) },
    },
    reuseHMaxRequestTimes: {
      get() { return this.readReuseValue('h_max_request_times') },
      set(newValue: string) { this.writeReuseValue('h_max_request_times', newValue) },
    },
    reuseHMaxReusableSecs: {
      get() { return this.readReuseValue('h_max_reusable_secs') },
      set(newValue: string) { this.writeReuseValue('h_max_reusable_secs', newValue) },
    },
  },
  methods: {
    ensureReuseSettings(): ReuseSettings {
      const raw = this.$props.transport.reuse_settings
      if (!raw || typeof raw !== 'object' || Array.isArray(raw)) {
        this.$props.transport.reuse_settings = {}
      }
      return this.$props.transport.reuse_settings as ReuseSettings
    },
    readReuseValue(key: keyof ReuseSettings): string {
      const reuse = this.$props.transport.reuse_settings
      if (!reuse || typeof reuse !== 'object' || Array.isArray(reuse)) return ''
      const value = reuse[key]
      if (typeof value === 'string') return value
      if (typeof value === 'number' && Number.isFinite(value)) return String(value)
      return ''
    },
    writeReuseValue(key: keyof ReuseSettings, value: string) {
      const trimmed = value.trim()
      const reuse = this.ensureReuseSettings()
      if (trimmed.length > 0) {
        reuse[key] = trimmed
      } else {
        delete reuse[key]
      }
      this.cleanupReuseSettings()
    },
    cleanupReuseSettings() {
      const reuse = this.$props.transport.reuse_settings
      if (!reuse || typeof reuse !== 'object' || Array.isArray(reuse)) {
        delete this.$props.transport.reuse_settings
        return
      }
      if (Object.keys(reuse as Record<string, unknown>).length === 0) {
        delete this.$props.transport.reuse_settings
      }
    },
  },
  components: { Headers },
}
</script>
