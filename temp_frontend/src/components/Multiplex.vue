<template>
  <v-card :subtitle="$t('objects.multiplex')">
    <v-row>
      <v-col cols="12" sm="6" md="4">
        <v-switch color="primary" :label="$t('mux.enable')" v-model="muxEnable" hide-details></v-switch>
      </v-col>
      <template v-if="mux.enabled">
        <template v-if="direction=='out'">
          <v-col cols="12" sm="6" md="4">
            <v-select
              hide-details
              :items="[ 'smux', 'yamux', 'h2mux']"
              :label="$t('protocol')"
              clearable
              @click:clear="mux.protocol=undefined"
              v-model="mux.protocol">
            </v-select>
          </v-col>
          <v-col cols="12" sm="6" md="4">
            <v-text-field
            :label="$t('mux.maxConn')"
            hide-details
            type="number"
            min=0
            step="1"
            v-model.number="max_connections">
            </v-text-field>
          </v-col>
          <v-col cols="12" sm="6" md="4">
            <v-text-field
            :label="$t('mux.minStr')"
            hide-details
            type="number"
            min=0
            step="1"
            v-model.number="min_streams">
            </v-text-field>
          </v-col>
          <v-col cols="12" sm="6" md="4">
            <v-text-field
            :label="$t('mux.maxStr')"
            hide-details
            type="number"
            :min="min_streams"
            step="1"
            v-model.number="max_streams">
            </v-text-field>
          </v-col>
        </template>
        <v-col cols="12" sm="6" md="4">
          <v-switch color="primary" :label="$t('mux.padding')" v-model="mux.padding" hide-details></v-switch>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-switch color="primary" :label="$t('mux.enableBrutal')" v-model="burtalEnable" hide-details></v-switch>
        </v-col>
      </template>
    </v-row>
    <v-row v-if="mux.brutal?.enabled">
      <v-col cols="12" sm="6" md="4">
        <v-text-field
        :label="$t('stats.upload')"
        hide-details
        type="number"
        min="0"
        step="1"
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
        step="1"
        v-model.number="down_mbps">
        </v-text-field>
      </v-col>
    </v-row>
  </v-card>
</template>

<script lang="ts">
import { oMultiplex } from '@/types/multiplex'
export default {
  props: ['data', 'direction'],
  data() {
    return {}
  },
  methods: {
    normalizeNonNegativeInteger(value: unknown): number | undefined {
      if (value === '' || value === null || value === undefined) return undefined
      const normalized = Number(value)
      return Number.isSafeInteger(normalized) && normalized >= 0 ? normalized : undefined
    },
    sanitizeMultiplexNumericFields() {
      const data = this.$props.data as Record<string, any>
      const rawMultiplex = data?.multiplex
      if (!rawMultiplex || typeof rawMultiplex !== 'object' || Array.isArray(rawMultiplex)) {
        if (data?.multiplex !== undefined) delete data.multiplex
        return
      }
      const mux = rawMultiplex as Record<string, any>
      if (mux.enabled !== true) {
        delete data.multiplex
        return
      }
      mux.enabled = true
      const protocol = typeof mux.protocol === 'string' ? mux.protocol.trim().toLowerCase() : ''
      if (mux.protocol !== undefined) {
        if (['smux', 'yamux', 'h2mux'].includes(protocol)) mux.protocol = protocol
        else delete mux.protocol
      }
      for (const key of ['max_connections', 'min_streams', 'max_streams']) {
        const normalized = this.normalizeNonNegativeInteger(mux[key])
        if (normalized === undefined) delete mux[key]
        else mux[key] = normalized
      }
      if (!mux.brutal || typeof mux.brutal !== 'object' || Array.isArray(mux.brutal)) {
        delete mux.brutal
        return
      }
      for (const key of ['up_mbps', 'down_mbps']) {
        const normalized = this.normalizeNonNegativeInteger(mux.brutal[key])
        if (normalized === undefined) delete mux.brutal[key]
        else mux.brutal[key] = normalized
      }
    },
  },
  mounted() {
    this.sanitizeMultiplexNumericFields()
  },
  watch: {
    data() {
      this.sanitizeMultiplexNumericFields()
    },
    direction() {
      this.sanitizeMultiplexNumericFields()
    },
  },
  computed: {
    mux(): oMultiplex {
      const multiplex = this.$props.data.multiplex
      return multiplex && typeof multiplex === 'object' && !Array.isArray(multiplex)
        ? <oMultiplex>multiplex
        : <oMultiplex>{}
    },
    muxEnable: {
      get(): boolean { return this.mux.enabled === true },
      set(newValue:boolean) {
        if (newValue) this.$props.data.multiplex = { enabled: true }
        else delete this.$props.data.multiplex
      }
    },
    max_connections: {
      get(): number { return this.mux.max_connections ? this.mux.max_connections : 0 },
      set(newValue:number) {
        const normalized = this.normalizeNonNegativeInteger(newValue)
        if (normalized === undefined) delete this.mux.max_connections
        else this.mux.max_connections = normalized
      }
    },
    min_streams: {
      get(): number { return this.mux.min_streams ? this.mux.min_streams : 0 },
      set(newValue:number) {
        const normalized = this.normalizeNonNegativeInteger(newValue)
        if (normalized === undefined) delete this.mux.min_streams
        else this.mux.min_streams = normalized
      }
    },
    max_streams: {
      get(): number { return this.mux.max_streams ? this.mux.max_streams : 0 },
      set(newValue:number) {
        const normalized = this.normalizeNonNegativeInteger(newValue)
        if (normalized === undefined) delete this.mux.max_streams
        else this.mux.max_streams = normalized
      }
    },
    burtalEnable: {
      get(): boolean { return this.mux.brutal ? this.mux.brutal.enabled : false },
      set(newValue:boolean) {
        if (newValue) this.mux.brutal = { enabled: true, up_mbps: 100, down_mbps: 100 }
        else delete this.mux.brutal
      }
    },
    down_mbps: {
      get() { return this.mux.brutal && this.mux.brutal.down_mbps ? this.mux.brutal.down_mbps : 0 },
      set(newValue:any) { 
        if (this.mux.brutal){
          const normalized = this.normalizeNonNegativeInteger(newValue)
          if (normalized === undefined) delete (this.mux.brutal as Partial<{ down_mbps: number }>).down_mbps
          else this.mux.brutal.down_mbps = normalized
        }
      }
    },
    up_mbps: {
      get() { return this.mux.brutal && this.mux.brutal.up_mbps ? this.mux.brutal.up_mbps : 0 },
      set(newValue:any) {
        if (this.mux.brutal){
          const normalized = this.normalizeNonNegativeInteger(newValue)
          if (normalized === undefined) delete (this.mux.brutal as Partial<{ up_mbps: number }>).up_mbps
          else this.mux.brutal.up_mbps = normalized
        }
      }
    },
  }
}
</script>
