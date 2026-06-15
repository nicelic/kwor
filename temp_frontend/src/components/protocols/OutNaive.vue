<template>
  <div>
    <v-row>
      <v-col cols="12" sm="6" md="4">
        <v-switch
          color="primary"
          label="QUIC"
          v-model="quic"
          hide-details
        ></v-switch>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-text-field
          label="Insecure Concurrency"
          type="number"
          min="0"
          v-model.number="insecureConcurrency"
          hide-details
        ></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-select
          hide-details
          label="QUIC Congestion Control"
          :items="congestionAlgorithms"
          v-model="quicCongestionControl"
        ></v-select>
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="6" md="4">
        <v-select
          hide-details
          label="UDP over TCP"
          :items="udpOverTcpItems"
          v-model="udpOverTcpVersion"
        ></v-select>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-switch
          color="primary"
          label="Extra Headers"
          v-model="extraHeadersEnabled"
          hide-details
        ></v-switch>
      </v-col>
    </v-row>
    <v-row v-if="extraHeadersEnabled">
      <v-col cols="12">
        <v-textarea
          label="extra_headers"
          auto-grow
          rows="5"
          spellcheck="false"
          v-model="extraHeadersText"
          :error="extraHeadersInvalid"
          :error-messages="extraHeadersError"
          @update:modelValue="updateExtraHeaders"
        ></v-textarea>
      </v-col>
    </v-row>
  </div>
</template>

<script lang="ts">
export default {
  props: ['data'],
  data() {
    return {
      congestionAlgorithms: [
        { title: "(omit)", value: '' },
        { title: "BBR", value: 'bbr' },
        { title: "BBRv2", value: 'bbr2' },
        { title: "CUBIC", value: 'cubic' },
        { title: "New Reno", value: 'reno' },
      ],
      udpOverTcpItems: [
        { title: this.$t('disable'), value: 0 },
        { title: "1", value: 1 },
        { title: "2", value: 2 },
      ],
      extraHeadersText: '{}',
      extraHeadersError: '',
    }
  },
  methods: {
    ensureDefaults() {
      if (this.$props.data.quic === undefined) {
        this.$props.data.quic = false
      }
      if (typeof this.$props.data.insecure_concurrency !== 'number') {
        this.$props.data.insecure_concurrency = 0
      }
      if (this.$props.data.quic_congestion_control === undefined) {
        this.$props.data.quic_congestion_control = 'bbr2'
      }
      if (this.$props.data.udp_over_tcp === undefined) {
        this.$props.data.udp_over_tcp = false
      }
    },
    syncExtraHeadersText() {
      this.extraHeadersError = ''
      if (!this.extraHeadersEnabled) {
        this.extraHeadersText = '{}'
        return
      }

      const extraHeaders = this.$props.data.extra_headers
      if (extraHeaders && typeof extraHeaders === 'object' && !Array.isArray(extraHeaders)) {
        this.extraHeadersText = JSON.stringify(extraHeaders, null, 2)
      } else {
        this.extraHeadersText = '{}'
      }
    },
    updateExtraHeaders(value: string) {
      const trimmed = (value ?? '').trim()
      if (trimmed === '') {
        this.$props.data.extra_headers = {}
        this.extraHeadersError = ''
        return
      }

      try {
        const parsed = JSON.parse(trimmed)
        if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
          this.extraHeadersError = 'extra_headers must be a JSON object'
          return
        }
        this.$props.data.extra_headers = parsed
        this.extraHeadersError = ''
      } catch {
        this.extraHeadersError = 'extra_headers must be valid JSON'
      }
    },
  },
  computed: {
    quic: {
      get(): boolean {
        return this.$props.data.quic === true
      },
      set(v: boolean) {
        this.$props.data.quic = v
      }
    },
    insecureConcurrency: {
      get(): number {
        return typeof this.$props.data.insecure_concurrency === 'number'
          ? this.$props.data.insecure_concurrency
          : 0
      },
      set(v: number) {
        const value = Number.isFinite(Number(v)) && Number(v) >= 0 ? Math.floor(Number(v)) : 0
        this.$props.data.insecure_concurrency = value
      }
    },
    quicCongestionControl: {
      get(): string {
        return this.$props.data.quic_congestion_control ?? 'bbr2'
      },
      set(v: string) {
        this.$props.data.quic_congestion_control = typeof v === 'string' ? v : 'bbr2'
      }
    },
    udpOverTcpVersion: {
      get(): number {
        const udpOverTcp = this.$props.data.udp_over_tcp
        if (udpOverTcp && typeof udpOverTcp === 'object' && udpOverTcp.enabled === true) {
          return Number(udpOverTcp.version ?? 1)
        }
        return 0
      },
      set(v: number) {
        if (Number(v) > 0) {
          this.$props.data.udp_over_tcp = {
            enabled: true,
            version: Number(v),
          }
          return
        }
        this.$props.data.udp_over_tcp = false
      }
    },
    extraHeadersEnabled: {
      get(): boolean {
        return this.$props.data.extra_headers !== undefined
      },
      set(v: boolean) {
        if (v) {
          if (!this.$props.data.extra_headers || Array.isArray(this.$props.data.extra_headers) || typeof this.$props.data.extra_headers !== 'object') {
            this.$props.data.extra_headers = {}
          }
          this.syncExtraHeadersText()
          return
        }
        delete this.$props.data.extra_headers
        this.extraHeadersError = ''
      }
    },
    extraHeadersInvalid(): boolean {
      return this.extraHeadersError !== ''
    },
  },
  watch: {
    data: {
      handler() {
        this.ensureDefaults()
        this.syncExtraHeadersText()
      },
      immediate: true,
    },
  },
}
</script>
