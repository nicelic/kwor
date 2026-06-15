<template>
  <v-card :subtitle="cardSubtitle">
    <template v-if="direction == 'in'">
      <v-row>
        <v-col cols="12" sm="6" md="5">
          <v-select
            v-model="network"
            :items="networkItems"
            label="Network"
            multiple
            chips
            hide-details>
          </v-select>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-select
            v-model="congestionController"
            :items="congestionControllers"
            label="Congestion Control"
            hide-details
            clearable>
          </v-select>
        </v-col>
      </v-row>
    </template>
    <template v-else>
      <v-row v-if="direction == 'out'">
        <v-col cols="12" sm="6">
          <v-text-field
            v-model="data.username"
            label="Username"
            hide-details>
          </v-text-field>
        </v-col>
        <v-col cols="12" sm="6">
          <v-text-field
            v-model="data.password"
            label="Password"
            hide-details>
          </v-text-field>
        </v-col>
      </v-row>
      <v-row>
        <v-col cols="12" sm="6" md="3">
          <v-select
            v-model="congestionController"
            :items="congestionControllers"
            label="Congestion Control"
            hide-details
            clearable>
          </v-select>
        </v-col>
        <v-col cols="12" sm="6" md="3" v-if="!hideCommonUDPField">
          <v-switch
            v-model="data.udp"
            color="primary"
            label="UDP"
            hide-details>
          </v-switch>
        </v-col>
        <v-col cols="12" sm="6" md="3">
          <v-switch
            v-model="data.health_check"
            color="primary"
            label="Health Check"
            hide-details>
          </v-switch>
        </v-col>
        <v-col cols="12" sm="6" md="3" v-if="showReuseOptions">
          <v-switch
            v-model="reuseOptionsEnabled"
            color="primary"
            label="Reuse Options"
            hide-details>
          </v-switch>
        </v-col>
      </v-row>
      <v-row v-if="showReuseOptions && reuseOptionsEnabled">
        <v-col cols="12" sm="6" md="4">
          <v-text-field
            v-model.number="maxConnections"
            label="max-connections"
            type="number"
            min="0"
            step="1"
            hide-details>
          </v-text-field>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-text-field
            v-model.number="minStreams"
            label="min-streams"
            type="number"
            min="0"
            step="1"
            hide-details>
          </v-text-field>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-text-field
            v-model.number="maxStreams"
            label="max-streams"
            type="number"
            min="0"
            step="1"
            hide-details>
          </v-text-field>
        </v-col>
      </v-row>
      <v-row>
        <v-col cols="12" sm="6" md="4">
          <v-switch
            v-model="data.quic"
            color="primary"
            label="QUIC"
            hide-details>
          </v-switch>
        </v-col>
      </v-row>
    </template>
  </v-card>
</template>

<script lang="ts">
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
      networkItems: ['tcp', 'udp'],
      congestionControllers: [
        { title: 'Default', value: '' },
        { title: 'BBR', value: 'bbr' },
        { title: 'Cubic', value: 'cubic' },
        { title: 'New Reno', value: 'new_reno' },
      ],
    }
  },
  created() {
    this.clearClientSideNetwork()
  },
  methods: {
    syncCompatibilityFields() {
      if (this.$props.direction === 'in' || !this.$props.data) return

      const rawProxy = this.$props.data._mihomo_clash_proxy
      if (this.$props.data.udp === undefined) {
        if (typeof rawProxy?.udp === 'boolean') {
          this.$props.data.udp = rawProxy.udp
        } else if (Array.isArray(this.$props.data.network)) {
          this.$props.data.udp = this.$props.data.network.includes('udp')
        } else if (typeof this.$props.data.network === 'string') {
          this.$props.data.udp = this.$props.data.network.trim().toLowerCase() === 'udp'
        }
      }
      if (this.$props.data.health_check === undefined) {
        if (typeof this.$props.data['health-check'] === 'boolean') {
          this.$props.data.health_check = this.$props.data['health-check']
        } else if (typeof rawProxy?.['health-check'] === 'boolean') {
          this.$props.data.health_check = rawProxy['health-check']
        }
      }

      this.syncReuseOptionsCompatibility(rawProxy)

      delete this.$props.data['health-check']
    },
    parseNonNegativeInteger(raw: unknown): number | undefined {
      if (typeof raw === 'number' && Number.isFinite(raw) && raw >= 0) {
        return Math.floor(raw)
      }
      if (typeof raw === 'string') {
        const trimmed = raw.trim()
        if (/^\d+$/.test(trimmed)) {
          return parseInt(trimmed, 10)
        }
      }
      return undefined
    },
    readFirstNonNegativeInteger(...rawValues: unknown[]): number | undefined {
      for (const value of rawValues) {
        const parsed = this.parseNonNegativeInteger(value)
        if (parsed !== undefined) {
          return parsed
        }
      }
      return undefined
    },
    syncReuseOptionsCompatibility(rawProxy?: Record<string, unknown>) {
      if (!this.$props.data) return

      const maxConnections = this.readFirstNonNegativeInteger(
        this.$props.data.max_connections,
        this.$props.data['max-connections'],
        rawProxy?.['max-connections'],
      )
      if (maxConnections !== undefined) {
        this.$props.data.max_connections = maxConnections
      }

      const minStreams = this.readFirstNonNegativeInteger(
        this.$props.data.min_streams,
        this.$props.data['min-streams'],
        rawProxy?.['min-streams'],
      )
      if (minStreams !== undefined) {
        this.$props.data.min_streams = minStreams
      }

      const maxStreams = this.readFirstNonNegativeInteger(
        this.$props.data.max_streams,
        this.$props.data['max-streams'],
        rawProxy?.['max-streams'],
      )
      if (maxStreams !== undefined) {
        this.$props.data.max_streams = maxStreams
      }

      delete this.$props.data['max-connections']
      delete this.$props.data['min-streams']
      delete this.$props.data['max-streams']
    },
    clearClientSideNetwork() {
      if (this.$props.direction !== 'in' && this.$props.data) {
        this.syncCompatibilityFields()
        delete this.$props.data.network
      }
    },
  },
  computed: {
    cardSubtitle(): string | undefined {
      return this.$props.direction === 'out_json' ? undefined : 'TrustTunnel'
    },
    showReuseOptions(): boolean {
      return this.$props.namespace === 'mihomo' && this.$props.direction !== 'in'
    },
    hideCommonUDPField(): boolean {
      return this.$props.namespace === 'mihomo' && this.$props.direction === 'out_json'
    },
    network: {
      get(): string[] {
        return Array.isArray(this.$props.data.network) && this.$props.data.network.length > 0
          ? this.$props.data.network
          : ['tcp']
      },
      set(v: string[]) {
        const normalized = (v ?? []).filter((item, index, list) => (item === 'tcp' || item === 'udp') && list.indexOf(item) === index)
        this.$props.data.network = normalized.length > 0 ? normalized : undefined
      }
    },
    congestionController: {
      get(): string {
        return this.$props.data.congestion_controller ?? 'bbr'
      },
      set(v: string) {
        this.$props.data.congestion_controller = v && v.trim() !== '' ? v : 'bbr'
      }
    },
    reuseOptionsEnabled: {
      get(): boolean {
        return this.parseNonNegativeInteger(this.$props.data.max_connections) !== undefined
          || this.parseNonNegativeInteger(this.$props.data.min_streams) !== undefined
          || this.parseNonNegativeInteger(this.$props.data.max_streams) !== undefined
      },
      set(v: boolean) {
        if (!v) {
          delete this.$props.data.max_connections
          delete this.$props.data.min_streams
          delete this.$props.data.max_streams
          return
        }

        if (this.parseNonNegativeInteger(this.$props.data.max_connections) === undefined) {
          this.$props.data.max_connections = 1
        }
        if (this.parseNonNegativeInteger(this.$props.data.min_streams) === undefined) {
          this.$props.data.min_streams = 0
        }
        if (this.parseNonNegativeInteger(this.$props.data.max_streams) === undefined) {
          this.$props.data.max_streams = 0
        }
      }
    },
    maxConnections: {
      get(): number | '' {
        const value = this.parseNonNegativeInteger(this.$props.data.max_connections)
        return value === undefined ? '' : value
      },
      set(newValue: number) {
        const value = this.parseNonNegativeInteger(newValue)
        if (value === undefined) {
          delete this.$props.data.max_connections
        } else {
          this.$props.data.max_connections = value
        }
      },
    },
    minStreams: {
      get(): number | '' {
        const value = this.parseNonNegativeInteger(this.$props.data.min_streams)
        return value === undefined ? '' : value
      },
      set(newValue: number) {
        const value = this.parseNonNegativeInteger(newValue)
        if (value === undefined) {
          delete this.$props.data.min_streams
        } else {
          this.$props.data.min_streams = value
        }
      },
    },
    maxStreams: {
      get(): number | '' {
        const value = this.parseNonNegativeInteger(this.$props.data.max_streams)
        return value === undefined ? '' : value
      },
      set(newValue: number) {
        const value = this.parseNonNegativeInteger(newValue)
        if (value === undefined) {
          delete this.$props.data.max_streams
        } else {
          this.$props.data.max_streams = value
        }
      },
    },
  },
  watch: {
    data() {
      this.clearClientSideNetwork()
    },
    direction() {
      this.clearClientSideNetwork()
    },
    namespace() {
      this.clearClientSideNetwork()
    },
  }
}
</script>
