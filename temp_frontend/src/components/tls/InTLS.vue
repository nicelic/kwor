<template>
  <v-card :subtitle="$t('objects.tls')">
    <v-row>
      <v-col cols="12" sm="6" md="4">
        <v-select
          hide-details
          :label="$t('template')"
          :items="tlsItems"
          v-model="inbound.tls_id">
        </v-select>
      </v-col>
    </v-row>
  </v-card>
</template>

<script lang="ts">
import { i18n } from '@/locales'
export default {
  props: ['inbound', 'tlsConfigs', 'namespace'],
  computed: {
    supportsMihomoWrapperModes(): boolean {
      return ['vmess', 'vless', 'trojan', 'anytls', 'shadowsocks'].includes(String(this.$props.inbound?.type ?? '').toLowerCase())
    },
    tlsItems(): any[] {
      const inboundType = String(this.$props.inbound?.type ?? '').toLowerCase()
      const modeOf = (t: any): string => {
        return typeof t?.mode === 'string' && t.mode ? t.mode : 'tls'
      }
      const modeLabel = (mode: string): string => ({ 'tls': 'TLS', 'reality': 'Reality', 'shadow-tls': 'ShadowTLS', 'restls': 'Restls', 'jls': 'JLS' } as Record<string, string>)[mode] ?? mode
      const allConfigs = Array.isArray(this.$props.tlsConfigs) ? this.$props.tlsConfigs : []
      const configs = allConfigs.filter((t: any) => {
        const mode = modeOf(t)
        const isMihomo = this.$props.namespace === 'mihomo'
        if (isMihomo && inboundType === 'shadowsocks') {
          return ['shadow-tls', 'restls', 'jls'].includes(mode)
        }
        if (!isMihomo && ['shadow-tls', 'restls', 'jls'].includes(mode)) return false
        if (['shadow-tls', 'restls', 'jls'].includes(mode) && (!isMihomo || !this.supportsMihomoWrapperModes)) return false
        return true
      })
      const selectedTLSID = Number(this.$props.inbound?.tls_id ?? 0)
      const selectedConfig = selectedTLSID > 0
        ? allConfigs.find((config: any) => Number(config?.id ?? 0) === selectedTLSID)
        : undefined
      if (selectedConfig && !configs.some((config: any) => Number(config?.id ?? 0) === selectedTLSID)) {
        configs.unshift(selectedConfig)
      }
      return [
        { title: i18n.global.t('none'), value: 0 },
        ...configs.map((t: any) => ({ title: `${t.name} · ${modeLabel(modeOf(t))}`, value: t.id }))
      ]
    }
  }
}
</script>
