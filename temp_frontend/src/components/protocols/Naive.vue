<template>
  <v-card subtitle="Naive">
    <v-row>
      <v-col cols="12" sm="6" md="4">
        <v-select
          hide-details
          :label="$t('network')"
          :items="networkItems"
          multiple
          chips
          closable-chips
          v-model="networkSelection"
        ></v-select>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-select
          hide-details
          label="QUIC 拥塞控制"
          :items="congestionAlgorithms"
          v-model="quicCongestionControl"
        ></v-select>
      </v-col>
    </v-row>
    <v-row v-if="subscriptionTlsNotice">
      <v-col cols="12">
        <v-alert type="warning" variant="tonal" density="compact">
          {{ subscriptionTlsNotice }}
        </v-alert>
      </v-col>
    </v-row>
  </v-card>
</template>

<script lang="ts">
export default {
  props: ['inbound', 'tlsConfigs'],
  data() {
    return {
      networkItems: ['tcp', 'udp'],
      congestionAlgorithms: [
        { title: "(omit)", value: '' },
        { title: "BBR", value: 'bbr' },
        { title: "BBRv2", value: 'bbr2' },
        { title: "CUBIC", value: 'cubic' },
        { title: "New Reno", value: 'reno' },
      ],
    }
  },
  computed: {
    networkSelection: {
      get(): string[] {
        const network = this.$props.inbound.network
        if (network === 'tcp' || network === 'udp') {
          return [network]
        }
        return ['tcp', 'udp']
      },
      set(v: string[]) {
        const normalized = (v ?? []).filter((item, index, list) =>
          (item === 'tcp' || item === 'udp') && list.indexOf(item) === index
        )
        if (normalized.length === 1) {
          this.$props.inbound.network = normalized[0]
        } else {
          this.$props.inbound.network = undefined
        }
      }
    },
    quicCongestionControl: {
      get(): string {
        if (this.$props.inbound.naive_quic_congestion_control_omit === true) {
          return ''
        }
        if (typeof this.$props.inbound.quic_congestion_control === 'string' && this.$props.inbound.quic_congestion_control.trim() !== '') {
          return this.$props.inbound.quic_congestion_control
        }
        return this.$props.inbound.id === 0 ? 'bbr2' : ''
      },
      set(v: string) {
        if (!v || v.trim() === '') {
          this.$props.inbound.naive_quic_congestion_control_omit = true
          delete this.$props.inbound.quic_congestion_control
          return
        }
        delete this.$props.inbound.naive_quic_congestion_control_omit
        this.$props.inbound.quic_congestion_control = v
      }
    },
    subscriptionTlsNotice(): string {
      const tlsId = Number(this.$props.inbound?.tls_id ?? 0)
      if (!tlsId || !Array.isArray(this.$props.tlsConfigs)) {
        return ''
      }

      const selectedTls = this.$props.tlsConfigs.find((item: any) => Number(item?.id ?? 0) === tlsId)
      if (!selectedTls) {
        return ''
      }

      const serverTls = selectedTls.server ?? {}
      const clientTls = selectedTls.client ?? {}
      const hasALPN = (Array.isArray(serverTls.alpn) && serverTls.alpn.length > 0) ||
        (Array.isArray(clientTls.alpn) && clientTls.alpn.length > 0)
      const hasUTLS = clientTls.utls !== undefined

      if (!hasALPN && !hasUTLS) {
        return ''
      }
      return 'Naive 的 JSON 订阅会忽略当前 TLS 模板中的 ALPN 和 uTLS。'
    },
  },
}
</script>
